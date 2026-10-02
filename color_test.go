package demy_test

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"samuelks.com/demy"
)

func TestRenderColorPalette(t *testing.T) {
	cfg := demy.Config{Palette: demy.PaletteBWRY}
	root := demy.HStack(
		demy.Box(demy.Background(demy.Red), demy.Frame(10, 10)),
		demy.Box(demy.Background(demy.Yellow), demy.Frame(10, 10)),
		demy.Box(demy.Background(demy.Black), demy.Frame(10, 10)),
	)
	img := render(t, root, 40, 10, demy.Paletted, cfg)
	p, ok := img.(*image.Paletted)
	if !ok {
		t.Fatalf("got %T, want *image.Paletted", img)
	}
	if len(p.Palette) != len(demy.PaletteBWRY) {
		t.Fatalf("palette has %d colors, want %d", len(p.Palette), len(demy.PaletteBWRY))
	}
	for _, tt := range []struct {
		x    int
		want uint8
	}{{5, 3}, {15, 2}, {25, 0}, {35, 1}} {
		if got := p.ColorIndexAt(tt.x, 5); got != tt.want {
			t.Errorf("pixel at x=%d = index %d, want %d", tt.x, got, tt.want)
		}
	}
}

// TestRenderColorImageDither checks per-image dithering with a color palette:
// a pink image dithers to red and white only.
func TestRenderColorImageDither(t *testing.T) {
	pink := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for i := 0; i < len(pink.Pix); i += 4 {
		pink.Pix[i], pink.Pix[i+1], pink.Pix[i+2], pink.Pix[i+3] = 255, 160, 160, 255
	}
	img := render(t, demy.Image(pink, demy.ImageStretch, demy.FloydSteinberg), 20, 20, demy.Paletted, demy.Config{Palette: demy.PaletteBWR})
	counts := make([]int, 3)
	for _, v := range img.(*image.Paletted).Pix { //nolint:forcetypeassert // Paletted renders *image.Paletted
		counts[v]++
	}
	if counts[0] != 0 || counts[1] == 0 || counts[2] == 0 {
		t.Errorf("black, white, red counts = %v, want red and white only", counts)
	}
}

func TestRenderColorConcurrent(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Palette = demy.PaletteSpectra6
	c := demy.MustNew(cfg)
	gradient := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			gradient.SetRGBA(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := c.Render(t.Context(), demy.Image(gradient, demy.Atkinson), demy.Paletted, demy.FloydSteinberg); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

func TestRenderPalettedWithoutPalette(t *testing.T) {
	c := demy.MustNew(demy.Config{PixelWidth: 4, PixelHeight: 4, PhysicalWidthMM: 4, PhysicalHeightMM: 4})
	if _, err := c.Render(t.Context(), demy.Frame(2, 2), demy.Paletted); err == nil {
		t.Error("expected error")
	}
}
