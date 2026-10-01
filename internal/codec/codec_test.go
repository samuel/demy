package codec

import (
	"image/color"
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
