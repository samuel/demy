package demy

import (
	"math"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
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

func TestFaceSelection(t *testing.T) {
	ff := MustLoadFontBytes("sans", goregular.TTF)
	regular := ff.faces[FontRegular]
	if fs := ff.face(FontInherit); fs.face != regular || fs.fauxBold != 0 || fs.fauxItalic != 0 {
		t.Errorf("inherit: %+v, want plain regular", fs)
	}
	if fs := ff.face(FontBold | FontItalic); fs.face != regular || fs.fauxBold != 0.02 || fs.fauxItalic != 0.3 {
		t.Errorf("bold italic from regular: %+v, want faux bold and italic", fs)
	}
	if fs := ff.face(FontLight); fs.fauxBold != 0 {
		t.Errorf("light: faux bold %v, want none", fs.fauxBold)
	}
	if err := ff.AddStyle(gobold.TTF, FontBold); err != nil {
		t.Fatal(err)
	}
	if fs := ff.face(FontBold); fs.face != ff.faces[FontBold] || fs.fauxBold != 0 {
		t.Errorf("bold: %+v, want the bold face", fs)
	}
	if fs := ff.face(FontExtraBold); fs.face != ff.faces[FontBold] || math.Abs(fs.fauxBold-0.01) > 1e-12 {
		t.Errorf("extra bold: %+v, want the bold face with faux bold 0.01", fs)
	}
}
