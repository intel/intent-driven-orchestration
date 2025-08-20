"""
Script started by the unittests.
"""

import argparse
import json
import logging

from wsgiref.simple_server import make_server

logging.basicConfig(level=logging.INFO)

TEST_DATA = {
    'default/p99latency': [200, 150, 100, 50],
    'default/p95latency': [120, 100, 80, 40],
    "default/my-power": [10, 20, 30, 40]
}


def predict_app(environ, start_response):
    """
    Predicts the effect on a latency target.
    """
    try:
        body_size = int(environ.get('CONTENT_LENGTH', 0))
    except ValueError:
        body_size = 0

    request_body = environ['wsgi.input'].read(body_size)
    body = json.loads(request_body)

    objectives = list(body['objectives'])

    data = {}
    for objective in objectives:
        data[objective] = TEST_DATA[objective]

    status = '200 OK'
    headers = [('Content-type', 'application/json')]
    start_response(status, headers)

    tmp = json.dumps(data)
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
    parser.add_argument('profiles', type=str,
                        help='List of profiles ordered for power draw (comma '
                             'seperated).')
    parser.add_argument('--port', type=int, default=8321,
                        help='Port to listen on.')
    ARGS = parser.parse_args()
    serve(ARGS)
