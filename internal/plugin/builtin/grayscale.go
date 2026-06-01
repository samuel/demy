package builtin

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"image/png"

	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/plugin"
	"golang.org/x/image/font/gofont/goregular"
)

// GrayscaleRenderer renders a 16-square test pattern covering the 4-bit gray palette.
type GrayscaleRenderer struct{}

// Render renders the grayscale test pattern to PNG bytes.
func (r *GrayscaleRenderer) Render(_ context.Context, cfg demy.Config, _ *model.UserPlugin, _ plugin.ResourceCache) ([]byte, error) {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}

	c, err := demy.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating canvas: %w", err)
	}

	outImg, err := c.Render(buildGrayscaleScene(), plugin.EffectiveColorMode(cfg), demy.Atkinson)
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
