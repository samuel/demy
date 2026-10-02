package demy

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
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

func TestTextBudget(t *testing.T) {
	cfg := &Config{
		Fonts:            map[string]*FontFamily{"sans": MustLoadFontBytes("sans", goregular.TTF)},
		DefaultTextStyle: Style{Font: "sans", Size: 12},
	}
	texts := func(count, size int) Node {
		return ZStack(children(count, func() Node { return Text(strings.Repeat("x", size)) }))
	}
	tests := map[string]struct {
		tree Node
		want string // empty for no error
	}{
		"one at the limit":   {texts(1, maxTextBytes), ""},
		"one over the limit": {texts(1, maxTextBytes+1), "Text has"},
		// Each invalid byte becomes a 3-byte U+FFFD.
		"sanitized at the limit":   {Text(strings.Repeat("\xff", maxTextBytes/3)), ""},
		"sanitized over the limit": {Text(strings.Repeat("\xff", maxTextBytes/3+1)), "Text has"},
		"tree at the limit":        {texts(maxTreeTextBytes/maxTextBytes, maxTextBytes), ""},
		"tree over the limit":      {texts(maxTreeTextBytes/maxTextBytes+1, maxTextBytes), "bytes of text"},
		"shared over the limit": {func() Node {
			shared := Text(strings.Repeat("x", maxTextBytes))
			return ZStack(children(maxTreeTextBytes/maxTextBytes+1, func() Node { return shared }))
		}(), "bytes of text"},
	}
	for name, tt := range tests {
		_, err := buildNode(tt.tree, cfg)
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("%s: err = %v, want %q error", name, err, tt.want)
		}
	}
}
