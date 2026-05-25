// Package demy provides a composable rendering context for e-ink and
// other more static displays, built on tdewolff/canvas with a
// SwiftUI-inspired layout model.
package demy

import (
	"image"
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
// name identifies the family within demy (used as the key in Config.Fonts).
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
// semantic color roles, pre-loaded fonts, and named styles.
type Config struct {
	PixelWidth       int
	PixelHeight      int
	PhysicalWidthMM  float64
	PhysicalHeightMM float64
	Palette          []color.Color
	Roles            Roles
	Fonts            map[string]*FontFamily
	Images           map[string]image.Image // pre-loaded images by key, for plugin contexts
	DefaultTextStyle Style                  // fallback when Text() receives no style opts
	Styles           map[string]Style       // named typographic scale (body, caption, h1, …)
}

// DPMM returns dots per millimetre for this Config.
func (cfg Config) DPMM() float64 {
	if cfg.PhysicalWidthMM == 0 {
		return 0
	}
	return float64(cfg.PixelWidth) / cfg.PhysicalWidthMM
}

// Pt converts n points to pixels based on the display PPI.
func (cfg Config) Pt(n float64) float64 { return n * cfg.DPMM() * 25.4 / 72 }

// MM converts n millimetres to pixels based on the display PPI.
func (cfg Config) MM(n float64) float64 { return n * cfg.DPMM() }

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

func (Style) demyTextOption() {}

// mergeStyle overlays non-zero fields from override on top of base.
func mergeStyle(base, override Style) Style {
	if override.Font != "" {
		base.Font = override.Font
	}
	if override.Size != 0 {
		base.Size = override.Size
	}
	if override.Weight != 0 {
		base.Weight = override.Weight
	}
	if override.Color != nil {
		base.Color = override.Color
	}
	if override.Align != 0 {
		base.Align = override.Align
	}
	if override.LineHeight != 0 {
		base.LineHeight = override.LineHeight
	}
	if override.MaxLines != 0 {
		base.MaxLines = override.MaxLines
	}
	if override.Overflow != 0 {
		base.Overflow = override.Overflow
	}
	return base
}

// StyleClass references a named entry in Config.Styles.
// It is resolved at render time and merged under any inline Style fields.
type StyleClass string

func (StyleClass) demyTextOption() {}

// Class returns a StyleClass TextOption referencing the named style in Config.Styles.
func Class(name string) StyleClass { return StyleClass(name) }

// ImageScale controls how images scale to fit their container.
type ImageScale int

const (
	ImageFit     ImageScale = iota // preserve aspect ratio, show all content
	ImageFill                      // preserve aspect ratio, fill container (may crop)
	ImageStretch                   // stretch to fill exactly
)

func (ImageScale) demyOption() {}

// Alignment is the cross-axis alignment for HStack and VStack.
type Alignment int

const (
	AlignStart Alignment = iota
	AlignCenter
	AlignEnd
)

func (Alignment) demyOption() {}

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

func (ZAlignment) demyOption() {}

// ColorMode controls the bit depth and palette of the output PNG.
type ColorMode int

const (
	Grayscale ColorMode = iota // 8-bit grayscale
	OneBit                     // 1-bit black/white
	TwoBit                     // 2-bit 4-shade (black/dark/light/white)
	BWRed                      // black+white+red two-layer encoder
)

// Dither selects the dithering algorithm used in OneBit and TwoBit modes.
type Dither int

const (
	NoDithering Dither = iota - 1
	FloydSteinberg
	Atkinson
)

// --- Interfaces ---

// Node is a composable, serializable widget. All layout and widget constructors
// return a Node. The interface is sealed — implementations are provided by this
// package only.
type Node interface {
	demyNode()
}

// Option is implemented by layout options (Gap, Padding, Alignment, …) and by
// Node itself, allowing children to be passed inline to layout constructors.
// The interface is sealed to this package.
type Option interface {
	demyOption()
}

// TextOption is implemented by Style and future per-property text overrides.
// It does NOT implement Option, preventing text-only options from being
// accidentally passed to layout containers.
type TextOption interface {
	demyTextOption()
}

// RoleColor is a color.Color that references a semantic role from Config.Roles.
// Used in deserialized scene trees (scene.Unmarshal) where colors are specified
// by role name. Resolved to a concrete color during the build pass.
type RoleColor struct {
	Name string // "foreground", "background", "accent", or "muted"
}

// RGBA implements color.Color. Returns black; actual value resolved in buildNode.
func (r RoleColor) RGBA() (uint32, uint32, uint32, uint32) { return 0, 0, 0, 0xffff }

// resolveColor replaces a RoleColor with the matching concrete color from roles.
func resolveColor(c color.Color, roles Roles) color.Color {
	if c == nil {
		return nil
	}
	rc, ok := c.(RoleColor)
	if !ok {
		return c
	}
	switch rc.Name {
	case "foreground":
		return roles.Foreground
	case "background":
		return roles.Background
	case "accent":
		return roles.Accent
	case "muted":
		return roles.Muted
	}
	return c
}

// --- Exported node types ---

// VStackNode lays children out vertically.
type VStackNode struct {
	Gap      float64
	Padding  float64
	Align    Alignment // default AlignStart
	Children []Node
}

func (*VStackNode) demyNode()   {}
func (*VStackNode) demyOption() {}

// HStackNode lays children out horizontally.
type HStackNode struct {
	Gap      float64
	Padding  float64
	Align    Alignment // default AlignStart
	Children []Node
}

func (*HStackNode) demyNode()   {}
func (*HStackNode) demyOption() {}

// ZStackNode layers children on top of each other.
type ZStackNode struct {
	Align    ZAlignment // default TopLeading
	Children []Node
}

func (*ZStackNode) demyNode()   {}
func (*ZStackNode) demyOption() {}

// BoxNode is a container with optional background, border, corner radius, and padding.
type BoxNode struct {
	Padding      float64
	BorderWidth  float64
	BorderColor  color.Color
	Background   color.Color
	CornerRadius float64
	Clip         bool
	Children     []Node
}

func (*BoxNode) demyNode()   {}
func (*BoxNode) demyOption() {}

// FrameNode constrains its children to exactly Width×Height pixels.
type FrameNode struct {
	Width    float64
	Height   float64
	Children []Node
}

func (*FrameNode) demyNode()   {}
func (*FrameNode) demyOption() {}

// SpacerNode fills remaining space in a stack. Weight defaults to 1.
type SpacerNode struct {
	Weight float64
}

func (*SpacerNode) demyNode()   {}
func (*SpacerNode) demyOption() {}

// WeightedNode wraps a child and allocates it a proportional share of the
// parent stack's remaining space. It particidemys in the same weight pool as
// SpacerNode: non-weighted children are measured at their natural size first,
// and weighted/spacer nodes divide the remainder proportionally.
//
// Example — three columns at 25 / 25 / 50 %:
//
//	demy.HStack(
//	    demy.Weighted(1, child1),
//	    demy.Weighted(1, child2),
//	    demy.Weighted(2, child3),
//	)
type WeightedNode struct {
	Weight float64
	Child  Node
}

func (*WeightedNode) demyNode()   {}
func (*WeightedNode) demyOption() {}

// DividerNode draws a 1px rule. Orientation is inferred from the enclosing stack.
type DividerNode struct{}

func (*DividerNode) demyNode()   {}
func (*DividerNode) demyOption() {}

// TextNode renders a string. Style defaults to Config.DefaultTextStyle when Font is empty.
// If Class is set, the named style from Config.Styles is merged as a base before inline Style fields.
type TextNode struct {
	Text  string
	Class StyleClass
	Style Style
}

func (*TextNode) demyNode()   {}
func (*TextNode) demyOption() {}

// ImageNode renders an image. Set Image directly or use Key to reference
// a pre-loaded image in Config.Images (required for serialized contexts).
type ImageNode struct {
	Image image.Image
	Key   string
	Scale ImageScale
	Clip  bool
}

func (*ImageNode) demyNode()   {}
func (*ImageNode) demyOption() {}

// --- Layout option types ---

// gapOpt sets spacing between stack children (pixels).
type gapOpt float64

func (gapOpt) demyOption() {}

// Gap returns a gap option for stack layouts.
func Gap(px float64) gapOpt { return gapOpt(px) }

// paddingOpt sets padding inside a container (pixels).
type paddingOpt float64

func (paddingOpt) demyOption() {}

// Padding returns a padding option for stack and box layouts.
func Padding(px float64) paddingOpt { return paddingOpt(px) }

// borderOpt draws a border inside a box.
type borderOpt struct {
	width float64
	color color.Color
}

func (borderOpt) demyOption() {}

// Border returns a border option for Box.
func Border(width float64, c color.Color) borderOpt { return borderOpt{width, c} }

// backgroundOpt sets a box fill color.
type backgroundOpt struct{ color color.Color }

func (backgroundOpt) demyOption() {}

// Background returns a background color option for Box.
func Background(c color.Color) backgroundOpt { return backgroundOpt{c} }

// cornerRadiusOpt sets rounded corners (pixels).
type cornerRadiusOpt float64

func (cornerRadiusOpt) demyOption() {}

// CornerRadius returns a corner-radius option for Box.
func CornerRadius(px float64) cornerRadiusOpt { return cornerRadiusOpt(px) }

// clipOpt enables clipping children to a container's bounds.
type clipOpt struct{}

func (clipOpt) demyOption() {}

// Clip enables clipping when passed as a Box or Image option.
var Clip = clipOpt{} //nolint:gochecknoglobals

// childrenOpt wraps a slice of Nodes as a single Option for variadic expansion.
type childrenOpt []Node

func (childrenOpt) demyOption() {}

// Children wraps nodes as an Option, enabling slice expansion into layout constructors:
//
//	demy.VStack(demy.Gap(4), demy.Children(rows...))
func Children(nodes ...Node) childrenOpt { return childrenOpt(nodes) }

// appendChildren appends opts that are Node or childrenOpt values to dst.
func appendChildren(dst []Node, opts []Option) []Node {
	for _, opt := range opts {
		switch v := opt.(type) {
		case childrenOpt:
			dst = append(dst, []Node(v)...)
		case Node:
			dst = append(dst, v)
		}
	}
	return dst
}

// --- Constructor functions ---

// VStack creates a vertical stack. Options: Gap, Padding, Alignment, child Nodes, Children.
func VStack(opts ...Option) *VStackNode {
	n := &VStackNode{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case gapOpt:
			n.Gap = float64(v)
		case paddingOpt:
			n.Padding = float64(v)
		case Alignment:
			n.Align = v
		}
	}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// HStack creates a horizontal stack. Options: Gap, Padding, Alignment, child Nodes, Children.
func HStack(opts ...Option) *HStackNode {
	n := &HStackNode{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case gapOpt:
			n.Gap = float64(v)
		case paddingOpt:
			n.Padding = float64(v)
		case Alignment:
			n.Align = v
		}
	}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// ZStack creates a layered stack. Options: ZAlignment, child Nodes, Children.
func ZStack(opts ...Option) *ZStackNode {
	n := &ZStackNode{}
	for _, opt := range opts {
		if v, ok := opt.(ZAlignment); ok {
			n.Align = v
		}
	}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// Box creates a styled container. Options: Padding, Background, Border, CornerRadius, Clip,
// child Nodes, Children.
func Box(opts ...Option) *BoxNode {
	n := &BoxNode{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case paddingOpt:
			n.Padding = float64(v)
		case borderOpt:
			n.BorderWidth = v.width
			n.BorderColor = v.color
		case backgroundOpt:
			n.Background = v.color
		case cornerRadiusOpt:
			n.CornerRadius = float64(v)
		case clipOpt:
			n.Clip = true
		}
	}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// Frame constrains its children to exactly w×h pixels.
func Frame(w, h float64, opts ...Option) *FrameNode {
	n := &FrameNode{Width: w, Height: h}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// Spacer fills remaining space in a stack. Optional weight defaults to 1.
func Spacer(weight ...float64) *SpacerNode {
	w := 1.0
	if len(weight) > 0 {
		w = weight[0]
	}
	return &SpacerNode{Weight: w}
}

// Weighted wraps child and allocates it a proportional share of the enclosing
// stack's remaining space. Weight 0 is treated as 1. Multiple weighted children
// share the remaining space in proportion to their weights.
func Weighted(weight float64, child Node) *WeightedNode {
	return &WeightedNode{Weight: weight, Child: child}
}

// Divider draws a 1px rule. Orientation is inferred from the enclosing stack
// at render time: horizontal in a VStack, vertical in an HStack.
func Divider() *DividerNode { return &DividerNode{} }

// Text renders s with the given style options. When no TextOption is provided,
// Config.DefaultTextStyle is used at render time.
func Text(s string, opts ...TextOption) *TextNode {
	n := &TextNode{Text: s}
	for _, opt := range opts {
		switch v := opt.(type) {
		case Style:
			n.Style = v
		case StyleClass:
			n.Class = v
		}
	}
	return n
}

// Image renders img. Options: ImageScale (ImageFit, ImageFill, ImageStretch), Clip.
func Image(img image.Image, opts ...Option) *ImageNode {
	n := &ImageNode{Image: img}
	for _, opt := range opts {
		switch v := opt.(type) {
		case ImageScale:
			n.Scale = v
		case clipOpt:
			n.Clip = true
		}
	}
	return n
}

// ImageKey renders the image registered under key in Config.Images.
// Used in serialized plugin contexts where image.Image cannot be transmitted.
func ImageKey(key string, opts ...Option) *ImageNode {
	n := &ImageNode{Key: key}
	for _, opt := range opts {
		switch v := opt.(type) {
		case ImageScale:
			n.Scale = v
		case clipOpt:
			n.Clip = true
		}
	}
	return n
}
