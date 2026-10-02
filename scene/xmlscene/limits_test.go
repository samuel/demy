package xmlscene_test

import (
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
		_, _ = canvas.Render(n, demy.Grayscale)
	})
}
