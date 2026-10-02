package jsonscene_test

import (
	"strings"
	"testing"

	"samuelks.com/demy"
	"samuelks.com/demy/scene/jsonscene"
)

func TestUnmarshalDepthLimit(t *testing.T) {
	nested := func(depth int) []byte {
		return []byte(strings.Repeat(`{"type":"vstack","children":[`, depth-1) + `{"type":"vstack"}` + strings.Repeat("]}", depth-1))
	}
	if _, err := jsonscene.Unmarshal(nested(1000)); err != nil {
		t.Fatalf("1000 levels: %v", err)
	}
	if _, err := jsonscene.Unmarshal(nested(1001)); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("1001 levels: err = %v, want nesting error", err)
	}
	scene := []byte(`{"root":` + string(nested(1001)) + `}`)
	if _, _, err := jsonscene.UnmarshalScene(scene); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("scene with 1001 levels: err = %v, want nesting error", err)
	}
}

func TestUnmarshalNodeLimit(t *testing.T) {
	spacers := func(count int) []byte {
		return []byte(`{"type":"vstack","children":[` + strings.Repeat(`{"type":"spacer"},`, count-1) + `{"type":"spacer"}]}`)
	}
	if _, err := jsonscene.Unmarshal(spacers(1<<14 - 1)); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if _, err := jsonscene.Unmarshal(spacers(1 << 14)); err == nil || !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("over the limit: err = %v, want node limit error", err)
	}
}

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
		_, _ = canvas.Render(t.Context(), n, demy.Grayscale)
	})
}
