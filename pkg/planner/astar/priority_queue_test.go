package astar

import (
	"container/heap"
	"testing"
)

// Tests for success.

// TestLenForSuccess tests for success.
func TestLenForSuccess(_ *testing.T) {
	queue := make(PriorityQueue, 0)
	heap.Init(&queue)
	queue.Len()
}

// TestLessForSuccess tests for success.
func TestLessForSuccess(_ *testing.T) {
	queue := make(PriorityQueue, 0)
	heap.Init(&queue)
	item1 := &Item{
		value:    "foo",
		priority: 1,
	}
	item2 := &Item{
		value:    "bar",
		priority: 2,
	}
	heap.Push(&queue, item1)
	heap.Push(&queue, item2)
	queue.Less(0, 1)
}

// TestSwapForSuccess tests for success.
func TestSwapForSuccess(_ *testing.T) {
	queue := make(PriorityQueue, 0)
	heap.Init(&queue)
	item1 := &Item{
		value:    "foo",
		priority: 1,
	}
	item2 := &Item{
		value:    "bar",
		priority: 2,
	}
	heap.Push(&queue, item1)
	heap.Push(&queue, item2)
	queue.Swap(0, 1)
}

// TestPushForSuccess tests for success.
func TestPushForSuccess(_ *testing.T) {
	queue := make(PriorityQueue, 0)
	heap.Init(&queue)
	item := &Item{
		value:    "item0",
		priority: 1,
	}
	heap.Push(&queue, item)
}

// TestPopForSuccess tests for success.
func TestPopForSuccess(_ *testing.T) {
	queue := make(PriorityQueue, 0)
	heap.Init(&queue)
	item := &Item{
		value:    "item0",
		priority: 1,
	}
	heap.Push(&queue, item)
	heap.Pop(&queue)
}

// Test for failure.

// N/A

// Test for sanity.

// TestPushPopForSanity tests for sanity.
func TestPushPopForSanity(t *testing.T) {
	queue := make(PriorityQueue, 0)
	heap.Init(&queue)

	// push elements on heap and ...
	for i, val := range []float64{3.0, 0.0, 3.0, 5.0, 2.0} {
		tmp := &Item{
			value:    i,
			priority: val,
		}
		heap.Push(&queue, tmp)
	}

	// ... now pop them again.
	for _, expected := range []float64{0.0, 2.0, 3.0, 3.0, 5.0} {
		val := heap.Pop(&queue).(*Item)
		if val.priority != expected {
			t.Errorf("Expected '%f' got: '%f'.", expected, val.priority)
		}
	}
}
