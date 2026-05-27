package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"image/png"
	"time"

	"github.com/samuel/demy"
	"golang.org/x/image/font/gofont/goregular"
)

// GrayscalePlugin renders a 16-square test pattern covering the 4-bit gray palette.
type GrayscalePlugin struct{}

func newGrayscalePlugin(_ HAConfig, _ *time.Location, _ map[string]any) (*GrayscalePlugin, error) {
	return &GrayscalePlugin{}, nil
}

func (p *GrayscalePlugin) Render(_ context.Context, cfg demy.Config) ([]byte, error) {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}

	c, err := demy.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating canvas: %w", err)
	}

	outImg, err := c.Render(buildGrayscaleScene(), effectiveColorMode(cfg), demy.Atkinson)
	if err != nil {
		return nil, fmt.Errorf("rendering grayscale: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, outImg); err != nil {
		return nil, fmt.Errorf("encoding png: %w", err)
	}
	return buf.Bytes(), nil
}

func buildGrayscaleScene() demy.Node {
	var cells []demy.Node
	for i := range 16 {
		shade := uint8(i * 255 / 15)

		var fg color.Color = demy.Black
		if shade < 128 {
			fg = demy.White
		}

		style := demy.Style{
			Font:  "sans",
			Size:  9,
			Color: fg,
			Align: demy.TextAlignCenter,
		}

		cells = append(cells, demy.Box(
			demy.Background(color.Gray{Y: shade}),
			demy.VStack(
				demy.AlignCenter,
				demy.Spacer(),
				demy.Text(fmt.Sprintf("0x%02X\n%d", shade, shade), style),
				demy.Spacer(),
			),
		))
	}

	return demy.Grid(
		demy.Columns(demy.GridFr(1), demy.GridFr(1), demy.GridFr(1), demy.GridFr(1)),
		demy.Rows(demy.GridFr(1), demy.GridFr(1), demy.GridFr(1), demy.GridFr(1)),
		demy.Gap(2),
		demy.Children(cells...),
	)
}
