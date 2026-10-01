package demy_test

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

var (
	testBlue  = color.RGBA{B: 0xff, A: 0xff}
	testGreen = color.RGBA{G: 0xff, A: 0xff}
)

func redBox(w, h float64) *demy.BoxNode {
	return demy.Box(demy.Background(testRed), demy.Frame(w, h))
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

// TestClippedContentPhysicalResolution renders the same rounded box with and
// without a clipping Frame at Scale 2; both must rasterize identically.
func TestClippedContentPhysicalResolution(t *testing.T) {
	shape := func() *demy.BoxNode {
		return demy.Box(demy.Background(testRed), demy.CornerRadius(5), demy.Frame(16, 16))
	}
	cfg := demy.Config{Scale: 2}
	plain := render(t, demy.HStack(demy.Frame(3, 16), shape()), 48, 48, demy.RGBA, cfg)
	clipped := render(t, demy.HStack(demy.Frame(3, 16), demy.Frame(16, 16, shape())), 48, 48, demy.RGBA, cfg)
	for y := range 48 {
		for x := range 48 {
			a, b := rgbaAt(plain, x, y), rgbaAt(clipped, x, y)
			if absDiff(a.R, b.R) > 2 || absDiff(a.G, b.G) > 2 || absDiff(a.B, b.B) > 2 {
				t.Fatalf("pixel (%d,%d): unclipped %v, clipped %v", x, y, a, b)
			}
		}
	}
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestRootPixelAligned checks that a 10px box ends exactly on a pixel row on a
// display whose physical height doesn't match its width-derived density.
func TestRootPixelAligned(t *testing.T) {
	cfg := demy.TRMNLOriginal
	c := demy.MustNew(cfg)
	img, err := c.Render(demy.VStack(redBox(10, 10)), demy.RGBA)
	if err != nil {
		t.Fatal(err)
	}
	if got := rgbaAt(img, 5, 9); got != testRed {
		t.Errorf("last box row = %v, want %v", got, testRed)
	}
	if got := rgbaAt(img, 5, 10); got != testWhite {
		t.Errorf("row below box = %v, want %v", got, testWhite)
	}
}

func TestVerticalDividerInVStack(t *testing.T) {
	n := demy.VStack(
		demy.HStack(demy.Frame(10, 10), demy.Divider(), demy.Frame(10, 10)),
		redBox(10, 10),
	)
	img := render(t, n, 40, 40, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 5, 15); got != testRed {
		t.Errorf("box below HStack = %v, want %v", got, testRed)
	}
	if got := rgbaAt(img, 10, 5); got == testWhite {
		t.Error("vertical divider not drawn")
	}
}

func TestGridFrRowsInVStack(t *testing.T) {
	n := demy.VStack(
		demy.Grid(demy.Columns(demy.GridFr(1)), demy.Rows(demy.GridFr(1)), demy.Frame(10, 10)),
		redBox(10, 10),
	)
	img := render(t, n, 40, 40, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 5, 15); got != testRed {
		t.Errorf("box below grid = %v, want %v", got, testRed)
	}
}

func TestGridExplicitRowsBeyondChildren(t *testing.T) {
	g := demy.Grid(
		demy.Columns(demy.GridFixed(10)),
		demy.Rows(demy.GridFixed(10), demy.GridFixed(10), demy.GridFixed(10)),
		demy.Frame(10, 10),
	)
	img := render(t, demy.VStack(g, redBox(10, 10)), 40, 50, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 5, 25); got != testWhite {
		t.Errorf("empty grid row = %v, want white", got)
	}
	if got := rgbaAt(img, 5, 35); got != testRed {
		t.Errorf("box below grid = %v, want %v", got, testRed)
	}
}

func TestImageFillCropsCenter(t *testing.T) {
	// 30×10 source: blue | red | green; Fill into 10×10 keeps the red middle.
	src := image.NewRGBA(image.Rect(0, 0, 30, 10))
	for y := range 10 {
		for x := range 30 {
			c := testRed
			switch {
			case x < 10:
				c = testBlue
			case x >= 20:
				c = testGreen
			}
			src.Set(x, y, c)
		}
	}
	img := render(t, demy.Image(src, demy.ImageFill), 10, 10, demy.RGBA, demy.Config{})
	for _, x := range []int{2, 5, 7} {
		if got := rgbaAt(img, x, 5); got.R < 0xf0 || got.B > 0x10 || got.G > 0x10 {
			t.Errorf("pixel (%d,5) = %v, want red", x, got)
		}
	}
}

func TestImageFitUpscalesInVStack(t *testing.T) {
	img := render(t, demy.VStack(demy.Image(solid(5, 5, testRed))), 20, 20, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 15, 15); got.R < 0xf0 || got.G > 0x10 {
		t.Errorf("pixel (15,15) = %v, want red", got)
	}
}

func TestWeightedCrossAxisSize(t *testing.T) {
	n := demy.VStack(
		demy.HStack(demy.Weighted(1, demy.Frame(10, 20))),
		redBox(10, 10),
	)
	img := render(t, n, 40, 40, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 5, 15); got != testWhite {
		t.Errorf("pixel inside weighted row = %v, want white", got)
	}
	if got := rgbaAt(img, 5, 25); got != testRed {
		t.Errorf("box below HStack = %v, want %v", got, testRed)
	}
}

func TestBoxBorderInsetsContent(t *testing.T) {
	n := demy.Box(demy.Border(2, testRed), demy.Box(demy.Background(testBlue), demy.Frame(10, 10)))
	img := render(t, demy.VStack(n), 20, 20, demy.RGBA, demy.Config{})
	if got := rgbaAt(img, 0, 5); got != testRed {
		t.Errorf("border pixel = %v, want %v", got, testRed)
	}
	if got := rgbaAt(img, 2, 5); got != testBlue {
		t.Errorf("content pixel = %v, want %v", got, testBlue)
	}
}

func TestDitherImagePartlyOffCanvas(t *testing.T) {
	gray := solid(10, 30, color.Gray{Y: 128})
	n := demy.ZStack(demy.Bottom, demy.VStack(demy.Image(gray, demy.FloydSteinberg)))
	img := render(t, n, 10, 10, demy.OneBit, demy.Config{})
	var black, white int
	for y := range 10 {
		for x := range 10 {
			if rgbaAt(img, x, y).R == 0 {
				black++
			} else {
				white++
			}
		}
	}
	if black == 0 || white == 0 {
		t.Errorf("visible image not dithered: %d black, %d white", black, white)
	}
}

// TestContentOverDitheredImage checks that content drawn over a dithered
// image is quantized on its own rather than dithered with the image.
func TestContentOverDitheredImage(t *testing.T) {
	photo := solid(40, 40, color.Gray{Y: 200})
	mid := color.Gray{Y: 128} // quantizes to white in OneBit without dithering
	for name, overlay := range map[string]demy.Node{
		"box":   demy.Box(demy.Background(mid), demy.Frame(40, 40)),
		"image": demy.Image(solid(40, 40, mid), demy.ImageStretch),
	} {
		for _, wrap := range []func(demy.Node) demy.Node{
			func(n demy.Node) demy.Node { return n },
			func(n demy.Node) demy.Node { return demy.Frame(40, 40, demy.Children(n)) },
		} {
			root := wrap(demy.ZStack(demy.Image(photo, demy.ImageStretch, demy.FloydSteinberg), demy.Children(overlay)))
			img := render(t, root, 40, 40, demy.OneBit, demy.Config{})
			for y := range 40 {
				for x := range 40 {
					if c := rgbaAt(img, x, y); c != testWhite {
						t.Fatalf("%s: pixel (%d,%d) = %v, want white", name, x, y, c)
					}
				}
			}
		}
	}
}

// TestTextOverDitheredImage checks that vector text over a dithered white
// image matches the same text over a plain white background.
func TestTextOverDitheredImage(t *testing.T) {
	cfg := demy.Config{Fonts: map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}}
	text := demy.Text("Dither", demy.Style{Font: "sans", Size: 16})
	white := solid(80, 30, testWhite)
	plain := render(t, demy.ZStack(demy.Box(demy.Background(testWhite), demy.Frame(80, 30)), text), 80, 30, demy.TwoBit, cfg)
	over := render(t, demy.ZStack(demy.Image(white, demy.ImageStretch, demy.FloydSteinberg), text), 80, 30, demy.TwoBit, cfg)
	if !imagesEqual(plain, over) {
		t.Error("text over a dithered image differs from text over a plain background")
	}
}

func TestBarcodeCrispAtScale2(t *testing.T) {
	img := render(t, demy.Barcode("demy"), 84, 84, demy.RGBA, demy.Config{Scale: 2})
	for y := range 84 {
		for x := range 84 {
			if c := rgbaAt(img, x, y); c != testWhite && c != (color.RGBA{A: 0xff}) {
				t.Fatalf("pixel (%d,%d) = %v, want pure black or white", x, y, c)
			}
		}
	}
}

func TestBarcodeTooSmall(t *testing.T) {
	err := renderErr(demy.Frame(10, 10, demy.Barcode("demy")))
	if err == nil || !strings.Contains(err.Error(), "barcode") {
		t.Fatalf("err = %v, want barcode size error", err)
	}
}

func TestMissingFont(t *testing.T) {
	err := renderErr(demy.Text("a", demy.Style{Font: "nope", Size: 12}))
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v, want missing font error", err)
	}
}

func TestVectorTextEllipsis(t *testing.T) {
	cfg := demy.Config{Fonts: map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}}
	style := demy.Style{Font: "sans", Size: 12, MaxLines: 1}
	long := strings.Repeat("word ", 40)
	wrap := render(t, demy.Text(long, style), 120, 40, demy.RGBA, cfg)
	style.Overflow = demy.TextOverflowEllipsis
	ellipsis := render(t, demy.Text(long, style), 120, 40, demy.RGBA, cfg)
	if imagesEqual(wrap, ellipsis) {
		t.Error("ellipsis rendering matches plain truncation")
	}
}

func imagesEqual(a, b image.Image) bool {
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if rgbaAt(a, x, y) != rgbaAt(b, x, y) {
				return false
			}
		}
	}
	return true
}

// TestPathPartlyOffCanvas checks that a shape extending past the canvas is
// still filled where it's visible.
func TestPathPartlyOffCanvas(t *testing.T) {
	n := demy.ZStack(demy.Bottom, demy.VStack(redBox(30, 30)))
	img := render(t, n, 10, 10, demy.RGBA, demy.Config{})
	for y := range 10 {
		for x := range 10 {
			if got := rgbaAt(img, x, y); got != testRed {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, testRed)
			}
		}
	}
}
