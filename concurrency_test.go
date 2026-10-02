package demy_test

import (
	"bytes"
	"image"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

// TestConcurrentCanvasRender renders on one Canvas from many goroutines with
// shared fonts and checks each image against a render made alone with fresh
// fonts; run with -race.
func TestConcurrentCanvasRender(t *testing.T) {
	newCanvas := func() *demy.Canvas {
		sans := demy.MustLoadFontBytes("sans", goregular.TTF)
		if err := sans.AddStyle(gobold.TTF, demy.FontBold); err != nil {
			t.Fatal(err)
		}
		cfg := demy.TRMNLOriginal
		cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
		return demy.MustNew(cfg)
	}
	scene := func(i int) demy.Node {
		// Each goroutine uses different characters so glyph caches fill concurrently.
		text := strings.Repeat(string(rune('A'+i))+string(rune('a'+i))+"é€ ", 20)
		return demy.VStack(
			demy.Text(text, demy.Style{Font: "sans", Size: 14}),
			demy.Text(text, demy.Style{Font: "sans", Size: 20, Weight: demy.FontBold, Overflow: demy.TextOverflowEllipsis, MaxLines: 1}),
			// Faux bold and italic outlines are cached per family too.
			demy.Text(text, demy.Style{Font: "sans", Size: 16, Weight: demy.FontBlack | demy.FontItalic, MaxLines: 2}),
		)
	}
	const n = 16
	want := make([][]byte, n)
	for i := range n {
		img, err := newCanvas().Render(t.Context(), scene(i), demy.Paletted)
		if err != nil {
			t.Fatal(err)
		}
		want[i] = img.(*image.Paletted).Pix
	}

	c := newCanvas()
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			for range 4 {
				img, err := c.Render(t.Context(), scene(i), demy.Paletted)
				if err != nil {
					t.Error(err)
					return
				}
				if !bytes.Equal(img.(*image.Paletted).Pix, want[i]) {
					t.Errorf("scene %d: concurrent render differs from a render alone", i)
					return
				}
			}
		})
	}
	wg.Wait()
}
