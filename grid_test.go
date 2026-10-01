package demy_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

// renderGridCfg renders g on a w×h canvas, failing the test on error.
func renderGridCfg(t *testing.T, g *demy.GridNode, w, h int, cfg demy.Config) {
	t.Helper()
	cfg.PixelWidth = w
	cfg.PixelHeight = h
	cfg.PhysicalWidthMM = float64(w)
	cfg.PhysicalHeightMM = float64(h)
	c := demy.MustNew(cfg)
	if _, err := c.Render(g, demy.RGBA); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

func renderGrid(t *testing.T, g *demy.GridNode, w, h int) {
	t.Helper()
	cfg := demy.Config{
		PixelWidth:       w,
		PixelHeight:      h,
		PhysicalWidthMM:  float64(w),
		PhysicalHeightMM: float64(h),
	}
	c := demy.MustNew(cfg)
	_, err := c.Render(g, demy.RGBA)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
}

func TestGridBuildRoundTrip(t *testing.T) {
	// Verify that a GridNode with all track kinds builds and renders without error.
	g := demy.Grid(
		demy.Columns(demy.GridFixed(100), demy.GridAuto(), demy.GridFr(1)),
		demy.Rows(demy.GridAuto(), demy.GridFixed(40)),
		demy.Gap(4),
		demy.Padding(8),
		demy.Text("A"), demy.Text("B"), demy.Text("C"),
		demy.Text("D"), demy.Text("E"), demy.Text("F"),
	)
	renderGrid(t, g, 400, 200)
}

func TestGridEmptyChildren(t *testing.T) {
	g := demy.Grid(
		demy.Columns(demy.GridFr(1), demy.GridFr(1)),
	)
	renderGrid(t, g, 200, 100)
}

func TestGridNoColumns(t *testing.T) {
	// No columns — should produce a zero-size node without panicking.
	g := demy.Grid(demy.Text("x"))
	renderGrid(t, g, 100, 100)
}

func TestGridConstructorGap(t *testing.T) {
	// Gap() should set both RowGap and ColGap.
	g := demy.Grid(
		demy.Columns(demy.GridFr(1)),
		demy.Gap(10),
	)
	if g.RowGap != 10 || g.ColGap != 10 {
		t.Errorf("want RowGap=10 ColGap=10, got %v %v", g.RowGap, g.ColGap)
	}
}

func TestGridConstructorRowColGap(t *testing.T) {
	// RowGap/ColGap should override Gap independently.
	g := demy.Grid(
		demy.Columns(demy.GridFr(1)),
		demy.Gap(10),
		demy.RowGap(4),
		demy.ColGap(6),
	)
	if g.RowGap != 4 || g.ColGap != 6 {
		t.Errorf("want RowGap=4 ColGap=6, got %v %v", g.RowGap, g.ColGap)
	}
}

func TestGridTrackAccessors(t *testing.T) {
	auto := demy.GridAuto()
	if !auto.IsAuto() || auto.IsFixed() || auto.IsFr() {
		t.Error("GridAuto() IsAuto should be true")
	}
	fixed := demy.GridFixed(50)
	if fixed.IsAuto() || !fixed.IsFixed() || fixed.IsFr() {
		t.Error("GridFixed() IsFixed should be true")
	}
	if fixed.Size() != 50 {
		t.Errorf("GridFixed(50).Size() = %v, want 50", fixed.Size())
	}
	fr := demy.GridFr(2)
	if fr.IsAuto() || fr.IsFixed() || !fr.IsFr() {
		t.Error("GridFr() IsFr should be true")
	}
	if fr.Weight() != 2 {
		t.Errorf("GridFr(2).Weight() = %v, want 2", fr.Weight())
	}
}

func TestGridMixedTracks(t *testing.T) {
	// All three track kinds in the same grid.
	g := demy.Grid(
		demy.Columns(demy.GridFixed(80), demy.GridAuto(), demy.GridFr(1), demy.GridFr(2)),
		demy.Gap(2),
		demy.Text("a"), demy.Text("bb"), demy.Text("c"), demy.Text("d"),
	)
	renderGrid(t, g, 400, 200)
}

func TestGridFrRowsExpandToFill(t *testing.T) {
	// Fr rows should not panic when given a bounded height.
	g := demy.Grid(
		demy.Columns(demy.GridFr(1)),
		demy.Rows(demy.GridFr(1), demy.GridFr(2)),
		demy.Text("top"),
		demy.Text("bottom"),
	)
	renderGrid(t, g, 200, 300)
}

func TestGridChildrenOption(t *testing.T) {
	items := []demy.Node{demy.Text("x"), demy.Text("y"), demy.Text("z")}
	g := demy.Grid(
		demy.Columns(demy.GridFr(1), demy.GridFr(1)),
		demy.Children(items...),
	)
	if len(g.Children) != 3 {
		t.Errorf("want 3 children, got %d", len(g.Children))
	}
	renderGrid(t, g, 200, 100)
}

func TestGridExplicitRowsCap(t *testing.T) {
	// Explicit rows shorter than ceil(children/cols) should silently drop excess children.
	g := demy.Grid(
		demy.Columns(demy.GridFr(1)),
		demy.Rows(demy.GridAuto()),
		demy.Text("a"), demy.Text("b"), demy.Text("c"),
	)
	renderGrid(t, g, 200, 100)
}

func TestGridRowFlatten(t *testing.T) {
	sans := demy.MustLoadFontBytes("sans", goregular.TTF)
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	g := demy.Grid(
		demy.Columns(demy.GridAuto(), demy.GridFr(1)),
		demy.RowGap(4),
		demy.ColGap(8),
		demy.GridRow(demy.Text("Name"), demy.Text("Value")),
		demy.GridRow(demy.Text("Temp"), demy.Text("72 °F")),
	)
	if len(g.Children) != 2 {
		t.Fatalf("want 2 GridRowNode children, got %d", len(g.Children))
	}
	renderGridCfg(t, g, 300, 100, cfg)
}

func TestGridRowMixedFlat(t *testing.T) {
	// Mix of bare cells and GridRowNode children should both work.
	sans := demy.MustLoadFontBytes("sans", goregular.TTF)
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	g := demy.Grid(
		demy.Columns(demy.GridAuto(), demy.GridFr(1)),
		// bare cells
		demy.Text("A"), demy.Text("B"),
		// row-wrapped cells
		demy.GridRow(demy.Text("C"), demy.Text("D")),
	)
	renderGridCfg(t, g, 300, 100, cfg)
}
