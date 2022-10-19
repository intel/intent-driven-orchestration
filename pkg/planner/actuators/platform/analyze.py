#!/usr/bin/env python3
"""
Analytics script to determine the effect an RDT configuration option has on
given workload.

WARNING: These scripts are for proof of concept only; For production system
please consider using more advanced analytical capabilities.
"""

import argparse
import base64
import dataclasses
import datetime
import io
import logging
import os
import pickle  # nosec - we need this to store model in knowledge base.

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


def _parse_pods(pod_data):
    for _, item in pod_data.items():
        if 'annotations' in item:
            if item['annotations'] and 'rdt_config' in item['annotations']:
                # XXX: assuming all pods have same annotation for now.
                return item['annotations']['rdt_config'], item['qosclass']
            return None, item['qosclass']
        return None, item['qosclass']


def _parse_load(data, name):
    """
    Parse node level load variables.
    """
    res = 0.0
    i = 0
    if name in data:
        for _, item in data[name].items():
            res += item
            i += 1
    if i != 0:
        return res / i
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
                     'timestamp': 1,
                     '_id': 0}, sort=[('_id', -1)], limit=args.max_vals)
    for item in res:
        if item['pods'] is None:
            continue
        tmp[item['timestamp']] = item['current_objectives']
        # TODO: resources.
        rdt, qos = _parse_pods(item['pods'])
        tmp[item['timestamp']]['load'] = _parse_load(item['data'], 'cpu_value')
        tmp[item['timestamp']]['llc_value'] = _parse_load(item['data'],
                                                          'llc_value')
        tmp[item['timestamp']]['rdt_config'] = rdt or 'None'
        tmp[item['timestamp']]['qosclass'] = qos
        tmp[item['timestamp']]['replicas'] = len(
            ['0' for pod in item['pods'].values()
             if pod['state'] == 'Running'])
    data = pd.DataFrame.from_dict(tmp, orient='index')

    # make sure we only do all this stuff when we actually have seen rdt...
    if len(data['rdt_config'].unique()) <= 1:
        return None

    cols = data.columns
    data.drop_columns = data.drop
    data.drop_columns(columns=[item for item in cols if
                               item not in (args.latency,
                                            'rdt_config',
                                            'load',
                                            'llc_value',
                                            'qosclass',
                                            'replicas')],
                      inplace=True)

    return data


def store_data(data, args):
    """
    Store the results back to knowledge base.
    """
    client = pymongo.MongoClient(args.mongo_uri)
    dbs = client['intents']
    coll = dbs['effects']

    doc = {'name': args.name,
           'profileName': args.latency,
           'group': 'rdt',
           'data': data,
           'static': False,
           'timestamp': datetime.datetime.now()}
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
    # data = data[(np.abs(stats.zscore(data)) < 3).all(axis=1)]

    # some cleanup
    data = data[(data[args.latency] != -1.0)]
    data = data.dropna()

    res = data.apply(pd.to_numeric)
    return res, features, feat_map


def _plot_results(tree, data, features, feat_map, args):
    """
    Visualize the results and return base6 encoded img.
    """
    fig = plt.Figure(figsize=(10, 6))
    axes = fig.add_subplot()

    features.append(('load', 'rdt_config'))
    pdp = inspection.PartialDependenceDisplay.from_estimator(
        tree,
        data,
        features,
        kind=['both', 'both', 'both', 'both', 'both', 'average'],
        n_jobs=N_JOBS,
        grid_resolution=25,
        ax=axes,
        contour_kw={'cmap': 'Blues'}
    )
    pdp.figure_.suptitle(f'Partial dependence on {args.latency}, with '
                         f'gradient boosting.')
    pdp.figure_.subplots_adjust(wspace=0.3, hspace=0.3)

    # tweaking tick labels.
    cos_labels = feat_map['rdt_config'].copy()
    cos_ticks = range(len(cos_labels))
    pdp.axes_[1][2].yaxis.set_major_locator(ticker.FixedLocator(cos_ticks))
    pdp.axes_[1][2].set_yticklabels(cos_labels)
    pdp.axes_[0][2].xaxis.set_major_locator(ticker.FixedLocator(cos_ticks))
    pdp.axes_[0][2].set_xticklabels(cos_labels)
    qos_labels = feat_map['qosclass'].copy()
    qos_ticks = range(len(qos_labels))
    pdp.axes_[1][0].xaxis.set_major_locator(ticker.FixedLocator(qos_ticks))
    pdp.axes_[1][0].set_xticklabels(qos_labels)

    buf = io.BytesIO()
    fig.tight_layout()
    fig.savefig(buf, format='png')
    tmp = buf.getbuffer()
    fig.clf()

    return base64.b64encode(tmp).decode('ascii')


def train_dt(data, args):
    """
    Train a decision tree based model.
    """
    data, features, feat_map = _pre_process(data, args)

    feat = data[features]
    target = data[args.latency]
    if len(target) >= 15:
        clf = ensemble.RandomForestRegressor(n_estimators=50,
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
    max_vals: int
    mongo_uri: str

    @staticmethod
    def from_args(args):
        """
        Set the arguments based on the parsed data.
        """
        return Arguments(
            name=args['name'],
            latency=args['latency'],
            max_vals=int(args['max_vals']),
            mongo_uri=args['mongo_uri'],
        )


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('name', type=str,
                        help='Name of the objective.')
    parser.add_argument('latency', type=str,
                        help='Name of the latency objective.')
    parser.add_argument('--max_vals', type=int, default=250,
                        help='Limits the number of records to retrieve.')
    parser.add_argument('--mongo_uri', type=str,
                        default=os.environ.get('MONGO_URL',
                                               'mongodb://localhost:27100'),
                        help='Mongo connection string.')
    parsed_args = dict(**vars(parser.parse_args()))
    arguments = Arguments.from_args(parsed_args)
    main(arguments)
