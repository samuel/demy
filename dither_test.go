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

// TestDitherSubImage checks that dithering reads a SubImage (non-zero origin,
// stride wider than the image) the same as an equivalent standalone image.
func TestDitherSubImage(t *testing.T) {
	full := gradient(40, 30)
	sub := full.SubImage(image.Rect(5, 7, 25, 22)).(*image.Gray) //nolint:forcetypeassert // Gray.SubImage returns *image.Gray
	flat := image.NewGray(image.Rect(0, 0, sub.Bounds().Dx(), sub.Bounds().Dy()))
	draw.Draw(flat, flat.Bounds(), sub, sub.Bounds().Min, draw.Src)
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

func TestDitherRGBAKeepsAlpha(t *testing.T) {
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
	ditherRGBA(img, img.Bounds(), []color.Gray{{Y: 0}, {Y: 255}}, FloydSteinberg)
	var black, white int
	for y := range 4 {
		for x := range 8 {
			p := img.RGBAAt(x, y)
			want := uint8(255)
			if x >= 4 {
				want = 128
			}
			if p.A != want {
				t.Fatalf("(%d,%d) alpha = %d, want %d", x, y, p.A, want)
			}
			switch p.R {
			case 0:
				black++
			case p.A:
				white++
			default:
				t.Fatalf("(%d,%d) = %v, want black or white at alpha %d", x, y, p, p.A)
			}
		}
	}
	if black == 0 || white == 0 {
		t.Errorf("got %d black and %d white pixels, want a mix", black, white)
	}
}
