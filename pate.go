// Package pate provides a composable rendering context for e-ink and
// other more static displays, built on tdewolff/canvas with a
// SwiftUI-inspired layout model.
package pate

import (
	"image/color"

	cv "github.com/tdewolff/canvas"
)

// Color constants for display palettes.
var (
	Black     = color.Gray{Y: 0}
	DarkGray  = color.Gray{Y: 85}
	LightGray = color.Gray{Y: 170}
	White     = color.Gray{Y: 255}
	Red       = color.RGBA{R: 255, A: 255}
	Yellow    = color.RGBA{R: 255, G: 255, A: 255}
)

// Roles maps semantic color names to concrete display colors.
type Roles struct {
	Foreground color.Color
	Background color.Color
	Accent     color.Color
	Muted      color.Color
}

// FontStyle specifies a font weight and optional italic flag.
// Weights can be OR'd with FontItalic, e.g. FontBold|FontItalic.
type FontStyle int

const (
	FontRegular    FontStyle = iota // weight 400
	FontThin                        // weight 100
	FontExtraLight                  // weight 200
	FontLight                       // weight 300
	FontMedium                      // weight 500
	FontSemiBold                    // weight 600
	FontBold                        // weight 700
	FontExtraBold                   // weight 800
	FontBlack                       // weight 900

	FontItalic FontStyle = 1 << 8 // OR with a weight constant
)

// FontFamily wraps a canvas font family. Callers never use canvas.FontFamily directly.
type FontFamily struct {
	family *cv.FontFamily
}

// LoadFontFile loads a TTF or OTF font file as the Regular style of a new family.
// name identifies the family within pate (used as the key in Config.Fonts).
func LoadFontFile(name, path string) (*FontFamily, error) {
	ff := cv.NewFontFamily(name)
	if err := ff.LoadFontFile(path, cv.FontRegular); err != nil {
		return nil, err
	}
	return &FontFamily{family: ff}, nil
}

// MustLoadFontFile loads a font file and panics on error.
func MustLoadFontFile(name, path string) *FontFamily {
	f, err := LoadFontFile(name, path)
	if err != nil {
		panic(err)
	}
	return f
}

// LoadFontBytes loads a font from raw TrueType or OpenType bytes as the Regular
// style of a new family. Use index 0 for single-font files.
// This accepts data from golang.org/x/image/font/gofont packages directly,
// e.g. LoadFontBytes("go", goregular.TTF).
func LoadFontBytes(name string, data []byte) (*FontFamily, error) {
	ff := cv.NewFontFamily(name)
	if err := ff.LoadFont(data, 0, cv.FontRegular); err != nil {
		return nil, err
	}
	return &FontFamily{family: ff}, nil
}

// MustLoadFontBytes loads a font from bytes and panics on error.
func MustLoadFontBytes(name string, data []byte) *FontFamily {
	f, err := LoadFontBytes(name, data)
	if err != nil {
		panic(err)
	}
	return f
}

// AddStyle loads an additional weight/style variant (e.g. FontBold, FontItalic,
// FontBold|FontItalic) into an existing FontFamily from raw font bytes.
// Use index 0 for single-font files.
func (ff *FontFamily) AddStyle(data []byte, style FontStyle) error {
	return ff.family.LoadFont(data, 0, cv.FontStyle(style))
}

// Config defines a display's pixel dimensions, physical dimensions, color palette,
// semantic color roles, and pre-loaded fonts.
type Config struct {
	PixelWidth       int
	PixelHeight      int
	PhysicalWidthMM  float64
	PhysicalHeightMM float64
	Palette          []color.Color
	Roles            Roles
	Fonts            map[string]*FontFamily
}

// TRMNLOriginal is the preset Config for the TRMNL Original display (800×480 px, ~163×98 mm).
var TRMNLOriginal = Config{
	PixelWidth:       800,
	PixelHeight:      480,
	PhysicalWidthMM:  163,
	PhysicalHeightMM: 98,
	Palette:          []color.Color{Black, DarkGray, LightGray, White},
	Roles: Roles{
		Foreground: Black,
		Background: White,
		Accent:     DarkGray,
		Muted:      LightGray,
	},
}

// TextAlign controls horizontal text alignment within a text box.
type TextAlign int

const (
	TextAlignLeft TextAlign = iota
	TextAlignCenter
	TextAlignRight
)

// TextOverflow controls how text behaves when it exceeds its container.
type TextOverflow int

const (
	TextOverflowWrap TextOverflow = iota
	TextOverflowClip
	TextOverflowEllipsis
)

// Style defines text rendering properties.
type Style struct {
	Font       string
	Size       float64   // in points
	Weight     FontStyle // 0 defaults to FontRegular
	Color      color.Color
	Align      TextAlign
	LineHeight float64 // line height multiplier; 0 defaults to 1.0
	MaxLines   int     // 0 = unlimited
	Overflow   TextOverflow
}

// ImageScale controls how images scale to fit their container.
type ImageScale int

const (
	ImageFit     ImageScale = iota // preserve aspect ratio, show all content
	ImageFill                      // preserve aspect ratio, fill container (may crop)
	ImageStretch                   // stretch to fill exactly
)

// Alignment is the cross-axis alignment for HStack and VStack.
type Alignment int

const (
	AlignStart Alignment = iota
	AlignCenter
	AlignEnd
)

// ZAlignment is the 9-position alignment for ZStack children.
type ZAlignment int

const (
	TopLeading     ZAlignment = iota // 0
	Top                              // 1
	TopTrailing                      // 2
	Leading                          // 3
	Center                           // 4
	Trailing                         // 5
	BottomLeading                    // 6
	Bottom                           // 7
	BottomTrailing                   // 8
)

// ColorMode controls the bit depth and palette of the output PNG.
type ColorMode int

const (
	Grayscale ColorMode = iota // 8-bit grayscale
	OneBit                     // 1-bit black/white via dithering
	TwoBit                     // 2-bit 4-shade (black/dark/light/white) via dithering
	BWRed                      // black+white+red two-layer encoder
)

// Dither selects the dithering algorithm used in OneBit and TwoBit modes.
type Dither int

const (
	FloydSteinberg Dither = iota
	Atkinson
)

// --- Layout option types ---

// gapOpt sets spacing between stack children (pixels).
type gapOpt float64

// Gap returns a gap option for stack layouts.
func Gap(px float64) gapOpt { return gapOpt(px) }

// paddingOpt sets padding inside a container (pixels).
type paddingOpt float64

// Padding returns a padding option for stack and box layouts.
func Padding(px float64) paddingOpt { return paddingOpt(px) }

// borderOpt draws a border inside a box.
type borderOpt struct {
	width float64
	color color.Color
}

// Border returns a border option for Box.
func Border(width float64, c color.Color) borderOpt { return borderOpt{width, c} }

// backgroundOpt sets a box fill color.
type backgroundOpt struct{ color color.Color }

// Background returns a background color option for Box.
func Background(c color.Color) backgroundOpt { return backgroundOpt{c} }

// cornerRadiusOpt sets rounded corners (pixels).
type cornerRadiusOpt float64

// CornerRadius returns a corner-radius option for Box.
func CornerRadius(px float64) cornerRadiusOpt { return cornerRadiusOpt(px) }

// clipOpt enables clipping children to a container's bounds.
type clipOpt struct{}

// Clip enables clipping when passed as a Box or Image option.
var Clip = clipOpt{} //nolint:gochecknoglobals
