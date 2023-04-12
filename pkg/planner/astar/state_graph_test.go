package astar

import (
	"os"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/planner"
)

// Tests for success.

// TestAddNodeForSuccess tests for success.
func TestAddNodeForSuccess(t *testing.T) {
	g := newStateGraph()
	node0 := Node{"foo"}
	node1 := Node{"bar"}
	g.addNode(node0)
	g.addNode(node1)
}

// TestAddEdgeForSuccess tests for success.
func TestAddEdgeForSuccess(t *testing.T) {
	g := newStateGraph()
	node0 := Node{"foo"}
	node1 := Node{"bar"}
	g.addNode(node0)
	g.addNode(node1)
	action := planner.Action{Name: "foo"}
	g.addEdge(node0, node1, 1.0, action)
}

// TestToDotForSuccess tests for sanity.
func TestToDotForSuccess(t *testing.T) {
	g := newStateGraph()
	err := g.toDot([]Node{}, "tmp.dot")
	if err != nil {
		t.Errorf("Error to use the tmp file: %s", err)
	}
	err = os.Remove("tmp.dot")
	if err != nil {
		t.Errorf("Could not cleanup temporary file - BE CAREFUL NOW! %s", err)
	}
}

// Tests for failure.

// n/a.

// Tests for sanity.

// TestAddNodeForSanity tests for sanity.
func TestAddNodeForSanity(t *testing.T) {
	g := newStateGraph()
	node0 := Node{"node0"}
	g.addNode(node0)
	if len(g.nodes) != 1 || g.nodes[0] != node0 {
		t.Errorf("Expected node not found.")
	}
}

// TestAddEdgeForSanity tests for sanity.
func TestAddEdgeForSanity(t *testing.T) {
	g := newStateGraph()
	node0 := Node{"foo"}
	node1 := Node{"bar"}
	g.addNode(node0)
	g.addNode(node1)
	action := planner.Action{Name: "bla"}
	g.addEdge(node0, node1, 1.0, action)

	if len(g.successors[node0]) != 1 || g.successors[node0][0].node != node1 {
		t.Errorf("Expecting node1 to be the successor of node0.")
	}
	if len(g.successors[node1]) != 0 {
		t.Errorf("Not expecting any succesors for node1.")
	}
}

// TestToDotForSanity tests for sanity.
func TestToDotForSanity(t *testing.T) {
	g := newStateGraph()
	start := Node{"start"}
	goal := Node{"goal"}
	g.addNode(start)
	g.addNode(goal)
	g.addEdge(start, goal, 0.0, planner.Action{Name: "run"})

	err := g.toDot([]Node{start, goal}, "tmp.dot")
	if err != nil {
		t.Errorf("Failed to write DOT file: %s", err)
	}
	err = os.Remove("tmp.dot")
	if err != nil {
		t.Errorf("Could not cleanup temporary file - BE CAREFUL NOW!, %s", err)
	}
}

// TestStatGraphForSanity tests for sanity.
func TestStatGraphForSanity(t *testing.T) {
	g := newStateGraph()
	start := Node{"DeploymentWithOnePod"}
	morePods := Node{"DeploymentWithTwoPods"}
	resTune := Node{"DeploymentWithBetterResources"}
	goal := Node{"goal"}

	g.addNode(start)
	g.addNode(morePods)
	g.addNode(resTune)
	g.addNode(goal)

	scale := planner.Action{Name: "scale_out"}
	tune := planner.Action{Name: "tune_res"}
	nothing := planner.Action{Name: "nothing"}

	g.addEdge(start, morePods, 1.0, scale)
	g.addEdge(start, resTune, 0.9, tune)

	g.addEdge(morePods, goal, 0.0, nothing)
	g.addEdge(resTune, goal, 0.0, nothing)
}

// BenchmarkStateGraphCreation does a quick benchmark test.
func BenchmarkStateGraphCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		g := newStateGraph()
		for j := 0; j < 9999; j++ {
			tmp := Node{"blurbs"}
			g.addNode(tmp)
			if j > 1 {
				g.addEdge(tmp, g.nodes[j], 0.0, planner.Action{Name: "dummy"})
			}
		}
	}
}
