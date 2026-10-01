package demy

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func testBitmapFont(t *testing.T) *bitmapFont {
	t.Helper()
	bf, err := parseBDF([]byte(testBDF))
	if err != nil {
		t.Fatal(err)
	}
	return bf
}

func TestBitmapEllipsisFits(t *testing.T) {
	bf := testBitmapFont(t)
	lines := layoutBitmapText(bf, "AAAA AAAA AAAA", 40, Style{MaxLines: 1, Overflow: TextOverflowEllipsis})
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	if w := bmpLineWidth(bf, lines[0]); w > 40 {
		t.Errorf("line %q width %d exceeds 40", string(lines[0]), w)
	}
	if !strings.HasSuffix(string(lines[0]), "...") {
		t.Errorf("line %q missing ellipsis", string(lines[0]))
	}
}

func TestBitmapLongWordBreaks(t *testing.T) {
	bf := testBitmapFont(t)
	lines := layoutBitmapText(bf, strings.Repeat("A", 10), 40, Style{})
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for _, l := range lines {
		if w := bmpLineWidth(bf, l); w > 40 {
			t.Errorf("line %q width %d exceeds 40", string(l), w)
		}
	}
}

func TestBitmapCRLF(t *testing.T) {
	bf := testBitmapFont(t)
	lines := layoutBitmapText(bf, "A\r\nA\rA", 0, Style{})
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	for _, l := range lines {
		if string(l) != "A" {
			t.Errorf("line %q, want %q", string(l), "A")
		}
	}
}

func TestBitmapFractionalScaleMeasure(t *testing.T) {
	ff := &FontFamily{bitmap: map[FontStyle]*bitmapFont{FontRegular: testBitmapFont(t)}}
	n := &textNode{text: "AA", style: Style{Font: "bdf"}}
	mctx := &measureCtx{fonts: map[string]*FontFamily{"bdf": ff}, scale: 1.5, availW: unbounded, availH: unbounded}
	got := n.measure(mctx)
	// Two 8px glyphs drawn at 2× physical are 32 physical px, 32/1.5 logical.
	if want := 32 / 1.5; got.w != want {
		t.Errorf("width = %v, want %v", got.w, want)
	}
}

func TestVectorEllipsisTextBox(t *testing.T) {
	ff := MustLoadFontBytes("sans", goregular.TTF)
	n := &textNode{
		text:  strings.Repeat("word ", 40),
		style: Style{Font: "sans", Size: 12, MaxLines: 2, Overflow: TextOverflowEllipsis},
	}
	tb := n.textBox(ff.family.Face(12, Black), 30)
	if tb.OverflowsY {
		t.Error("ellipsized text still overflows")
	}
	if !strings.HasSuffix(tb.Text, "…") {
		t.Errorf("text %q missing ellipsis", tb.Text)
	}
	if tb.Lines() != 2 {
		t.Errorf("lines = %d, want 2", tb.Lines())
	}
}
