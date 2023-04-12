package astar

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/intel/intent-driven-orchestration/pkg/planner"

	"k8s.io/klog/v2"
)

// Node within a state graph.
type Node struct {
	value interface{}
}

// StringRepresentation generates a representation of the node's value.
func (state *Node) StringRepresentation() string {
	// try to marshall into a JSON like format...
	tmp, err := json.MarshalIndent(state.value, "", "\t")
	if err != nil {
		// ...if it doesn't work use %+v.
		return fmt.Sprintf("%+v", state.value)
	}
	return strings.Replace(strings.Replace(string(tmp), "\"", "'", -1), "\n", "\\l", -1)
}

// edge within of a state graph.
type edge struct {
	node    Node
	utility float64
	action  planner.Action
}

// stateGraph the Items graph
type stateGraph struct {
	nodes      []Node
	successors map[Node][]edge
}

// newStateGraph initializes a new state graph.
func newStateGraph() *stateGraph {
	return &stateGraph{
		nodes:      make([]Node, 0),
		successors: make(map[Node][]edge),
	}
}

// addNode adds a Node to the graph
func (sg *stateGraph) addNode(node Node) {
	sg.nodes = append(sg.nodes, node)
}

// addEdge adds an edge to the graph
func (sg *stateGraph) addEdge(src Node, trg Node, utility float64, action planner.Action) {
	sg.successors[src] = append(sg.successors[src], edge{trg, utility, action})
}

// toDot converts a state graph into graphviz's dot format.
func (sg *stateGraph) toDot(highlight []Node, fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	err = os.Chmod(fileName, 0600)
	if err != nil {
		klog.Fatalf("failed to change file permission %v", err)
	}

	var sb strings.Builder
	sb.WriteString("digraph plan {\n")
	for node, edges := range sg.successors {
		tmp := fmt.Sprintf("\"%v\" [label=\"%v\\l\" shape=box]; \n", node.value, node.StringRepresentation())
		sb.WriteString(tmp)
		for _, edge := range edges {
			color := "#262626"
			width := 1
			for j := range highlight {
				if j < (len(highlight)-1) && edge.node == highlight[j+1] && node == highlight[j] {
					color = "#0068b5"
					width = 2
				}
			}
			tmp := fmt.Sprintf("\"%v\" -> \"%v\" [label=\"%v - %f\" color=\"%s\" penwidth=%d];\n",
				node.value, edge.node.value, edge.action, edge.utility, color, width)
			sb.WriteString(tmp)
		}
	}
	sb.WriteString("}")

	if _, err = f.WriteString(sb.String()); err != nil {
		return err
	}
	return f.Close()
}
