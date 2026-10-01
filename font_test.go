package demy

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestTextFontErrors(t *testing.T) {
	sans := MustLoadFontBytes("sans", goregular.TTF)
	px := MustLoadBitmapFontBDF("px", []byte(testBDF))
	fonts := map[string]*FontFamily{"sans": sans, "px": px, "zero": {}}
	tests := map[string]struct {
		def  Style
		node *TextNode
		want string // substring of the error; empty for success
	}{
		"no font":            {node: Text("a"), want: "has no font"},
		"unknown font":       {node: Text("a", Style{Font: "serif", Size: 12}), want: `"serif" not found`},
		"zero-value family":  {node: Text("a", Style{Font: "zero", Size: 12}), want: "not loaded"},
		"zero-value default": {def: Style{Font: "zero", Size: 12}, node: Text("a"), want: "not loaded"},
		"vector no size":     {node: Text("a", Style{Font: "sans"}), want: "has no size"},
		"vector default size": {
			def:  Style{Size: 12},
			node: Text("a", Style{Font: "sans"}),
		},
		"bitmap no size":   {node: Text("a", Style{Font: "px"})},
		"default font":     {def: Style{Font: "sans", Size: 12}, node: Text("a")},
		"empty text":       {node: Text(""), want: "has no font"},
		"class size":       {node: Text("a", Class("h"), Style{Font: "sans"})},
		"class zero font":  {node: Text("a", Class("z")), want: "not loaded"},
		"inline over zero": {def: Style{Font: "zero"}, node: Text("a", Style{Font: "sans", Size: 12})},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Config{
				PixelWidth: 40, PixelHeight: 40, PhysicalWidthMM: 40, PhysicalHeightMM: 40,
				Fonts:            fonts,
				DefaultTextStyle: tt.def,
				Styles:           map[string]Style{"h": {Size: 10}, "z": {Font: "zero", Size: 10}},
			}
			_, err := MustNew(cfg).Render(VStack(tt.node), Grayscale)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("Render: %v", err)
			case tt.want != "" && err == nil:
				t.Fatalf("Render: expected error containing %q", tt.want)
			case tt.want != "" && !strings.Contains(err.Error(), tt.want):
				t.Fatalf("Render error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestZeroValueFontFamilyAddStyle(t *testing.T) {
	var ff FontFamily
	if err := ff.AddStyle(gobold.TTF, FontBold); !errors.Is(err, errFontNotLoaded) {
		t.Errorf("AddStyle on zero value: err = %v, want errFontNotLoaded", err)
	}
	if err := ff.AddStyleBDF([]byte(testBDF), FontBold); !errors.Is(err, errFontNotLoaded) {
		t.Errorf("AddStyleBDF on zero value: err = %v, want errFontNotLoaded", err)
	}
}
