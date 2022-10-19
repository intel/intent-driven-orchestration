# Planning

The interface for the planner is defined [here](../pkg/planner/interface.go). The main method is defined as:

    CreatePlan(current common.State, desired common.State, profiles map[string]common.Profile) []Action

Given the current and desired states the planner should be able to come up with a set of actions that will enable an
Intent Driven Orchestration model. Basically translating intents (in form of objectives/SLOs/KPIs) into something
actionable.

Similar to the scheduler in a control plane the planner is configurable and supports an extension model.
Pluggable [Actuators](actuators.md) and the concept that planning algorithms can be swapped out refined this idea.

## A* based planner

[The A*](../pkg/planner/astar/astar_planner.go) planner uses a graph search algorithm to come up with a plan. This is an
elegant general purpose planner; more advanced planning capabilities can easily be implemented as long as they follow
the previously described interface.

The planner first determines the overall state graph with the help of the [actuators](actuators.md). For each given
state it determines a possible set of follow-up states. If the follow state(s) are better than the desired state an edge
is added connecting the state to the desired goal state.

The edges in the state graph are annotated with the (potential) actions and the utility cost of those actions. Using
this utility value the most efficient path from current to desired goal state is determined, ultimately defining the
plan of actions that need to be performed.

The planner uses a heuristic based search - to do so it uses a Euclidean distance (defined for the states) function.

It supports opportunistic planning capabilities (which can be configured) in case that state graph contains no path to
the desired state, in that case the planner will try to connect state in the state graph which are close to the desired
state.
