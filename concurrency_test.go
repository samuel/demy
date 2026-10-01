package demy_test

import (
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

// TestConcurrentCanvasRender renders on one Canvas from many goroutines with
// shared fonts; run with -race.
func TestConcurrentCanvasRender(t *testing.T) {
	sans := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := sans.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		t.Fatal(err)
	}
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
	c := demy.MustNew(cfg)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			// Each goroutine uses different characters so glyph caches fill concurrently.
			text := strings.Repeat(string(rune('A'+i))+string(rune('a'+i))+"é€ ", 20)
			root := demy.VStack(
				demy.Text(text, demy.Style{Font: "sans", Size: 14}),
				demy.Text(text, demy.Style{Font: "sans", Size: 20, Weight: demy.FontBold, Overflow: demy.TextOverflowEllipsis, MaxLines: 1}),
			)
			if _, err := c.Render(root, demy.TwoBit); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
