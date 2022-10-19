package astar

import (
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
)

// newTestGraph return a state graph for testing purposes.
func newTestGraph() (stateGraph, Node, Node) {
	// Example taken from https://upload.wikimedia.org/wikipedia/commons/9/98/AstarExampleEn.gif
	sg := newStateGraph()
	start := Node{"start"}
	goal := Node{"goal"}
	nodeA := Node{"a"}
	nodeB := Node{"b"}
	nodeC := Node{"c"}
	nodeD := Node{"d"}
	nodeE := Node{"e"}
	sg.addNode(start)
	sg.addNode(goal)
	sg.addNode(nodeA)
	sg.addNode(nodeB)
	sg.addNode(nodeC)
	sg.addNode(nodeD)
	sg.addNode(nodeE)
	sg.addEdge(start, nodeA, 1.5, planner.Action{Name: "walk"})
	sg.addEdge(nodeA, nodeB, 2.0, planner.Action{Name: "walk"})
	sg.addEdge(nodeB, nodeC, 3.0, planner.Action{Name: "walk"})
	sg.addEdge(nodeC, goal, 4.0, planner.Action{Name: "done"})
	sg.addEdge(start, nodeD, 2.0, planner.Action{Name: "walk"})
	sg.addEdge(nodeD, nodeE, 3.0, planner.Action{Name: "walk"})
	sg.addEdge(nodeE, goal, 2.0, planner.Action{Name: "done"})
	return *sg, start, goal
}

// testHeuristic matches nodes in the graph to "real" distance on a map.
func testHeuristic(one, _ Node, _ map[string]common.Profile) float64 {
	switch one.value {
	case "a", "c":
		return 4.0
	case "b", "e":
		return 2.0
	case "d":
		return 4.5
	default:
		return 99.0
	}
}

// Tests for success.

// TestSolveForSuccess tests for success.
func TestSolveForSuccess(t *testing.T) {
	sg, start, goal := newTestGraph()
	profiles := map[string]common.Profile{}
	solve(sg, start, goal, testHeuristic, true, profiles)
}

// TestResolvePathForSuccess tests for success.
func TestResolvePathForSuccess(t *testing.T) {
	node0 := Node{value: "a"}
	node1 := Node{value: "b"}
	node2 := Node{value: "c"}
	data := map[Node]dataEntry{
		node2: {node1, planner.Action{Name: "walk"}, false},
		node1: {node0, planner.Action{Name: "walk"}, false},
		node0: {done: true},
	}
	resolvePath(data, &node0)
}

// Tests for failure

// TestSolveForFailure tests for failure.
func TestSolveForFailure(t *testing.T) {
	sg, start, _ := newTestGraph()
	profiles := map[string]common.Profile{}
	detached := Node{"detached_goal"}
	path, _ := solve(sg, start, detached, testHeuristic, true, profiles)
	if path != nil {
		t.Errorf("Results should be nil!")
	}
}

// Tests for sanity.

// TestSolveForSanity tests for sanity.
func TestSolveForSanity(t *testing.T) {
	sg, start, goal := newTestGraph()
	profiles := map[string]common.Profile{}
	path, actions := solve(sg, start, goal, testHeuristic, true, profiles)
	expectedPath := []string{"start", "d", "e", "goal"}
	for i, node := range expectedPath {
		if path[i].value != node {
			t.Errorf("Found %s - expected %s!", path[i].value, node)
		}
	}
	expectedActions := []string{"walk", "walk", "done"}
	for i, action := range expectedActions {
		if actions[i].Name != action {
			t.Errorf("Found %s - expected %s!", actions[i].Name, action)
		}
	}

	// no costs should not change the path...d -> e is still shortest.
	path, actions = solve(sg, start, goal, testHeuristic, false, profiles)
	expectedPath = []string{"start", "d", "e", "goal"}
	for i, node := range expectedPath {
		if path[i].value != node {
			t.Errorf("Found %s - expected %s!", path[i].value, node)
		}
	}
	expectedActions = []string{"walk", "walk", "done"}
	for i, action := range expectedActions {
		if actions[i].Name != action {
			t.Errorf("Found %s - expected %s!", actions[i].Name, action)
		}
	}
}

// TestResolvePathForSanity tests for sanity.
func TestResolvePathForSanity(t *testing.T) {
	node0 := Node{value: "a"}
	node1 := Node{value: "b"}
	node2 := Node{value: "c"}
	data := map[Node]dataEntry{
		node2: {node1, planner.Action{Name: "walk"}, false},
		node1: {node0, planner.Action{Name: "walk"}, false},
		node0: {done: true},
	}
	path, actions := resolvePath(data, &node2)
	if len(path) != 3 {
		t.Errorf("Results should have 3 elements!")
	}
	if path[0].value != "a" {
		t.Errorf("First element should be a!")
	}
	if path[1].value != "b" {
		t.Errorf("Second element should be b!")
	}
	if path[2].value != "c" {
		t.Errorf("Third element should be c!")
	}
	if len(actions) != 2 {
		t.Errorf("Actions should have 2 elements!")
	}
	if actions[0].Name != "walk" {
		t.Errorf("First element should be walk.")
	}
	if actions[1].Name != "walk" {
		t.Errorf("Second element should be walk.")
	}
}
