"""
Script to analyze the effect a power profile selection has on objectives.

WARNING: These scripts are for proof of concept only; For production system
please consider using more advanced analytical capabilities.
"""

import argparse
import base64
import dataclasses
import io
import logging
import os
import pickle
from typing import List
from datetime import datetime, timezone

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
import pymongo

from sklearn import ensemble

FORMAT = '%(asctime)s - %(filename)-15s - %(threadName)-10s - ' \
         '%(funcName)-24s - %(lineno)-3s - %(levelname)-7s - %(message)s'
THRESHOLD = 3

logging.basicConfig(format=FORMAT, level=logging.INFO)


def _get_data(args):
    client = pymongo.MongoClient(args.mongo_uri)
    dbs = client['intents']
    coll = dbs['events']

    tmp = {}
    res = coll.find({'name': args.name},
                    {'current_objectives': 1,
                     'data': 1,
                     'resources': 1,
                     'pods': 1,
                     'timestamp': 1,
                     '_id': 0}, sort=[('_id', -1)], limit=args.max_vals)
    for item in res:
        if item['pods'] is None or item['resources'] is None:
            continue
        replicas = len(['0' for pod in item['pods'].values()
                        if pod['state'] == 'Running'])
        if replicas != len(item['pods']):
            continue
        tmp[item['timestamp']] = {}
        for target in args.targets:
            if target in item['current_objectives']:
                tmp[item['timestamp']][target] = (
                    item)['current_objectives'][target]
            else:
                tmp[item['timestamp']][target] = -1.0

        group = 'None'
        cpu_val = -1.0
        last = 0
        for res, val in item['resources'].items():
            if (res.find('cpu') != -1 and res.find('request') != -1 and
                    int(res.split('_')[0]) >= last):
                cpu_val = int(val) / 1000
                last = int(res.split('_')[0])
            if res.find('power.intel.com') != -1 and res.find('request') != -1:
                group = res.split('_')[1]
                cpu_val = int(val) / 1000
                break
        tmp[item['timestamp']][group] = cpu_val

    data = pd.DataFrame.from_dict(tmp, orient='index')
    data = data.fillna(0)

    if data.empty:
        return None

    # remove missing values.
    for target in args.targets:
        data.drop(data[data[target] < 0.0].index, inplace=True)

    # remove outliers.
    mean = data[target].mean()
    std_dev = data[target].std()
    z_scores = (data[target] - mean) / std_dev
    data = data[(np.abs(z_scores) < THRESHOLD)]

    return data


def _train(target, args, data):
    target = data[target]
    features = data[data.columns[~data.columns.isin(args.targets)]]
    features = features.reindex(sorted(features.columns), axis=1)

    # This orders the columns (low power draw --> high power draw).
    order = [column for column in args.profiles if column in features.columns]
    features = features[order]

    if (len(target) >= args.min_vals and len(features.columns)
            >= args.min_features):
        clf = ensemble.RandomForestRegressor(n_estimators=50,
                                             criterion='absolute_error')
        clf.fit(features.values, target.values)
        clf_s = pickle.dumps(clf)
        return clf, clf_s, features
    logging.warning('Not enough data to determine model for: %s.',
                    target)
    return None, None, None


def _plot_results(args, model, target, data):
    fig, axes = plt.subplots(1, 1, figsize=(8, 4))

    prof = np.linspace(0, 3, num=4)
    cpu = np.linspace(1, data.max().max(), num=int(data.max().max()))

    lat_pred = []
    for cores in range(1, int(data.max().max()) + 1):
        for i in range(len(args.profiles)):
            feature_list = [0] * len(args.profiles)
            feature_list[i] = cores
            forecast = model.predict([feature_list])[0]
            lat_pred.append(forecast)
    forecast = np.array(lat_pred).reshape(len(cpu), len(prof)).T

    axes.imshow(forecast, cmap='Blues')
    for i in range(len(cpu)):
        for j in range(len(prof)):
            axes.text(i, j, round(forecast[j, i], 2),
                      ha="center", va="center", color="darkorange")
    axes.set_xticks(np.arange(len(cpu)), labels=cpu)
    axes.set_yticks(np.arange(len(args.profiles)),
                    labels=[profile.replace('/', '/\n') for profile
                            in args.profiles])

    # Add titles and labels
    plt.title(f'Partial dependence on {target}.')
    plt.ylabel('power profile')
    plt.xlabel('resources / ($cpu$)')

    buf = io.BytesIO()
    fig.tight_layout()
    fig.savefig(buf, format='png')
    tmp = buf.getbuffer()
    fig.clf()

    return base64.b64encode(tmp).decode('ascii')


def _store_res(args, model, target, image, feat_names):
    client = pymongo.MongoClient(args.mongo_uri)
    dbs = client['intents']
    coll = dbs['effects']

    doc = {
        'name': args.name,
        'profileName': target,
        'group': 'energy',
        'data': {
            'model': model,
            'image': image,
            'features': feat_names,
        },
        'static': False,
        'timestamp': datetime.now(timezone.utc)
    }
    coll.insert_one(doc)


def main(args):
    """
    Kick off training for a set of targeted objectives.
    """
    data = _get_data(args)
    if data is None:
        logging.warning('Dataframe was empty for %s - %s.', args.name,
                        args.targets)
        return
    for target in args.targets:
        model, p_model, features = _train(target, args, data)
        if model:
            image = _plot_results(args, model, target, features)
            _store_res(args, p_model, target, image, list(features.columns))
            logging.info("Found model for %s - %s.", args.name, target)


@dataclasses.dataclass
class Arguments:
    """
    Dataclass that hold the arguments as parsed from argparse.
    """

    name: str
    targets: List[str]
    profiles: List[str]
    min_features: int
    min_vals: int
    max_vals: int
    mongo_uri: str

    @staticmethod
    def from_args(args):
        """
        Set the arguments based on the parsed data.
        """
        return Arguments(
            name=str(args['name']),
            targets=args['targets'].split(','),
            profiles=args['profiles'].split(','),
            min_features=int(args['min_features']),
            min_vals=int(args['min_vals']),
            max_vals=int(args['max_vals']),
            mongo_uri=args['mongo_uri'],
        )


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('name', type=str,
                        help='Name of the intent.')
    parser.add_argument('targets', type=str,
                        help='List of names of the target objectives (comma '
                             'seperated).')
    parser.add_argument('profiles', type=str,
                        help='List of profiles ordered for power draw (comma '
                             'seperated).')
    parser.add_argument('--min_features', type=int, default=2,
                        help='Minim number of features needed.')
    parser.add_argument('--min_vals', type=int, default=15,
                        help='Minim number of values needed.')
    parser.add_argument('--max_vals', type=int, default=500,
                        help='Limits the number of records to retrieve.')
    parser.add_argument('--mongo_uri', type=str,
                        default=os.environ.get('MONGO_URL',
                                               'mongodb://localhost:27100'),
                        help='Mongo connection string.')
    parsed_args = {**vars(parser.parse_args())}
    arguments = Arguments.from_args(parsed_args)
    main(arguments)
