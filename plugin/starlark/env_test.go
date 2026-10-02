package starlarkplugin_test

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	sl "go.starlark.net/starlark"

	"github.com/samuel/demy"
	starlarkplugin "github.com/samuel/demy/plugin/starlark"
)

func TestStylesReturnsCopy(t *testing.T) {
	src := "styles = {\"h\": Style(size=20)}\ndef render(d): return Text(\"\")\n"
	p, err := (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte(src), testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := p.Styles()
	s["h"] = demy.Style{Size: 1}
	s["new"] = demy.Style{}
	got := p.Styles()
	if len(got) != 1 || got["h"].Size != 20 {
		t.Errorf("Styles() = %v after modifying an earlier result", got)
	}

	p, err = (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte("def render(d): return Text(\"\")\n"), testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := p.Styles(); s != nil {
		t.Errorf("Styles() = %v, want nil", s)
	}
}

func TestPluginKeepsEnvironmentFromLoad(t *testing.T) {
	var loadLog, laterLog bytes.Buffer
	loadTime := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	env := &starlarkplugin.Environment{
		Logger: slog.New(slog.NewTextHandler(&loadLog, nil)),
		Now:    func() time.Time { return loadTime },
	}
	env.AddValue("label", sl.String("before"))
	src := `
def render(d):
    print("rendered")
    n = 0
    for i in range(100):
        n += i
    return Text(label + " " + now().format("2006-01-02"))
`
	p, err := env.LoadBytes(t.Context(), "t.star", []byte(src), testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}

	env.MaxSteps = 1
	env.Logger = slog.New(slog.NewTextHandler(&laterLog, nil))
	env.Now = func() time.Time { return loadTime.AddDate(1, 0, 0) }
	env.AddValue("label", sl.String("after"))

	n, err := p.Render(t.Context(), nil)
	if err != nil {
		t.Fatalf("Render after changing the Environment: %v", err)
	}
	if got := textOf(t, n); got != "before 2026-01-02" {
		t.Errorf("text = %q, want the label and time from Load", got)
	}
	if !strings.Contains(loadLog.String(), "rendered") || laterLog.Len() != 0 {
		t.Errorf("print went to the wrong logger: load %q, later %q", loadLog.String(), laterLog.String())
	}
}

func TestRenderConcurrent(t *testing.T) {
	src := `
rows = [Text("row %d" % i) for i in range(10)]

def render(d):
    return VStack(Text(d["title"]), Children(rows), HStack(Spacer(), Text("x")))
`
	p, err := (&starlarkplugin.Environment{}).LoadBytes(t.Context(), "t.star", []byte(src), testCfg(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := demy.MustNew(testCfg())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 5 {
				n, err := p.Render(t.Context(), map[string]any{"title": "t"})
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := c.Render(t.Context(), n, demy.Paletted); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}
