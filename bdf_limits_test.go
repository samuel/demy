package demy

import (
	"strings"
	"testing"
)

const bdfHeader = `STARTFONT 2.1
FONTBOUNDINGBOX 8 8 0 -2
FONT_ASCENT 6
FONT_DESCENT 2
`

func TestParseBDFShortBitmapPadded(t *testing.T) {
	f, err := parseBDF([]byte(bdfHeader + `STARTCHAR A
ENCODING 65
DWIDTH 8 0
BBX 8 8 0 0
BITMAP
FF
FF
ENDCHAR
`))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	g := f.glyphs['A']
	if len(g.bits) != 8 {
		t.Fatalf("len(bits) = %d, want 8", len(g.bits))
	}
	if g.pixelSet(0, 7) {
		t.Error("padded row should be unset")
	}
}

func TestParseBDFExtraRowsIgnored(t *testing.T) {
	f, err := parseBDF([]byte(bdfHeader + `STARTCHAR A
ENCODING 65
DWIDTH 8 0
BBX 8 1 0 0
BITMAP
FF
FF
FF
ENDCHAR
`))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	if n := len(f.glyphs['A'].bits); n != 1 {
		t.Fatalf("len(bits) = %d, want 1", n)
	}
}

func TestParseBDFMissingDWidth(t *testing.T) {
	f, err := parseBDF([]byte(bdfHeader + `STARTCHAR A
ENCODING 65
BBX 5 1 0 0
BITMAP
F8
ENDCHAR
`))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	if a := f.glyphs['A'].advance; a != 5 {
		t.Errorf("advance = %d, want 5", a)
	}
}

func TestParseBDFInvalid(t *testing.T) {
	tests := map[string]string{
		"negative bbx": `STARTCHAR A
ENCODING 65
BBX -9 1 0 0
BITMAP
ENDCHAR
`,
		"huge bbx": `STARTCHAR A
ENCODING 65
BBX 999999999 1 0 0
BITMAP
ENDCHAR
`,
		"odd hex": `STARTCHAR A
ENCODING 65
BBX 8 1 0 0
BITMAP
F
ENDCHAR
`,
		"missing endchar": `STARTCHAR A
ENCODING 65
BBX 8 1 0 0
BITMAP
FF
`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseBDF([]byte(bdfHeader + body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseBDFEncodingOutOfRange(t *testing.T) {
	f, err := parseBDF([]byte(bdfHeader + `STARTCHAR A
ENCODING 65
BBX 0 0 0 0
BITMAP
ENDCHAR
STARTCHAR big
ENCODING 4294967361
BBX 0 0 0 0
BITMAP
ENDCHAR
`))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	if len(f.glyphs) != 1 {
		t.Errorf("glyphs = %d, want 1", len(f.glyphs))
	}
}

func TestParseBundledFonts(t *testing.T) {
	for _, path := range []string{
		"cmd/demy-tutorial/fonts/spleen-8x16.bdf",
		"cmd/demy-tutorial/fonts/tom-thumb.bdf",
	} {
		if _, err := LoadBitmapFontBDFFile("f", path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func FuzzParseBDF(f *testing.F) {
	f.Add([]byte(testBDF))
	f.Add([]byte(bdfHeader + "STARTCHAR A\nENCODING 65\nBBX 8 8 -2 -2\nBITMAP\nFF\nENDCHAR\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		ff, err := LoadBitmapFontBDF("bdf", data)
		if err != nil {
			return
		}
		cfg := Config{
			PixelWidth: 64, PixelHeight: 32, PhysicalWidthMM: 64, PhysicalHeightMM: 32,
			Fonts: map[string]*FontFamily{"bdf": ff},
		}
		var b strings.Builder
		for r := range ff.bitmap[FontRegular].glyphs {
			if r != 0 && b.Len() < 64 {
				b.WriteRune(r)
			}
		}
		_, _ = MustNew(cfg).Render(Text(b.String()+" A g", Style{Font: "bdf"}), Grayscale)
	})
}
