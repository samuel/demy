package demy_test

import (
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	"github.com/samuel/demy"
)

// lazyImage has bounds but no pixel storage and no SubImage method.
type lazyImage struct{ r image.Rectangle }

func (lazyImage) ColorModel() color.Model   { return color.RGBAModel }
func (m lazyImage) Bounds() image.Rectangle { return m.r }
func (lazyImage) At(x, y int) color.Color   { return checker(x, y) }
func checker(x, y int) color.Color {
	if (x/3+y/3)%2 == 0 {
		return color.Black
	}
	return color.White
}

// noSubImage hides the SubImage method of the embedded image.
type noSubImage struct{ image.Image }

func TestImageInvalid(t *testing.T) {
	var nilRGBA *image.RGBA
	tests := map[string]*demy.ImageNode{
		"uniform":      demy.Image(image.NewUniform(color.Black)),
		"uniform fill": demy.Image(image.NewUniform(color.Black), demy.ImageFill),
		"too wide":     demy.Image(lazyImage{image.Rect(0, 0, 1<<15, 10)}),
		"too tall":     demy.Image(lazyImage{image.Rect(0, 0, 10, 1<<15)}),
		"too large":    demy.Image(lazyImage{image.Rect(0, 0, 10000, 10000)}, demy.ImageFill),
		"overflow":     demy.Image(lazyImage{image.Rect(-1<<62, 0, 1<<62, 10)}),
		"empty":        demy.Image(image.NewRGBA(image.Rect(0, 0, 0, 10))),
		"nil":          demy.Image(nil),
		"typed nil":    demy.Image(nilRGBA),
		"no image":     {},
		"unknown key":  demy.ImageKey("missing"),
	}
	for name, n := range tests {
		t.Run(name, func(t *testing.T) {
			err := renderErr(demy.Frame(20, 20, n))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "Image") && !strings.Contains(err.Error(), "image") {
				t.Errorf("error %q does not mention the image", err)
			}
		})
	}
}

func TestImageFillWithoutSubImage(t *testing.T) {
	// Bounds not starting at (0,0) check that the view keeps absolute coordinates.
	r := image.Rect(5, 7, 65, 27)
	rgba := image.NewRGBA(r)
	draw.Draw(rgba, r, lazyImage{r}, r.Min, draw.Src)
	sources := map[string]image.Image{
		"lazy":    lazyImage{r},
		"wrapped": noSubImage{rgba},
	}
	for _, d := range []demy.Dither{demy.NoDithering, demy.FloydSteinberg} {
		render := func(img image.Image) image.Image {
			cfg := textConfig(40, 40)
			got, err := demy.MustNew(cfg).Render(
				demy.Frame(40, 40, demy.Image(img, demy.ImageFill, d)), demy.Grayscale)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			return got
		}
		want := render(rgba)
		for name, src := range sources {
			got := render(src)
			b := want.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					if wy, gy := color.GrayModel.Convert(want.At(x, y)).(color.Gray).Y, //nolint:forcetypeassert // GrayModel returns Gray
						color.GrayModel.Convert(got.At(x, y)).(color.Gray).Y; absDiff(wy, gy) > 2 { //nolint:forcetypeassert // GrayModel returns Gray
						t.Fatalf("%s dither %v: pixel (%d,%d) = %d, want %d (as *image.RGBA)", name, d, x, y, gy, wy)
					}
				}
			}
		}
	}
}
