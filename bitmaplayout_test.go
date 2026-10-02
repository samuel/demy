package demy

import (
	"math"
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

func TestVectorEllipsis(t *testing.T) {
	ff := MustLoadFontBytes("sans", goregular.TTF)
	st := Style{Font: "sans", Size: 12, MaxLines: 2, Overflow: TextOverflowEllipsis}
	lay := ff.layoutText(strings.Repeat("word ", 40), st, 16, 100)
	if len(lay.lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lay.lines))
	}
	ellipsis, _ := lay.face.face.NominalGlyph('…')
	last := lay.lines[1].glyphs
	if len(last) == 0 || last[len(last)-1].id != ellipsis {
		t.Error("last line doesn't end in an ellipsis")
	}
	for i, l := range lay.lines {
		if l.width > 100 {
			t.Errorf("line %d is %v wide, want at most 100", i, l.width)
		}
	}
}

// TestVectorTextLines checks line breaking at newlines and the height of the
// lines, which follows the font's ascent, descent, and line gap.
func TestVectorTextLines(t *testing.T) {
	ff := MustLoadFontBytes("sans", goregular.TTF)
	ascent, descent, gap := lineMetrics(ff.faces[FontRegular], 16)
	for _, tt := range []struct {
		text  string
		lines int
	}{
		{"", 0}, {"a", 1}, {"a\n", 2}, {"\n", 2}, {"a\n\nb", 3}, {"x\r\ny", 2}, {"x\ry", 2},
	} {
		lay := ff.layoutText(tt.text, Style{}, 16, unbounded)
		if len(lay.lines) != tt.lines {
			t.Errorf("%q: %d lines, want %d", tt.text, len(lay.lines), tt.lines)
			continue
		}
		var want float64
		if tt.lines > 0 {
			want = ascent + descent + float64(tt.lines-1)*(ascent+descent+gap)
		}
		if math.Abs(lay.height-want) > 1e-9 {
			t.Errorf("%q: height = %v, want %v", tt.text, lay.height, want)
		}
	}
	lay := ff.layoutText("one two three four five", Style{MaxLines: 2}, 16, 40)
	if len(lay.lines) != 2 {
		t.Errorf("MaxLines 2: %d lines", len(lay.lines))
	}
}
