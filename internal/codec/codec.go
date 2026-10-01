// Package codec holds the string encodings shared by the scene formats and the
// Starlark host: colors, enums, font weights, and grid tracks. Every parser
// rejects unknown values.
package codec

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"reflect"
	"slices"
	"strings"

	"github.com/samuel/demy"
)

// Roles lists the role color names accepted in place of hex colors.
var Roles = []string{"foreground", "background", "accent", "muted"}

// ColorString encodes c as a role name or a "#rrggbb" or "#rrggbbaa" hex string
// (straight alpha). A nil color encodes as "".
func ColorString(c color.Color) (string, error) {
	if c == nil {
		return "", nil
	}
	if rc, ok := c.(demy.RoleColor); ok {
		if !isRole(rc.Name) {
			return "", fmt.Errorf("unknown role color %q (want one of %s)", rc.Name, strings.Join(Roles, ", "))
		}
		return rc.Name, nil
	}
	n := color.NRGBAModel.Convert(c).(color.NRGBA) //nolint:forcetypeassert // NRGBAModel always returns color.NRGBA
	if n.A == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", n.R, n.G, n.B), nil
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", n.R, n.G, n.B, n.A), nil
}

// ParseColor decodes a role name or a "#rgb", "#rrggbb", or "#rrggbbaa" hex
// string. "" decodes as nil (the default color). Opaque colors decode as
// color.RGBA and translucent ones as color.NRGBA.
func ParseColor(s string) (color.Color, error) {
	if s == "" {
		return nil, nil
	}
	if isRole(s) {
		return demy.RoleColor{Name: s}, nil
	}
	h, ok := strings.CutPrefix(s, "#")
	if !ok {
		return nil, fmt.Errorf("unknown color %q (want a role name or #rrggbb)", s)
	}
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 3 && len(b) != 4) {
		return nil, fmt.Errorf("invalid hex color %q (want #rgb, #rrggbb, or #rrggbbaa)", s)
	}
	// Opaque colors decode as color.RGBA however they're written, so each
	// color has one decoded form.
	if len(b) == 3 || b[3] == 0xff {
		return color.RGBA{R: b[0], G: b[1], B: b[2], A: 0xff}, nil
	}
	return color.NRGBA{R: b[0], G: b[1], B: b[2], A: b[3]}, nil
}

func isRole(name string) bool {
	return slices.Contains(Roles, name)
}

// Enum maps the values of one enum type to their names. The default value
// encodes as "" and "" decodes to it.
type Enum[T comparable] struct {
	kind   string
	def    T
	values []T
	names  []string
}

// NewEnum returns an Enum whose default is def. pairs alternates values and
// their names.
func NewEnum[T comparable](kind string, def T, pairs ...any) Enum[T] {
	e := Enum[T]{kind: kind, def: def}
	for i := 0; i+1 < len(pairs); i += 2 {
		e.values = append(e.values, pairs[i].(T))      //nolint:forcetypeassert // fixed tables in this package
		e.names = append(e.names, pairs[i+1].(string)) //nolint:forcetypeassert // fixed tables in this package
	}
	return e
}

// Parse decodes s, returning an error for an unknown name.
func (e Enum[T]) Parse(s string) (T, error) {
	if s == "" {
		return e.def, nil
	}
	for i, n := range e.names {
		if n == s {
			return e.values[i], nil
		}
	}
	var zero T
	return zero, fmt.Errorf("unknown %s %q (want one of %s)", e.kind, s, strings.Join(e.names, ", "))
}

// String encodes v, returning "" for the default and an error for a value with
// no name.
func (e Enum[T]) String(v T) (string, error) {
	if v == e.def {
		return "", nil
	}
	for i, ev := range e.values {
		if ev == v {
			return e.names[i], nil
		}
	}
	return "", fmt.Errorf("invalid %s %v", e.kind, v)
}

var (
	Align = NewEnum("alignment", demy.AlignStart,
		demy.AlignStart, "start", demy.AlignCenter, "center", demy.AlignEnd, "end")
	ZAlign = NewEnum("z-alignment", demy.TopLeading,
		demy.TopLeading, "top-leading", demy.Top, "top", demy.TopTrailing, "top-trailing",
		demy.Leading, "leading", demy.Center, "center", demy.Trailing, "trailing",
		demy.BottomLeading, "bottom-leading", demy.Bottom, "bottom", demy.BottomTrailing, "bottom-trailing")
	TextAlign = NewEnum("text alignment", demy.TextAlignInherit,
		demy.TextAlignLeft, "left", demy.TextAlignCenter, "center", demy.TextAlignRight, "right")
	Overflow = NewEnum("overflow", demy.TextOverflowInherit,
		demy.TextOverflowWrap, "wrap", demy.TextOverflowClip, "clip", demy.TextOverflowEllipsis, "ellipsis")
	Dither = NewEnum("dither", demy.NoDithering,
		demy.FloydSteinberg, "floyd-steinberg", demy.Atkinson, "atkinson")
	ImageScale = NewEnum("image scale", demy.ImageFit,
		demy.ImageFit, "fit", demy.ImageFill, "fill", demy.ImageStretch, "stretch")
	BarcodeType = NewEnum("barcode type", demy.BarcodeTypeQR,
		demy.BarcodeTypeQR, "qr")
	// ECLevel encodes Medium, the Barcode constructor's default, as "".
	ECLevel = NewEnum("error correction level", demy.ECLevelMedium,
		demy.ECLevelLow, "low", demy.ECLevelMedium, "medium",
		demy.ECLevelQuartile, "quartile", demy.ECLevelHigh, "high")
	weight = NewEnum("font weight", demy.FontInherit,
		demy.FontRegular, "regular", demy.FontThin, "thin", demy.FontExtraLight, "extra-light",
		demy.FontLight, "light", demy.FontMedium, "medium", demy.FontSemiBold, "semi-bold",
		demy.FontBold, "bold", demy.FontExtraBold, "extra-bold", demy.FontBlack, "black")
)

// WeightString encodes a font style as a weight name with an optional
// "-italic" suffix, or "italic" alone for FontItalic. FontInherit encodes as "".
func WeightString(w demy.FontStyle) (string, error) {
	italic := w&demy.FontItalic != 0
	s, err := weight.String(w &^ demy.FontItalic)
	if err != nil {
		return "", err
	}
	switch {
	case !italic:
		return s, nil
	case s == "":
		return "italic", nil
	default:
		return s + "-italic", nil
	}
}

// ParseWeight decodes a name produced by WeightString.
func ParseWeight(s string) (demy.FontStyle, error) {
	if s == "italic" {
		return demy.FontItalic, nil
	}
	base, italic := strings.CutSuffix(s, "-italic")
	if italic && base == "" {
		return 0, fmt.Errorf("unknown font weight %q", s)
	}
	w, err := weight.Parse(base)
	if err != nil {
		return 0, err
	}
	if italic {
		w |= demy.FontItalic
	}
	return w, nil
}

// MaxDepth bounds node nesting in decoded scenes so deeply nested input can't
// exhaust the stack.
const MaxDepth = 1000

// Grid track kinds.
const (
	TrackAuto  = "auto"
	TrackFixed = "fixed"
	TrackFr    = "fr"
)

// TrackKind returns the kind name of t.
func TrackKind(t demy.GridTrack) string {
	switch {
	case t.IsFixed():
		return TrackFixed
	case t.IsFr():
		return TrackFr
	default:
		return TrackAuto
	}
}

// ParseTrack builds a track of the named kind. size applies to fixed tracks
// and weight to fr tracks.
func ParseTrack(kind string, size, weight float64) (demy.GridTrack, error) {
	switch kind {
	case TrackAuto:
		return demy.GridAuto(), nil
	case TrackFixed:
		return demy.GridFixed(size), nil
	case TrackFr:
		return demy.GridFr(weight), nil
	default:
		return demy.GridTrack{}, fmt.Errorf("unknown grid track kind %q (want auto, fixed, or fr)", kind)
	}
}

// IsNilNode reports whether n is nil or a typed nil pointer.
func IsNilNode(n demy.Node) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// ErrNilRoot reports a nil root node passed to a marshal function.
var ErrNilRoot = errors.New("root node is nil")

// CheckImage reports an error for an image that can't be serialized because
// it has no key.
func CheckImage(n *demy.ImageNode) error {
	if n.Key == "" && n.Image != nil {
		return errors.New("image has no key; set ImageNode.Key and Config.Images to serialize it")
	}
	return nil
}

// The Decode helpers below record the first error in *errp and return the
// decoded value, so a struct literal can decode several fields before one
// error check.

func keepFirst(errp *error, err error) {
	if *errp == nil && err != nil {
		*errp = err
	}
}

// Decode parses s, recording any error in *errp.
func (e Enum[T]) Decode(s string, errp *error) T {
	v, err := e.Parse(s)
	keepFirst(errp, err)
	return v
}

// DecodeColor parses s with ParseColor, recording any error in *errp.
func DecodeColor(s string, errp *error) color.Color {
	c, err := ParseColor(s)
	keepFirst(errp, err)
	return c
}

// DecodeWeight parses s with ParseWeight, recording any error in *errp.
func DecodeWeight(s string, errp *error) demy.FontStyle {
	w, err := ParseWeight(s)
	keepFirst(errp, err)
	return w
}

// Encode formats v, recording any error in *errp.
func (e Enum[T]) Encode(v T, errp *error) string {
	s, err := e.String(v)
	keepFirst(errp, err)
	return s
}

// EncodeColor formats c with ColorString, recording any error in *errp.
func EncodeColor(c color.Color, errp *error) string {
	s, err := ColorString(c)
	keepFirst(errp, err)
	return s
}

// EncodeWeight formats w with WeightString, recording any error in *errp.
func EncodeWeight(w demy.FontStyle, errp *error) string {
	s, err := WeightString(w)
	keepFirst(errp, err)
	return s
}
