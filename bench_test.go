package demy_test

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"samuelks.com/demy"
)

// dashboard builds a scene shaped like a typical e-ink dashboard: a header,
// three columns of text separated by dividers, a metrics grid, an image, and a
// footer bar.
func dashboard() demy.Node {
	title := demy.Style{Font: "sans", Size: 28}
	label := demy.Style{Font: "sans", Size: 11, Color: demy.DarkGray}
	body := demy.Style{Font: "sans", Size: 13}
	var tasks []demy.Node
	for i := range 6 {
		tasks = append(tasks, demy.Text(fmt.Sprintf("Task number %d with a longer description that wraps", i), body))
	}
	var metrics []demy.Node
	for i := range 8 {
		metrics = append(metrics, demy.Text(fmt.Sprintf("Metric %d", i), label), demy.Text(fmt.Sprintf("%d.%d", i*13, i), body))
	}
	photo := image.NewGray(image.Rect(0, 0, 120, 80))
	for y := range 80 {
		for x := range 120 {
			photo.SetGray(x, y, color.Gray{Y: uint8((x + y) * 2)})
		}
	}
	return demy.VStack(demy.Padding(16), demy.Gap(8),
		demy.HStack(demy.Text("Daily Brief", title), demy.Spacer(), demy.Text("Friday, May 22", body)),
		demy.Divider(),
		demy.HStack(demy.Gap(12),
			demy.Weighted(1, demy.VStack(demy.Gap(4), demy.Text("WEATHER", label), demy.Text("72°F", title),
				demy.Image(photo, demy.FloydSteinberg))),
			demy.Divider(),
			demy.Weighted(1, demy.VStack(demy.Gap(4), demy.Text("TASKS", label), demy.Children(tasks...))),
			demy.Divider(),
			demy.Weighted(1, demy.Grid(demy.Columns(demy.GridAuto(), demy.GridFr(1)), demy.Gap(4), demy.Children(metrics...))),
		),
		demy.Spacer(),
		demy.Box(demy.Background(demy.Black), demy.Padding(6), demy.CornerRadius(4),
			demy.Text("Last updated 9:00 AM", demy.Style{Font: "sans", Size: 11, Color: demy.White})),
	)
}

func BenchmarkRenderDashboard(b *testing.B) {
	fonts := map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}
	bwry := demy.TRMNLOriginal
	bwry.Palette = demy.PaletteBWRY
	for _, preset := range []struct {
		name string
		cfg  demy.Config
	}{{"Original", demy.TRMNLOriginal}, {"X", demy.TRMNLX}, {"OriginalBWRY", bwry}} {
		b.Run(preset.name, func(b *testing.B) {
			cfg := preset.cfg
			cfg.Fonts = fonts
			c := demy.MustNew(cfg)
			root := dashboard()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := c.Render(b.Context(), root, demy.Paletted); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// textPage builds a page of wrapped paragraphs, so text layout dominates.
func textPage() demy.Node {
	body := demy.Style{Font: "sans", Size: 12}
	var paras []demy.Node
	for i := range 12 {
		paras = append(paras, demy.Text(fmt.Sprintf("Paragraph %d. %s", i, strings.Repeat("The quick brown fox jumps over the lazy dog. ", 4)), body))
	}
	return demy.VStack(demy.Padding(16), demy.Gap(6), demy.Children(paras...))
}

// BenchmarkRenderText renders textPage on TRMNL Original.
func BenchmarkRenderText(b *testing.B) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}
	c := demy.MustNew(cfg)
	root := textPage()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Render(b.Context(), root, demy.Paletted); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderParallel renders the dashboard and textPage from concurrent
// goroutines sharing one Canvas and font family. Compare ns/op with
// BenchmarkRenderDashboard/Original and BenchmarkRenderText.
func BenchmarkRenderParallel(b *testing.B) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}
	c := demy.MustNew(cfg)
	for _, scene := range []struct {
		name string
		root demy.Node
	}{{"Dashboard", dashboard()}, {"Text", textPage()}} {
		b.Run(scene.name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := c.Render(context.Background(), scene.root, demy.Paletted); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}

// BenchmarkRenderNested renders HStacks and Grids nested 16 deep around text,
// which measure their children at more than one size.
func BenchmarkRenderNested(b *testing.B) {
	style := demy.Style{Font: "sans", Size: 12}
	wraps := []struct {
		name string
		wrap func(demy.Node) demy.Node
	}{
		{"HStack", func(n demy.Node) demy.Node {
			return demy.HStack(demy.Gap(1), demy.Text("words that overflow the row", style), demy.Children(n))
		}},
		{"Grid", func(n demy.Node) demy.Node {
			return demy.Grid(demy.Columns(demy.GridAuto(), demy.GridFr(1)), demy.Rows(demy.GridFr(1)), demy.Gap(1),
				demy.Text("words", style), demy.Children(n))
		}},
	}
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}
	c := demy.MustNew(cfg)
	for _, w := range wraps {
		b.Run(w.name, func(b *testing.B) {
			var root demy.Node = demy.Text("leaf", style)
			for range 16 {
				root = w.wrap(root)
			}
			for b.Loop() {
				if _, err := c.Render(b.Context(), root, demy.Paletted); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
