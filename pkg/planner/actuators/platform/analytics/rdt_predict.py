#!/usr/bin/env python3
"""
Script that predicts potential effect of RDT settings for a given workload.

WARNING: These scripts are for proof of concept only and offer no guarantee
of security or robustness. Do not use in a production environment.
"""

import argparse
import importlib
import io
import json
import logging
import os
import pickle  # nosec - we safely unpickle; see below.
import warnings
from datetime import datetime, timedelta, timezone

from wsgiref.simple_server import make_server

import pymongo

MONGO_URI = ''
FORMAT = '%(asctime)s - %(filename)-15s - %(threadName)-10s - ' \
         '%(funcName)-24s - %(lineno)-3s - %(levelname)-7s - %(message)s'
logging.basicConfig(format=FORMAT, level=logging.INFO)
warnings.simplefilter("ignore")

ALLOWED_MOD = [
    'sklearn.ensemble._forest',
    'sklearn.tree._classes',
    'numpy._core.multiarray',
    'numpy',
    'sklearn.tree._tree'
]
ALLOWED_CLASS = [
    'ExtraTreesRegressor',
    'ExtraTreeRegressor',
    '_reconstruct',
    'ndarray',
    'dtype',
    'Tree'
]


class SafeUnpickler(pickle.Unpickler):
    """
    Check if the object we try to unpickle is indeed the right model.
    """

    def find_class(self, module, name):
        if module in ALLOWED_MOD and name in ALLOWED_CLASS:
            my_mod = importlib.import_module(module)
            return getattr(my_mod, name)
        raise pickle.UnpicklingError(f'Unpickling {module}.{name} is '
                                     f'forbidden!')


def _get_model(name, target):
    lookback = datetime.now(timezone.utc) - timedelta(minutes=LOOKBACK)
    client = pymongo.MongoClient(MONGO_URI)
    dbs = client["intents"]
    coll = dbs["effects"]
    items = coll.find(
        {'group': 'rdt',
         'name': name,
         'profileName': target,
         '$or': [{'static': True}, {'timestamp': {'$gt': lookback}}],
         },
        {'data': {'model': 1, 'features_map': 1},
         'timestamp': 1}).sort('_id', pymongo.DESCENDING).limit(1)
    items = list(items)
    if len(items) > 0:
        model = items[0]['data']['model']
        feats = items[0]['data']['features_map']
        clf = SafeUnpickler(io.BytesIO(model)).load()
        return clf, feats
    return None, None


def predict_app(environ, start_response):
    """
    Predicts the effect on a latency target, or return -1.0.
    """
    try:
        body_size = int(environ.get('CONTENT_LENGTH', 0))
    except ValueError:
        body_size = 0

    request_body = environ['wsgi.input'].read(body_size)
    body = json.loads(request_body)

    result = -1.0
    model, feature_map = _get_model(body['name'], body['target'])
    if model is not None:
        try:
            res = model.predict([[
                body['cpu'],
                feature_map['rdt_config'].index(body['option']),
                body['replicas']
            ]])
            result = res[0]
        except ValueError as exp:
            logging.warning('Could not predict: %s.', exp)

    status = '200 OK'
    headers = [('Content-type', 'application/json')]
    start_response(status, headers)

    tmp = json.dumps({"val": result})
    return [tmp.encode()]


def main(args):
    """
    Launch a wsgi ref server.
    """
    logging.info('Listening on port: %s', int(args.port))
    httpd = make_server('127.0.0.1', args.port, predict_app)
    httpd.serve_forever()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('--lookback', type=int, default=20,
                        help="Allowed age of model (defaults to 20) in min.")
    parser.add_argument("--port", type=int, default=8000,
                        help="Port to listen on.")
    parser.add_argument("--mongo_uri", type=str,
                        default=os.environ.get('MONGO_URL',
                                               'mongodb://localhost:27100'),
                        help="Mongo connection string.")
    ARGS = parser.parse_args()
    MONGO_URI = ARGS.mongo_uri
    LOOKBACK = ARGS.lookback
    main(ARGS)
