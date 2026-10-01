package demy_test

import (
	"math"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

var sansFont = sync.OnceValue(func() *demy.FontFamily { return demy.MustLoadFontBytes("sans", goregular.TTF) })

// textConfig returns a w×h config whose default text style is 12pt "sans".
func textConfig(w, h int) demy.Config {
	return demy.Config{
		PixelWidth: w, PixelHeight: h, PhysicalWidthMM: float64(w), PhysicalHeightMM: float64(h),
		Fonts:            map[string]*demy.FontFamily{"sans": sansFont()},
		DefaultTextStyle: demy.Style{Font: "sans", Size: 12},
	}
}

func renderErr(n demy.Node) error {
	_, err := demy.MustNew(textConfig(40, 40)).Render(n, demy.RGBA)
	return err
}

func TestGridRowOutsideGrid(t *testing.T) {
	tests := map[string]demy.Node{
		"in vstack":   demy.VStack(demy.GridRow(demy.Text("a"))),
		"nested rows": demy.Grid(demy.Columns(demy.GridAuto()), demy.GridRow(demy.GridRow(demy.Text("a")))),
	}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			if err := renderErr(n); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestTypedNilChildSkipped(t *testing.T) {
	var txt *demy.TextNode
	if err := renderErr(demy.VStack(txt, demy.Text("a"))); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

func TestInvalidDimensions(t *testing.T) {
	tests := map[string]demy.Node{
		"nan frame":        demy.Frame(math.NaN(), 10),
		"inf frame":        demy.Frame(math.Inf(1), 10),
		"huge frame":       demy.Frame(1e9, 1e9),
		"negative gap":     demy.VStack(demy.Gap(-1)),
		"negative padding": demy.Box(demy.Padding(-1)),
		"negative spacer":  demy.Spacer(-1),
		"negative fr":      demy.Grid(demy.Columns(demy.GridFr(-1))),
		"nan fixed track":  demy.Grid(demy.Columns(demy.GridFixed(math.NaN()))),
		"negative divider": &demy.DividerNode{Size: -1},
		"negative lines":   demy.Text("a", demy.Style{MaxLines: -2}),
	}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			if err := renderErr(n); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLargeClippedFrameBounded(t *testing.T) {
	// The sub-canvas is limited to the visible region, so this renders without
	// allocating a 30000×30000 image.
	n := demy.HStack(
		demy.Frame(30, 40),
		demy.Frame(30000, 30000, demy.Box(demy.Background(testRed), demy.Frame(30000, 30000))),
	)
	img := render(t, n, 40, 40, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 35, 5); got != testRed {
		t.Errorf("visible frame pixel = %v, want %v", got, testRed)
	}
	if got := rgbaAt(img, 25, 5); got != testWhite {
		t.Errorf("pixel left of frame = %v, want %v", got, testWhite)
	}
}

func TestNodeDepthLimit(t *testing.T) {
	var n demy.Node = demy.Text("a")
	for range 2000 {
		n = demy.VStack(demy.Children(n))
	}
	err := renderErr(n)
	if err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("err = %v, want nesting error", err)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	ok := demy.Config{PixelWidth: 10, PixelHeight: 10, PhysicalWidthMM: 10, PhysicalHeightMM: 10}
	if _, err := demy.New(ok); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	tests := map[string]func(*demy.Config){
		"zero width":      func(c *demy.Config) { c.PixelWidth = 0 },
		"negative height": func(c *demy.Config) { c.PixelHeight = -1 },
		"huge width":      func(c *demy.Config) { c.PixelWidth = 1 << 20 },
		"negative mm":     func(c *demy.Config) { c.PhysicalWidthMM = -10 },
		"nan mm":          func(c *demy.Config) { c.PhysicalHeightMM = math.NaN() },
		"inf mm":          func(c *demy.Config) { c.PhysicalWidthMM = math.Inf(1) },
		"negative scale":  func(c *demy.Config) { c.Scale = -1 },
		"nan scale":       func(c *demy.Config) { c.Scale = math.NaN() },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := ok
			mutate(&cfg)
			if _, err := demy.New(cfg); err == nil {
				t.Error("expected error")
			}
		})
	}
}
