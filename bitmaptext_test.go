package demy

import (
	"image"
	"testing"
)

// renderBitmap renders root with a bitmap font registered as "px" and returns
// the quantized grayscale result.
func renderBitmap(t *testing.T, root Node, w, h int, scale float64, mode ColorMode) *image.Gray {
	t.Helper()
	cfg := Config{
		PixelWidth:       w,
		PixelHeight:      h,
		PhysicalWidthMM:  float64(w),
		PhysicalHeightMM: float64(h),
		Scale:            scale,
		Fonts:            map[string]*FontFamily{"px": MustLoadBitmapFontBDF("px", []byte(testBDF))},
		Roles:            Roles{Background: White, Foreground: Black},
	}
	c := MustNew(cfg)
	img, err := c.Render(t.Context(), root, mode)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	g, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("expected *image.Gray, got %T", img)
	}
	return g
}

func TestBitmapTextScale1(t *testing.T) {
	g := renderBitmap(t, Text("A", Style{Font: "px", Color: Black}), 64, 32, 1, OneBit)

	// 'A' row 0 = 0x18 -> columns 3 and 4 set; everything else clear.
	if g.GrayAt(3, 0).Y != 0 || g.GrayAt(4, 0).Y != 0 {
		t.Errorf("row 0 cols 3,4 not black: %d %d", g.GrayAt(3, 0).Y, g.GrayAt(4, 0).Y)
	}
	for _, x := range []int{0, 2, 5} {
		if g.GrayAt(x, 0).Y != 255 {
			t.Errorf("row 0 col %d = %d, want 255", x, g.GrayAt(x, 0).Y)
		}
	}
	// 'A' row 3 = 0x7E -> columns 1..6 set.
	for d := range 6 {
		if g.GrayAt(d+1, 3).Y != 0 {
			t.Errorf("row 3 col %d = %d, want 0", d+1, g.GrayAt(d+1, 3).Y)
		}
	}
	// Bitmap text bypasses antialiasing: every pixel is a pure palette extreme.
	for i, v := range g.Pix {
		if v != 0 && v != 255 {
			t.Fatalf("pixel index %d = %d, not pure black/white", i, v)
		}
	}
}

func TestBitmapTextScale2(t *testing.T) {
	// Scale=2 (retina): each font pixel becomes a 2x2 nearest-neighbor block.
	g := renderBitmap(t, Text("A", Style{Font: "px", Color: Black}), 64, 32, 2, OneBit)

	// Font pixel (3,0) -> physical block cols 6-7, rows 0-1; (4,0) -> cols 8-9.
	for _, p := range []image.Point{{6, 0}, {7, 0}, {6, 1}, {7, 1}, {8, 0}, {9, 1}} {
		if g.GrayAt(p.X, p.Y).Y != 0 {
			t.Errorf("2x block pixel %v = %d, want 0", p, g.GrayAt(p.X, p.Y).Y)
		}
	}
	// Font column 2 is clear -> physical cols 4,5 stay white; col 5 -> cols 10,11.
	for _, p := range []image.Point{{4, 0}, {5, 0}, {10, 0}} {
		if g.GrayAt(p.X, p.Y).Y != 255 {
			t.Errorf("gap pixel %v = %d, want 255", p, g.GrayAt(p.X, p.Y).Y)
		}
	}
}

func TestBitmapTextPureAcrossModes(t *testing.T) {
	for _, mode := range []ColorMode{OneBit, TwoBit, FourBit} {
		g := renderBitmap(t, Text("A", Style{Font: "px", Color: Black}), 64, 32, 1, mode)
		if got := g.GrayAt(3, 0).Y; got != 0 {
			t.Errorf("mode %d: text pixel = %d, want 0", mode, got)
		}
		if got := g.GrayAt(0, 0).Y; got != 255 {
			t.Errorf("mode %d: background pixel = %d, want 255", mode, got)
		}
	}
}

func TestBitmapTextOverDither(t *testing.T) {
	mkImg := func() *image.Gray {
		im := image.NewGray(image.Rect(0, 0, 64, 32))
		for i := range im.Pix {
			im.Pix[i] = 220
		}
		return im
	}

	// Precondition: a light-gray image dithers to mostly white, so the glyph
	// region is NOT already solid black from dithering alone.
	bg := renderBitmap(t, Image(mkImg(), FloydSteinberg), 64, 32, 1, OneBit)
	anyWhite := false
	for d := range 6 {
		if bg.GrayAt(d+1, 3).Y == 255 {
			anyWhite = true
		}
	}
	if !anyWhite {
		t.Fatal("precondition failed: dithered background already solid black under glyph")
	}

	// Black text over the dithered image must produce a solid black glyph,
	// proving blits are composited after quantization (text wins).
	over := renderBitmap(t,
		ZStack(Image(mkImg(), FloydSteinberg), Text("A", Style{Font: "px", Color: Black})),
		64, 32, 1, OneBit)
	for d := range 6 {
		if got := over.GrayAt(d+1, 3).Y; got != 0 {
			t.Errorf("text-over-dither pixel (%d,3) = %d, want 0", d+1, got)
		}
	}
}

// TestBitmapTextClipped draws an 8px-wide glyph into a 4px-wide clip at x=3
// and checks that no glyph pixels land outside the clip.
func TestBitmapTextClipped(t *testing.T) {
	text := func() *TextNode { return Text("A", Style{Font: "px", Color: Black}) }
	wrappers := map[string]func() Node{
		"frame": func() Node { return Frame(4, 8, text()) },
		"box":   func() Node { return Grid(Columns(GridFixed(4)), Box(Clip, text())) },
	}
	for name, wrap := range wrappers {
		for _, scale := range []int{1, 2} {
			g := renderBitmap(t, HStack(Frame(3, 8), Children(wrap())), 32*scale, 16*scale, float64(scale), OneBit)
			clip := image.Rect(3*scale, 0, 7*scale, 8*scale)
			// 'A' row 3 = 0x7E sets glyph columns 1..6, i.e. logical x 4..9.
			if got := g.GrayAt(5*scale, 3*scale).Y; got != 0 {
				t.Errorf("%s scale %d: pixel inside clip = %d, want 0", name, scale, got)
			}
			for y := range g.Bounds().Dy() {
				for x := range g.Bounds().Dx() {
					if !image.Pt(x, y).In(clip) && g.GrayAt(x, y).Y != 255 {
						t.Fatalf("%s scale %d: pixel (%d,%d) outside clip = %d, want 255", name, scale, x, y, g.GrayAt(x, y).Y)
					}
				}
			}
		}
	}
}
