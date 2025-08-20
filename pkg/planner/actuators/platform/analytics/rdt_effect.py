#!/usr/bin/env python3
"""
Analytics script to determine the effect an RDT configuration option has on
 a given workload.

WARNING: These scripts are for proof of concept only and offer no guarantee
of security or robustness. Do not use in a production environment.
"""

import argparse
import base64
import dataclasses
import io
import logging
import os
import pickle  # nosec - we need this to store model in knowledge base.
from datetime import datetime, timezone

import matplotlib.pyplot as plt
import pandas as pd
import pymongo

from matplotlib import ticker

from pymongo import errors

from sklearn import ensemble
from sklearn import inspection
from sklearn import preprocessing

FORMAT = '%(asctime)s - %(filename)-15s - %(threadName)-10s - ' \
         '%(funcName)-24s - %(lineno)-3s - %(levelname)-7s - %(message)s'
N_JOBS = 2

logging.basicConfig(format=FORMAT, level=logging.INFO)


def _parse_resources(resource_data):
    last = -1
    res = 0
    for key, val in resource_data.items():
        tmp = key.split('_')
        if int(tmp[0]) > last and tmp[1] == 'cpu' and tmp[2] == 'limits':
            res = int(val) / 1000
            last = int(tmp[0])
    return res


def get_data(args):
    """
    Return data from MongoDB. This is for demo purposes only, in production
    systems this should go back to the storage parts of the observability
    stack.
    """
    client = pymongo.MongoClient(args.mongo_uri)
    dbs = client['intents']
    coll = dbs['events']

    tmp = {}
    res = coll.find({'name': args.name},
                    {'current_objectives': 1,
                     'data': 1,
                     'pods': 1,
                     'resources': 1,
                     'annotations': 1,
                     'timestamp': 1,
                     '_id': 0}, sort=[('_id', -1)], limit=args.max_vals)
    for item in res:
        if 'resources' not in item or 'pods' not in item or \
                item['pods'] is None or len(item['pods']) == 0:
            continue

        if _parse_resources(item['resources']) < 0.75:
            # doesn't make sense to look at tiny apps in context of RDT.
            continue

        tmp[item['timestamp']] = item['current_objectives']
        rdt = 'None'
        if 'annotations' in item and item['annotations'] and \
                args.annotation_name in item['annotations']:
            # XXX: assuming all pods have the same annotation for now.
            rdt = item['annotations'][args.annotation_name]
        tmp[item['timestamp']]['cpu'] = _parse_resources(item['resources'])
        tmp[item['timestamp']]['rdt_config'] = rdt
        tmp[item['timestamp']]['replicas'] = len(
            ['0' for pod in item['pods'].values()
             if pod['state'] == 'Running'])
    data = pd.DataFrame.from_dict(tmp, orient='index')

    # make sure we only do all this stuff when we actually have seen rdt...
    if 'rdt_config' not in data or len(data['rdt_config'].unique()) <= 1:
        return None

    cols = data.columns
    data.drop_columns = data.drop
    data.drop_columns(columns=[item for item in cols if
                               item not in (args.latency,
                                            'rdt_config',
                                            'cpu',
                                            'replicas')],
                      inplace=True)

    return data


def store_data(data, args):
    """
    Store the results back to the knowledge base.
    """
    client = pymongo.MongoClient(args.mongo_uri)
    dbs = client['intents']
    coll = dbs['effects']

    doc = {'name': args.name,
           'profileName': args.latency,
           'group': 'rdt',
           'data': data,
           'static': False,
           'timestamp': datetime.now(timezone.utc)}
    try:
        coll.insert_one(doc)
    except errors.ExecutionTimeout as err:
        logging.error('Connection to database timed out - could not store '
                      'effect: %s', err)


def _pre_process(data, args):
    features = []
    feat_map = {}
    data.fillna(value='None', inplace=True)
    for item in data.columns:
        if item == args.latency:
            continue
        features.append(item)
        if data[item].dtype == object:
            encoder = preprocessing.LabelEncoder()
            data[item] = encoder.fit_transform(data[item])
            feat_map[item] = list(encoder.classes_)

    # remove outliers.
    model = ensemble.IsolationForest(random_state=42, contamination=0.2)
    model.fit(data[['cpu', args.latency]])
    data['anomaly'] = model.predict(data[['cpu', args.latency]])
    data = data.drop(data[data['anomaly'] == -1.0].index)

    # some cleanup
    data = data[(data[args.latency] != -1.0)]
    data = data.dropna()

    res = data.apply(pd.to_numeric)
    return res, features, feat_map


def _plot_results(tree, data, features, feat_map, args):
    """
    Visualize the results and return base6 encoded img.
    """
    fig, axes = plt.subplots(1, 1, figsize=(10, 6))

    tmp = features.copy()
    tmp.append(('cpu', 'rdt_config'))
    pdp = inspection.PartialDependenceDisplay.from_estimator(
        tree,
        data,
        tmp,
        kind=['both', 'both', 'both', 'average'],
        n_jobs=N_JOBS,
        grid_resolution=10,
        ax=axes,
        contour_kw={'cmap': 'Blues'}
    )
    pdp.figure_.suptitle(f'Partial dependence on {args.latency}, with '
                         f'gradient boosting.')
    pdp.figure_.subplots_adjust(wspace=0.3, hspace=0.3)

    # tweaking tick labels.
    cos_labels = feat_map['rdt_config'].copy()
    cos_ticks = range(len(cos_labels))
    pdp.axes_[1][0].yaxis.set_major_locator(ticker.FixedLocator(cos_ticks))
    pdp.axes_[1][0].set_yticklabels(cos_labels)
    pdp.axes_[0][1].xaxis.set_major_locator(ticker.FixedLocator(cos_ticks))
    pdp.axes_[0][1].set_xticklabels(cos_labels)

    buf = io.BytesIO()
    fig.tight_layout()
    fig.savefig(buf, format='png')
    tmp = buf.getbuffer()
    fig.clf()

    return base64.b64encode(tmp).decode('ascii')


def train_dt(data, args):
    """
    Train a decision-tree-based model.
    """
    data, features, feat_map = _pre_process(data, args)

    feat = data[features]
    target = data[args.latency]
    if len(target) >= args.min_vals:
        clf = ensemble.ExtraTreesRegressor(n_estimators=50,
                                           warm_start=True,
                                           n_jobs=N_JOBS)
        clf.fit(feat, target)

        image = _plot_results(clf, feat, features, feat_map, args)
        clf_s = pickle.dumps(clf)
        return clf_s, image, features, feat_map
    logging.warning('Not enough data to determine model for: %s.',
                    args.latency)
    return None, None, None, None


def main(args):
    """
    Triggers the basic analytics.
    """
    # get data
    data = get_data(args)
    if data is not None:
        # create a model
        model, image, features, feat_map = train_dt(data, args)
        # store it.
        if model is not None:
            logging.info('Found model for: %s - %s.',
                         args.name, args.latency)
            data = {
                'image': image,
                'model': model,
                'training_features': features,
                'features_map': feat_map
            }
            store_data(data, args)


@dataclasses.dataclass
class Arguments:
    """
    Dataclass that hold the arguments as parsed from argparse.
    """

    name: str
    latency: str
    annotation_name: str
    max_vals: int
    min_vals: int
    mongo_uri: str

    @staticmethod
    def from_args(args):
        """
        Set the arguments based on the parsed data.
        """
        return Arguments(
            name=args['name'],
            latency=args['latency'],
            annotation_name=args['annotation_name'],
            max_vals=int(args['max_vals']),
            min_vals=int(args['min_vals']),
            mongo_uri=args['mongo_uri'],
        )


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('name', type=str,
                        help='Name of the objective.')
    parser.add_argument('latency', type=str,
                        help='Name of the latency objective.')
    parser.add_argument('annotation_name', type=str,
                        help='Name of the annotation indicating the COS.')
    parser.add_argument('--max_vals', type=int, default=500,
                        help='Limits the number of records to retrieve.')
    parser.add_argument('--min_vals', type=int, default=15,
                        help='Minimum required values to train model.')
    parser.add_argument('--mongo_uri', type=str,
                        default=os.environ.get('MONGO_URL',
                                               'mongodb://localhost:27100'),
                        help='Mongo connection string.')
    parsed_args = {**vars(parser.parse_args())}
    arguments = Arguments.from_args(parsed_args)
    main(arguments)
