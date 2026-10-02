package starlarkplugin_test

import (
	"context"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sl "go.starlark.net/starlark"

	"github.com/samuel/demy"
	starlarkplugin "github.com/samuel/demy/plugin/starlark"
)

// evalText renders `Text(expr)` and returns the text.
func evalText(t *testing.T, env *starlarkplugin.Environment, expr string) string {
	t.Helper()
	if env == nil {
		env = &starlarkplugin.Environment{}
	}
	n, err := run(t, env, testCfg(), "def render(d): return Text("+expr+")", nil)
	if err != nil {
		t.Fatal(err)
	}
	return textOf(t, n)
}

// TestBuiltinArgumentErrors checks the argument count and type errors of each
// builtin, and that each error names the builtin.
func TestBuiltinArgumentErrors(t *testing.T) {
	tests := map[string]struct{ expr, want string }{
		"frame no args":        {`Frame(10)`, "Frame: expected at least 2"},
		"frame w string":       {`Frame("a", 10)`, "Frame: w must be a number"},
		"frame h string":       {`Frame(10, "b")`, "Frame: h must be a number"},
		"frame bad option":     {`Frame(10, 10, 3)`, "Frame: expected node or option, got int"},
		"spacer two args":      {`Spacer(1, 2)`, "Spacer: expected at most 1"},
		"spacer string":        {`Spacer("1")`, "Spacer: weight must be a number"},
		"weighted one arg":     {`Weighted(1)`, "Weighted: expected 2"},
		"weighted string":      {`Weighted("1", Text("x"))`, "Weighted: weight must be a number"},
		"divider positional":   {`Divider(2)`, "Divider: takes keyword arguments only"},
		"divider bad size":     {`Divider(size="2")`, "Divider: size must be a number"},
		"text no args":         {`Text()`, "Text: expected at least 1"},
		"text not string":      {`Text(1)`, "Text: text must be a string"},
		"class no args":        {`Text("x", Class())`, "Class: expected 1"},
		"class not string":     {`Text("x", Class(1))`, "Class: name must be a string"},
		"image no args":        {`ImageKey()`, "ImageKey: expected at least 1"},
		"image key int":        {`ImageKey(1)`, "ImageKey: key must be a string"},
		"image bad option":     {`ImageKey("k", "fit")`, "ImageKey: expected node or option, got string"},
		"barcode no args":      {`Barcode()`, "Barcode: expected at least 1"},
		"barcode int":          {`Barcode(1)`, "Barcode: content must be a string"},
		"barcode bad option":   {`Barcode("x", 1)`, "Barcode: expected node or option"},
		"style positional":     {`Text("x", Style("sans"))`, "Style: takes keyword arguments only"},
		"style class int":      {`Text("x", Style(**{"class": 1}))`, "Style: class must be a string"},
		"style max_lines str":  {`Text("x", Style(max_lines="1"))`, "Style: "},
		"style weight int":     {`Text("x", Style(weight=700))`, "Style: weight must be a string"},
		"style color empty":    {`Text("x", Style(color=""))`, "Style: color must not be empty"},
		"style color int":      {`Text("x", Style(color=0))`, "Style: color must be a string"},
		"background no args":   {`Box(Background())`, "Background: expected 1"},
		"border one arg":       {`Box(Border(1))`, "Border: expected 2"},
		"border width string":  {`Box(Border("1", Black))`, "Border: width must be a number"},
		"border bad color":     {`Box(Border(1, "nope"))`, "Border: color"},
		"border kwarg":         {`Box(Border(1, Black, w=1))`, `Border: unexpected keyword argument "w"`},
		"gap no args":          {`VStack(Gap())`, "Gap: expected 1 argument, got 0"},
		"gridrow non-node":     {`Grid(Columns(GridAuto()), GridRow(1))`, "GridRow: expected node, got int"},
		"gridauto arg":         {`Grid(Columns(GridAuto(1)))`, "GridAuto: takes no arguments"},
		"gridfixed no args":    {`Grid(Columns(GridFixed()))`, "GridFixed: expected 1 argument (px)"},
		"gridfr string":        {`Grid(Columns(GridFr("1")))`, "GridFr: weight must be a number"},
		"columns non-track":    {`Grid(Columns(1))`, "Columns: expected track or list of tracks, got int"},
		"columns list item":    {`Grid(Columns([GridAuto(), 1]))`, "Columns: list item must be a track, got int"},
		"rows non-track":       {`Grid(Columns(GridAuto()), Rows("auto"))`, "Rows: expected track"},
		"children non-node":    {`VStack(Children(1))`, "Children: expected node or list, got int"},
		"children list item":   {`VStack(Children([Text("x"), 1]))`, "Children: list item must be a node, got int"},
		"container style":      {`VStack(Style())`, "VStack: expected node or option, got style"},
		"container track":      {`HStack(GridAuto())`, "HStack: expected node or option, got track"},
		"container class":      {`ZStack(Class("h1"))`, "ZStack: expected node or option, got class"},
		"vstack non-option":    {`VStack(Text)`, "VStack: expected node or option, got builtin_function_or_method"},
		"time unknown attr":    {`Text(now().century)`, "no .century field or method"},
		"style unhashable":     {`Text({Style(): 1})`, "unhashable: style"},
		"node unhashable":      {`Text({Text("x"): 1})`, "unhashable: node"},
		"option unhashable":    {`Text({Gap(1): 1})`, "unhashable: option"},
		"track unhashable":     {`Text({GridAuto(): 1})`, "unhashable: track"},
		"class unhashable":     {`Text({Class("a"): 1})`, "unhashable: class"},
		"time unhashable":      {`Text({now(): 1})`, "unhashable: time"},
		"weighted non-node ch": {`Weighted(1, Gap(1))`, "Weighted: child must be a node, got option"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := run(t, &starlarkplugin.Environment{}, testCfg(), "def render(d): return "+tt.expr, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s: err = %v, want %q", tt.expr, err, tt.want)
			}
		})
	}
}

// TestNodeNotOption checks that a host node that isn't an Option can't be
// passed to a container.
func TestNodeNotOption(t *testing.T) {
	env := &starlarkplugin.Environment{}
	env.AddBuiltin("raw", func(*sl.Thread, sl.Tuple, []sl.Tuple) (sl.Value, error) {
		return starlarkplugin.WrapNode(notOption{}), nil
	})
	_, err := run(t, env, testCfg(), "def render(d): return VStack(raw())", nil)
	if err == nil || !strings.Contains(err.Error(), "is not an Option") {
		t.Errorf("err = %v, want an is not an Option error", err)
	}
}

// notOption is a Node that doesn't implement Option.
type notOption struct{ demy.Node }

func TestBuiltinValues(t *testing.T) {
	n, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Box(
    Border(2, Accent), Background("#ff000080"), Padding(3), Clip,
    Frame(40, 20, AlignCenter, Spacer(), Spacer(2)),
    Grid(Columns([GridFixed(10), GridAuto()], GridFr(2)), Rows([GridAuto()]), RowGap(1), ColGap(2),
        GridRow(Text("a"), Text("b"))),
    Divider(size=3, color=Muted),
)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	box := n.(*demy.BoxNode) //nolint:forcetypeassert // test
	if box.BorderWidth != 2 || box.BorderColor != (demy.RoleColor{Name: "accent"}) || !box.Clip || box.Padding != 3 {
		t.Errorf("box = %+v", box)
	}
	if want := (color.NRGBA{R: 0xff, A: 0x80}); box.Background != want {
		t.Errorf("background = %#v, want %#v", box.Background, want)
	}
	frame := box.Children[0].(*demy.FrameNode) //nolint:forcetypeassert // test
	if frame.Width != 40 || frame.Height != 20 || len(frame.Children) != 2 {
		t.Errorf("frame = %+v", frame)
	}
	if s := frame.Children[1].(*demy.SpacerNode); s.Weight != 2 { //nolint:forcetypeassert // test
		t.Errorf("spacer weight = %v, want 2", s.Weight)
	}
	grid := box.Children[1].(*demy.GridNode) //nolint:forcetypeassert // test
	if len(grid.Columns) != 3 || !grid.Columns[0].IsFixed() || !grid.Columns[1].IsAuto() || grid.Columns[2].Weight() != 2 {
		t.Errorf("columns = %+v", grid.Columns)
	}
	if len(grid.Rows) != 1 || grid.RowGap != 1 || grid.ColGap != 2 {
		t.Errorf("grid = %+v", grid)
	}
	div := box.Children[2].(*demy.DividerNode) //nolint:forcetypeassert // test
	if div.Size != 3 || div.Color != (demy.RoleColor{Name: "muted"}) {
		t.Errorf("divider = %+v", div)
	}
}

func TestStyleFields(t *testing.T) {
	cfg := testCfg()
	cfg.Styles = map[string]demy.Style{"base": {Font: "sans", Size: 10, MaxLines: 2}}
	src := `def render(d): return Text("x", Style(size=14, line_height=1.5,
    color=Red, weight="bold-italic", align="center", overflow="ellipsis", **{"class": "base"}))`
	n, err := run(t, &starlarkplugin.Environment{}, cfg, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := demy.Style{
		Font: "sans", Size: 14, LineHeight: 1.5, Color: demy.Red, MaxLines: 2,
		Weight: demy.FontBold | demy.FontItalic, Align: demy.TextAlignCenter, Overflow: demy.TextOverflowEllipsis,
	}
	if got := n.(*demy.TextNode).Style; got != want { //nolint:forcetypeassert // test
		t.Errorf("style = %+v\nwant    %+v", got, want)
	}
}

// TestValueStrings checks str() and type() of each value type, and that
// values stored in module globals survive freezing.
func TestValueStrings(t *testing.T) {
	env := &starlarkplugin.Environment{Now: func() time.Time { return time.Date(2026, 5, 22, 9, 30, 15, 0, time.UTC) }}
	src := `
vals = [Text("x"), Gap(1), Style(), GridAuto(), Class("h1"), now()]
def render(d):
    return Text(" ".join([str(v) + ":" + type(v) + ":" + str(bool(v)) for v in vals]))
`
	n, err := run(t, env, testCfg(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `<*demy.TextNode>:node:True <demy.gapOpt>:option:True <style>:style:True <track>:track:True ` +
		`Class("h1"):class:True 2026-05-22T09:30:15Z:time:True`
	if got := textOf(t, n); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTimeAttributes(t *testing.T) {
	env := &starlarkplugin.Environment{Now: func() time.Time { return time.Date(2026, 5, 22, 9, 30, 15, 0, time.UTC) }}
	got := evalText(t, env, `" ".join([str(getattr(now(), a)) for a in dir(now()) if a != "format"])`)
	// dir() lists attributes in sorted order.
	want := "22 9 30 5 15 UTC 1779442215 5 Friday 2026"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestWrapNode(t *testing.T) {
	env := &starlarkplugin.Environment{}
	env.AddBuiltin("header", func(*sl.Thread, sl.Tuple, []sl.Tuple) (sl.Value, error) {
		return starlarkplugin.WrapNode(demy.Text("header")), nil
	})
	env.AddBuiltin("text_of", func(_ *sl.Thread, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
		n, ok := starlarkplugin.UnwrapNode(args[0])
		if !ok {
			return sl.String("not a node"), nil
		}
		return sl.String(n.(*demy.TextNode).Text), nil //nolint:forcetypeassert // test
	})
	n, err := run(t, env, testCfg(), `def render(d): return VStack(header(), Text(text_of(Text("a")) + text_of(1)))`, nil)
	if err != nil {
		t.Fatal(err)
	}
	vs := n.(*demy.VStackNode) //nolint:forcetypeassert // test
	if got := textOf(t, vs.Children[0]); got != "header" {
		t.Errorf("wrapped node text = %q", got)
	}
	if got := textOf(t, vs.Children[1]); got != "anot a node" {
		t.Errorf("unwrapped = %q, want %q", got, "anot a node")
	}
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.star")
	if err := os.WriteFile(path, []byte(`def render(d): return Text("file")`), 0o600); err != nil {
		t.Fatal(err)
	}
	var env starlarkplugin.Environment
	p, err := env.Load(t.Context(), path, testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := p.Render(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, n); got != "file" {
		t.Errorf("text = %q", got)
	}
	if _, err := env.Load(t.Context(), filepath.Join(t.TempDir(), "missing.star"), testCfg(), nil); err == nil {
		t.Error("missing file: expected error")
	}
}

// TestNilContext checks that Load and Render accept a nil context.
func TestNilContext(t *testing.T) {
	var env starlarkplugin.Environment
	//nolint:staticcheck // testing nil context handling
	p, err := env.LoadBytes(nil, "t.star", []byte(`def render(d): return Text("x")`), testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:staticcheck // testing nil context handling
	if _, err := p.Render(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestContextFromThreadDefault(t *testing.T) {
	if ctx := starlarkplugin.ContextFromThread(&sl.Thread{}); ctx != context.Background() {
		t.Errorf("ctx = %v, want context.Background()", ctx)
	}
}

// TestLoadErrors checks Load errors that don't come from evaluation.
func TestLoadErrors(t *testing.T) {
	tests := map[string]struct{ src, want string }{
		"syntax":         {"def render(d) return 1\n", "t.star:1"},
		"styles key":     {"styles = {1: Style()}\ndef render(d): return Text(\"\")\n", "styles key must be a string, got int"},
		"empty styles":   {"styles = {}\ndef render(d): return Text(\"\")\n", ""},
		"style globals":  {"x = H1\ndef render(d): return Text(\"\")\n", ""},
		"bad style name": {"x = Section_label\ndef render(d): return Text(\"\")\n", "undefined: Section_label"},
	}
	cfg := testCfg()
	cfg.Styles = map[string]demy.Style{"h1": {Size: 24}, "section-label": {Size: 9}, "": {Size: 1}}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte(tt.src), cfg, nil)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if p.Styles() != nil {
					t.Errorf("styles = %v, want nil", p.Styles())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestRenderPanicRecovered checks that a panic outside a host builtin, here
// in a host value's String method called by str(), fails the render.
func TestRenderPanicRecovered(t *testing.T) {
	env := &starlarkplugin.Environment{}
	env.AddValue("bad", panicValue{})
	_, err := run(t, env, testCfg(), `def render(d): return Text(str(bad))`, nil)
	if err == nil || !strings.Contains(err.Error(), "render: panic: bad value") {
		t.Errorf("err = %v, want a render panic error", err)
	}
}

type panicValue struct{}

func (panicValue) String() string        { panic("bad value") }
func (panicValue) Type() string          { return "panic" }
func (panicValue) Freeze()               {}
func (panicValue) Truth() sl.Bool        { return sl.True }
func (panicValue) Hash() (uint32, error) { return 0, nil }

func TestConverterEdgeCases(t *testing.T) {
	var nilIface any
	type named string
	tests := map[string]struct {
		in   any
		want string
	}{
		"nil":           {nil, "None"},
		"nil map":       {map[string]int(nil), "{}"},
		"nil slice":     {[]int(nil), "[]"},
		"nil interface": {[]any{nilIface}, "[None]"},
		"nil pointer":   {(*string)(nil), "None"},
		"bool":          {true, "True"},
		"float32":       {float32(0.5), "0.5"},
		"uint8":         {uint8(255), "255"},
		"named string":  {named("n"), `"n"`},
		"starlark":      {sl.MakeInt(3), "3"},
		"json int":      {json.Number("-12"), "-12"},
		"json float":    {json.Number("1e3"), "1000.0"},
		"nested":        {map[string][]map[string]int{"a": {{"b": 1}}}, `{"a": [{"b": 1}]}`},
		"array":         {[3]string{"x", "y", "z"}, `["x", "y", "z"]`},
		"time":          {time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "2026-01-02T03:04:05Z"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			v, err := starlarkplugin.ToStarlark(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got := v.String(); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}

	errTests := map[string]struct {
		in   any
		want string
	}{
		"int map key":      {map[int]string{1: "a"}, "unsupported map key type int"},
		"bad json.Number":  {json.Number("1x"), `invalid json.Number "1x"`},
		"nested map error": {map[string]any{"k": make(chan int)}, `["k"]: unsupported type chan int`},
		"slice error":      {[]any{1, func() {}}, "[1]: unsupported type func()"},
		"struct":           {struct{ A int }{1}, "unsupported type struct"},
		"complex":          {complex(1, 2), "unsupported type complex128"},
	}
	for name, tt := range errTests {
		t.Run(name, func(t *testing.T) {
			_, err := starlarkplugin.ToStarlark(tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
