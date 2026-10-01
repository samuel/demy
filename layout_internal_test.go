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
		if v.measuredSizes[0] != want {
			t.Errorf("child state at width %v = %v, want %v", w, v.measuredSizes[0], want)
		}
	}
}
