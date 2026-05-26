// Package scene provides JSON serialization for demy Node trees.
// Used to pass scene descriptions across process boundaries (e.g. WASM plugins).
//
// Color encoding:
//   - Role names ("foreground", "background", "accent", "muted") are stored as-is
//     and resolved against Config.Roles at render time via demy.RoleColor.
//   - All other colors are encoded as "#rrggbb" or "#rrggbbaa" hex strings and
//     parsed into color.RGBA at unmarshal time.
//
// Two serialization formats are supported:
//   - Bare node: {"type":"vstack",...} — used by Marshal/Unmarshal
//   - Scene envelope: {"styles":{...},"root":{...}} — used by MarshalScene/UnmarshalScene
package scene

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/samuel/demy"
)

type Node struct {
	Type         string  `json:"type"`
	Gap          float64 `json:"gap,omitempty"`
	Padding      float64 `json:"padding,omitempty"`
	BorderWidth  float64 `json:"border_width,omitempty"`
	BorderColor  string  `json:"border_color,omitempty"`
	Background   string  `json:"background,omitempty"`
	CornerRadius float64 `json:"corner_radius,omitempty"`
	Clip         bool    `json:"clip,omitempty"`
	Width        float64 `json:"width,omitempty"`
	Height       float64 `json:"height,omitempty"`
	Weight       float64 `json:"weight,omitempty"`
	Align        string  `json:"align,omitempty"`
	Text         string  `json:"text,omitempty"`
	Class        string  `json:"class,omitempty"`
	Style        *Style  `json:"style,omitempty"`
	ImageKey     string  `json:"image_key,omitempty"`
	Scale        string  `json:"scale,omitempty"`
	Content      string  `json:"content,omitempty"`
	BarcodeType  string  `json:"barcode_type,omitempty"`
	ECLevel      string  `json:"ec_level,omitempty"`
	Children     []Node  `json:"children,omitempty"`
}

type Style struct {
	Font       string  `json:"font,omitempty"`
	Size       float64 `json:"size,omitempty"`
	Weight     string  `json:"weight,omitempty"`
	Color      string  `json:"color,omitempty"`
	Align      string  `json:"align,omitempty"`
	LineHeight float64 `json:"line_height,omitempty"`
	MaxLines   int     `json:"max_lines,omitempty"`
	Overflow   string  `json:"overflow,omitempty"`
}

// Scene is the top-level JSON envelope for a node tree with embedded style definitions.
type Scene struct {
	Styles map[string]Style `json:"styles,omitempty"`
	Root   Node             `json:"root"`
}

// Marshal serializes a Node tree to JSON (bare node format, no styles envelope).
func Marshal(n demy.Node) ([]byte, error) {
	sn, err := nodeToScene(n)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sn)
}

// Unmarshal deserializes bare-node JSON to a Node tree. Role-name colors ("muted", etc.)
// are stored as demy.RoleColor and resolved against Config.Roles at render time.
// Image keys are resolved against Config.Images during the build pass.
func Unmarshal(data []byte) (demy.Node, error) {
	var sn Node
	if err := json.Unmarshal(data, &sn); err != nil {
		return nil, err
	}
	return sceneToNode(sn)
}

// MarshalScene serializes a Node tree and named style definitions to a JSON Scene
// envelope. Callers that consume the returned data should call UnmarshalScene and
// merge the returned styles into Config.Styles before rendering.
func MarshalScene(n demy.Node, styles map[string]demy.Style) ([]byte, error) {
	root, err := nodeToScene(n)
	if err != nil {
		return nil, err
	}
	s := Scene{Root: root}
	if len(styles) > 0 {
		s.Styles = make(map[string]Style, len(styles))
		for k, v := range styles {
			s.Styles[k] = Style{
				Font:       v.Font,
				Size:       v.Size,
				Weight:     weightStr(v.Weight),
				Color:      colorStr(v.Color),
				Align:      textAlignStr(v.Align),
				LineHeight: v.LineHeight,
				MaxLines:   v.MaxLines,
				Overflow:   overflowStr(v.Overflow),
			}
		}
	}
	return json.Marshal(s)
}

// UnmarshalScene deserializes a JSON Scene envelope, returning the root node and
// style definitions. Merge the returned styles into Config.Styles before rendering
// so that class references in the node tree resolve correctly.
func UnmarshalScene(data []byte) (demy.Node, map[string]demy.Style, error) {
	var s Scene
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, nil, err
	}
	n, err := sceneToNode(s.Root)
	if err != nil {
		return nil, nil, err
	}
	var outStyles map[string]demy.Style
	if len(s.Styles) > 0 {
		outStyles = make(map[string]demy.Style, len(s.Styles))
		for k, ss := range s.Styles {
			parsed, err := parseStyle(&ss)
			if err != nil {
				return nil, nil, fmt.Errorf("scene: style %q: %w", k, err)
			}
			outStyles[k] = parsed
		}
	}
	return n, outStyles, nil
}

// --- marshal helpers ---

func nodeToScene(n demy.Node) (Node, error) {
	if n == nil {
		return Node{Type: "spacer", Weight: 0}, nil
	}
	switch v := n.(type) {
	case *demy.VStackNode:
		sn := Node{
			Type:    "vstack",
			Gap:     v.Gap,
			Padding: v.Padding,
			Align:   alignStr(v.Align),
		}
		for _, child := range v.Children {
			cs, err := nodeToScene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *demy.HStackNode:
		sn := Node{
			Type:    "hstack",
			Gap:     v.Gap,
			Padding: v.Padding,
			Align:   alignStr(v.Align),
		}
		for _, child := range v.Children {
			cs, err := nodeToScene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *demy.ZStackNode:
		sn := Node{
			Type:  "zstack",
			Align: zAlignStr(v.Align),
		}
		for _, child := range v.Children {
			cs, err := nodeToScene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *demy.BoxNode:
		sn := Node{
			Type:         "box",
			Padding:      v.Padding,
			BorderWidth:  v.BorderWidth,
			BorderColor:  colorStr(v.BorderColor),
			Background:   colorStr(v.Background),
			CornerRadius: v.CornerRadius,
			Clip:         v.Clip,
		}
		for _, child := range v.Children {
			cs, err := nodeToScene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *demy.FrameNode:
		sn := Node{
			Type:   "frame",
			Width:  v.Width,
			Height: v.Height,
		}
		for _, child := range v.Children {
			cs, err := nodeToScene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *demy.SpacerNode:
		return Node{Type: "spacer", Weight: v.Weight}, nil
	case *demy.WeightedNode:
		var children []Node
		if v.Child != nil {
			cs, err := nodeToScene(v.Child)
			if err != nil {
				return Node{}, err
			}
			children = []Node{cs}
		}
		return Node{Type: "weighted", Weight: v.Weight, Children: children}, nil
	case *demy.DividerNode:
		return Node{Type: "divider"}, nil
	case *demy.TextNode:
		sn := Node{
			Type:  "text",
			Text:  v.Text,
			Class: string(v.Class),
			Style: styleToScene(v.Style),
		}
		return sn, nil
	case *demy.ImageNode:
		return Node{
			Type:     "image",
			ImageKey: v.Key,
			Scale:    scaleStr(v.Scale),
			Clip:     v.Clip,
		}, nil
	case *demy.BarcodeNode:
		return Node{
			Type:        "barcode",
			Content:     v.Content,
			BarcodeType: barcodeTypeStr(v.Type),
			ECLevel:     ecLevelStr(v.ECLevel),
		}, nil
	default:
		return Node{}, fmt.Errorf("scene: unknown node type %T", n)
	}
}

func styleToScene(s demy.Style) *Style {
	if s == (demy.Style{}) {
		return nil
	}
	return &Style{
		Font:       s.Font,
		Size:       s.Size,
		Weight:     weightStr(s.Weight),
		Color:      colorStr(s.Color),
		Align:      textAlignStr(s.Align),
		LineHeight: s.LineHeight,
		MaxLines:   s.MaxLines,
		Overflow:   overflowStr(s.Overflow),
	}
}

// --- unmarshal helpers ---

func sceneToNode(sn Node) (demy.Node, error) {
	switch sn.Type {
	case "vstack":
		n := &demy.VStackNode{
			Gap:     sn.Gap,
			Padding: sn.Padding,
			Align:   parseAlign(sn.Align),
		}
		for _, child := range sn.Children {
			c, err := sceneToNode(child)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		}
		return n, nil
	case "hstack":
		n := &demy.HStackNode{
			Gap:     sn.Gap,
			Padding: sn.Padding,
			Align:   parseAlign(sn.Align),
		}
		for _, child := range sn.Children {
			c, err := sceneToNode(child)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		}
		return n, nil
	case "zstack":
		n := &demy.ZStackNode{Align: parseZAlign(sn.Align)}
		for _, child := range sn.Children {
			c, err := sceneToNode(child)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		}
		return n, nil
	case "box":
		bc, err := parseColor(sn.BorderColor)
		if err != nil {
			return nil, err
		}
		bg, err := parseColor(sn.Background)
		if err != nil {
			return nil, err
		}
		n := &demy.BoxNode{
			Padding:      sn.Padding,
			BorderWidth:  sn.BorderWidth,
			BorderColor:  bc,
			Background:   bg,
			CornerRadius: sn.CornerRadius,
			Clip:         sn.Clip,
		}
		for _, child := range sn.Children {
			c, err := sceneToNode(child)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		}
		return n, nil
	case "frame":
		n := &demy.FrameNode{Width: sn.Width, Height: sn.Height}
		for _, child := range sn.Children {
			c, err := sceneToNode(child)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		}
		return n, nil
	case "spacer":
		return &demy.SpacerNode{Weight: sn.Weight}, nil
	case "weighted":
		var child demy.Node
		if len(sn.Children) > 0 {
			var err error
			child, err = sceneToNode(sn.Children[0])
			if err != nil {
				return nil, err
			}
		}
		return &demy.WeightedNode{Weight: sn.Weight, Child: child}, nil
	case "divider":
		return &demy.DividerNode{}, nil
	case "text":
		style, err := parseStyle(sn.Style)
		if err != nil {
			return nil, err
		}
		return &demy.TextNode{Text: sn.Text, Class: demy.StyleClass(sn.Class), Style: style}, nil
	case "image":
		return &demy.ImageNode{
			Key:   sn.ImageKey,
			Scale: parseScale(sn.Scale),
			Clip:  sn.Clip,
		}, nil
	case "barcode":
		return &demy.BarcodeNode{
			Content: sn.Content,
			Type:    parseBarcodeType(sn.BarcodeType),
			ECLevel: parseECLevel(sn.ECLevel),
		}, nil
	default:
		return nil, fmt.Errorf("scene: unknown node type %q", sn.Type)
	}
}

func parseStyle(ss *Style) (demy.Style, error) {
	if ss == nil {
		return demy.Style{}, nil
	}
	clr, err := parseColor(ss.Color)
	if err != nil {
		return demy.Style{}, err
	}
	return demy.Style{
		Font:       ss.Font,
		Size:       ss.Size,
		Weight:     parseWeight(ss.Weight),
		Color:      clr,
		Align:      parseTextAlign(ss.Align),
		LineHeight: ss.LineHeight,
		MaxLines:   ss.MaxLines,
		Overflow:   parseOverflow(ss.Overflow),
	}, nil
}

// --- color helpers ---

func colorStr(c color.Color) string {
	if c == nil {
		return ""
	}
	if rc, ok := c.(demy.RoleColor); ok {
		return rc.Name
	}
	r, g, b, a := c.RGBA()
	r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
	if a8 == 255 {
		return fmt.Sprintf("#%02x%02x%02x", r8, g8, b8)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", r8, g8, b8, a8)
}

func parseColor(s string) (color.Color, error) {
	if s == "" {
		return nil, nil
	}
	switch s {
	case "foreground", "background", "accent", "muted":
		return demy.RoleColor{Name: s}, nil
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		switch len(hex) {
		case 3:
			rv, _ := strconv.ParseUint(hex[0:1], 16, 8)
			gv, _ := strconv.ParseUint(hex[1:2], 16, 8)
			bv, _ := strconv.ParseUint(hex[2:3], 16, 8)
			return color.RGBA{R: uint8(rv<<8 | rv), G: uint8(gv<<8 | gv), B: uint8(bv<<8 | bv), A: 255}, nil
		case 6:
			rv, _ := strconv.ParseUint(hex[0:2], 16, 8)
			gv, _ := strconv.ParseUint(hex[2:4], 16, 8)
			bv, _ := strconv.ParseUint(hex[4:6], 16, 8)
			return color.RGBA{R: uint8(rv), G: uint8(gv), B: uint8(bv), A: 255}, nil
		case 8:
			rv, _ := strconv.ParseUint(hex[0:2], 16, 8)
			gv, _ := strconv.ParseUint(hex[2:4], 16, 8)
			bv, _ := strconv.ParseUint(hex[4:6], 16, 8)
			av, _ := strconv.ParseUint(hex[6:8], 16, 8)
			return color.RGBA{R: uint8(rv), G: uint8(gv), B: uint8(bv), A: uint8(av)}, nil
		}
	}
	return nil, fmt.Errorf("scene: unknown color %q", s)
}

// --- alignment helpers ---

func alignStr(a demy.Alignment) string {
	switch a {
	case demy.AlignCenter:
		return "center"
	case demy.AlignEnd:
		return "end"
	default:
		return "" // AlignStart is the default; omit
	}
}

func parseAlign(s string) demy.Alignment {
	switch s {
	case "center":
		return demy.AlignCenter
	case "end":
		return demy.AlignEnd
	default:
		return demy.AlignStart
	}
}

func zAlignStr(a demy.ZAlignment) string {
	switch a {
	case demy.Top:
		return "top"
	case demy.TopTrailing:
		return "top-trailing"
	case demy.Leading:
		return "leading"
	case demy.Center:
		return "center"
	case demy.Trailing:
		return "trailing"
	case demy.BottomLeading:
		return "bottom-leading"
	case demy.Bottom:
		return "bottom"
	case demy.BottomTrailing:
		return "bottom-trailing"
	default:
		return "" // TopLeading is the default; omit
	}
}

func parseZAlign(s string) demy.ZAlignment {
	switch s {
	case "top":
		return demy.Top
	case "top-trailing":
		return demy.TopTrailing
	case "leading":
		return demy.Leading
	case "center":
		return demy.Center
	case "trailing":
		return demy.Trailing
	case "bottom-leading":
		return demy.BottomLeading
	case "bottom":
		return demy.Bottom
	case "bottom-trailing":
		return demy.BottomTrailing
	default:
		return demy.TopLeading
	}
}

// --- text helpers ---

func textAlignStr(a demy.TextAlign) string {
	switch a {
	case demy.TextAlignCenter:
		return "center"
	case demy.TextAlignRight:
		return "right"
	default:
		return "" // left is default; omit
	}
}

func parseTextAlign(s string) demy.TextAlign {
	switch s {
	case "center":
		return demy.TextAlignCenter
	case "right":
		return demy.TextAlignRight
	default:
		return demy.TextAlignLeft
	}
}

func overflowStr(o demy.TextOverflow) string {
	switch o {
	case demy.TextOverflowClip:
		return "clip"
	case demy.TextOverflowEllipsis:
		return "ellipsis"
	default:
		return "" // wrap is default; omit
	}
}

func parseOverflow(s string) demy.TextOverflow {
	switch s {
	case "clip":
		return demy.TextOverflowClip
	case "ellipsis":
		return demy.TextOverflowEllipsis
	default:
		return demy.TextOverflowWrap
	}
}

// --- font weight helpers ---

func weightStr(w demy.FontStyle) string {
	italic := w&demy.FontItalic != 0
	var s string
	switch w &^ demy.FontItalic {
	case demy.FontThin:
		s = "thin"
	case demy.FontExtraLight:
		s = "extra-light"
	case demy.FontLight:
		s = "light"
	case demy.FontMedium:
		s = "medium"
	case demy.FontSemiBold:
		s = "semi-bold"
	case demy.FontBold:
		s = "bold"
	case demy.FontExtraBold:
		s = "extra-bold"
	case demy.FontBlack:
		s = "black"
	default:
		s = "" // regular; omit
	}
	if italic {
		if s == "" {
			return "italic"
		}
		return s + "-italic"
	}
	return s
}

func parseWeight(s string) demy.FontStyle {
	italic := strings.HasSuffix(s, "-italic") || s == "italic"
	base := strings.TrimSuffix(s, "-italic")
	var w demy.FontStyle
	switch base {
	case "thin":
		w = demy.FontThin
	case "extra-light":
		w = demy.FontExtraLight
	case "light":
		w = demy.FontLight
	case "medium":
		w = demy.FontMedium
	case "semi-bold":
		w = demy.FontSemiBold
	case "bold":
		w = demy.FontBold
	case "extra-bold":
		w = demy.FontExtraBold
	case "black":
		w = demy.FontBlack
	default:
		w = demy.FontRegular
	}
	if italic {
		return w | demy.FontItalic
	}
	return w
}

// --- image scale helpers ---

func scaleStr(s demy.ImageScale) string {
	switch s {
	case demy.ImageFill:
		return "fill"
	case demy.ImageStretch:
		return "stretch"
	default:
		return "" // fit is default; omit
	}
}

func parseScale(s string) demy.ImageScale {
	switch s {
	case "fill":
		return demy.ImageFill
	case "stretch":
		return demy.ImageStretch
	default:
		return demy.ImageFit
	}
}

// --- barcode helpers ---

func barcodeTypeStr(t demy.BarcodeType) string {
	switch t {
	default:
		return "" // BarcodeTypeQR is the default; omit
	}
}

func parseBarcodeType(s string) demy.BarcodeType {
	switch s {
	default:
		return demy.BarcodeTypeQR
	}
}

func ecLevelStr(l demy.ECLevel) string {
	switch l {
	case demy.ECLevelLow:
		return "low"
	case demy.ECLevelQuartile:
		return "quartile"
	case demy.ECLevelHigh:
		return "high"
	default: // ECLevelMedium is the default; omit
		return ""
	}
}

func parseECLevel(s string) demy.ECLevel {
	switch s {
	case "low":
		return demy.ECLevelLow
	case "quartile":
		return demy.ECLevelQuartile
	case "high":
		return demy.ECLevelHigh
	default:
		return demy.ECLevelMedium
	}
}
