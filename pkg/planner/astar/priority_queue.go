package astar

// See: https://pkg.go.dev/container/heap priority queue example.

// Item represents an entity in the queue.
type Item struct {
	// TODO: generics: type Item[T any, PT *T] struct {
	value    interface{}
	priority float64
	index    int
}

// PriorityQueue is a min priority queue.
type PriorityQueue []*Item

// Len returns the length of the queue.
func (queue PriorityQueue) Len() int {
	return len(queue)
}

// Less determines an item with the lowest priority.
func (queue PriorityQueue) Less(i, j int) bool {
	return queue[i].priority < queue[j].priority
}

// Swap swaps to items in the queue.
func (queue PriorityQueue) Swap(i, j int) {
	queue[i], queue[j] = queue[j], queue[i]
	queue[i].index = i
	queue[j].index = j
}

// Push adds an item to the queue.
func (queue *PriorityQueue) Push(x interface{}) {
	n := len(*queue)
	item := x.(*Item)
	item.index = n
	*queue = append(*queue, item)
}

// Pop returns item with the lowest priority from the queue.
func (queue *PriorityQueue) Pop() interface{} {
	old := *queue
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*queue = old[0 : n-1]
	return item
}
