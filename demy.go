// Package demy is a declarative rendering library for e-ink and other
// low-refresh displays, with a SwiftUI-inspired layout model.
//
// # Untrusted input
//
// Canvas.Render can render node trees and BDF fonts from untrusted sources.
// It returns errors instead of panicking and enforces fixed limits: at most
// 16384 nodes (counting each use of a shared node), 128 clipped nodes, 16
// barcodes, 8192 bytes per Text and 65536 bytes of text per tree, 1000 levels
// of nesting, and dimensions in [0, 32768]. BDF fonts are limited in glyph
// size, metrics, and bitmap size, and the visible area of bitmap glyphs is
// limited to 16 times the canvas. Render stops when its context is done, so
// give untrusted renders a deadline. Memory grows with the canvas size.
//
// Config is trusted: vector fonts are parsed without further checks, and images
// are the host's.
package demy

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sync"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
)

// Color constants for display palettes.
var (
	Black     = color.Gray{Y: 0}
	DarkGray  = color.Gray{Y: 85}
	LightGray = color.Gray{Y: 170}
	White     = color.Gray{Y: 255}
	Red       = color.RGBA{R: 255, A: 255}
	Yellow    = color.RGBA{R: 255, G: 255, A: 255}
	Green     = color.RGBA{G: 255, A: 255}
	Blue      = color.RGBA{B: 255, A: 255}
	Orange    = color.RGBA{R: 255, G: 128, A: 255}
)

// Roles maps semantic color names to concrete display colors.
type Roles struct {
	Foreground color.Color
	Background color.Color
	Accent     color.Color
	Muted      color.Color
}

// FontStyle specifies a font weight and optional italic flag.
// Weights can be OR'd with FontItalic, e.g. FontBold|FontItalic. The zero value
// inherits the weight in a Style and is Regular otherwise; FontItalic alone is
// Regular italic.
type FontStyle int

const (
	FontInherit    FontStyle = iota // inherit; Regular when nothing sets a weight
	FontRegular                     // weight 400
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

// normalized resolves FontInherit to FontRegular, keeping the italic flag.
func (s FontStyle) normalized() FontStyle {
	if s&^FontItalic == FontInherit {
		return FontRegular | s&FontItalic
	}
	return s
}

// fauxWeight is the outline offset, as a fraction of the em, that
// synthesizes s's weight from a Regular face.
func (s FontStyle) fauxWeight() float64 {
	switch s.normalized() &^ FontItalic {
	case FontThin:
		return -0.02
	case FontExtraLight:
		return -0.01
	case FontLight:
		return -0.005
	case FontMedium:
		return 0.005
	case FontSemiBold:
		return 0.01
	case FontBold:
		return 0.02
	case FontExtraBold:
		return 0.03
	case FontBlack:
		return 0.04
	}
	return 0
}

func (s FontStyle) italic() bool { return s&FontItalic != 0 }

// FontFamily holds the weight/style variants of one font family. A family is
// either a vector family (TTF/OTF) or a bitmap family (BDF); the two cannot be
// mixed. Text references a family by its key in Config.Fonts.
//
// A FontFamily may be shared by concurrent renders and canvases. Add all of its
// styles before the first Render that uses it: AddStyle and AddStyleBDF are not
// safe to call while a render may be reading the family. Create one with a Load
// function; a zero value is not usable and rendering text with it is an error.
type FontFamily struct {
	name string
	// faces holds one vector face per style; non-nil marks a vector family.
	faces map[FontStyle]*font.Face
	// bitmap holds one font per style; non-nil marks a bitmap family.
	bitmap map[FontStyle]*bitmapFont

	// mu serializes shaping and outline loading: go-text's shaper, line
	// wrapper, and faces keep caches that aren't safe for concurrent use.
	mu       sync.Mutex
	shaper   shaping.HarfbuzzShaper
	wrapper  shaping.LineWrapper
	seg      shaping.Segmenter
	outlines map[outlineKey][]opentype.Segment // glyph outlines with faux styling applied
}

// isBitmap reports whether this is a bitmap (pixel) font family.
func (ff *FontFamily) isBitmap() bool { return ff.bitmap != nil }

// errFontNotLoaded is returned for a FontFamily not created by one of the Load
// functions, such as a zero value.
var errFontNotLoaded = errors.New("demy: font family is not loaded; create it with LoadFontBytes, LoadFontFile, or LoadBDFBytes")

// isLoaded reports whether the family was created by one of the Load functions.
func (ff *FontFamily) isLoaded() bool { return ff.faces != nil || ff.bitmap != nil }

// bitmapVariant returns the bitmap font for the requested style, falling back
// to the Regular variant (then any variant) when the exact style is missing.
func (ff *FontFamily) bitmapVariant(style FontStyle) *bitmapFont {
	if bf, ok := ff.bitmap[style.normalized()]; ok {
		return bf
	}
	if bf, ok := ff.bitmap[FontRegular]; ok {
		return bf
	}
	for _, bf := range ff.bitmap {
		return bf
	}
	return nil
}

// LoadFontFile loads a TTF or OTF file as the Regular style of a new family.
func LoadFontFile(name, path string) (*FontFamily, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadFontBytes(name, data)
}

// MustLoadFontFile loads a font file and panics on error.
func MustLoadFontFile(name, path string) *FontFamily {
	f, err := LoadFontFile(name, path)
	if err != nil {
		panic(err)
	}
	return f
}

// LoadFontBytes loads TrueType or OpenType bytes as the Regular style of a new
// family, e.g. LoadFontBytes("go", goregular.TTF).
func LoadFontBytes(name string, data []byte) (*FontFamily, error) {
	face, err := parseFont(data)
	if err != nil {
		return nil, err
	}
	return &FontFamily{name: name, faces: map[FontStyle]*font.Face{FontRegular: face}}, nil
}

// parseFont parses a TrueType or OpenType font, or the first font of a
// collection.
func parseFont(data []byte) (*font.Face, error) {
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err == nil {
		return face, nil
	}
	if faces, cerr := font.ParseTTC(bytes.NewReader(data)); cerr == nil && len(faces) > 0 {
		return faces[0], nil
	}
	return nil, fmt.Errorf("demy: parsing font: %w", err)
}

// MustLoadFontBytes loads a font from bytes and panics on error.
func MustLoadFontBytes(name string, data []byte) *FontFamily {
	f, err := LoadFontBytes(name, data)
	if err != nil {
		panic(err)
	}
	return f
}

// AddStyle adds a weight/style variant (e.g. FontBold, FontBold|FontItalic)
// from TTF/OTF bytes. It returns an error for a bitmap family. Call it before
// the family is used in a Render.
func (ff *FontFamily) AddStyle(data []byte, style FontStyle) error {
	if !ff.isLoaded() {
		return errFontNotLoaded
	}
	if ff.isBitmap() {
		return errors.New("demy: cannot add a vector style to a bitmap font family")
	}
	face, err := parseFont(data)
	if err != nil {
		return err
	}
	ff.faces[style.normalized()] = face
	return nil
}

// LoadBDFBytes parses a BDF (Adobe Bitmap Distribution Format) font as
// the Regular style of a new bitmap family.
//
// Bitmap fonts render without antialiasing at their native pixel size times
// the integer display scale; Style.Size is ignored. Bitmap text is composited
// after quantization, so it always draws on top of other content.
func LoadBDFBytes(name string, data []byte) (*FontFamily, error) {
	bf, err := parseBDF(data)
	if err != nil {
		return nil, err
	}
	return &FontFamily{name: name, bitmap: map[FontStyle]*bitmapFont{FontRegular: bf}}, nil
}

// MustLoadBDFBytes loads a BDF bitmap font and panics on error.
func MustLoadBDFBytes(name string, data []byte) *FontFamily {
	f, err := LoadBDFBytes(name, data)
	if err != nil {
		panic(err)
	}
	return f
}

// LoadBDFFile loads a BDF bitmap font from a file path.
func LoadBDFFile(name, path string) (*FontFamily, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadBDFBytes(name, data)
}

// MustLoadBDFFile loads a BDF bitmap font from a file and panics on error.
func MustLoadBDFFile(name, path string) *FontFamily {
	f, err := LoadBDFFile(name, path)
	if err != nil {
		panic(err)
	}
	return f
}

// AddStyleBDF adds a weight/style variant (e.g. FontBold) from BDF bytes.
// It returns an error for a vector family. Call it before the family is used
// in a Render.
func (ff *FontFamily) AddStyleBDF(data []byte, style FontStyle) error {
	if !ff.isLoaded() {
		return errFontNotLoaded
	}
	if !ff.isBitmap() {
		return errors.New("demy: cannot add a bitmap style to a vector font family")
	}
	bf, err := parseBDF(data)
	if err != nil {
		return err
	}
	ff.bitmap[style.normalized()] = bf
	return nil
}

// Config describes a display and the resources available when rendering to it.
//
// New copies the Config but not the maps and slices it holds (Fonts, Images,
// Styles, Palette), which the Canvas shares with the caller. Don't modify them
// after the first Render; build a new Config and Canvas instead.
type Config struct {
	PixelWidth       int
	PixelHeight      int
	PhysicalWidthMM  float64
	PhysicalHeightMM float64
	// Scale maps logical pixels to physical pixels. 0 auto-computes it from
	// the pixel density (1 for ~125 PPI displays, 2 for ~250 PPI). The
	// effective scale must be in [0.5, 64].
	Scale float64
	// Palette lists the display's colors, which Paletted mode quantizes
	// to. The rendered image's palette indices follow this order, so list
	// the colors in the order the display's driver numbers them. Use the
	// colors the panel actually shows (often duller than pure sRGB) for
	// closer dithering; the indices stay the same.
	Palette          []color.Color
	Roles            Roles
	ColorMode        ColorMode              // default mode when Render is called with ColorModeDefault
	Fonts            map[string]*FontFamily // keyed by Style.Font
	Images           map[string]image.Image // referenced by ImageKey
	DefaultTextStyle Style                  // base style that Text styles are merged over
	Styles           map[string]Style       // named styles referenced by Class
}

// PhysicalDPMM returns the display's physical pixels per millimeter.
// For layout, use DPMM.
func (cfg Config) PhysicalDPMM() float64 {
	if cfg.PhysicalWidthMM == 0 {
		return 0
	}
	return float64(cfg.PixelWidth) / cfg.PhysicalWidthMM
}

// DPMM returns logical pixels per millimeter (PhysicalDPMM / EffectiveScale).
func (cfg Config) DPMM() float64 { return cfg.PhysicalDPMM() / cfg.EffectiveScale() }

// EffectiveScale returns Scale, or max(1, round(PhysicalDPMM/5)) when Scale is 0.
func (cfg Config) EffectiveScale() float64 {
	if cfg.Scale != 0 {
		return cfg.Scale
	}
	return math.Max(1, math.Round(cfg.PhysicalDPMM()/5.0))
}

// Pt converts n points to logical pixels.
func (cfg Config) Pt(n float64) float64 { return n * cfg.DPMM() * 25.4 / 72 }

// MM converts n millimeters to logical pixels.
func (cfg Config) MM(n float64) float64 { return n * cfg.DPMM() }

// TextAlign controls horizontal text alignment within a text box. The zero
// value inherits in a Style and is left-aligned otherwise.
type TextAlign int

const (
	TextAlignInherit TextAlign = iota
	TextAlignLeft
	TextAlignCenter
	TextAlignRight
)

// TextOverflow controls how text behaves when it exceeds MaxLines. Wrap and
// Clip truncate at MaxLines; Ellipsis also marks the truncation. The zero value
// inherits in a Style and wraps otherwise.
type TextOverflow int

const (
	TextOverflowInherit TextOverflow = iota
	TextOverflowWrap
	TextOverflowClip
	TextOverflowEllipsis
)

// MaxLinesUnlimited sets Style.MaxLines to no limit, overriding a limit
// inherited from a class or Config.DefaultTextStyle.
const MaxLinesUnlimited = -1

// Style defines text rendering properties. Zero-valued fields inherit from
// Config.DefaultTextStyle and any StyleClass; each field's type documents how
// to set it explicitly back to its default.
type Style struct {
	Font       string    // key in Config.Fonts; required after merging
	Size       float64   // in points; required after merging for vector fonts, ignored for bitmap fonts
	Weight     FontStyle // FontInherit (0) inherits
	Color      color.Color
	Align      TextAlign
	LineHeight float64 // line height multiplier; 0 inherits, default 1.0
	MaxLines   int     // 0 inherits; MaxLinesUnlimited for no limit
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

// StyleClass references a named entry in Config.Styles. It is merged over
// Config.DefaultTextStyle and under any inline Style fields.
type StyleClass string

func (StyleClass) demyTextOption() {}

// Class returns a StyleClass referencing the named style in Config.Styles.
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
	ZAlignTopLeading     ZAlignment = iota // 0
	ZAlignTop                              // 1
	ZAlignTopTrailing                      // 2
	ZAlignLeading                          // 3
	ZAlignCenter                           // 4
	ZAlignTrailing                         // 5
	ZAlignBottomLeading                    // 6
	ZAlignBottom                           // 7
	ZAlignBottomTrailing                   // 8
)

func (ZAlignment) demyOption() {}

// ColorMode selects the format of the rendered image.
type ColorMode int

const (
	ColorModeDefault ColorMode = iota // use Config.ColorMode
	RGBA                              // full color, unquantized (*image.RGBA)
	Grayscale                         // 8-bit luminance, unquantized (*image.Gray)
	Paletted                          // quantized to Config.Palette (*image.Paletted)
)

// Dither selects the dithering algorithm used when quantizing to
// Config.Palette.
type Dither int

const (
	NoDithering Dither = iota
	FloydSteinberg
	Atkinson
)

func (Dither) demyOption() {}

// --- Interfaces ---

// Node is an element of a render tree. All node constructors return a Node.
// The interface is sealed to this package.
type Node interface {
	demyNode()
}

// Option is implemented by layout options (Gap, Padding, Alignment, …) and by
// every node type, so children can be passed inline to container constructors.
// The interface is sealed to this package.
type Option interface {
	demyOption()
}

// TextOption is implemented by Style and StyleClass. It is distinct from
// Option so text styles cannot be passed to layout containers.
type TextOption interface {
	demyTextOption()
}

// RoleColor is a color.Color that refers to a role in Config.Roles by name.
// It is resolved to the concrete role color at render time. Scene decoders
// produce it for colors given by role name.
type RoleColor struct {
	Name string // "foreground", "background", "accent", or "muted"
}

// RGBA implements color.Color. It returns opaque black; the real color comes
// from Config.Roles at render time.
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
	Align    Alignment
	Children []Node
}

func (*VStackNode) demyNode()   {}
func (*VStackNode) demyOption() {}

// HStackNode lays children out horizontally.
type HStackNode struct {
	Gap      float64
	Padding  float64
	Align    Alignment
	Children []Node
}

func (*HStackNode) demyNode()   {}
func (*HStackNode) demyOption() {}

// ZStackNode layers children on top of each other.
type ZStackNode struct {
	Align    ZAlignment
	Children []Node
}

func (*ZStackNode) demyNode()   {}
func (*ZStackNode) demyOption() {}

// BoxNode is a container with optional background, border, corner radius, and padding.
// When Clip is set, children are clipped to the box's rectangular bounds; corner
// radius does not affect clipping.
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

// FrameNode sizes its children to exactly Width×Height logical pixels and clips
// them to that area.
type FrameNode struct {
	Width    float64
	Height   float64
	Children []Node
}

func (*FrameNode) demyNode()   {}
func (*FrameNode) demyOption() {}

// SpacerNode fills remaining space in a stack. A Weight of 0 is treated as 1.
type SpacerNode struct {
	Weight float64
}

func (*SpacerNode) demyNode()   {}
func (*SpacerNode) demyOption() {}

// WeightedNode gives its child a share of the enclosing stack's remaining space
// in proportion to Weight (0 is treated as 1). Weighted nodes and spacers share
// the space left after other children take their natural size.
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

// DividerNode draws a rule, horizontal in a VStack and vertical in an HStack.
// Size is the thickness in logical pixels (0 means 1). A nil Color uses the
// muted role color.
type DividerNode struct {
	Color color.Color
	Size  float64
}

func (*DividerNode) demyNode()   {}
func (*DividerNode) demyOption() {}

// TextNode renders a string. The effective style is Config.DefaultTextStyle,
// overlaid by the Class style, then by the non-zero fields of Style. Rendering
// fails if the effective style has no loaded font, or no size for a vector font.
type TextNode struct {
	Text  string
	Class StyleClass
	Style Style
}

func (*TextNode) demyNode()   {}
func (*TextNode) demyOption() {}

// ImageNode renders an image. Set Image directly, or Key to reference an image
// in Config.Images (required when the tree is serialized). Rendering fails if
// there is no image, or its bounds are empty, wider or taller than 16384
// pixels, or larger than 2²⁶ pixels (which rejects image.Uniform).
type ImageNode struct {
	Image  image.Image
	Key    string
	Scale  ImageScale
	Dither Dither // if set, overrides the Render dithering algorithm within this image
}

func (*ImageNode) demyNode()   {}
func (*ImageNode) demyOption() {}

// BarcodeType selects the barcode encoding format.
type BarcodeType int

func (BarcodeType) demyOption() {}

const (
	BarcodeTypeQR BarcodeType = iota // QR code (default)
)

// ECLevel sets the QR code error correction level. Higher levels tolerate more
// damage but reduce data capacity.
type ECLevel int

func (ECLevel) demyOption() {}

const (
	ECLevelLow      ECLevel = iota // ~7% recovery
	ECLevelMedium                  // ~15% recovery
	ECLevelQuartile                // ~25% recovery
	ECLevelHigh                    // ~30% recovery
)

// BarcodeNode renders a barcode. The Barcode constructor defaults to
// BarcodeTypeQR with ECLevelMedium. Rendering fails if the barcode's space is
// too small for one physical pixel per module.
type BarcodeNode struct {
	Content string
	Type    BarcodeType
	ECLevel ECLevel
}

func (*BarcodeNode) demyNode()   {}
func (*BarcodeNode) demyOption() {}

type gridTrackKind uint8

const (
	gridTrackAuto gridTrackKind = iota
	gridTrackFixed
	gridTrackFr
)

// GridTrack defines the size of one column or row in a GridNode.
// Construct it with GridAuto, GridFixed, or GridFr.
type GridTrack struct {
	kind   gridTrackKind
	size   float64 // gridTrackFixed only
	weight float64 // gridTrackFr only
}

// IsAuto reports whether the track sizes to content.
func (t GridTrack) IsAuto() bool { return t.kind == gridTrackAuto }

// IsFixed reports whether the track has a fixed size.
func (t GridTrack) IsFixed() bool { return t.kind == gridTrackFixed }

// IsFr reports whether the track takes a proportional share of remaining space.
func (t GridTrack) IsFr() bool { return t.kind == gridTrackFr }

// Size returns the size of a GridFixed track in logical pixels; 0 for other kinds.
func (t GridTrack) Size() float64 { return t.size }

// Weight returns the fractional weight for a GridFr track; 0 for other kinds.
// Weights are relative to the other Fr tracks in the same grid.
func (t GridTrack) Weight() float64 { return t.weight }

// GridAuto returns a track sized to the widest (or tallest) cell in it.
func GridAuto() GridTrack { return GridTrack{kind: gridTrackAuto} }

// GridFixed returns a track of exactly px logical pixels.
func GridFixed(px float64) GridTrack { return GridTrack{kind: gridTrackFixed, size: px} }

// GridFr returns a track that takes a share of the space left after fixed and
// auto tracks, proportional to weight (like CSS fr units).
func GridFr(weight float64) GridTrack { return GridTrack{kind: gridTrackFr, weight: weight} }

// GridNode lays children out in a 2D grid, filling cells row by row, left to
// right. Columns are required; rendering a grid without columns is an error.
// When Rows is empty every row is GridAuto; otherwise the grid has len(Rows)
// rows and children beyond the last cell are dropped.
type GridNode struct {
	Columns  []GridTrack // number of columns is len(Columns)
	Rows     []GridTrack
	RowGap   float64
	ColGap   float64
	Padding  float64
	Children []Node
}

func (*GridNode) demyNode()   {}
func (*GridNode) demyOption() {}

// GridRowNode groups cells for readability within a GridNode. Its children are
// spliced into the grid's cell list in place, so it does not force a row
// break. Bare cells and GridRow children may be mixed.
type GridRowNode struct {
	Children []Node
}

func (*GridRowNode) demyNode()   {}
func (*GridRowNode) demyOption() {}

// Grid creates a grid. Options: Columns (required), Rows, Gap, RowGap, ColGap,
// Padding, child Nodes, Children. Gap sets both gaps; RowGap and ColGap set one.
func Grid(opts ...Option) *GridNode {
	n := &GridNode{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case columnsOpt:
			n.Columns = []GridTrack(v)
		case rowsOpt:
			n.Rows = []GridTrack(v)
		case gapOpt:
			n.RowGap = float64(v)
			n.ColGap = float64(v)
		case rowGapOpt:
			n.RowGap = float64(v)
		case colGapOpt:
			n.ColGap = float64(v)
		case paddingOpt:
			n.Padding = float64(v)
		}
	}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// GridRow groups cells within a Grid. See GridRowNode.
func GridRow(children ...Node) *GridRowNode {
	return &GridRowNode{Children: children}
}

// Barcode renders content as a barcode. Options: BarcodeType (default
// BarcodeTypeQR), ECLevel (default ECLevelMedium).
func Barcode(content string, opts ...Option) *BarcodeNode {
	n := &BarcodeNode{Content: content, ECLevel: ECLevelMedium}
	for _, opt := range opts {
		switch v := opt.(type) {
		case BarcodeType:
			n.Type = v
		case ECLevel:
			n.ECLevel = v
		}
	}
	return n
}

// --- Layout option types ---

type gapOpt float64

func (gapOpt) demyOption() {}

// Gap sets the spacing between children of a stack or grid, in logical pixels.
func Gap(px float64) Option { return gapOpt(px) }

type paddingOpt float64

func (paddingOpt) demyOption() {}

// Padding sets the inset of a stack, box, or grid, in logical pixels.
func Padding(px float64) Option { return paddingOpt(px) }

type borderOpt struct {
	width float64
	color color.Color
}

func (borderOpt) demyOption() {}

// Border sets a Box border.
func Border(width float64, c color.Color) Option { return borderOpt{width, c} }

type backgroundOpt struct{ color color.Color }

func (backgroundOpt) demyOption() {}

// Background sets a Box fill color.
func Background(c color.Color) Option { return backgroundOpt{c} }

type cornerRadiusOpt float64

func (cornerRadiusOpt) demyOption() {}

// CornerRadius sets a Box corner radius, in logical pixels.
func CornerRadius(px float64) Option { return cornerRadiusOpt(px) }

type clipOpt struct{}

func (clipOpt) demyOption() {}

// Clip clips a Box's children to its bounds.
var Clip Option = clipOpt{} //nolint:gochecknoglobals

type columnsOpt []GridTrack

func (columnsOpt) demyOption() {}

type rowsOpt []GridTrack

func (rowsOpt) demyOption() {}

type rowGapOpt float64

func (rowGapOpt) demyOption() {}

type colGapOpt float64

func (colGapOpt) demyOption() {}

// Columns sets the column tracks of a Grid.
func Columns(tracks ...GridTrack) Option { return columnsOpt(tracks) }

// Rows sets the row tracks of a Grid.
func Rows(tracks ...GridTrack) Option { return rowsOpt(tracks) }

// RowGap sets the spacing between Grid rows, in logical pixels.
func RowGap(px float64) Option { return rowGapOpt(px) }

// ColGap sets the spacing between Grid columns, in logical pixels.
func ColGap(px float64) Option { return colGapOpt(px) }

type childrenOpt []Node

func (childrenOpt) demyOption() {}

// Children passes a slice of nodes as a single option:
//
//	demy.VStack(demy.Gap(4), demy.Children(rows...))
//
// Nil children, including typed nil pointers such as a nil *TextNode, are
// skipped when the tree is rendered, in every container. In a Grid a skipped
// child takes no cell, so later children move up one cell.
func Children(nodes ...Node) Option { return childrenOpt(nodes) }

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

// Frame sizes its children to exactly w×h logical pixels and clips them.
func Frame(w, h float64, opts ...Option) *FrameNode {
	n := &FrameNode{Width: w, Height: h}
	n.Children = appendChildren(n.Children, opts)
	return n
}

// Spacer fills remaining space in a stack. The optional weight defaults to 1.
func Spacer(weight ...float64) *SpacerNode {
	w := 1.0
	if len(weight) > 0 {
		w = weight[0]
	}
	return &SpacerNode{Weight: w}
}

// Weighted gives child a share of the enclosing stack's remaining space. See
// WeightedNode.
func Weighted(weight float64, child Node) *WeightedNode {
	return &WeightedNode{Weight: weight, Child: child}
}

// Divider draws a 1px rule in the muted role color, horizontal in a VStack and
// vertical in an HStack.
func Divider() *DividerNode { return &DividerNode{} }

// Text renders s. Options: Style, StyleClass. See TextNode for how styles
// combine.
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

// Image renders img. Options: ImageScale (ImageFit, ImageFill, ImageStretch), Dither.
func Image(img image.Image, opts ...Option) *ImageNode {
	n := &ImageNode{Image: img}
	for _, opt := range opts {
		switch v := opt.(type) {
		case ImageScale:
			n.Scale = v
		case Dither:
			n.Dither = v
		}
	}
	return n
}

// ImageKey renders the image stored under key in Config.Images. Use it for
// trees that are serialized. Options are the same as Image.
func ImageKey(key string, opts ...Option) *ImageNode {
	n := &ImageNode{Key: key}
	for _, opt := range opts {
		switch v := opt.(type) {
		case ImageScale:
			n.Scale = v
		case Dither:
			n.Dither = v
		}
	}
	return n
}
