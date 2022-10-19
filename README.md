

# Intent Driven Orchestration Planner

![planner.png](planner.png)

Today’s container orchestration engine solutions promote a model of requesting a specific quantity of resources (e.g.
number of vCPUs.), a quantity range (e.g. min/max number of vCPUs) or not specifying them at all, to support the
appropriate placement of workloads. This applies at the Cloud and at the Edge using [Kubernetes](https://kubernetes.io/)
or [K3s](https://k3s.io/) (although the concept is not limited to Kubernetes based systems). The end state for the
resource allocation is declared, but that state is an imperative definition of what resources are required. This model
has proven effective, but has a number of challenges:

* experience shows that often suboptimal information is declared leading to over allocating resource or sub-optimal
  performance,
* these declarations have context that the users needs to understand, but often do not (e.g. resource requests do not
  dynamically change based on the chosen instance type that underpins a cluster).

This project proposes a new way to do orchestration – moving from an imperative model to an intent driven way, in which
the user express their intents in form of objectives (e.g. as required latency, throughput, or reliability targets) and
the orchestration stack itself determines what resources in the infrastructure are required to fulfill the objectives.
This new approach will continue to benefit from community investments in  _**scheduling**_ (determining **when & where**
to place workloads) and be augmented with a continuous running **_planning_** loop determining **what/how** to configure
in the system.

While this repository holds the planning component implementation it is key to note that it works closely together with
schedulers, the observability and potentially analytics stacks. It is key that those schedulers are fed with the right
information to make their placement decisions.

The planning component is essential for enabling Intent Driven Orchestration (IDO), as it will break down the higher-
level objectives (e.g. a latency compliance targets) into dynamic actionable plans (e.g. policies for platform resource
allocation, dynamic vertical & horizontal scaling, etc.). This enables hierarchical controlled systems in which Service
Level Objectives(SLOs) are broken down to finer grained goal settings for the platform. A key input to the planning
components, to determine the right set of actions, are the models that describe workload behaviour and the platform
effects on the associated Quality of Service (QoS).

The initial goal is to focus on managing the QoS of a set of instances of a workload. Subsequently, the goals are 
expected to shift to End-to-End (E2E) management of QoS parameters in multi-tenant environments with mixed criticality.
It is also a goal that the planning components will be easily extended and administrators will have the ability to swap 
in and out functionality through a plugin model. The architecture is intended to be extensible to support  proactive 
planning and coordination between planners to fulfill overarching intents. It is expected that the imperative model and
an Intent Driven Orchestration model will coexist.

## Example

To see the benefit of this model please review the deployment and associated objective manifest files:

* The Deployment [spec](artefacts/examples/example_deployment.yaml) only defines which container image to use (hence
  fully abstracted from platform resources. This is different but still analogous to Serverless methodology).
* The Objective [spec](artefacts/examples/example_intent.yaml) defines a P-95 latency compliance target of less than
  4ms as measured by e.g. a Service Mesh and requests an availability target of 2 nines (i.e. 99% availability).

## Running the planning component

Step 1) add the CRDs:

    $ k apply -f artefacts/intents_crds_v1alpha1.yaml

Step 2) deploy the planner (make sure to adapt the configs to your environment):

    $ k apply -f artefacts/deploy/manifest.yaml

Step 3) deploy the actuators of interest using:

    $ k apply -f plugins/<name>/<name>.yaml

These steps should be followed by setting up your default profiles (if needed).

For more information on running and configuring the planner see the [getting started](docs/getting_started.md) guide.

## Internals

There are three key packages enabling the Intent Driven Orchestration model:

1. The framework which enables the continuous running feedback loop monitoring the workload resources and matching the
   current to the desired states of the objectives in the systems.
2. The planning component which actively works on tuning resource requirements to bring the current state of the
   objectives closer to the desired states.
3. A set of actuators which enable the planner to:
    * predict the effect an orchestration activity has,
    * perform that action if required and,
    * (optionally) re-calculates/re-training the underling model that enables the earlier mentioned prediction.

Documentation and implementation notes for these components can be found here:

* Framework - [general notes on the framework](docs/framework.md)
* Planner - [general notes on planner & A* planner](docs/planner.md)
* Actuators - [notes on implementing actuators](docs/actuators.md)

Furthermore, notes on the pluggability can be found [here](docs/pluggability.md) and general design notes can be
found [here](docs/design_doc.md).

## Communication and contribution

Report a bug by [filing a new issue](https://github.com/intel/Intent-Driven-Orchestration/issues).

Contribute by [opening a pull request](https://github.com/intel/Intent-Driven-Orchestration/pulls). Please also see
[CONTRIBUTING](CONTRIBUTING.md) for more information.

Learn [about pull requests](https://help.github.com/articles/using-pull-requests/).

**Reporting a Potential Security Vulnerability:** If you have discovered potential security vulnerability in
Intent-Driven Orchestration, please send an e-mail to secure@intel.com. For issues related to Intel Products, please
visit [Intel Security Center](https://security-center.intel.com).

It is important to include the following details:

* The projects and versions affected
* Detailed description of the vulnerability
* Information on known exploits

Vulnerability information is extremely sensitive. Please encrypt all security vulnerability reports using our
[PGP key](https://www.intel.com/content/www/us/en/security-center/pgp-public-key.html).

A member of the Intel Product Security Team will review your e-mail and contact you to collaborate on resolving the
issue. For more information on how Intel works to resolve security issues, see:
[vulnerability handling guidelines](https://www.intel.com/content/www/us/en/security-center/vulnerability-handling-guidelines.html).
