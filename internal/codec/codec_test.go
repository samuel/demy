package codec

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/samuel/demy"
)

func TestParseColor(t *testing.T) {
	tests := []struct {
		in   string
		want color.Color
	}{
		{"", nil},
		{"muted", demy.RoleColor{Name: "muted"}},
		{"#fff", color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
		{"#1a2", color.RGBA{R: 0x11, G: 0xaa, B: 0x22, A: 0xff}},
		{"#123456", color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}},
		{"#ff000080", color.NRGBA{R: 0xff, A: 0x80}},
		{"#123456ff", color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}},
	}
	for _, tt := range tests {
		got, err := ParseColor(tt.in)
		if err != nil {
			t.Fatalf("ParseColor(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseColor(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
}

func TestParseColorInvalid(t *testing.T) {
	for _, in := range []string{"#zzz", "#12345g", "#12345", "red", "#"} {
		if _, err := ParseColor(in); err == nil {
			t.Errorf("ParseColor(%q): expected error", in)
		}
	}
}

func TestColorStringRoundTrip(t *testing.T) {
	for _, c := range []color.Color{
		color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff},
		color.NRGBA{R: 0xff, A: 0x80},
		demy.RoleColor{Name: "accent"},
	} {
		s, err := ColorString(c)
		if err != nil {
			t.Fatalf("ColorString(%v): %v", c, err)
		}
		got, err := ParseColor(s)
		if err != nil {
			t.Fatalf("ParseColor(%q): %v", s, err)
		}
		if got != c {
			t.Errorf("round trip %#v → %q → %#v", c, s, got)
		}
	}
	// Premultiplied half-transparent red encodes as straight alpha.
	if s, _ := ColorString(color.RGBA{R: 0x80, A: 0x80}); s != "#ff000080" {
		t.Errorf("ColorString(premultiplied) = %q, want #ff000080", s)
	}
}

func TestColorStringBadRole(t *testing.T) {
	for _, name := range []string{"", "primary"} {
		if _, err := ColorString(demy.RoleColor{Name: name}); err == nil {
			t.Errorf("ColorString(RoleColor{%q}): expected error", name)
		}
	}
}

func TestEnum(t *testing.T) {
	if v, err := Align.Parse(""); err != nil || v != demy.AlignStart {
		t.Errorf(`Parse("") = %v, %v`, v, err)
	}
	if v, err := Align.Parse("end"); err != nil || v != demy.AlignEnd {
		t.Errorf(`Parse("end") = %v, %v`, v, err)
	}
	if _, err := Align.Parse("right"); err == nil {
		t.Error(`Parse("right"): expected error`)
	}
	if s, err := Align.String(demy.AlignStart); err != nil || s != "" {
		t.Errorf("String(default) = %q, %v", s, err)
	}
	if _, err := Align.String(demy.Alignment(99)); err == nil {
		t.Error("String(99): expected error")
	}
	if s, _ := ECLevel.String(demy.ECLevelLow); s != "low" {
		t.Errorf("ECLevel low = %q", s)
	}
	if v, _ := ECLevel.Parse(""); v != demy.ECLevelMedium {
		t.Errorf(`ECLevel "" = %v, want medium`, v)
	}
}

func TestWeight(t *testing.T) {
	for _, w := range []demy.FontStyle{
		demy.FontInherit, demy.FontRegular, demy.FontBold,
		demy.FontItalic, demy.FontRegular | demy.FontItalic, demy.FontBold | demy.FontItalic,
	} {
		s, err := WeightString(w)
		if err != nil {
			t.Fatalf("WeightString(%v): %v", w, err)
		}
		got, err := ParseWeight(s)
		if err != nil || got != w {
			t.Errorf("round trip %v → %q → %v, %v", w, s, got, err)
		}
	}
	for _, s := range []string{"semibold", "-italic", "bold-oblique"} {
		if _, err := ParseWeight(s); err == nil {
			t.Errorf("ParseWeight(%q): expected error", s)
		}
	}
}

func TestParseTrack(t *testing.T) {
	if _, err := ParseTrack("px", 1, 0); err == nil {
		t.Error("expected error for unknown kind")
	}
	tr, err := ParseTrack(TrackFr, 0, 2)
	if err != nil || !tr.IsFr() || tr.Weight() != 2 {
		t.Errorf("ParseTrack(fr) = %v, %v", tr, err)
	}
}

// checkEnum checks that every value of e round-trips through String and Parse,
// that unknown names and values are errors, and that the default is "".
func checkEnum[T comparable](t *testing.T, e Enum[T], invalid T) {
	t.Helper()
	if len(e.values) == 0 || len(e.values) != len(e.names) {
		t.Fatalf("%s: %d values, %d names", e.kind, len(e.values), len(e.names))
	}
	for _, v := range e.values {
		s, err := e.String(v)
		if err != nil {
			t.Errorf("%s: String(%v): %v", e.kind, v, err)
			continue
		}
		if (s == "") != (v == e.def) {
			t.Errorf("%s: String(%v) = %q; only the default encodes as \"\"", e.kind, v, s)
		}
		if got, err := e.Parse(s); err != nil || got != v {
			t.Errorf("%s: round trip %v → %q → %v, %v", e.kind, v, s, got, err)
		}
	}
	for i, n := range e.names {
		if got, err := e.Parse(n); err != nil || got != e.values[i] {
			t.Errorf("%s: Parse(%q) = %v, %v, want %v", e.kind, n, got, err, e.values[i])
		}
	}
	if got, err := e.Parse(""); err != nil || got != e.def {
		t.Errorf(`%s: Parse("") = %v, %v, want %v`, e.kind, got, err, e.def)
	}
	for _, s := range []string{"bogus", " " + e.names[0], strings.ToUpper(e.names[0])} {
		if _, err := e.Parse(s); err == nil || !strings.Contains(err.Error(), e.kind) {
			t.Errorf("%s: Parse(%q): err = %v, want an unknown %s error", e.kind, s, err, e.kind)
		}
	}
	if _, err := e.String(invalid); err == nil {
		t.Errorf("%s: String(%v): expected error", e.kind, invalid)
	}
}

func TestEnumTables(t *testing.T) {
	t.Run("Align", func(t *testing.T) { checkEnum(t, Align, demy.Alignment(99)) })
	t.Run("ZAlign", func(t *testing.T) { checkEnum(t, ZAlign, demy.ZAlignment(99)) })
	t.Run("TextAlign", func(t *testing.T) { checkEnum(t, TextAlign, demy.TextAlign(99)) })
	t.Run("Overflow", func(t *testing.T) { checkEnum(t, Overflow, demy.TextOverflow(99)) })
	t.Run("Dither", func(t *testing.T) { checkEnum(t, Dither, demy.Dither(99)) })
	t.Run("ImageScale", func(t *testing.T) { checkEnum(t, ImageScale, demy.ImageScale(99)) })
	t.Run("BarcodeType", func(t *testing.T) { checkEnum(t, BarcodeType, demy.BarcodeType(99)) })
	t.Run("ECLevel", func(t *testing.T) { checkEnum(t, ECLevel, demy.ECLevel(99)) })
	t.Run("weight", func(t *testing.T) { checkEnum(t, weight, demy.FontStyle(99)) })
}

func TestWeightStringInvalid(t *testing.T) {
	for _, w := range []demy.FontStyle{99, 99 | demy.FontItalic} {
		if _, err := WeightString(w); err == nil {
			t.Errorf("WeightString(%v): expected error", w)
		}
	}
}

func TestTrackRoundTrip(t *testing.T) {
	for _, tr := range []demy.GridTrack{demy.GridAuto(), demy.GridFixed(120), demy.GridFr(2.5)} {
		got, err := ParseTrack(TrackKind(tr), tr.Size(), tr.Weight())
		if err != nil {
			t.Fatalf("ParseTrack(%q): %v", TrackKind(tr), err)
		}
		if got != tr {
			t.Errorf("round trip %+v → %q → %+v", tr, TrackKind(tr), got)
		}
	}
	for kind, want := range map[demy.GridTrack]string{demy.GridAuto(): TrackAuto, demy.GridFixed(1): TrackFixed, demy.GridFr(1): TrackFr} {
		if got := TrackKind(kind); got != want {
			t.Errorf("TrackKind(%+v) = %q, want %q", kind, got, want)
		}
	}
	if _, err := ParseTrack("", 0, 0); err == nil {
		t.Error(`ParseTrack(""): expected error`)
	}
}

func TestIsNilNode(t *testing.T) {
	tests := []struct {
		n    demy.Node
		want bool
	}{
		{nil, true},
		{(*demy.TextNode)(nil), true},
		{(*demy.VStackNode)(nil), true},
		{demy.Text("x"), false},
		{demy.VStack(), false},
	}
	for _, tt := range tests {
		if got := IsNilNode(tt.n); got != tt.want {
			t.Errorf("IsNilNode(%#v) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

func TestCheckImage(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 1, 1))
	tests := map[string]struct {
		n       *demy.ImageNode
		wantErr bool
	}{
		"key":           {demy.ImageKey("logo"), false},
		"key and image": {&demy.ImageNode{Key: "logo", Image: img}, false},
		"image only":    {demy.Image(img), true},
		"neither":       {&demy.ImageNode{}, false},
	}
	for name, tt := range tests {
		if err := CheckImage(tt.n); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, want error %v", name, err, tt.wantErr)
		}
	}
}

// TestDecodeEncodeKeepFirst checks that the Decode and Encode helpers return
// each value and keep only the first error.
func TestDecodeEncodeKeepFirst(t *testing.T) {
	var err error
	if v := Align.Decode("center", &err); v != demy.AlignCenter || err != nil {
		t.Errorf("Decode(center) = %v, %v", v, err)
	}
	if c := DecodeColor("#000", &err); c != (color.RGBA{A: 0xff}) || err != nil {
		t.Errorf("DecodeColor(#000) = %v, %v", c, err)
	}
	if w := DecodeWeight("bold", &err); w != demy.FontBold || err != nil {
		t.Errorf("DecodeWeight(bold) = %v, %v", w, err)
	}
	Align.Decode("left", &err)
	first := err
	if first == nil || !strings.Contains(first.Error(), `alignment "left"`) {
		t.Fatalf("err = %v, want an unknown alignment error", first)
	}
	DecodeColor("red", &err)
	DecodeWeight("heavy", &err)
	if err != first {
		t.Errorf("err = %v, want the first error %v", err, first)
	}

	err = nil
	if s := Align.Encode(demy.AlignEnd, &err); s != "end" || err != nil {
		t.Errorf("Encode(end) = %q, %v", s, err)
	}
	if s := EncodeColor(demy.RoleColor{Name: "muted"}, &err); s != "muted" || err != nil {
		t.Errorf("EncodeColor(muted) = %q, %v", s, err)
	}
	if s := EncodeWeight(demy.FontBold|demy.FontItalic, &err); s != "bold-italic" || err != nil {
		t.Errorf("EncodeWeight(bold italic) = %q, %v", s, err)
	}
	Align.Encode(demy.Alignment(99), &err)
	first = err
	if first == nil {
		t.Fatal("Encode(99): expected error")
	}
	EncodeColor(demy.RoleColor{Name: "primary"}, &err)
	EncodeWeight(99, &err)
	if err != first {
		t.Errorf("err = %v, want the first error %v", err, first)
	}
}
