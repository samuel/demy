package starlarkplugin_test

import (
	"testing"

	starlarkplugin "github.com/samuel/pate/plugin/starlark"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/pate"
)

func TestRenderBasic(t *testing.T) {
	sans := pate.MustLoadFontBytes("sans", goregular.TTF)
	cfg := pate.TRMNLOriginal
	cfg.Fonts = map[string]*pate.FontFamily{"sans": sans}
	cfg.DefaultTextStyle = pate.Style{Font: "sans", Size: 12}
	cfg.Styles = map[string]pate.Style{
		"h1":      {Font: "sans", Size: 24},
		"body":    {Font: "sans", Size: 12},
		"caption": {Font: "sans", Size: 9},
	}

	script := []byte(`
def render(data):
    rows = [Text(item["name"], Body) for item in data["items"]]
    return VStack(Gap(8), Padding(12),
        Text(data["title"], H1),
        Children(rows),
        Divider(),
        HStack(
            Text(data["footer"], Caption),
            Spacer(),
            Text(data["count"], Style(font="sans", size=9, color=Muted)),
        ),
    )
`)

	p, err := starlarkplugin.LoadBytes("test.star", script, cfg)
	if err != nil {
		t.Fatal(err)
	}

	node, err := p.Render(map[string]any{
		"title":  "Weather",
		"footer": "Updated 12:34",
		"count":  "3 items",
		"items": []any{
			map[string]any{"name": "Portland"},
			map[string]any{"name": "Seattle"},
			map[string]any{"name": "Vancouver"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	c, err := pate.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(node); err != nil {
		t.Fatal(err)
	}
}

func TestRenderLayoutVariants(t *testing.T) {
	cfg := pate.TRMNLOriginal
	cfg.DefaultTextStyle = pate.Style{Font: "sans", Size: 12}
	cfg.Fonts = map[string]*pate.FontFamily{
		"sans": pate.MustLoadFontBytes("sans", goregular.TTF),
	}

	script := []byte(`
def render(data):
    return ZStack(TopLeading,
        Frame(200, 100,
            Box(Background(Muted), Padding(4), CornerRadius(8),
                Text("hello", Style(font="sans", size=10, weight="bold")),
            ),
        ),
        VStack(AlignEnd,
            Spacer(),
            HStack(AlignCenter,
                Text("72", Style(font="sans", size=32)),
                Text("°F", Style(font="sans", size=14, color=Accent)),
            ),
        ),
    )
`)

	p, err := starlarkplugin.LoadBytes("layout.star", script, cfg)
	if err != nil {
		t.Fatal(err)
	}
	node, err := p.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := pate.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(node); err != nil {
		t.Fatal(err)
	}
}

func TestRenderMissingRenderFunction(t *testing.T) {
	cfg := pate.TRMNLOriginal
	_, err := starlarkplugin.LoadBytes("bad.star", []byte(`x = 1`), cfg)
	if err == nil {
		t.Fatal("expected error for missing render function")
	}
}

func TestRenderBadReturnType(t *testing.T) {
	cfg := pate.TRMNLOriginal
	p, err := starlarkplugin.LoadBytes("bad.star", []byte(`def render(data): return 42`), cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Render(nil)
	if err == nil {
		t.Fatal("expected error for non-node return type")
	}
}
