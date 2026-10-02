package demy

import (
	"bytes"
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestSanitizeText(t *testing.T) {
	tests := map[string]string{
		"plain":        "plain",
		"a\xffb":       "a�b",
		"\xe2\x82":     "��",
		"a\x00b\x01c":  "a�b�c",
		"a\x7fb":       "a�b",
		"a\u0085b":     "a�b",
		"a\tb\nc\r\nd": "a\tb\nc\r\nd",
		"a�b":          "a�b",
		"日本 語":         "日本 語",
	}
	for in, want := range tests {
		if got := sanitizeText(in); got != want {
			t.Errorf("sanitizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderInvalidText(t *testing.T) {
	fonts := map[string]*FontFamily{
		"sans": MustLoadFontBytes("sans", goregular.TTF),
		"px":   MustLoadBDFBytes("px", []byte(testBDF)),
	}
	render := func(t *testing.T, font, s string) []byte {
		t.Helper()
		cfg := Config{
			PixelWidth: 120, PixelHeight: 30, PhysicalWidthMM: 120, PhysicalHeightMM: 30,
			Fonts: fonts, DefaultTextStyle: Style{Font: font, Size: 14},
		}
		img, err := MustNew(cfg).Render(t.Context(), Text(s), Grayscale)
		if err != nil {
			t.Fatalf("Render(%q): %v", s, err)
		}
		return img.(*image.Gray).Pix //nolint:forcetypeassert // Grayscale renders *image.Gray
	}
	for _, font := range []string{"sans", "px"} {
		for _, s := range []string{"a\xffb", "\xe2\x82", "a\x01b", "a\u0085b"} {
			t.Run(font+" "+s, func(t *testing.T) {
				if !bytes.Equal(render(t, font, s), render(t, font, sanitizeText(s))) {
					t.Errorf("%q renders differently from %q", s, sanitizeText(s))
				}
			})
		}
	}
}
