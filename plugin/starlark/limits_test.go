package starlarkplugin_test

import (
	"bytes"
	"context"
	"image/color"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/samuel/demy"
	starlarkplugin "github.com/samuel/demy/plugin/starlark"
	sl "go.starlark.net/starlark"
)

const spinScript = `
def render(data):
    n = 0
    for i in range(1 << 40):
        n += i
    return Text(str(n))
`

func TestRenderStepLimit(t *testing.T) {
	env := starlarkplugin.Environment{MaxSteps: 10_000}
	p, err := env.LoadBytes(t.Context(), "spin.star", []byte(spinScript), demy.TRMNLOriginal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Render(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "too many steps") {
		t.Fatalf("err = %v, want step limit error", err)
	}
}

// TestSharedNodeBlowup checks that a script returning a tree whose nodes are
// shared, and so expand to 2²⁰ nodes, fails to render quickly.
func TestSharedNodeBlowup(t *testing.T) {
	const script = `
def render(data):
    x = Text("a")
    for i in range(20):
        x = VStack(x, x)
    return x
`
	cfg := testCfg()
	n, err := run(t, &starlarkplugin.Environment{}, cfg, script, nil)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := demy.MustNew(cfg).Render(t.Context(), n, demy.Grayscale); err == nil || !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("err = %v, want node limit error", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("render took %v, want well under a second", d)
	}
}

func TestLoadStepLimit(t *testing.T) {
	env := starlarkplugin.Environment{MaxSteps: 10_000}
	src := "x = [i for i in range(1 << 40)]\ndef render(data): return Text(\"\")\n"
	if _, err := env.LoadBytes(t.Context(), "spin.star", []byte(src), demy.TRMNLOriginal, nil); err == nil {
		t.Fatal("expected step limit error")
	}
}

func TestRenderCancel(t *testing.T) {
	env := starlarkplugin.Environment{MaxSteps: 1 << 62}
	p, err := env.LoadBytes(t.Context(), "spin.star", []byte(spinScript), demy.TRMNLOriginal, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := p.Render(ctx, nil); err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("err = %v, want cancellation error", err)
	}
}

func TestPrintLogged(t *testing.T) {
	var buf bytes.Buffer
	env := starlarkplugin.Environment{Logger: slog.New(slog.NewTextHandler(&buf, nil))}
	src := "def render(data):\n    print(\"hello from script\")\n    return Text(\"\")\n"
	p, err := env.LoadBytes(t.Context(), "print.star", []byte(src), demy.TRMNLOriginal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Render(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "hello from script") || !strings.Contains(out, "script=print.star") {
		t.Errorf("log output = %q", out)
	}
}

func TestTranslucentColor(t *testing.T) {
	var env starlarkplugin.Environment
	src := "def render(data): return Box(Background(\"#ff000080\"))\n"
	p, err := env.LoadBytes(t.Context(), "c.star", []byte(src), demy.TRMNLOriginal, nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := p.Render(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := color.NRGBA{R: 0xff, A: 0x80}
	if got := n.(*demy.BoxNode).Background; got != want { //nolint:forcetypeassert // test
		t.Errorf("background = %#v, want %#v", got, want)
	}
}

// TestBuiltinPanic checks that a panic in a host builtin, registered directly
// or in a module, fails the call at the script line that made it.
func TestBuiltinPanic(t *testing.T) {
	boom := func(*sl.Thread, sl.Tuple, []sl.Tuple) (sl.Value, error) { panic("boom") }
	env := &starlarkplugin.Environment{}
	env.AddBuiltin("explode", boom)
	env.AddModule("svc", map[string]starlarkplugin.BuiltinFunc{"explode": boom})
	for _, call := range []string{"explode()", "svc.explode()"} {
		src := "def render(data):\n    x = 1\n    return " + call + "\n"
		_, err := run(t, env, testCfg(), src, nil)
		if err == nil || !strings.Contains(err.Error(), "panic: boom") || !strings.Contains(err.Error(), "t.star:3") {
			t.Errorf("%s: err = %v, want a panic error at t.star:3", call, err)
		}
	}
	// A panic during Load is also returned as an error.
	if _, err := env.LoadBytes(t.Context(), "load.star", []byte("x = explode()\n"), testCfg(), nil); err == nil || !strings.Contains(err.Error(), "panic: boom") {
		t.Errorf("load: err = %v, want a panic error", err)
	}
}

func TestDataLimits(t *testing.T) {
	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := []any{nil}
	cyclicSlice[0] = cyclicSlice
	var cyclicPtr any
	cyclicPtr = &cyclicPtr
	var deep any = 1
	for range 200 {
		deep = []any{deep}
	}
	// Each level references the one below twice: 2^40 values in all.
	var shared any = 1
	for range 40 {
		shared = []any{shared, shared}
	}
	tests := map[string]struct {
		data any
		want string
	}{
		"cyclic map":   {cyclicMap, "cyclic"},
		"cyclic slice": {cyclicSlice, "cyclic"},
		"cyclic ptr":   {cyclicPtr, "cyclic"},
		"deep":         {deep, "nested deeper"},
		"shared":       {shared, "values"},
	}
	for name, tt := range tests {
		start := time.Now()
		_, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Text("")`, map[string]any{"x": tt.data})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q error", name, err, tt.want)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: took %v", name, d)
		}
		if _, err := starlarkplugin.ToStarlark(tt.data); err == nil {
			t.Errorf("%s: ToStarlark: expected error", name)
		}
	}
	// Sharing without a cycle is allowed.
	leaf := []any{1, 2}
	if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), `def render(d): return Text("")`, map[string]any{"a": leaf, "b": []any{leaf, leaf}}); err != nil {
		t.Errorf("shared leaf: %v", err)
	}
}

// TestPrintLimits checks that long print() messages are truncated and that
// output past the per-run limit is dropped.
func TestPrintLimits(t *testing.T) {
	var buf bytes.Buffer
	env := starlarkplugin.Environment{Logger: slog.New(slog.NewTextHandler(&buf, nil))}
	src := `def render(data):
    print("long:" + "x" * 100000)
    for i in range(10000):
        print("flood " + "y" * 100)
    return Text("")
`
	p, err := env.LoadBytes(t.Context(), "print.star", []byte(src), demy.TRMNLOriginal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Render(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "long:"+strings.Repeat("x", 100)) || strings.Contains(out, strings.Repeat("x", 5000)) {
		t.Error("long message not truncated to about 4 KiB")
	}
	if n := strings.Count(out, "flood"); n == 0 || n > 1000 {
		t.Errorf("logged %d flood messages, want some but far fewer than 10000", n)
	}
	if !strings.Contains(out, "print output limit reached") {
		t.Error("missing limit warning")
	}
	if len(out) > 1<<17 {
		t.Errorf("logged %d bytes, want about 64 KiB", len(out))
	}
	// The limit is per run.
	buf.Reset()
	if _, err := p.Render(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "long:") {
		t.Error("second render logged nothing")
	}
}

// TestNumberRange checks that numbers out of range fail at the script line
// that passed them, not when rendering.
func TestNumberRange(t *testing.T) {
	for _, expr := range []string{
		`Frame(int("1" + "0" * 400), 10)`,
		`Frame(float("inf"), 10)`,
		`Frame(float("nan"), 10)`,
		`VStack(Gap(-1))`,
		`Spacer(40000)`,
		`Text("x", Style(size=1e300))`,
		`Grid(Columns(GridFixed(99999)))`,
	} {
		src := "def render(data):\n    x = 1\n    return " + expr + "\n"
		_, err := run(t, &starlarkplugin.Environment{}, testCfg(), src, nil)
		if err == nil || !strings.Contains(err.Error(), "out of range") || !strings.Contains(err.Error(), "t.star:3") {
			t.Errorf("%s: err = %v, want range error at t.star:3", expr, err)
		}
	}
	if _, err := run(t, &starlarkplugin.Environment{}, testCfg(), "def render(data): return Frame(32768, 0)\n", nil); err != nil {
		t.Errorf("at the limit: %v", err)
	}
}

// TestStepLimitDoesNotBoundMemory documents that MaxSteps limits execution
// steps, not memory: one step allocates 512 MiB and the render succeeds.
// Untrusted scripts need memory limits from outside the process.
func TestStepLimitDoesNotBoundMemory(t *testing.T) {
	env := starlarkplugin.Environment{MaxSteps: 1000}
	src := "def render(data):\n    s = \"x\" * (1 << 29)\n    return Text(str(len(s)))\n"
	p, err := env.LoadBytes(t.Context(), "mem.star", []byte(src), demy.TRMNLOriginal, nil)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	n, err := p.Render(t.Context(), nil)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("render failed: %v; if memory is now limited, update the docs", err)
	}
	if got := n.(*demy.TextNode).Text; got != "536870912" { //nolint:forcetypeassert // the script returns Text
		t.Errorf("text = %q, want 536870912", got)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc < 1<<29 {
		t.Errorf("allocated %d bytes, want at least 512 MiB", alloc)
	}
}
