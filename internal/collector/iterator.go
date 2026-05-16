package collector

// TreeIterator implements the Iterator pattern over a MetricSource composite tree.
// It performs breadth-first traversal using a cursor (head index) instead of
// reslicing. This guarantees true zero allocations after the backing array
// has grown to its steady-state size.
type TreeIterator struct {
	nodes []MetricSource
	head  int // cursor index (avoids reslicing and preserves backing array capacity)
}

// NewIterator creates a BFS iterator rooted at root.
func NewIterator(root MetricSource) *TreeIterator {
	it := &TreeIterator{}
	it.Reset(root)
	return it
}

// Reset prepares the iterator for a new BFS traversal.
// It reuses the backing array and is allocation-free after the first cycle.
func (it *TreeIterator) Reset(root MetricSource) {
	it.head = 0
	if it.nodes == nil {
		it.nodes = make([]MetricSource, 0, 32)
	}
	it.nodes = it.nodes[:0]
	if root != nil {
		it.nodes = append(it.nodes, root)
	}
}

// Next returns the next node in breadth-first order, or nil when exhausted.
// Uses an index cursor instead of reslicing so the backing array capacity is preserved.
func (it *TreeIterator) Next() MetricSource {
	if it.head >= len(it.nodes) {
		return nil
	}
	node := it.nodes[it.head]
	it.head++

	// Enqueue children for BFS (append to end, process from head)
	it.nodes = append(it.nodes, node.Children()...)
	return node
}

// HasNext reports whether more nodes remain.
func (it *TreeIterator) HasNext() bool {
	return it.head < len(it.nodes)
}
