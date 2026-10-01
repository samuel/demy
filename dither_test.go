package demy

import (
	"image"
	"image/color"
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

// TestDitherSubImage checks that dithering reads a SubImage (non-zero origin,
// stride wider than the image) the same as an equivalent standalone image.
func TestDitherSubImage(t *testing.T) {
	full := gradient(40, 30)
	sub := full.SubImage(image.Rect(5, 7, 25, 22)).(*image.Gray) //nolint:forcetypeassert // Gray.SubImage returns *image.Gray
	flat := subGray(full, sub.Bounds())
	for name, fn := range map[string]func(*image.Gray, []color.Gray) *image.Gray{
		"none": ditherNone, "floyd-steinberg": ditherFloydSteinberg, "atkinson": ditherAtkinson,
	} {
		a, b := fn(sub, testPalette), fn(flat, testPalette)
		for y := range flat.Bounds().Dy() {
			for x := range flat.Bounds().Dx() {
				if ga, gb := a.GrayAt(sub.Bounds().Min.X+x, sub.Bounds().Min.Y+y), b.GrayAt(x, y); ga != gb {
					t.Fatalf("%s: pixel (%d,%d) = %v, want %v", name, x, y, ga, gb)
				}
			}
		}
	}
}

func TestDitherNoneMatchesNearest(t *testing.T) {
	src := gradient(256, 1)
	out := ditherNone(src, testPalette)
	for x := range 256 {
		want := testPalette[nearestPalette(float64(src.Pix[x]), testPalette)]
		if got := out.GrayAt(x, 0); got != want {
			t.Fatalf("value %d → %v, want %v", src.Pix[x], got, want)
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

func TestCopyGrayRegion(t *testing.T) {
	src := gradient(10, 8)
	dst := image.NewGray(image.Rect(0, 0, 30, 20))
	copyGrayRegion(dst, src, image.Pt(12, 5))
	for y := range 8 {
		for x := range 10 {
			if got, want := dst.GrayAt(12+x, 5+y), src.GrayAt(x, y); got != want {
				t.Fatalf("(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}
