import json
from wsgiref.simple_server import make_server

import sys

import pandas as pd
from sklearn import ensemble

FILENAME = "traces/trace_rdt/p95_rdt.csv"
MODEL = ensemble.ExtraTreesRegressor(n_estimators=50)
FEAT_MAP = ['None', 'besteffort', 'burstable', 'guaranteed']


def _train():
    df = pd.read_csv(FILENAME, index_col=0, parse_dates=True)
    MODEL.fit(df[['cpu', 'rdt_config', 'replicas']], df['default/p95latency'])


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

    option = body['option']
    cpu = body['cpu']
    replicas = body['replicas']

    tmp = MODEL.predict([[cpu, FEAT_MAP.index(option), replicas]])
    res = tmp[0]

    status = '200 OK'
    headers = [('Content-type', 'application/json')]
    start_response(status, headers)

    tmp = json.dumps({'val': res})
    return [tmp.encode()]


def serve():
    _train()
    with make_server('127.0.0.1', 8000, predict_app) as httpd:
        httpd.serve_forever()


if __name__ == '__main__':
    sys.stdout.write(str(serve()))
    sys.stdout.flush()
