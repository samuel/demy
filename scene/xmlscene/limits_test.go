package xmlscene_test

import (
	"strings"
	"testing"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
)

func TestUnmarshalDepthLimit(t *testing.T) {
	src := strings.Repeat("<vstack>", 2000) + strings.Repeat("</vstack>", 2000)
	if _, err := xmlscene.Unmarshal([]byte(src), demy.Config{}); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Fatalf("err = %v, want nesting error", err)
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
