package astar

import (
	"container/heap"
	"reflect"

	"k8s.io/klog/v2"

	plugins "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
)

// emptyActionName defines the name of the action connected nodes to the desired goal state.
const emptyActionName string = "done"

// opportunisticActionName defines the name of the action for opportunistic planning.
const opportunisticActionName string = "opportunistic"

// APlanner represent a planner using the A* algorithm.
type APlanner struct {
	cfg common.Config
	pm  plugins.ActuatorsPluginManager
}

// NewAPlanner initializes a new planner.
func NewAPlanner(actuators []actuators.Actuator, config common.Config) *APlanner {
	aPlanner := &APlanner{
		cfg: config,
		pm: plugins.NewPluginManagerServer(
			actuators,
			config.Planner.AStar.PluginManagerEndpoint,
			config.Planner.AStar.PluginManagerPort,
		),
	}
	// start the grpc plugin manager
	err := aPlanner.pm.Start()
	if err != nil {
		klog.ErrorS(err, "Error starting planner's plugin manager")
		return nil
	}
	return aPlanner
}

// getNodeForState return either an existing node in the graph representing the same state, or a new node.
func getNodeForState(sg stateGraph, state common.State) (Node, bool) {
	// reverse as we expect items to show up at end of list; hence we can break out of loop faster!
	for i := len(sg.nodes) - 1; i >= 0; i-- {
		node := sg.nodes[i]
		// TODO: check perf impact of reflect!
		if reflect.DeepEqual(*(node.value.(*common.State)), state) {
			return node, true
		}
	}
	return Node{&state}, false
}

// generateStateGraph creates the overall state graph.
func (p APlanner) generateStateGraph(start common.State, goal common.State, profiles map[string]common.Profile) (stateGraph, Node, Node, bool) {
	// let planning algorithm expand successors in future - for now this is easier to "knit in" the goal state, deal with duplicate states, etc.
	sg := newStateGraph()
	startNode := Node{&start}
	endNode := Node{&goal}
	sg.addNode(startNode)
	sg.addNode(endNode)
	queue := []Node{startNode}

	hasGoal := false

	for len(queue) > 0 && len(sg.nodes) < p.cfg.Planner.AStar.MaxStates {
		// current element...
		current := queue[0]
		queue = queue[1:]
		// find all success elements using actuators...
		itFct := func(a actuators.Actuator) {
			candidates, utils, actions := a.NextState(current.value.(*common.State), &goal, profiles)
			i := 0
			for i < len(candidates) && i < p.cfg.Planner.AStar.MaxCandidates {
				state := candidates[i]
				// TODO: add safeguard - we do not need 10 actions which lead to the same outcome.
				stateNode, found := getNodeForState(*sg, state)
				if !found {
					sg.addNode(stateNode)
					queue = append(queue, stateNode)
				}
				sg.addEdge(current, stateNode, utils[i], actions[i])
				if state.IsBetter(&goal, profiles) && !found {
					// if current better than desired - add edge to goal.
					sg.addEdge(stateNode, endNode, 0.0, planner.Action{Name: emptyActionName})
					hasGoal = true
				}
				i++
			}
		}
		p.pm.Iter(itFct)
	}
	// if desired > goal we also add a shortcut path with the cost of the depth of the graph. Additionally, we add a
	// little costs if any action in the current graph would have modified sth.
	if start.IsBetter(&goal, profiles) && len(sg.successors) > 0 {
		shortestPath, _ := solve(*sg, startNode, endNode, hEmpty, false, profiles)
		if shortestPath != nil {
			tmp := float64(len(shortestPath) - 2)
			lastItem := shortestPath[len(shortestPath)-2]
			if len(start.CurrentPods) > len(lastItem.value.(*common.State).CurrentPods) || lastItem.value.(*common.State).LessResources(&start) {
				// shortest path already brought a change, so we should make a bit more unlikely to take the shortcut.
				tmp *= 1.01
			}
			// we add an intermediate node so the whole trick with the distance heuristic works.
			intermediate := start.DeepCopy()
			intermediate.Intent.TargetKey = "intermediate"
			intermediateNode := Node{&intermediate}
			sg.addNode(intermediateNode)
			sg.addEdge(startNode, intermediateNode, tmp, planner.Action{Name: emptyActionName})
			sg.addEdge(intermediateNode, endNode, 0.0, planner.Action{Name: emptyActionName})
			hasGoal = true
		}
	}
	return *sg, startNode, endNode, hasGoal
}

// heuristic for finding shorted path.
func hEmpty(_ Node, _ Node, _ map[string]common.Profile) float64 {
	return 0.0
}

// heuristic based on the distance of the states.
func h(one Node, other Node, profiles map[string]common.Profile) float64 {
	state := one.value.(*common.State)
	return state.Distance(other.value.(*common.State), profiles)
}

// addAdditionalStates will add edges between n states with the closest distance to the goal state to the state graph.
func (p APlanner) addAdditionalStates(sg stateGraph, start Node, goal Node, profiles map[string]common.Profile) stateGraph {
	minDistances := make(PriorityQueue, 0)
	heap.Init(&minDistances)
	for _, node := range sg.nodes {
		if node == goal || node == start {
			continue
		}
		heap.Push(&minDistances, &Item{
			value:    node,
			priority: node.value.(*common.State).Distance(goal.value.(*common.State), profiles),
		})
	}
	for i := 0; i < p.cfg.Planner.AStar.OpportunisticCandidates; i++ {
		if len(minDistances) > 0 {
			item := heap.Pop(&minDistances).(*Item)
			sg.addEdge(item.value.(Node), goal, 0.0, planner.Action{Name: opportunisticActionName})
		} else {
			break
		}
	}
	return sg
}

func (p APlanner) CreatePlan(current common.State, desired common.State, profiles map[string]common.Profile) []planner.Action {
	klog.V(2).Infof("Trying to create a plan to get from %v to %v.", current, desired)
	var plan []planner.Action

	sg, s0, g0, goal := p.generateStateGraph(current, desired, profiles)
	klog.V(2).Infof("State graph has %d nodes.", len(sg.nodes))
	if goal {
		_, actions := solve(sg, s0, g0, h, true, profiles)
		plan = actions
	} else {
		klog.Warning("No path to goal state possible!")
		if p.cfg.Planner.AStar.OpportunisticCandidates > 0 {
			klog.Infof("Opportunistic planning is enabled - will add %d states with closest distance to the "+
				"desired state to the state graph.", p.cfg.Planner.AStar.OpportunisticCandidates)
			sg = p.addAdditionalStates(sg, s0, g0, profiles)
			_, actions := solve(sg, s0, g0, h, true, profiles)
			plan = actions
		}
	}
	var finalPlan []planner.Action
	for _, item := range plan {
		if item.Name == emptyActionName || item.Name == opportunisticActionName {
			continue
		}
		finalPlan = append(finalPlan, item)
	}
	klog.V(2).Infof("A*star planner found: %v.", finalPlan)
	return finalPlan
}

func (p APlanner) ExecutePlan(state common.State, plan []planner.Action) {
	klog.V(2).Info("Execute plan called.")
	itFct := func(a actuators.Actuator) {
		a.Perform(&state, plan)
	}
	p.pm.Iter(itFct)
}

func (p APlanner) TriggerEffect(current common.State, profiles map[string]common.Profile) {
	klog.V(2).Info("Trigger effect re-calculation on all actuators.")
	itFct := func(a actuators.Actuator) {
		// TODO fix go routines, spawn them in the actuators
		go a.Effect(&current, profiles)
		// TODO do we want to have a wait channel here and block until everything done?
	}
	p.pm.Iter(itFct)
}

func (p APlanner) Stop() {
	err := p.pm.Stop()
	if err != nil {
		klog.ErrorS(err, "Error stopping planner's plugin manager")
	}
}
