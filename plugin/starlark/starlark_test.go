package starlarkplugin_test

import (
	"context"
	"testing"

	sl "go.starlark.net/starlark"
	"golang.org/x/image/font/gofont/goregular"

	"samuelks.com/demy"
	starlarkplugin "samuelks.com/demy/plugin/starlark"
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

	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "test.star", script, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	node, err := p.Render(context.Background(), map[string]any{
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
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
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
    return ZStack(ZAlignTopLeading,
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

	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "layout.star", script, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	node, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
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

	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "test.star", script, cfg, nil)
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
	node, err := p.Render(context.Background(), map[string]any{"title": "Hello", "body": "world"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
		t.Fatalf("canvas.Render: %v", err)
	}
}

func TestGridBuiltins(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
	}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	script := []byte(`
def render(data):
    return Grid(
        Columns(GridFixed(120), GridAuto(), GridFr(1), GridFr(2)),
        Rows(GridAuto(), GridFixed(40)),
        RowGap(4),
        ColGap(8),
        Padding(12),
        Text("a"), Text("b"), Text("c"), Text("d"),
        Text("e"), Text("f"), Text("g"), Text("h"),
    )
`)

	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "grid.star", script, cfg, nil)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	node, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
		t.Fatalf("canvas.Render: %v", err)
	}
}

func TestGridGapBuiltin(t *testing.T) {
	// Gap() should set both RowGap and ColGap in Grid.
	cfg := demy.TRMNLOriginal
	script := []byte(`
def render(data):
    return Grid(Columns(GridFr(1), GridFr(1)), Gap(6), Text("x"), Text("y"))
`)
	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "gap.star", script, cfg, nil)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	node, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	g, ok := node.(*demy.GridNode)
	if !ok {
		t.Fatalf("expected *GridNode, got %T", node)
	}
	if g.RowGap != 6 || g.ColGap != 6 {
		t.Errorf("want RowGap=6 ColGap=6, got %v %v", g.RowGap, g.ColGap)
	}
}

func TestStylesDictBadValue(t *testing.T) {
	cfg := demy.TRMNLOriginal
	script := []byte(`
styles = {"h1": VStack()}
def render(data): return VStack()
`)
	var env starlarkplugin.Environment
	_, err := env.LoadBytes(t.Context(), "bad.star", script, cfg, nil)
	if err == nil {
		t.Fatal("expected error for non-Style value in styles dict")
	}
}

func TestRenderMissingRenderFunction(t *testing.T) {
	cfg := demy.TRMNLOriginal
	var env starlarkplugin.Environment
	_, err := env.LoadBytes(t.Context(), "bad.star", []byte(`x = 1`), cfg, nil)
	if err == nil {
		t.Fatal("expected error for missing render function")
	}
}

func TestRenderBadReturnType(t *testing.T) {
	cfg := demy.TRMNLOriginal
	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "bad.star", []byte(`def render(data): return 42`), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Render(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for non-node return type")
	}
}

func TestAddBuiltin(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
	}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	var env starlarkplugin.Environment
	env.AddBuiltin("greet", func(_ *sl.Thread, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
		var name string
		if err := sl.UnpackPositionalArgs("greet", args, nil, 1, &name); err != nil {
			return nil, err
		}
		return sl.String("Hello, " + name + "!"), nil
	})

	script := []byte(`
def render(data):
    return VStack(Text(greet(data["name"])))
`)
	p, err := env.LoadBytes(t.Context(), "test.star", script, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	node, err := p.Render(context.Background(), map[string]any{"name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
		t.Fatal(err)
	}
}

func TestAddModule(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
	}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	var env starlarkplugin.Environment
	env.AddModule("data_svc", map[string]starlarkplugin.BuiltinFunc{
		"get_title": func(_ *sl.Thread, _ sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
			return sl.String("Module Title"), nil
		},
	})

	script := []byte(`
def render(data):
    return VStack(Text(data_svc.get_title()))
`)
	p, err := env.LoadBytes(t.Context(), "test.star", script, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	node, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
		t.Fatal(err)
	}
}

func TestContextPropagation(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
	}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "from-context")

	var receivedVal string
	var env starlarkplugin.Environment
	env.AddBuiltin("ctx_val", func(thread *sl.Thread, _ sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
		c := starlarkplugin.ContextFromThread(thread)
		if v, ok := c.Value(ctxKey{}).(string); ok {
			receivedVal = v
		}
		return sl.String(receivedVal), nil
	})

	script := []byte(`
def render(data):
    return VStack(Text(ctx_val()))
`)
	p, err := env.LoadBytes(t.Context(), "test.star", script, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Render(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if receivedVal != "from-context" {
		t.Fatalf("context value not propagated: got %q", receivedVal)
	}
}

func TestGridRowBuiltin(t *testing.T) {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{
		"sans": demy.MustLoadFontBytes("sans", goregular.TTF),
	}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	script := []byte(`
def render(data):
    return Grid(
        Columns(GridAuto(), GridFr(1)),
        RowGap(4),
        ColGap(8),
        GridRow(Text("Name"), Text("Value")),
        GridRow(Text("Temp"), Text("72 F")),
    )
`)
	var env starlarkplugin.Environment
	p, err := env.LoadBytes(t.Context(), "gridrow.star", script, cfg, nil)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	node, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	g, ok := node.(*demy.GridNode)
	if !ok {
		t.Fatalf("expected *GridNode, got %T", node)
	}
	if len(g.Children) != 2 {
		t.Fatalf("want 2 GridRowNode children, got %d", len(g.Children))
	}
	if _, ok := g.Children[0].(*demy.GridRowNode); !ok {
		t.Errorf("expected *GridRowNode, got %T", g.Children[0])
	}
	c, err := demy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Render(t.Context(), node, demy.RGBA); err != nil {
		t.Fatalf("canvas.Render: %v", err)
	}
}
