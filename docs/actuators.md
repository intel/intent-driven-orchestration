# Actuators

Actuators are a key component to enabling Intent Driven Orchestration. Actuators are wrappers around orchestration
activities. Each actuator has a particular scope defined by the action it supports. For example a scale-out actuator
might be in charge of adding additional replicas to a Deployment.

The planner uses the actuators for a set of tasks:

1. given a state of
   a [Kubernetes workload resource](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/), determine
   how the action supported by the actuator would change the objectives.
2. actually perform the action if the planner deems that necessary.
3. (Optionally) trigger a (re-)training of the model describing the effect that the action has on the objectives.

## Design principles

* Actuators should be reasonably standalone & self-contained; ideally they are modular and support one action only.
* Actuators are key for the planner to determine a possible set of actions to perform, however they are separated from
  the planning process. Different planner implementation can reuse actuators if needed.
* Actuators are the component that shifts the responsibility of needing to know which actions are available or what
  their effect on a workload is from the service owner towards the orchestration system.

## Interface

The interface an actuator needs to implement is defined [here](../pkg/planner/actuators/types.go).

    // Actuator defines the interface for the actuators.
    type Actuator interface {
        Plugin
        // NextState should return a set of potential follow-up states for a given state if this actuator would potentially be used.
        NextState(state *common.State, goal *common.State, profiles map[string]common.Profile) ([]common.State, []float64, []planner.Action)
        // Perform should perform those actions of the plan that it is in charge off.
        Perform(state *common.State, plan []planner.Action)
        // Effect should (optionally) recalculate the effect this actuator has for ALL objectives for this workload.
        Effect(state *common.State, profiles map[string]common.Profile)
    }

Each actuator will have a unique name and a group assignment.

The planner will use ***NextState()*** during the planning process, once a plan is determined ***Perform()*** is called,
and finally ***Effect()*** is being triggered by the framework.

### Implementing ***NextState()***

The suggested basic flow to follow when implementing this is as follows:

1. create one or more potential follow-up states using a *DeepCopy()* of the current state.
2. determine the effect the action has on the objectives. For this loop over the state's objectives and using
   prediction/forecasting fill in the new values. Also manipulate any attributes of the states that might be effect by
   this action.
3. (Optionally - if you do not think another actuator will benefit from your changes) Check if the new state is better
   than the old state.
4. Calculate a utility/cost for this action (see notes below).
5. Add your state, the utility and the actions (with optionally parameters) to the results.

When predicting the possible follow-up states the implementation can either take the information as provided by current
state (which can include information on metrics such as CPU utilization, capacity, etc.) or even forecast where the
overall system will be once a potential plan is being triggered. For example, it could forecast the future throughput in
a dynamic system and scale out the workload resource, thought even right now all objectives are fulfilled. This enables
a proactive style of management. Also, it can let the planner trigger an action to test something out - in case it is
running in an online continuous learning mode - to learn the effect of the same.

Note that ***NextState()*** also enables the system to let two or more actuators to work together; Scaling a set of PODs
in the Burstable QoS class vs scaling a set of PODs in the Guaranteed QoS class might have different effects; so if one
actuator enables the right QoS class the scaling actuator might benefit from that information. See notes on utility
functions in the next section to better understand how the system selects what to do.

Furthermore, the implementation of ***NextState()*** can support the opportunistic planning capabilities, by adding new
states, that although they do not satisfy the desired still at least move the system in the right direction.

#### Utility/Cost functions

Utilities are used to steer the planner. Planners will deem an action to be favorable if the actuator returns a low
utility value and the associated new state brings it closer to the desired state. The default "A*"" based planner
will favor actions that have a utility in the range of [0.0 - 1.0], albeit the maximum value is not limited - actions
with a high value just become more unlikely to be added to the plan.

Utility functions that calculate the value can take multiple attributes into account (following the concept of
Multi-Attribute-Utility-Theory). A good utility functions balances between the benefits for the resource and service
owner while taking attributes into account from one of the following categories:

* the current state of the system (e.g. available headroom/capacity.)
* the potential new states of the system (e.g. predicted latency in relation to current latency.)
* the financial cost/credits associated with this action.
* miscellaneous other aspects such as the priority of the workload to balance its benefits in contrast to workload of
  other tenants.

The utility/costs can change based on the context the overall system is in. While the system is empty the actuators
might return lower numbers while in a full-system it returns higher numbers.

For example, the utility function for a scale-out action can take into account the effect the scale-out has on the
target latency numbers & currently associate capacity. While the system is empty and adding two more replicas will
reduce the latency the utility could be close to 0.0. While if the system is almost full and a scale out by 10 replicas
would be needed to fulfill the objectives might have a value > 1.0 and hence be unfavorable. Similarly, scaling a
high priority workload might have a low cost, the scaling of low priority might have a high cost.

### Implementing ***Perform()***

Implementing ***Perform()*** is straight forward - the actuator's implementation should loop though the actions in the
plan and actuate upon those that are managed by the actuator. The overall plan is given so that overall context is
understood. For example a plan with the following actions **[scale_out{"factor": 2}, rm_pod{"name": "pod_2"}]** might
actually just mean we need to increase the number of replicas by 1 and not 2.

Actions can do various things, such as:

* manipulate Kubernetes objects such as Deployment or POD specs - and implicitly give hints to e.g. the scheduler.;
    * this can include changing resource allocations, node & profile labels, annotations, selectors, affinities, etc.
* remove or add PODs;
* activate/change policies;
* trigger platform changes/configuration through IPMI;
* etc. etc. etc.

### Implementing ***Effect()***

***Note***: This is optional. For example, if two actuators use the same model, only one needs to implement this. Or in
case a static lookup table is used, this step can also be skipped.

It is recommended to let this function trigger the analytics stack to come up with a new model. Do this by e.g.
triggering a function that will execute an analytics scripts that makes use of data coming from the observability stack.

As described in the implementation notes for ***NextState()***, actuators must be able to predict/forecast what the
effect the action has on a (set of) objectives. We use models for this; the term models is used in the broad term as it
can represent a lookup-table, a simple regression model, a neural network (e.g. LSTM to predict throughput) etc.. As
long as given a current set of features it can predict the effect of the action, it is a reasonable model to use.

Models can be static or non-static. A model that does not change is flagged as being static, while a model which is
updated/re-trained in an online learning continuous loop will be non-static. Non-static models can time out - an
actuator can decide to not use a model that is older than some period (e.g. 1 hour).

Models can be shared across multiple actuators. Ideally all models used within the system share similar training
features, so a scale-out actuator can account for the fact that it might scale PODs that make use of some platform
feature and in contrast scale PODs that do not use that feature.

An actuator can use one or more models; one for predicting the effect of the action the actuator handles, and supporting
models that helps to make accurate predictions, e.g. a model enabling predicting what throughput capacity there might
be in the future.

The analytics parts can use the *events*' collection in the knowledge base, but is not required to do so and can take
into account a rich set of logs, counter, gauges or traces from the observability stack.

It is recommended to keep only as much data around as needed for IDO scalability reasons, and only trigger re-trainings
if really needed. This can be done by e.g. using anomaly detection or drift detection techniques to determine if a model
needs to be updated.

Models can be pre-determined in a DevOps kind of way in the CI/CD pipeline using various tools - for example
[SigOpt](https://sigopt.com/). If this route is taken the ***Effect()*** function can trigger the analytics part to
re-evaluate the need to re-train the model and/or trigger the re-training/updating of the model with new information in
the Prod system.

In the actuator's implementation a model can be exchanged with another one over time - a model used during the start
might be purely analytical (e.g. a curve-fitting model), but as more data becomes available the model can be switched to
a more empirical solution (e.g. a NN that requires more data-points).

## General notes

* Actuators and their action can indirectly interact - e.g. a scaling actuator can take into account the following
  features for it's model: *[resources, replicas, throughput]* to predict a *[latency]*. Although itself can only
  influence the replicas, another actuator might suggest setting the resources' allocation more appropriately. So
  multiple actions (as part of a plan) in combination can lead to the desired result. For example a *scale_out* action
  in combination with *set_resources* (such as CPU or Mem) leads to a desired P99 compliance level.
* As the actuator falls in the domain of the cluster administrator it can take a more global view of the overall system.
  For example, it can look into the resource allocated to other workload resources and take that into account;
  especially to balance between workload resource with varying priority in a multi-tenant environment.
