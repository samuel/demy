package codec

import (
	"reflect"
	"testing"
)

// FuzzColor checks that every color ParseColor accepts encodes to a string
// that parses back to the same value.
func FuzzColor(f *testing.F) {
	for _, s := range []string{"", "muted", "foreground", "#fff", "#1a2", "#123456", "#ff000080", "#ff0000ff", "#ABCDEF", "#00000000"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := ParseColor(s)
		if err != nil {
			return
		}
		enc, err := ColorString(c)
		if err != nil {
			t.Fatalf("ColorString(%#v) from %q: %v", c, s, err)
		}
		c2, err := ParseColor(enc)
		if err != nil {
			t.Fatalf("ParseColor(%q) from %q: %v", enc, s, err)
		}
		if !reflect.DeepEqual(c, c2) {
			t.Fatalf("%q -> %#v -> %q -> %#v", s, c, enc, c2)
		}
	})
}

// FuzzWeight checks that every weight ParseWeight accepts encodes to a string
// that parses back to the same value.
func FuzzWeight(f *testing.F) {
	for _, s := range []string{"", "regular", "bold", "italic", "bold-italic", "regular-italic", "extra-light-italic", "-italic"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		w, err := ParseWeight(s)
		if err != nil {
			return
		}
		enc, err := WeightString(w)
		if err != nil {
			t.Fatalf("WeightString(%v) from %q: %v", w, s, err)
		}
		w2, err := ParseWeight(enc)
		if err != nil {
			t.Fatalf("ParseWeight(%q) from %q: %v", enc, s, err)
		}
		if w != w2 {
			t.Fatalf("%q -> %v -> %q -> %v", s, w, enc, w2)
		}
	})
}
