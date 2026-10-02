package demy_test

import (
	"context"
	"image"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

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
	_, err := demy.MustNew(textConfig(40, 40)).Render(context.Background(), n, demy.RGBA)
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

// TestSharedNodeBlowup checks that a small tree whose nodes are shared, and
// so expand to 2²⁰ nodes, fails quickly instead of being laid out.
func TestSharedNodeBlowup(t *testing.T) {
	var n demy.Node = demy.Text("a")
	for range 20 {
		n = demy.VStack(demy.Children(n, n))
	}
	start := time.Now()
	err := renderErr(n)
	if err == nil || !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("err = %v, want node limit error", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("render took %v, want well under a second", d)
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
		"inf scale":       func(c *demy.Config) { c.Scale = math.Inf(1) },
		"tiny scale":      func(c *demy.Config) { c.Scale = 1e-12 },
		"below min scale": func(c *demy.Config) { c.Scale = 0.49 },
		"above max scale": func(c *demy.Config) { c.Scale = 64.5 },
		"huge scale":      func(c *demy.Config) { c.Scale = 1e300 },
		"huge auto scale": func(c *demy.Config) { c.PhysicalWidthMM = 0.001 },
		"bwred mode":      func(c *demy.Config) { c.ColorMode = demy.BWRed },
		"unknown mode":    func(c *demy.Config) { c.ColorMode = demy.ColorMode(99) },
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
	valid := map[string]func(*demy.Config){
		"min scale":     func(c *demy.Config) { c.Scale = 0.5 },
		"max scale":     func(c *demy.Config) { c.Scale = 64 },
		"two bit mode":  func(c *demy.Config) { c.ColorMode = demy.TwoBit },
		"fractional":    func(c *demy.Config) { c.Scale = 1.5 },
		"fine auto dpi": func(c *demy.Config) { c.PhysicalWidthMM = 0.5 }, // 20 px/mm → scale 4
	}
	for name, mutate := range valid {
		t.Run(name, func(t *testing.T) {
			cfg := ok
			mutate(&cfg)
			if _, err := demy.New(cfg); err != nil {
				t.Errorf("New: %v", err)
			}
		})
	}
}

func TestMinScaleKeepsLogicalSizeBounded(t *testing.T) {
	// The widest canvas at the smallest scale stays within the node dimension
	// limit, far below the threshold where a size is treated as unbounded.
	cfg := demy.Config{PixelWidth: 1 << 14, PixelHeight: 1 << 14, PhysicalWidthMM: 1000, PhysicalHeightMM: 1000, Scale: 0.5}
	c := demy.MustNew(cfg)
	if w, h := c.LogicalWidth(), c.LogicalHeight(); w != 1<<15 || h != 1<<15 {
		t.Errorf("logical size = %vx%v, want %vx%v", w, h, 1<<15, 1<<15)
	}
}

func TestRenderValidatesModes(t *testing.T) {
	c := demy.MustNew(textConfig(10, 10))
	root := demy.Frame(4, 4)
	bad := map[string]func() error{
		"bwred":          func() error { _, err := c.Render(t.Context(), root, demy.BWRed); return err },
		"unknown mode":   func() error { _, err := c.Render(t.Context(), root, demy.ColorMode(99)); return err },
		"negative mode":  func() error { _, err := c.Render(t.Context(), root, demy.ColorMode(-1)); return err },
		"unknown dither": func() error { _, err := c.Render(t.Context(), root, demy.OneBit, demy.Dither(9)); return err },
		"image dither": func() error {
			_, err := c.Render(t.Context(), demy.Image(image.NewGray(image.Rect(0, 0, 2, 2)), demy.Dither(9)), demy.OneBit)
			return err
		},
	}
	for name, render := range bad {
		t.Run(name, func(t *testing.T) {
			if err := render(); err == nil {
				t.Error("expected error")
			}
		})
	}
	for _, mode := range []demy.ColorMode{demy.RGBA, demy.Grayscale, demy.OneBit, demy.TwoBit, demy.FourBit} {
		if _, err := c.Render(t.Context(), root, mode, demy.Atkinson); err != nil {
			t.Errorf("mode %d: %v", mode, err)
		}
	}
}

func TestRenderUnsetMode(t *testing.T) {
	cfg := textConfig(10, 10)
	img, err := demy.MustNew(cfg).Render(t.Context(), demy.Frame(4, 4), demy.Unset)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.RGBA); !ok {
		t.Errorf("Unset with Unset Config.ColorMode: got %T, want *image.RGBA", img)
	}
	cfg.ColorMode = demy.TwoBit
	img, err = demy.MustNew(cfg).Render(t.Context(), demy.Frame(4, 4), demy.Unset)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.Gray); !ok {
		t.Errorf("Unset with TwoBit Config.ColorMode: got %T, want *image.Gray", img)
	}
}
