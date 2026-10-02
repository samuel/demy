package demy

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

var testPalette = []color.Gray{{Y: 0}, {Y: 85}, {Y: 170}, {Y: 255}}

func gradient(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray(x, y, color.Gray{Y: uint8((x*7 + y*13) % 256)})
		}
	}
	return img
}

func mustQuantizer(t *testing.T, pal []color.Color) *quantizer {
	t.Helper()
	q, err := newQuantizer(pal)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// TestDitherSubImage checks that dithering reads a SubImage (non-zero origin,
// stride wider than the image) the same as an equivalent standalone image.
func TestDitherSubImage(t *testing.T) {
	full := gradient(40, 30)
	sub := full.SubImage(image.Rect(5, 7, 25, 22)).(*image.Gray) //nolint:forcetypeassert // Gray.SubImage returns *image.Gray
	flat := image.NewGray(image.Rect(0, 0, sub.Bounds().Dx(), sub.Bounds().Dy()))
	draw.Draw(flat, flat.Bounds(), sub, sub.Bounds().Min, draw.Src)
	q := mustQuantizer(t, PaletteGray4)
	for _, algo := range []Dither{NoDithering, FloydSteinberg, Atkinson} {
		a, b := q.grayIndices(sub, algo), q.grayIndices(flat, algo)
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("dither %d: index %d = %d, want %d", algo, i, a[i], b[i])
			}
		}
	}
}

func TestDitherNoneMatchesNearest(t *testing.T) {
	src := gradient(256, 1)
	out := ditherNone(src, testPalette)
	for x := range 256 {
		if want := nearestPalette(float64(src.Pix[x]), testPalette); int(out[x]) != want {
			t.Fatalf("value %d → %d, want %d", src.Pix[x], out[x], want)
		}
	}
}

func TestToGrayCompositesOverWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 1))
	img.SetRGBA(0, 0, color.RGBA{})                                   // transparent
	img.SetRGBA(1, 0, color.RGBA{A: 0x80})                            // half-transparent black
	img.SetRGBA(2, 0, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}) // opaque white
	g := toGray(img)
	if got := g.GrayAt(0, 0).Y; got != 255 {
		t.Errorf("transparent = %d, want 255", got)
	}
	if got := g.GrayAt(1, 0).Y; got != 127 {
		t.Errorf("half-transparent black = %d, want 127", got)
	}
	if got := g.GrayAt(2, 0).Y; got != 255 {
		t.Errorf("white = %d, want 255", got)
	}
}

func TestDitherRGBAKeepsAlpha(t *testing.T) {
	for name, pal := range map[string][]color.Color{"bw": PaletteBW, "bwr": PaletteBWR} {
		img := image.NewRGBA(image.Rect(0, 0, 8, 4))
		for y := range 4 {
			for x := range 8 {
				// Gray 128 at full alpha in the left half, 50% alpha in the right.
				if x < 4 {
					img.SetRGBA(x, y, color.RGBA{128, 128, 128, 255})
				} else {
					img.SetRGBA(x, y, color.RGBA{64, 64, 64, 128})
				}
			}
		}
		mustQuantizer(t, pal).ditherRGBA(img, img.Bounds(), FloydSteinberg)
		var black, white int
		for y := range 4 {
			for x := range 8 {
				p := img.RGBAAt(x, y)
				want := uint8(255)
				if x >= 4 {
					want = 128
				}
				if p.A != want {
					t.Fatalf("%s: (%d,%d) alpha = %d, want %d", name, x, y, p.A, want)
				}
				switch {
				case p.R == 0 && p.G == 0 && p.B == 0:
					black++
				case p.R == p.A && p.G == p.A && p.B == p.A:
					white++
				default:
					t.Fatalf("%s: (%d,%d) = %v, want black or white at alpha %d", name, x, y, p, p.A)
				}
			}
		}
		if black == 0 || white == 0 {
			t.Errorf("%s: got %d black and %d white pixels, want a mix", name, black, white)
		}
	}
}

// solidRGBA returns an opaque w×h image filled with c.
func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

func TestQuantizeColorExact(t *testing.T) {
	for name, pal := range map[string][]color.Color{
		"bwr": PaletteBWR, "bwry": PaletteBWRY, "spectra6": PaletteSpectra6, "acep7": PaletteACeP7,
	} {
		q := mustQuantizer(t, pal)
		for i, c := range q.colors {
			for _, algo := range []Dither{NoDithering, FloydSteinberg, Atkinson} {
				for _, v := range q.colorIndices(solidRGBA(6, 4, c), image.Rect(0, 0, 6, 4), algo) {
					if int(v) != i {
						t.Fatalf("%s: color %d with dither %d → %d", name, i, algo, v)
					}
				}
			}
		}
	}
}

// TestQuantizeGraysAvoidColors checks that mid grays, such as antialiased
// text edges, never match a saturated palette color.
func TestQuantizeGraysAvoidColors(t *testing.T) {
	for name, pal := range map[string][]color.Color{"bwr": PaletteBWR, "bwry": PaletteBWRY, "spectra6": PaletteSpectra6} {
		q := mustQuantizer(t, pal)
		for v := range 256 {
			if got := q.nearest(color.RGBA{uint8(v), uint8(v), uint8(v), 255}); got > 1 {
				t.Errorf("%s: gray %d → %v, want black or white", name, v, q.colors[got])
			}
		}
	}
}

func TestQuantizeRedGradient(t *testing.T) {
	q := mustQuantizer(t, PaletteBWR)
	img := image.NewRGBA(image.Rect(0, 0, 64, 16))
	for y := range 16 {
		for x := range 64 {
			// Red to white.
			v := uint8(x * 4)
			img.SetRGBA(x, y, color.RGBA{255, v, v, 255})
		}
	}
	counts := make([]int, 3)
	for _, v := range q.colorIndices(img, img.Bounds(), FloydSteinberg) {
		counts[v]++
	}
	if counts[0] != 0 || counts[1] == 0 || counts[2] == 0 {
		t.Errorf("black, white, red counts = %v, want only white and red", counts)
	}
}

// TestQuantizeUnreachableColorBounded checks that a color the palette can't
// approach doesn't build up unbounded error.
func TestQuantizeUnreachableColorBounded(t *testing.T) {
	q := mustQuantizer(t, PaletteBWR)
	const w, h = 200, 40
	img := solidRGBA(w, h, color.RGBA{0, 0, 255, 255}) // blue
	// A white strip after the blue area must stay white.
	for y := range h {
		for x := w / 2; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	idx := q.colorIndices(img, img.Bounds(), FloydSteinberg)
	var off int
	for y := range h {
		for x := w/2 + 4; x < w; x++ {
			if idx[y*w+x] != 1 {
				off++
			}
		}
	}
	if off != 0 {
		t.Errorf("%d pixels in the white area aren't white", off)
	}
}

func TestQuantizerGrayDetection(t *testing.T) {
	for name, tt := range map[string]struct {
		pal  []color.Color
		gray bool
	}{
		"gray4":   {PaletteGray4, true},
		"gray16":  {PaletteGray16, true},
		"rgba bw": {[]color.Color{color.RGBA{A: 255}, color.RGBA{255, 255, 255, 255}}, true},
		"bwr":     {PaletteBWR, false},
	} {
		if got := mustQuantizer(t, tt.pal).gray; got != tt.gray {
			t.Errorf("%s: gray = %v, want %v", name, got, tt.gray)
		}
	}
}
