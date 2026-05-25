package starlarkplugin_test

import (
	"testing"

	starlarkplugin "github.com/samuel/demy/plugin/starlark"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

func TestRenderBasic(t *testing.T) {
	sans := demy.MustLoadFontBytes("sans", goregular.TTF)
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}
	cfg.Styles = map[string]demy.Style{
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

	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(node); err != nil {
		t.Fatal(err)
	}
}

func TestRenderLayoutVariants(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
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
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(node); err != nil {
		t.Fatal(err)
	}
}

func TestStylesDict(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
	}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	script := []byte(`
styles = {
    "h1":   Style(font="sans", size=24, weight="bold"),
    "body": Style(font="sans", size=12),
}

def render(data):
    return VStack(
        Text(data["title"], styles["h1"]),
        Text(data["body"],  styles["body"]),
    )
`)

	p, err := starlarkplugin.LoadBytes("test.star", script, cfg)
	if err != nil {
		t.Fatal(err)
	}

	got := p.Styles()
	want := map[string]demy.Style{
		"h1":   {Font: "sans", Size: 24, Weight: demy.FontBold},
		"body": {Font: "sans", Size: 12},
	}
	if len(got) != len(want) {
		t.Fatalf("Styles() len: want %d, got %d: %v", len(want), len(got), got)
	}
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("Styles() missing key %q", k)
			continue
		}
		if gv != wv {
			t.Errorf("Styles()[%q]: want %+v, got %+v", k, wv, gv)
		}
	}

	// Verify render works with script-defined styles used directly.
	node, err := p.Render(map[string]any{"title": "Hello", "body": "world"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(node); err != nil {
		t.Fatalf("canvas.Render: %v", err)
	}
}

func TestStylesDictBadValue(t *testing.T) {
	cfg := demy.TRMNLOriginal
	script := []byte(`
styles = {"h1": VStack()}
def render(data): return VStack()
`)
	_, err := starlarkplugin.LoadBytes("bad.star", script, cfg)
	if err == nil {
		t.Fatal("expected error for non-Style value in styles dict")
	}
}

func TestRenderMissingRenderFunction(t *testing.T) {
	cfg := demy.TRMNLOriginal
	_, err := starlarkplugin.LoadBytes("bad.star", []byte(`x = 1`), cfg)
	if err == nil {
		t.Fatal("expected error for missing render function")
	}
}

func TestRenderBadReturnType(t *testing.T) {
	cfg := demy.TRMNLOriginal
	p, err := starlarkplugin.LoadBytes("bad.star", []byte(`def render(data): return 42`), cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Render(nil)
	if err == nil {
		t.Fatal("expected error for non-node return type")
	}
}
