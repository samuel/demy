package demy

import (
	"image"
	"math"
	"testing"
)

func buildTest(t *testing.T, n Node) node {
	t.Helper()
	built, err := buildNode(n, &Config{}, dirVertical, 0)
	if err != nil {
		t.Fatalf("buildNode: %v", err)
	}
	return built
}

// TestGridFrColumnsNoSpace checks that Fr columns get zero width, never a
// negative or NaN width, when fixed and auto columns use all the space.
func TestGridFrColumnsNoSpace(t *testing.T) {
	tests := map[string]struct {
		grid *GridNode
		want []float64
	}{
		"fixed overflow": {
			Grid(Columns(GridFixed(150), GridFr(1), GridFr(2)), Frame(10, 10), Frame(10, 10), Frame(10, 10)),
			[]float64{150, 0, 0},
		},
		"fixed and auto fill exactly": {
			Grid(Columns(GridFixed(60), GridAuto(), GridFr(1)), Frame(10, 10), Frame(40, 10), Frame(10, 10)),
			[]float64{60, 40, 0},
		},
		"gaps overflow": {
			Grid(Columns(GridFr(1), GridFr(1)), ColGap(120), Frame(10, 10), Frame(10, 10)),
			[]float64{0, 0},
		},
	}
	for name, tt := range tests {
		g := buildTest(t, tt.grid).(*gridNode) //nolint:forcetypeassert // buildNode returns *gridNode for a GridNode
		g.measure(&measureCtx{dpmm: 1, scale: 1, availW: 100, availH: 100})
		g.layout(nodeRect{w: 100, h: 100})
		for i, w := range g.colWidths {
			if math.IsNaN(w) || w != tt.want[i] {
				t.Errorf("%s: column %d width = %v, want %v", name, i, w, tt.want[i])
			}
		}
	}
}

// TestMeasureMemoRemeasures checks that a memoized node measured at one size,
// then another, then the first again reflects the latest size in both its
// result and the state layout reads.
func TestMeasureMemoRemeasures(t *testing.T) {
	v := buildTest(t, VStack(Image(image.NewGray(image.Rect(0, 0, 10, 10))))).(*vstackNode) //nolint:forcetypeassert // buildNode returns *vstackNode for a VStackNode
	mctx := &measureCtx{dpmm: 1, scale: 1, availH: unbounded}
	for _, w := range []float64{20, 40, 20} {
		got := v.measure(mctx.with(w, unbounded))
		want := nodeSize{w: w, h: w}
		if got != want {
			t.Errorf("measure at width %v = %v, want %v", w, got, want)
		}
		if v.m.sizes[0] != want {
			t.Errorf("child state at width %v = %v, want %v", w, v.m.sizes[0], want)
		}
	}
}

// TestMeasureMemoRestoresDescendants checks that a container measured at one
// size, then another, then the first again lays out its descendants with the
// state from the first measurement.
func TestMeasureMemoRestoresDescendants(t *testing.T) {
	outer := buildTest(t, VStack(VStack(Image(image.NewGray(image.Rect(0, 0, 10, 10)))))).(*vstackNode) //nolint:forcetypeassert // buildNode returns *vstackNode for a VStackNode
	mctx := &measureCtx{dpmm: 1, scale: 1}
	for _, w := range []float64{20, 40, 20} {
		outer.measure(mctx.with(w, unbounded))
	}
	outer.layout(nodeRect{w: 20, h: 20})
	img := outer.children[0].(*vstackNode).children[0].(*imageNode) //nolint:forcetypeassert // built from VStack(Image)
	if want := (nodeRect{w: 20, h: 20}); img.bounds != want {
		t.Errorf("image bounds = %v, want %v", img.bounds, want)
	}
}

// wrapLeaf is a leaf that wraps like text: 300 pixels wide on one line, with
// a 10 pixel line for every 300 pixels of content. It counts its measurements.
type wrapLeaf struct {
	count  *int
	bounds nodeRect
}

func (n *wrapLeaf) measure(mctx *measureCtx) nodeSize {
	*n.count++
	w := math.Min(300, mctx.availW)
	if w <= 0 {
		return nodeSize{}
	}
	return nodeSize{w: w, h: 10 * math.Ceil(300/w)}
}

func (n *wrapLeaf) layout(bounds nodeRect) { n.bounds = bounds }
func (n *wrapLeaf) draw(*drawCtx)          {}

// TestNestedMeasureLinear checks that the number of measurements grows
// linearly with nesting depth for containers that measure children at more
// than one size.
func TestNestedMeasureLinear(t *testing.T) {
	const depth = 16
	wraps := map[string]func(leaf, child node) node{
		"hstack": func(leaf, child node) node {
			return &hstackNode{gap: 1, padding: 1, children: []node{leaf, child}}
		},
		"hstack weighted": func(leaf, child node) node {
			return &hstackNode{gap: 1, children: []node{leaf, &weightedNode{weight: 1, child: child}}}
		},
		"grid auto": func(leaf, child node) node {
			return &gridNode{columns: []GridTrack{GridAuto(), GridAuto()}, colGap: 1, children: []node{leaf, child}}
		},
		"grid fr": func(leaf, child node) node {
			return &gridNode{
				columns:  []GridTrack{GridAuto(), GridFr(1)},
				rows:     []GridTrack{GridFr(1)},
				colGap:   1,
				children: []node{leaf, child},
			}
		},
		"mixed": func(leaf, child node) node {
			return &hstackNode{gap: 1, children: []node{leaf, &gridNode{
				columns:  []GridTrack{GridFr(1)},
				children: []node{&vstackNode{children: []node{child}}},
			}}}
		},
	}
	for name, wrap := range wraps {
		var count, leaves int
		newLeaf := func() node {
			leaves++
			return &wrapLeaf{count: &count}
		}
		n := newLeaf()
		for range depth {
			n = wrap(newLeaf(), n)
		}
		n.measure(&measureCtx{dpmm: 1, scale: 1, availW: 800, availH: 480})
		n.layout(nodeRect{w: 800, h: 480})
		if limit := 10 * leaves; count > limit {
			t.Errorf("%s: %d leaf measurements at depth %d, want at most %d", name, count, depth, limit)
		}
	}
}
