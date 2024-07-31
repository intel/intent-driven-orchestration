#!/usr/bin/env python3
"""
Analytics script to determine the scaling properties for a given workload.

WARNING: These scripts are for proof of concept only and offer no guarantee
of security or robustness. Do not use in a production environment.
"""

import argparse
import base64
import dataclasses
import datetime
import io
import logging
import os

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
import pymongo

from pymongo import errors
from scipy import optimize

FIG_SIZE = (8, 6)
FORMAT = '%(asctime)s - %(filename)-15s - %(threadName)-10s - ' \
         '%(funcName)-24s - %(lineno)-3s - %(levelname)-7s - %(message)s'
logging.basicConfig(format=FORMAT, level=logging.INFO)


def latency_func(data, p_0, p_1, p_2):
    """
    Latency function relating n_cpus to latency.
    """
    return p_0 * np.exp(-p_1 * data) + p_2


def _get_cpu(resources):
    last = -1
    res = 0
    for key, val in resources.items():
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
    dbs = client["intents"]
    coll = dbs["events"]

    tmp = {}
    res = coll.find({"name": args.name},
                    {"current_objectives": 1,
                     "pods": 1,
                     "resources": 1,
                     "timestamp": 1,
                     "_id": 0}, sort=[('_id', -1)], limit=args.max_vals)
    for item in res:
        if not item["resources"]:
            continue
        if len(['1' for pod in item['pods'].values()
                if pod['state'] == 'Running']) != 1:
            continue
        if args.latency not in item["current_objectives"]:
            continue
        # it may consider a better solution to point to the right resources
        cpu = _get_cpu(item['resources'])
        if cpu > 0:
            tmp[item["timestamp"]] = item["current_objectives"]
            tmp[item["timestamp"]]["cpus"] = cpu
        else:
            continue

    data = pd.DataFrame.from_dict(tmp, orient="index")
    cols = data.columns
    data.drop_columns = data.drop
    data.drop_columns(columns=[item for item in cols if
                               item not in (args.latency,
                                            'cpus')],
                      inplace=True)

    if data.empty:
        return None
    data = data[(data[args.latency] != -1.0)]
    data = data.dropna()

    # keep the 10% of lowest values.
    n_items = round(len(data)*0.1)
    data = data.groupby(['cpus']).apply(
        lambda x: x.nsmallest(n=n_items,
                              columns=args.latency)).reset_index(drop=True)

    return data


def store_result(popt,
                 data,
                 img,
                 args):
    """
    Store the results back to knowledge base.
    """
    client = pymongo.MongoClient(args.mongo_uri)
    dbs = client["intents"]
    coll = dbs["effects"]

    latency_range = (min(data[args.latency]), max(data[args.latency]))
    cpu_range = (min(data["cpus"]), max(data["cpus"]))
    training_features = ["cpus"]
    timestamp = datetime.datetime.utcnow()
    doc = {"name": args.name,
           "profileName": args.latency,
           "group": "vertical_scaling",
           "data": {"latencyRange": latency_range,
                    "cpuRange": cpu_range,
                    "popt": popt.tolist(),
                    "trainingFeatures": training_features,
                    "targetFeature": args.latency,
                    "image": img},
           "static": False,
           "timestamp": timestamp}
    try:
        coll.insert_one(doc)
    except errors.ExecutionTimeout as err:
        logging.error("Connection to database timed out - could not store "
                      "effect: %s", err)


def analyse(data, args):
    """
    If enough unique data-points are available - do a curve fit and return
    parameters.
    """
    if data.empty or \
            len(data.index) < args.min_vals or \
            len(data["cpus"].unique()) < 2:
        logging.warning("Not enough (unique) data points available for: %s.",
                        args.name)
        logging.debug("Data was: %s", data)
        return None, None

    # do the actual curve fitting
    try:
        popt, _ = optimize.curve_fit(latency_func,
                                     data["cpus"],
                                     data[args.latency],
                                     bounds=([0, 0.2, 0], [np.inf, 5, np.inf]))
    except (ValueError, RuntimeError) as err:
        logging.warning("Could not curve fit: %s.", err)
        return None, None

    # check if we have a proper model
    if popt[0] > 0.0 and 0 < popt[1] < 50 and popt.sum() != 1.0:
        return popt, data
    logging.warning("Didn't find a proper plane: %s: %s (%s) - will discard.",
                    args.name, args.latency, popt)
    return None, None


def plot_results(data, popt, args):
    """
    Visualize the results and return base6 encoded img.
    """
    fig, axes = plt.subplots(1, 1, figsize=FIG_SIZE)

    axes.scatter(data["cpus"], data[args.latency],
                 marker="o", color="black", alpha=0.5)
    x_space = np.linspace(0.1, data["cpus"].max())
    axes.plot(x_space, latency_func(x_space, *popt), '-')
    # axes.set_yscale('log')
    axes.set_title("Resource allocation vs target latency.")
    axes.set_xlabel("resource allocation / ($cores$)")
    axes.set_ylabel("latency / ($ms$)")
    axes.grid(True)

    buf = io.BytesIO()
    fig.tight_layout()
    fig.savefig(buf, format="png")
    tmp = buf.getbuffer()

    fig.clf()
    return base64.b64encode(tmp).decode("ascii")


def main(args):
    """
    Main logic.
    """
    data = get_data(args)
    if data is None:
        logging.info("Not enough data collected for: %s - %s.",
                     args.name, args.latency)
        return
    popt, data = analyse(data, args)
    if popt is not None:
        img = plot_results(data, popt, args)
        store_result(popt,
                     data,
                     img,
                     args)
        logging.info("Found plane for: %s:%s (%s).",
                     args.name, args.latency, popt)


@dataclasses.dataclass
class Arguments:
    """
    Dataclass that hold the arguments as parsed from argparse.
    """

    name: str
    latency: str
    min_vals: int
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
            min_vals=int(args['min_vals']),
            max_vals=int(args['max_vals']),
            mongo_uri=args['mongo_uri'],
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("name", type=str,
                        help="Name of the objective.")
    parser.add_argument("latency", type=str,
                        help="Name of the latency objective.")
    parser.add_argument("--min_vals", type=int, default=20,
                        help="Amount of features we want to collect before "
                             "even training a model")
    parser.add_argument("--max_vals", type=int, default=500,
                        help="Amount of features we want to include in the "
                             "model (basically defines how long we look back "
                             "in time).")
    parser.add_argument("--mongo_uri", type=str,
                        default=os.environ.get('MONGO_URL',
                                               'mongodb://localhost:27100'),
                        help="Mongo connection string.")
    parsed_args = dict(**vars(parser.parse_args()))
    arguments = Arguments.from_args(parsed_args)
    main(arguments)
