package jsonscene_test

import (
	"testing"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/jsonscene"
)

func FuzzUnmarshalRender(f *testing.F) {
	f.Add(`{"type":"vstack","gap":4,"children":[{"type":"text","text":"hi"},{"type":"frame","width":10,"height":10}]}`)
	f.Add(`{"type":"grid","columns":[{"kind":"fr","weight":1}],"children":[{"type":"spacer"},{"type":"divider"}]}`)
	f.Add(`{"type":"box","clip":true,"padding":2,"children":[{"type":"hstack"}]}`)
	cfg := demy.Config{PixelWidth: 64, PixelHeight: 48, PhysicalWidthMM: 64, PhysicalHeightMM: 48}
	canvas := demy.MustNew(cfg)
	f.Fuzz(func(t *testing.T, src string) {
		n, err := jsonscene.Unmarshal([]byte(src))
		if err != nil {
			return
		}
		_, _ = canvas.Render(n, demy.Grayscale)
	})
}
