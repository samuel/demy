package starlarkplugin_test

import (
	"bytes"
	"context"
	"image/color"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/samuel/demy"
	starlarkplugin "github.com/samuel/demy/plugin/starlark"
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
	if _, err := demy.MustNew(cfg).Render(n, demy.Grayscale); err == nil || !strings.Contains(err.Error(), "nodes") {
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
