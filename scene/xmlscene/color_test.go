package xmlscene

import (
	"image/color"
	"testing"
)

func TestParseColorHex(t *testing.T) {
	tests := []struct {
		in   string
		want color.RGBA
	}{
		{"#fff", color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
		{"#1a2", color.RGBA{R: 0x11, G: 0xaa, B: 0x22, A: 0xff}},
		{"#123456", color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}},
	}
	for _, tt := range tests {
		got, err := parseColor(tt.in)
		if err != nil {
			t.Fatalf("parseColor(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("parseColor(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
