package xmlscene_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
)

func TestUnmarshalDepthLimit(t *testing.T) {
	src := strings.Repeat("<vstack>", 2000) + strings.Repeat("</vstack>", 2000)
	if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("err = %v, want nesting error", err)
	}
	src = strings.Repeat("<vstack>", 1000) + strings.Repeat("</vstack>", 1000)
	if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err != nil {
		t.Fatalf("1000 levels: %v", err)
	}
}

// TestUnmarshalNodeLimit checks that a scene with 10M elements fails once it
// passes the node limit rather than decoding them all.
func TestUnmarshalNodeLimit(t *testing.T) {
	src := "<vstack>" + strings.Repeat("<spacer/>", 10_000_000) + "</vstack>"
	start := time.Now()
	if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err == nil || !strings.Contains(err.Error(), "node elements") {
		t.Fatalf("err = %v, want node limit error", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("unmarshal took %v, want well under a second", d)
	}
	src = "<vstack>" + strings.Repeat("<spacer/>", 1<<14-1) + "</vstack>"
	if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
}

func TestUnmarshalNumbers(t *testing.T) {
	tests := map[string]float64{
		"4":     4,
		"+4":    4,
		"-0":    0,
		"4.5":   4.5,
		".5":    0.5,
		"1e2":   100,
		"1E2px": 100,
		"2.5px": 2.5,
		"007":   7,
	}
	for in, want := range tests {
		n, err := xmlscene.Unmarshal([]byte(`<frame width="`+in+`"/>`), demy.Config{})
		if err != nil {
			t.Errorf("width=%q: %v", in, err)
			continue
		}
		if got := n.(*demy.FrameNode).Width; got != want { //nolint:forcetypeassert // test
			t.Errorf("width=%q = %v, want %v", in, got, want)
		}
	}
}

func FuzzUnmarshalRender(f *testing.F) {
	f.Add(`<vstack gap="4"><text>hi</text><frame width="10" height="10"/></vstack>`)
	f.Add(`<grid><columns><fr weight="1"/><fixed size="20"/></columns><text>a</text><row><text>b</text></row></grid>`)
	f.Add(`<box clip="true" padding="2" border-width="1"><hstack><spacer/><divider/></hstack></box>`)
	cfg := demy.Config{PixelWidth: 64, PixelHeight: 48, PhysicalWidthMM: 64, PhysicalHeightMM: 48}
	canvas := demy.MustNew(cfg)
	f.Fuzz(func(t *testing.T, src string) {
		n, err := xmlscene.Unmarshal([]byte(src), cfg)
		if err != nil {
			return
		}
		_, _ = canvas.Render(t.Context(), n, demy.Grayscale)
	})
}

func TestExecuteOutputLimit(t *testing.T) {
	tmpl := "<vstack>{{range 10000}}" + strings.Repeat("<spacer/>", 100) + "{{end}}</vstack>"
	if _, err := xmlscene.Execute(t.Context(), []byte(tmpl), nil, demy.Config{}); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("err = %v, want output limit error", err)
	}
}

func TestExecuteCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := xmlscene.Execute(ctx, []byte(`<text>hi</text>`), nil, demy.Config{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestExecuteDeadline checks that templates that run for a long time without
// writing output stop at the deadline.
func TestExecuteDeadline(t *testing.T) {
	tests := map[string]struct {
		tmpl string
		data any
	}{
		"silent range": {`{{range 1000000000000}}{{end}}<spacer/>`, nil},
		// Each call makes two more, 2^40 in all.
		"recursion": {`{{define "a"}}{{if .}}{{template "a" (slice . 1)}}{{template "a" (slice . 1)}}{{end}}{{end}}{{template "a" .}}<spacer/>`, make([]int, 40)},
	}
	for name, tt := range tests {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		start := time.Now()
		_, err := xmlscene.Execute(ctx, []byte(tt.tmpl), tt.data, demy.Config{})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%s: err = %v, want context.DeadlineExceeded", name, err)
		}
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Errorf("%s: took %v after a 20ms deadline", name, d)
		}
	}
}
