package demy

import "testing"

// testBDF is a tiny 8px font with three glyphs: space (32), a baseline letter
// 'A' (65), and a descender 'g' (103) whose bitmap drops below the baseline.
const testBDF = `STARTFONT 2.1
FONT test
SIZE 8 75 75
FONTBOUNDINGBOX 8 8 0 -2
STARTPROPERTIES 3
FONT_ASCENT 6
FONT_DESCENT 2
DEFAULT_CHAR 65
ENDPROPERTIES
CHARS 3
STARTCHAR space
ENCODING 32
SWIDTH 480 0
DWIDTH 8 0
BBX 0 0 0 0
BITMAP
ENDCHAR
STARTCHAR A
ENCODING 65
SWIDTH 480 0
DWIDTH 8 0
BBX 8 6 0 0
BITMAP
18
24
42
7E
42
42
ENDCHAR
STARTCHAR g
ENCODING 103
SWIDTH 480 0
DWIDTH 8 0
BBX 8 8 0 -2
BITMAP
3C
42
42
3C
04
38
00
00
ENDCHAR
ENDFONT
`

func TestParseBDFMetrics(t *testing.T) {
	f, err := parseBDF([]byte(testBDF))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	if f.ascent != 6 || f.descent != 2 {
		t.Errorf("ascent/descent = %d/%d, want 6/2", f.ascent, f.descent)
	}
	if f.lineHeight != 8 || f.pixelSize != 8 {
		t.Errorf("lineHeight/pixelSize = %d/%d, want 8/8", f.lineHeight, f.pixelSize)
	}
	if f.defaultChar != 65 {
		t.Errorf("defaultChar = %d, want 65", f.defaultChar)
	}
	if f.spaceAdvance != 8 {
		t.Errorf("spaceAdvance = %d, want 8", f.spaceAdvance)
	}
}

func TestParseBDFGlyphBitmap(t *testing.T) {
	f, err := parseBDF([]byte(testBDF))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	a, ok := f.glyph('A')
	if !ok {
		t.Fatal("glyph 'A' not found")
	}
	if a.width != 8 || a.height != 6 || a.xOffset != 0 || a.yOffset != 0 || a.advance != 8 {
		t.Errorf("glyph A metrics = %+v, want w8 h6 x0 y0 adv8", a)
	}
	// Row 0 is 0x18 = 0b00011000 -> columns 3 and 4 set, 0 and 7 clear.
	if !a.pixelSet(3, 0) || !a.pixelSet(4, 0) {
		t.Error("expected A row 0 cols 3,4 set")
	}
	if a.pixelSet(0, 0) || a.pixelSet(7, 0) {
		t.Error("expected A row 0 cols 0,7 clear")
	}
}

func TestParseBDFDescenderOffset(t *testing.T) {
	f, err := parseBDF([]byte(testBDF))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	g, ok := f.glyph('g')
	if !ok {
		t.Fatal("glyph 'g' not found")
	}
	if g.height != 8 || g.yOffset != -2 {
		t.Errorf("glyph g = h%d y%d, want h8 y-2", g.height, g.yOffset)
	}
	// Bitmap text places the glyph top at ascent - height - yOffset below the
	// line top.
	top := f.ascent - g.height - g.yOffset
	if top != 0 {
		t.Errorf("descender top = %d, want 0", top)
	}
	// The bitmap bottom sits descent pixels below the baseline.
	bottom := top + g.height
	if bottom != f.ascent+f.descent {
		t.Errorf("descender bottom = %d, want %d", bottom, f.ascent+f.descent)
	}
}

func TestParseBDFSpaceGlyph(t *testing.T) {
	f, err := parseBDF([]byte(testBDF))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	sp, ok := f.glyph(' ')
	if !ok {
		t.Fatal("glyph space not found")
	}
	if sp.width != 0 || sp.advance != 8 || len(sp.bits) != 0 {
		t.Errorf("space glyph = w%d adv%d bits%d, want w0 adv8 bits0", sp.width, sp.advance, len(sp.bits))
	}
}

func TestBitmapFontGlyphFallback(t *testing.T) {
	f, err := parseBDF([]byte(testBDF))
	if err != nil {
		t.Fatalf("parseBDF: %v", err)
	}
	// 'Z' is absent; DEFAULT_CHAR is 'A', so glyph() returns the 'A' bitmap.
	z, ok := f.glyph('Z')
	if !ok {
		t.Fatal("expected fallback glyph for 'Z'")
	}
	a, _ := f.glyph('A')
	if z.width != a.width || z.height != a.height {
		t.Errorf("fallback glyph = w%d h%d, want A's w%d h%d", z.width, z.height, a.width, a.height)
	}
}
