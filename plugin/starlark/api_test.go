package starlarkplugin_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	sl "go.starlark.net/starlark"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
	starlarkplugin "github.com/samuel/demy/plugin/starlark"
)

func testCfg() demy.Config {
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": demy.MustLoadFontBytes("sans", goregular.TTF)}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}
	return cfg
}

// run loads src and renders it with data.
func run(t *testing.T, env *starlarkplugin.Environment, cfg demy.Config, src string, data map[string]any) (demy.Node, error) {
	t.Helper()
	p, err := env.LoadBytes(t.Context(), "t.star", []byte(src), cfg, nil)
	if err != nil {
		return nil, err
	}
	return p.Render(t.Context(), data)
}

func textOf(t *testing.T, n demy.Node) string {
	t.Helper()
	tn, ok := n.(*demy.TextNode)
	if !ok {
		t.Fatalf("got %T, want *TextNode", n)
	}
	return tn.Text
}

func TestWeighted(t *testing.T) {
	n, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return HStack(Weighted(2, Text("a")), Weighted(1, Spacer()))`, nil)
	if err != nil {
		t.Fatal(err)
	}
	hs := n.(*demy.HStackNode) //nolint:forcetypeassert // test
	w, ok := hs.Children[0].(*demy.WeightedNode)
	if !ok || w.Weight != 2 {
		t.Fatalf("child = %#v, want Weighted(2, ...)", hs.Children[0])
	}
	if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Weighted(1, "x")`, nil); err == nil {
		t.Error("Weighted with non-node child: expected error")
	}
}

func TestClassResolvesScriptStyles(t *testing.T) {
	src := `
styles = {"h1": Style(font="sans", size=24)}
def render(d): return Text("x", Class("h1"))
`
	cfg := testCfg()
	p, err := (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte(src), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := p.Render(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := n.(*demy.TextNode).Class; c != "h1" { //nolint:forcetypeassert // test
		t.Errorf("class = %q, want h1", c)
	}
	cfg.Styles = p.Styles()
	if _, err := demy.MustNew(cfg).Render(t.Context(), n, demy.Grayscale); err != nil {
		t.Errorf("render with merged styles: %v", err)
	}
	cfg.Styles = nil
	if _, err := demy.MustNew(cfg).Render(t.Context(), n, demy.Grayscale); err == nil {
		t.Error("render without the class: expected error")
	}
}

func TestStyleUnknownClass(t *testing.T) {
	if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Text("x", Style(class="nope"))`, nil); err == nil {
		t.Error("expected error")
	}
}

func TestBuiltinsRejectBadArguments(t *testing.T) {
	tests := map[string]string{
		"vstack kwarg":      `VStack(gap=8)`,
		"text kwarg":        `Text("x", style=Style())`,
		"spacer kwarg":      `Spacer(weight=2)`,
		"now arg":           `Text(str(now("UTC")))`,
		"now kwarg":         `Text(str(now(tz="UTC")))`,
		"text non-style":    `Text("x", 3)`,
		"size string":       `Text("x", Style(size="12"))`,
		"font int":          `Text("x", Style(font=1))`,
		"line height str":   `Text("x", Style(line_height="1.2"))`,
		"bad weight":        `Text("x", Style(weight="semibold"))`,
		"bad align":         `Text("x", Style(align="centre"))`,
		"bad overflow":      `Text("x", Style(overflow="elipsis"))`,
		"max lines < -1":    `Text("x", Style(max_lines=-2))`,
		"unknown field":     `Text("x", Style(colour="#000000"))`,
		"bad color":         `Box(Background("#zzz"))`,
		"divider bad color": `Divider(color="nope")`,
		"divider bad kwarg": `Divider(width=2)`,
		"gap string":        `VStack(Gap("4"))`,
		"class empty":       `Text("x", Class(""))`,
	}
	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), "def render(d): return "+expr, nil); err == nil {
				t.Errorf("%s: expected error", expr)
			}
		})
	}
	if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Text("x", Style(max_lines=-1))`, nil); err != nil {
		t.Errorf("max_lines=-1: %v", err)
	}
}

func TestStyleGlobalsDontShadowBuiltins(t *testing.T) {
	cfg := testCfg()
	cfg.Styles = map[string]demy.Style{
		"text":          {Size: 20},
		"center":        {Size: 20},
		"section-label": {Size: 9},
		"h1":            {Size: 24},
	}
	src := `def render(d): return ZStack(Center, Text("x", H1), Text("y", Class("section-label")))`
	n, err := run(t, &starlarkplugin.Environment{}, cfg, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	zs := n.(*demy.ZStackNode) //nolint:forcetypeassert // test
	if zs.Align != demy.Center {
		t.Errorf("align = %v, want Center", zs.Align)
	}
	if got := zs.Children[0].(*demy.TextNode).Style.Size; got != 24 { //nolint:forcetypeassert // test
		t.Errorf("H1 size = %v, want 24", got)
	}
}

func TestDataOrderStable(t *testing.T) {
	data := map[string]any{}
	for k := range strings.SplitSeq("q w e r t y u i o p a s d f g h j k l z", " ") {
		data[k] = 1
	}
	src := `def render(d): return Text(",".join(d.keys()))`
	for range 5 {
		n, err := run(t, &starlarkplugin.Environment{}, testCfg(), src, data)
		if err != nil {
			t.Fatal(err)
		}
		if got := textOf(t, n); got != "a,d,e,f,g,h,i,j,k,l,o,p,q,r,s,t,u,w,y,z" {
			t.Fatalf("keys = %q", got)
		}
	}
}

func TestAddValueFrozen(t *testing.T) {
	env := &starlarkplugin.Environment{}
	env.AddValue("cache", sl.NewDict(0))
	if _, err := run(t, env, testCfg(), "def render(d):\n    cache[\"k\"] = 1\n    return Text(\"\")\n", nil); err == nil {
		t.Error("mutating a shared value: expected error")
	}
}

func TestDataConversion(t *testing.T) {
	n7 := 7
	data := map[string]any{
		"strs":  []string{"a", "b"},
		"m":     map[string]string{"y": "2", "x": "1"},
		"u":     uint64(1 << 63),
		"i8":    int8(-3),
		"num":   json.Number("42"),
		"flt":   json.Number("1.5"),
		"ptr":   &n7,
		"nilp":  (*int)(nil),
		"rows":  []map[string]any{{"name": "r"}},
		"when":  time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC),
		"bytes": [2]int{1, 2},
	}
	src := `def render(d): return Text(" ".join([
    ",".join(d["strs"]), ",".join(d["m"].keys()), str(d["u"]), str(d["i8"]),
    str(d["num"] + 1), str(d["flt"]), str(d["ptr"]), str(d["nilp"]), d["rows"][0]["name"],
    str(d["when"].year), str(len(d["bytes"])),
]))`
	n, err := run(t, &starlarkplugin.Environment{}, testCfg(), src, data)
	if err != nil {
		t.Fatal(err)
	}
	want := "a,b x,y 9223372036854775808 -3 43 1.5 7 None r 2026 2"
	if got := textOf(t, n); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Text("")`, map[string]any{"c": make(chan int)}); err == nil {
		t.Error("unsupported type: expected error")
	}
}

func TestStylesMustBeDict(t *testing.T) {
	src := "styles = [Style(size=1)]\ndef render(d): return Text(\"\")\n"
	if _, err := (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte(src), testCfg(), nil); err == nil {
		t.Error("expected error")
	}
}

func TestNowHook(t *testing.T) {
	env := &starlarkplugin.Environment{Now: func() time.Time { return time.Date(2026, 5, 22, 9, 30, 0, 0, time.UTC) }}
	p, err := env.LoadBytes(t.Context(), "t.star", []byte(`def render(d): return Text(now().format("2006-01-02 15:04 MST"))`), testCfg(), time.FixedZone("X", 3600))
	if err != nil {
		t.Fatal(err)
	}
	n, err := p.Render(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, n); got != "2026-05-22 10:30 X" {
		t.Errorf("now = %q", got)
	}
}

func TestTimeFormatArgs(t *testing.T) {
	tests := map[string]string{
		"keyword":      `now().format(layout="2006")`,
		"extra kwarg":  `now().format("2006", x=1)`,
		"no args":      `now().format()`,
		"two args":     `now().format("2006", "01")`,
		"not a string": `now().format(2006)`,
	}
	for name, expr := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := run(t, &starlarkplugin.Environment{}, testCfg(), "def render(d): return Text("+expr+")", nil)
			if err == nil || !strings.Contains(err.Error(), "format: ") {
				t.Fatalf("err = %v, want an error prefixed with format:", err)
			}
		})
	}
}

func TestImageKeyAndBarcode(t *testing.T) {
	src := `def render(d): return VStack(
    ImageKey("logo", ImageFill, Atkinson),
    Barcode("https://example.com", ECLevelHigh),
)`
	n, err := run(t, &starlarkplugin.Environment{}, testCfg(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	vs := n.(*demy.VStackNode) //nolint:forcetypeassert // test
	img, ok := vs.Children[0].(*demy.ImageNode)
	if !ok || img.Key != "logo" || img.Scale != demy.ImageFill || img.Dither != demy.Atkinson {
		t.Errorf("image = %#v", vs.Children[0])
	}
	bc, ok := vs.Children[1].(*demy.BarcodeNode)
	if !ok || bc.Content != "https://example.com" || bc.ECLevel != demy.ECLevelHigh {
		t.Errorf("barcode = %#v", vs.Children[1])
	}
	for _, expr := range []string{`ImageKey(1)`, `ImageKey("k", "fill")`, `Barcode(1)`, `Barcode("x", Gap)`} {
		if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), "def render(d): return "+expr, nil); err == nil {
			t.Errorf("%s: expected error", expr)
		}
	}
}

// TestConcurrentRender renders one plugin from many goroutines; run with -race.
func TestConcurrentRender(t *testing.T) {
	src := `
HEADER = Text("header")
def render(d):
    rows = [Text(k + "=" + str(v)) for k, v in d.items()]
    return VStack(HEADER, Children(rows))
`
	p, err := (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte(src), testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := demy.MustNew(testCfg())
	errs := make(chan error, 8)
	for i := range 8 {
		go func() {
			n, err := p.Render(t.Context(), map[string]any{"i": i, "name": "x"})
			if err == nil {
				_, err = c.Render(t.Context(), n, demy.Grayscale)
			}
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}
