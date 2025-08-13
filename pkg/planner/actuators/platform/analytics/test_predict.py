"""
Script for testing.
"""

import argparse
import json
import logging
import sys

from wsgiref.simple_server import make_server

logging.basicConfig(level=logging.INFO)

TEST_DATA = {
    'default/function-intents': {
        'None': {
            'default/p99': 20.0,
            'default/p95': 17.0
        },
        'option_a': {
            'default/p99': 10.0,
            'default/p95': 7.5
        },
        'option_b': {
            'default/p99': 5.0,
            'default/p95': 2.5
        },
    }
}


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

    name = body['name']
    target = body['target']
    option = body['option']
    cpu = body['cpu']
    replicas = body['replicas']

    if cpu is None or option is None or replicas is None:
        sys.exit('missing values!')

    res = -1.0
    if name in TEST_DATA and \
        option in TEST_DATA[name] and \
            target in TEST_DATA[name][option]:
        res = TEST_DATA[name][option][target]

    status = '200 OK'
    headers = [('Content-type', 'application/json')]
    start_response(status, headers)

    tmp = json.dumps({'val': res})
    return [tmp.encode()]


def serve(args):
    """
    Launch a wsgi ref server.
    """
    logging.info('Listening on port: %s', int(args.port))
    httpd = make_server('127.0.0.1', args.port, predict_app)
    httpd.serve_forever()


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, default=8000,
                        help='Port to listen on.')
    ARGS = parser.parse_args()
    serve(ARGS)
