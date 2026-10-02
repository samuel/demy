package demy

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestStyleOverridesBackToDefaults(t *testing.T) {
	cfg := &Config{
		Fonts:            map[string]*FontFamily{"sans": MustLoadFontBytes("sans", goregular.TTF)},
		DefaultTextStyle: Style{Font: "sans", Size: 12},
		Styles: map[string]Style{
			"h": {Weight: FontBold, Align: TextAlignCenter, MaxLines: 2, Overflow: TextOverflowEllipsis},
		},
	}
	n, err := buildNode(Text("x", Class("h"), Style{
		Weight:   FontRegular,
		Align:    TextAlignLeft,
		MaxLines: MaxLinesUnlimited,
		Overflow: TextOverflowWrap,
	}), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := n.(*textNode).style //nolint:forcetypeassert // test
	if got.Weight != FontRegular || got.Align != TextAlignLeft || got.MaxLines != 0 || got.Overflow != TextOverflowWrap {
		t.Errorf("style = %+v, want class values overridden", got)
	}

	n, err = buildNode(Text("x", Class("h")), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := n.(*textNode).style; got.Weight != FontBold || got.MaxLines != 2 { //nolint:forcetypeassert // test
		t.Errorf("inherited style = %+v", got)
	}
}

func TestFontStyleCV(t *testing.T) {
	tests := map[FontStyle]FontStyle{
		FontInherit:           0,
		FontRegular:           0,
		FontItalic:            FontItalic,
		FontBold:              FontBold - FontRegular,
		FontBold | FontItalic: FontBold - FontRegular | FontItalic,
	}
	for in, want := range tests {
		if got := in.cvStyle(); int(got) != int(want) {
			t.Errorf("%v.cvStyle() = %v, want %v", in, got, want)
		}
	}
}
