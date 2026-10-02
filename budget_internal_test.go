package demy

import (
	"strings"
	"testing"
)

// children returns count nodes made by f.
func children(count int, f func() Node) Option {
	nodes := make([]Node, count)
	for i := range nodes {
		nodes[i] = f()
	}
	return Children(nodes...)
}

func TestNodeBudget(t *testing.T) {
	// Each tree has one container node plus count children.
	tests := map[string]struct {
		limit int
		tree  func(count int) Node
		want  string
	}{
		"nodes": {maxNodes - 1, func(count int) Node {
			return ZStack(children(count, func() Node { return Spacer() }))
		}, "nodes"},
		"grid rows": {maxNodes - 1, func(count int) Node {
			return Grid(Columns(GridAuto()), children(count, func() Node { return GridRow() }))
		}, "nodes"},
		"frames": {maxClippedNodes, func(count int) Node {
			return ZStack(children(count, func() Node { return Frame(1, 1) }))
		}, "clipped"},
		"clipped boxes": {maxClippedNodes, func(count int) Node {
			return ZStack(children(count, func() Node { return Box(Clip) }))
		}, "clipped"},
		"barcodes": {maxBarcodes, func(count int) Node {
			return ZStack(children(count, func() Node { return Barcode("x") }))
		}, "barcodes"},
	}
	for name, tt := range tests {
		if _, err := buildNode(tt.tree(tt.limit), &Config{}); err != nil {
			t.Errorf("%s: at the limit: %v", name, err)
		}
		_, err := buildNode(tt.tree(tt.limit+1), &Config{})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: over the limit: err = %v, want %q error", name, err, tt.want)
		}
	}
}

// TestNodeBudgetUnclippedBox checks that a Box without Clip doesn't count as
// a clipped node.
func TestNodeBudgetUnclippedBox(t *testing.T) {
	tree := ZStack(children(maxClippedNodes+1, func() Node { return Box() }))
	if _, err := buildNode(tree, &Config{}); err != nil {
		t.Fatal(err)
	}
}
