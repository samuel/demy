package demy

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
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
		"huge dwidth": `STARTCHAR A
ENCODING 65
DWIDTH 999999999 0
BBX 8 1 0 0
BITMAP
FF
ENDCHAR
`,
		"negative dwidth": `STARTCHAR A
ENCODING 65
DWIDTH -8 0
BBX 8 1 0 0
BITMAP
FF
ENDCHAR
`,
		"huge bbx offset": `STARTCHAR A
ENCODING 65
BBX 8 1 -999999999 0
BITMAP
FF
ENDCHAR
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

func TestParseBDFMetricsOutOfRange(t *testing.T) {
	glyph := "STARTCHAR A\nENCODING 65\nBBX 8 1 0 0\nBITMAP\nFF\nENDCHAR\n"
	for _, header := range []string{
		"STARTFONT 2.1\nFONTBOUNDINGBOX 8 999999999 0 -2\n",
		"STARTFONT 2.1\nFONTBOUNDINGBOX 8 8 0 -999999999\n",
		"STARTFONT 2.1\nFONT_ASCENT 999999999\nFONT_DESCENT -999999990\n",
	} {
		if _, err := parseBDF([]byte(header + glyph)); err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Errorf("%q: err = %v, want range error", header, err)
		}
	}
}

// TestParseBDFBitmapAmplification checks that a small font declaring large,
// empty glyphs, whose masks are padded to their bounding boxes, is rejected
// instead of taking far more memory than its size.
func TestParseBDFBitmapAmplification(t *testing.T) {
	var b strings.Builder
	b.WriteString(bdfHeader)
	// Each 4096×4096 glyph pads to 2 MiB.
	for i := range maxBDFBitmapBytes/(maxBDFGlyphSize*maxBDFGlyphSize/8) + 1 {
		fmt.Fprintf(&b, "STARTCHAR g\nENCODING %d\nBBX %d %d 0 0\nBITMAP\nENDCHAR\n", 65+i, maxBDFGlyphSize, maxBDFGlyphSize)
	}
	if b.Len() > 1024 {
		t.Fatalf("font is %d bytes, want a small font", b.Len())
	}
	if _, err := parseBDF([]byte(b.String())); err == nil || !strings.Contains(err.Error(), "glyph bitmaps") {
		t.Fatalf("err = %v, want bitmap size error", err)
	}
}

// TestBlitCoverageLimit checks that text in a font whose glyphs are large but
// don't advance, so they stack on the canvas, fails instead of blitting every
// glyph.
func TestBlitCoverageLimit(t *testing.T) {
	// The glyph's top is at the top of the line, so it covers the canvas.
	ff := MustLoadBitmapFontBDF("b", []byte(bdfHeader+
		"STARTCHAR A\nENCODING 65\nDWIDTH 0 0\nBBX 64 64 0 -58\nBITMAP\nENDCHAR\n"))
	cfg := Config{
		PixelWidth: 64, PixelHeight: 64, PhysicalWidthMM: 64, PhysicalHeightMM: 64,
		Fonts: map[string]*FontFamily{"b": ff},
	}
	c := MustNew(cfg)
	if _, err := c.Render(t.Context(), Text(strings.Repeat("A", maxBlitCoverage), Style{Font: "b"}), Grayscale); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	_, err := c.Render(t.Context(), Text(strings.Repeat("A", maxBlitCoverage+1), Style{Font: "b"}), Grayscale)
	if err == nil || !strings.Contains(err.Error(), "canvas area") {
		t.Fatalf("over the limit: err = %v, want coverage error", err)
	}
}

// TestBlitGlyphClipped checks that a glyph partly outside the image and the
// clip draws only its visible pixels, for each kind of destination image.
func TestBlitGlyphClipped(t *testing.T) {
	// A 2×2 glyph with its top-left and bottom-right pixels set, scaled 3×,
	// covering [-2, 4) on both axes, clipped to x < 3.
	b := textBlit{
		glyph: bitmapGlyph{width: 2, height: 2, bits: []byte{0x80, 0x40}},
		destX: -2, destY: -2, scale: 3,
		clip: image.Rect(0, 0, 3, 8),
	}
	want := func(x, y int) bool { return x < 3 && (x < 1) == (y < 1) && y < 4 }
	for _, dst := range []draw.Image{
		image.NewGray(image.Rect(0, 0, 8, 8)),
		image.NewRGBA(image.Rect(0, 0, 8, 8)),
		image.NewNRGBA(image.Rect(0, 0, 8, 8)),
	} {
		draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
		blitGlyph(dst, b)
		for y := range 8 {
			for x := range 8 {
				got := color.GrayModel.Convert(dst.At(x, y)).(color.Gray).Y == 0 //nolint:forcetypeassert // GrayModel returns color.Gray
				if got != want(x, y) {
					t.Errorf("%T: pixel (%d, %d) set = %v, want %v", dst, x, y, got, want(x, y))
				}
			}
		}
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
		_, _ = MustNew(cfg).Render(t.Context(), Text(b.String()+" A g", Style{Font: "bdf"}), Grayscale)
	})
}
