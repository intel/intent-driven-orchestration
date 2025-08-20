
# Getting Started

This guide will direct you through the setup and configuration of the planning component enabling the Intent Driven
Orchestration model.

# Setup

The following sections describe the required steps to set up the planner.

## Prerequisites

Observability data is one of the key enablers of the closed-loop system enabled by the planner. Especially telemetry
data describing the KPIs that have associated Intent targets. Hence, an observability stack needs to be present.
The planning component is currently capable of querying [Prometheus](https://prometheus.io/) based endpoints. Data in
one - or more - of such instances can be used by the planner.

Host based information - which can optionally be consumed - can be collected using tools such as
[collectd](https://collectd.org/) or [telegraf](https://www.influxdata.com/time-series-platform/telegraf/) -
which in turn can leverage [OpenTelemetry](https://opentelemetry.io/). Data from these sources may also be accessible
via a central endpoint such as Prometheus.

There are multiple ways of getting an application's KPIs into a Prometheus instance. Application's can make use of the
[Prometheus Client Libraries](https://prometheus.io/docs/instrumenting/clientlibs/) to expose an endpoint that can be
scraped by Prometheus. Service meshes - such as [Linkerd](https://linkerd.io/), or [Istio](https://istio.io/) can also
be used to collect information on KPIs. Both come with extensions that support the collection and storage of KPI in
Prometheus endpoints.

At minimum one Prometheus endpoint -- containing the KPIs IDO aims to control for -- needs to be accessible.

## Custom Resource Definitions

The Kubernetes API is extended through Custom Intent and KPI Profile resources. A description of the newly supported
kinds (mainly **Intent** and **KPIProfile**) can be found [here](framework.md).

Those can be added to the Kubernetes cluster using:

    $ kubectl apply -f artefacts/intents_crds_v1alpha1.yaml

This step can be verified by listing the custom resources known to the cluster:

    $ kubectl get crd

## Planner

The planning component can be deployed in the cluster using the provided example manifest file:

    $ kubectl apply -f artefacts/deploy/manifest.yaml

**_Note_**: This file contains a set of basic configurations and grants the necessary permissions to the planner. It is
advised to review and adapt the provided basic settings. The manifest also includes recommended resource requests and
limits, it is advised to review and change them according to you environment.

The planner will require a certain set of permissions. It will need to be able to "get", "list", and "watch" the
resources defined in the CRD, as well as the ability to update their status. To enable modifications of POD specs the
planner needs to be able to "get", "list", "watch", "patch" and "delete" PODs. Lastly, to enable modifications to
ReplicaSet and Deployment specs, the planner need "get", "patch", and "update" permissions on the same.

After deploying the basic framework enable the actuators that are of interest in, and deploy them using:

    $ kubectl apply -f plugins/<name>/<name>.yaml

A configuration reference for the planner and actuators is provided towards the end of this document.

### (Optional) Enable default KPI profiles

Each objective in an intent declaration needs to refer to a KPI profile. Service owners can either define their own
KPI profiles or use those specified by the resource provider through a set of default profiles. To allow for this,
a set of Prometheus based queries can be defined, and a link to that definition be added to the planner's
configuration file. If a KPI profiles is created and the name matches with an entry, the defined query will be used.

An example queries file is defined [here](../artefacts/examples/default_queries.json) - with a set of example queries
for a service mesh. Note that the query string does contain "%s" entries, which are replaced with the namespace, kind,
and name of the workload resource at runtime.

The matching set of KPI profiles can be applied using:

    $ kubectl apply -f artefacts/examples/default_profiles.yaml

# Demo

Once the planner is set up and ready to go the following steps enable a basic demo.

A simple deployment - note the absence of any detailed resource information - can be deployed using:

    $ kubectl apply -f artefacts/examples/example_deployment.yaml

This will bring up a simple application that can react to HTTP requests. For example, to do some matrix calculations
the following curl requests would do:

    $ curl -X GET http://function-service:8000/matrix/100

Next an intent can be associated with this Deployment - note that the numbers defined here will depend on the context
you are running your application in:

    $ kubectl apply -f artefacts/examples/example_intent.yaml

Now if the application is hit with various load levels (e.g. using a load generator), the planner will try to adhere to
declared Intents. Over time the planner will learn a scaling model and use that to scale the application appropriately.
For more details on models see the [actuators'](actuators.md) documentation.

This intent declaration with the demo assumes a service mesh is used to measure the KPIs. The KPI profiles used match
the default queries described earlier.

**_Note_** that for this demonstration, it is assumed that proactive and opportunistic planning are enabled. See the
configuration references for more details on this.

# Reference

## Configuration

The main configuration for the planning components are set in the [defaults.cfg](../defaults.json) file. It contains
sections for each of the [framework's major components](framework.md) as well as generic settings.

### Generic

| Property       | Description                                                                 |
|----------------|-----------------------------------------------------------------------------|
| mongo_endpoint | URI for the Mongo database - representing the knowledge base of the system. |
| log_file       | (Optional) Path to a log file to config klog.                               |

### Controller

| Property            | Description                                                                                                                                                                                                                |
|---------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| workers             | Amount of workers to use for processing the intents. Minimum is 1, maximum is equal to number of cores available.                                                                                                          |
| task_channel_length | Max length of the job queue for processing intents.                                                                                                                                                                        |
| informer_timeout    | Timeout in seconds for the informer factories for the CRDs and PODs.                                                                                                                                                       |
| controller_timeout  | Interval in seconds between each intent's reevaluation.                                                                                                                                                                    |
| plan_cache_ttl      | Time to live in ms for an entry in the planner's cache. After a plan has been determined this is the time the planner will not trigger the creation of a plan for the same intent.                                         |  
| plan_cache_timeout  | Timeout in ms between re-evaluating the entries in the planner's cache. Should be smaller than plan_cache_ttl.                                                                                                             |  
| telemetry_endpoint  | URI for a Prometheus API endpoint for the host/node level observability data information.                                                                                                                                  |  
| host_field          | String defining the tag that defines the hostnames.                                                                                                                                                                        |
| metrics             | List of key-value maps; Each map containing a _name_ and a _query_ property - defining the queries to run against the previous defined Prometheus query API. A string replacement is done for %s to define the host names. |

### Monitor

| Property        | Description                                                                                                                           |
|-----------------|---------------------------------------------------------------------------------------------------------------------------------------|
| pod.workers     | Amount of workers to use for processing the POD related events. Minimum is 1, maximum is equal to number of cores available.          |
| profile.workers | Amount of workers to use for processing the KPI profiles related events. Minimum is 1, maximum is equal to number of cores available. |
| profile.queries | Path to a JSON file defining default queries for a set of given KPI profiles.                                                         |
| intent.workers  | Amount of workers to use for processing the Intent related events. Minimum is 1, maximum is equal to number of cores available.       |

### Planner

| Property                       | Description                                                                                                                                        |
|--------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| astar.opportunistic_candidates | Number of states that can opportunistically be selected because of the distance to the desired state, in case the desired state cannot be reached. |
| astar.max_states               | Maximum number of states to add to the state graph as a whole.                                                                                     |
| astar.max_candidates           | Maximum number of states to add to the state graph from each individual actuator.                                                                  |
| astar.plugin_manager_endpoint  | String defining the plugin manager's endpoint to which actuators can register.                                                                     |
| astar.plugin_manager_port      | Port number of the plugin manager's endpoint to which actuators can register.                                                                      |

## Actuator configuration

Each actuator will have its own configuration.

### scale out actuator

| Property                 | Description                                                                               |
|--------------------------|-------------------------------------------------------------------------------------------|
| interpreter              | Path to a python interpreter.                                                             |
| analytics_script         | Path to the python script used for analytics.                                             |
| max_pods                 | Maximum number of PODs this actuator will scale to.                                       |
| look_back                | Time in minutes defining how old the ML model can be.                                     |
| max_proactive_scale_out  | Maximum numbers of PODs for testing how scaling affects the Objectives.                   |
| proactive_latency_factor | Float defining the potential drop in latency by scaling out by one POD.                   |
| endpoint                 | Name of the endpoint to use for registering this plugin.                                  |
| port                     | Port this actuator should listen on.                                                      |
| mongo_endpoint           | URI for the Mongo database - representing the knowledge base of the system.               |
| plugin_manager_endpoint  | String defining the plugin manager's endpoint to which actuators can register themselves. |
| plugin_manager_port      | Port number of the plugin manager's endpoint to which actuators can register themselves.  |

### remove pod actuator

| Property                | Description                                                                               |
|-------------------------|-------------------------------------------------------------------------------------------|
| min_pods                | Minimum number of PODs this actuator will scale to.                                       |
| look_back               | Time in minutes defining how old the ML model can be.                                     |
| endpoint                | Name of the endpoint to use for registering this plugin.                                  |
| port                    | Port this actuator should listen on.                                                      |
| mongo_endpoint          | URI for the Mongo database - representing the knowledge base of the system.               |
| plugin_manager_endpoint | String defining the plugin manager's endpoint to which actuators can register themselves. |
| plugin_manager_port     | Port number of the plugin manager's endpoint to which actuators can register themselves.  |

### cpu scale actuator

| Property                     | Description                                                                                                                                                                                                  |
|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| interpreter                  | Path to a python interpreter.                                                                                                                                                                                |
| analytics_script             | Path to the analytics python script used to determine the scaling model.                                                                                                                                     |
| cpu_max                      | Maximum CPU resource units (in millis) that the actuator will allow.                                                                                                                                         |
| cpu_rounding                 | Multiple of 10 defining how to round up CPU resource units.                                                                                                                                                  |
| cpu_safeguard_factor         | Define the factor the actuator will use to stay below the targeted objective.                                                                                                                                |
| boostFactor                  | Defines the multiplication factor for calculating resource limits from requests. If set to 1.0 PODs will be in a Guaranteed QoS, smaller or larger values lead to a BestEffort or Burstable QoS accordingly. |
| look_back                    | Time in minutes defining how old the ML model can be.                                                                                                                                                        |
| max_proactive_cpu            | Maximum CPU resource units (in millis) that the actuator will allow when proactively scaling. If set to 0, proactive planning is disabled. A fraction of this value is used for proactive scale ups/downs.   |
| proactive_latency_percentage | Float defining the potential percentage change in latency by scaling the resources.                                                                                                                          |
| endpoint                     | Name of the endpoint to use for registering this plugin.                                                                                                                                                     |
| port                         | Port this actuator should listen on.                                                                                                                                                                         |
| mongo_endpoint               | URI for the Mongo database - representing the knowledge base of the system.                                                                                                                                  |
| plugin_manager_endpoint      | String defining the plugin manager's endpoint to which actuators can register themselves.                                                                                                                    |
| plugin_manager_port          | Port number of the plugin manager's endpoint to which actuators can register themselves.                                                                                                                     |

### RDT actuator

| Property                | Description                                                                               |
|-------------------------|-------------------------------------------------------------------------------------------|
| interpreter             | Path to a python interpreter.                                                             |
| analytics_script        | Path to the python script used for analytics.                                             |
| prediction_script       | Path to the python script used for predicting affects of the actuator.                    |
| options                 | List of strings for the RDT configuration options.                                        |
| endpoint                | Name of the endpoint to use for registering this plugin.                                  |
| port                    | Port this actuator should listen on.                                                      |
| mongo_endpoint          | URI for the Mongo database - representing the knowledge base of the system.               |
| plugin_manager_endpoint | String defining the plugin manager's endpoint to which actuators can register themselves. |
| plugin_manager_port     | Port number of the plugin manager's endpoint to which actuators can register themselves.  |
