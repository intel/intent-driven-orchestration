package astar

import (
	"container/heap"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	"k8s.io/klog/v2"
)

// See: [wikipedia](https://en.wikipedia.org/wiki/A*_search_algorithm#Pseudocode)

// heuristic guides the planning algorithm.
type heuristic func(one Node, goal Node, profiles map[string]common.Profile) float64

// dataEntry helps keeping track of actions leasing to a certain node.
type dataEntry struct {
	node   Node
	action planner.Action
	done   bool
}

// resolvePath traces back the actual shorted Path.
func resolvePath(data map[Node]dataEntry, current *Node) ([]Node, []planner.Action) {
	path := []Node{*current}
	var actions []planner.Action
	for current != nil {
		entry := data[*current]
		current = &entry.node
		if entry.done {
			break
		}
		path = append([]Node{*current}, path...)
		actions = append([]planner.Action{entry.action}, actions...)
	}
	return path, actions
}

// solve finds a path between two nodes in the state graph.
func solve(sg stateGraph, start Node, goal Node, h heuristic, useUtility bool, profiles map[string]common.Profile) ([]Node, []planner.Action) {
	open := make(PriorityQueue, 0)
	heap.Init(&open)
	heap.Push(&open, &Item{
		value:    start,
		priority: 0.0,
	})
	gScore := map[Node]float64{start: 0.0}
	data := map[Node]dataEntry{start: {done: true}}

	for open.Len() > 0 {
		current := heap.Pop(&open).(*Item).value.(Node)
		if current == goal {
			return resolvePath(data, &current)
		}
		for _, edge := range sg.successors[current] {
			var newGScore float64
			if useUtility {
				newGScore = gScore[current] + edge.utility
			} else {
				newGScore = gScore[current] + 1.0
			}
			if _, ok := gScore[edge.node]; !ok || newGScore < gScore[edge.node] {
				gScore[edge.node] = newGScore
				priority := newGScore + h(edge.node, goal, profiles)
				heap.Push(&open, &Item{value: edge.node, priority: priority})
				data[edge.node] = dataEntry{current, edge.action, false}
			}
		}
	}
	klog.Warningf("Could not find path from %v to %v", start, goal)
	return nil, nil
}
