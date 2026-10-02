package demy_test

import (
	"image"
	"image/color"
	"testing"

	"samuelks.com/demy"
)

var (
	testRed   = color.RGBA{R: 0xff, A: 0xff}
	testWhite = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

func render(t *testing.T, n demy.Node, w, h int, mode demy.ColorMode, cfg demy.Config) image.Image {
	t.Helper()
	cfg.PixelWidth = w
	cfg.PixelHeight = h
	cfg.PhysicalWidthMM = float64(w)
	cfg.PhysicalHeightMM = float64(h)
	img, err := demy.MustNew(cfg).Render(t.Context(), n, mode)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return img
}

func rgbaAt(img image.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA) //nolint:forcetypeassert // RGBAModel always returns color.RGBA
}

func TestDividerRoleColor(t *testing.T) {
	cfg := demy.Config{Roles: demy.Roles{Muted: testRed}}
	n := demy.VStack(
		demy.Spacer(),
		&demy.DividerNode{Color: demy.RoleColor{Name: "muted"}, Size: 4},
		demy.Spacer(),
	)
	img := render(t, n, 20, 20, demy.RGBA, cfg)
	if got := rgbaAt(img, 10, 10); got != testRed {
		t.Errorf("divider pixel = %v, want %v", got, testRed)
	}
}

// clipBox returns a 20px-wide grid cell holding a Box whose red child is 60px wide.
func clipBox(clip bool) demy.Node {
	opts := []demy.Option{
		demy.Frame(60, 10, demy.Box(demy.Background(testRed), demy.Frame(60, 10))),
	}
	if clip {
		opts = append(opts, demy.Clip)
	}
	return demy.Grid(
		demy.Columns(demy.GridFixed(20)),
		demy.Box(opts...),
	)
}

func TestBoxClip(t *testing.T) {
	cfg := demy.Config{Roles: demy.Roles{Background: testWhite}}

	img := render(t, clipBox(false), 80, 10, demy.RGBA, cfg)
	if got := rgbaAt(img, 40, 5); got != testRed {
		t.Fatalf("unclipped overflow pixel = %v, want %v", got, testRed)
	}

	img = render(t, clipBox(true), 80, 10, demy.RGBA, cfg)
	if got := rgbaAt(img, 10, 5); got != testRed {
		t.Errorf("clipped inside pixel = %v, want %v", got, testRed)
	}
	if got := rgbaAt(img, 40, 5); got == testRed {
		t.Errorf("clipped overflow pixel = %v, want not red", got)
	}
}

func TestFrameDither(t *testing.T) {
	gray := image.NewUniform(color.Gray{Y: 128})
	src := image.NewGray(image.Rect(0, 0, 40, 40))
	for y := range 40 {
		for x := range 40 {
			src.Set(x, y, gray.C)
		}
	}
	n := demy.Frame(40, 40, demy.Image(src, demy.ImageStretch, demy.FloydSteinberg))
	img := render(t, n, 40, 40, demy.Paletted, demy.Config{Palette: demy.PaletteBW})

	var black, white int
	for y := range 40 {
		for x := range 40 {
			if rgbaAt(img, x, y).R == 0 {
				black++
			} else {
				white++
			}
		}
	}
	if black == 0 || white == 0 {
		t.Errorf("dithered frame has %d black and %d white pixels, want a mix", black, white)
	}
}
