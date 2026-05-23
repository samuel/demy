// Package scene provides JSON serialization for pate Node trees.
// Used to pass scene descriptions across process boundaries (e.g. WASM plugins).
//
// Color encoding:
//   - Role names ("foreground", "background", "accent", "muted") are stored as-is
//     and resolved against Config.Roles at render time via pate.RoleColor.
//   - All other colors are encoded as "#rrggbb" or "#rrggbbaa" hex strings and
//     parsed into color.RGBA at unmarshal time.
package scene

import (
	"encoding/json"
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/samuel/pate"
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
	Style        *Style  `json:"style,omitempty"`
	ImageKey     string  `json:"image_key,omitempty"`
	Scale        string  `json:"scale,omitempty"`
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

// Marshal serializes a Node tree to JSON.
func Marshal(n pate.Node) ([]byte, error) {
	sn, err := NoteToSchene(n)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sn)
}

// Unmarshal deserializes JSON to a Node tree. Role-name colors ("muted", etc.)
// are stored as pate.RoleColor and resolved against Config.Roles at render time.
// Image keys are resolved against Config.Images during the build pass.
func Unmarshal(data []byte) (pate.Node, error) {
	var sn Node
	if err := json.Unmarshal(data, &sn); err != nil {
		return nil, err
	}
	return sceneToNode(sn)
}

// --- marshal helpers ---

func NoteToSchene(n pate.Node) (Node, error) {
	if n == nil {
		return Node{Type: "spacer", Weight: 0}, nil
	}
	switch v := n.(type) {
	case *pate.VStackNode:
		sn := Node{
			Type:    "vstack",
			Gap:     v.Gap,
			Padding: v.Padding,
			Align:   alignStr(v.Align),
		}
		for _, child := range v.Children {
			cs, err := NoteToSchene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *pate.HStackNode:
		sn := Node{
			Type:    "hstack",
			Gap:     v.Gap,
			Padding: v.Padding,
			Align:   alignStr(v.Align),
		}
		for _, child := range v.Children {
			cs, err := NoteToSchene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *pate.ZStackNode:
		sn := Node{
			Type:  "zstack",
			Align: zAlignStr(v.Align),
		}
		for _, child := range v.Children {
			cs, err := NoteToSchene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *pate.BoxNode:
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
			cs, err := NoteToSchene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *pate.FrameNode:
		sn := Node{
			Type:   "frame",
			Width:  v.Width,
			Height: v.Height,
		}
		for _, child := range v.Children {
			cs, err := NoteToSchene(child)
			if err != nil {
				return Node{}, err
			}
			sn.Children = append(sn.Children, cs)
		}
		return sn, nil
	case *pate.SpacerNode:
		return Node{Type: "spacer", Weight: v.Weight}, nil
	case *pate.WeightedNode:
		var children []Node
		if v.Child != nil {
			cs, err := NoteToSchene(v.Child)
			if err != nil {
				return Node{}, err
			}
			children = []Node{cs}
		}
		return Node{Type: "weighted", Weight: v.Weight, Children: children}, nil
	case *pate.DividerNode:
		return Node{Type: "divider"}, nil
	case *pate.TextNode:
		sn := Node{
			Type:  "text",
			Text:  v.Text,
			Style: styleToScene(v.Style),
		}
		return sn, nil
	case *pate.ImageNode:
		return Node{
			Type:     "image",
			ImageKey: v.Key,
			Scale:    scaleStr(v.Scale),
			Clip:     v.Clip,
		}, nil
	default:
		return Node{}, fmt.Errorf("scene: unknown node type %T", n)
	}
}

func styleToScene(s pate.Style) *Style {
	if s == (pate.Style{}) {
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

func sceneToNode(sn Node) (pate.Node, error) {
	switch sn.Type {
	case "vstack":
		n := &pate.VStackNode{
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
		n := &pate.HStackNode{
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
		n := &pate.ZStackNode{Align: parseZAlign(sn.Align)}
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
		n := &pate.BoxNode{
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
		n := &pate.FrameNode{Width: sn.Width, Height: sn.Height}
		for _, child := range sn.Children {
			c, err := sceneToNode(child)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, c)
		}
		return n, nil
	case "spacer":
		return &pate.SpacerNode{Weight: sn.Weight}, nil
	case "weighted":
		var child pate.Node
		if len(sn.Children) > 0 {
			var err error
			child, err = sceneToNode(sn.Children[0])
			if err != nil {
				return nil, err
			}
		}
		return &pate.WeightedNode{Weight: sn.Weight, Child: child}, nil
	case "divider":
		return &pate.DividerNode{}, nil
	case "text":
		style, err := parseStyle(sn.Style)
		if err != nil {
			return nil, err
		}
		return &pate.TextNode{Text: sn.Text, Style: style}, nil
	case "image":
		return &pate.ImageNode{
			Key:   sn.ImageKey,
			Scale: parseScale(sn.Scale),
			Clip:  sn.Clip,
		}, nil
	default:
		return nil, fmt.Errorf("scene: unknown node type %q", sn.Type)
	}
}

func parseStyle(ss *Style) (pate.Style, error) {
	if ss == nil {
		return pate.Style{}, nil
	}
	clr, err := parseColor(ss.Color)
	if err != nil {
		return pate.Style{}, err
	}
	return pate.Style{
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
	if rc, ok := c.(pate.RoleColor); ok {
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
		return pate.RoleColor{Name: s}, nil
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

func alignStr(a pate.Alignment) string {
	switch a {
	case pate.AlignCenter:
		return "center"
	case pate.AlignEnd:
		return "end"
	default:
		return "" // AlignStart is the default; omit
	}
}

func parseAlign(s string) pate.Alignment {
	switch s {
	case "center":
		return pate.AlignCenter
	case "end":
		return pate.AlignEnd
	default:
		return pate.AlignStart
	}
}

func zAlignStr(a pate.ZAlignment) string {
	switch a {
	case pate.Top:
		return "top"
	case pate.TopTrailing:
		return "top-trailing"
	case pate.Leading:
		return "leading"
	case pate.Center:
		return "center"
	case pate.Trailing:
		return "trailing"
	case pate.BottomLeading:
		return "bottom-leading"
	case pate.Bottom:
		return "bottom"
	case pate.BottomTrailing:
		return "bottom-trailing"
	default:
		return "" // TopLeading is the default; omit
	}
}

func parseZAlign(s string) pate.ZAlignment {
	switch s {
	case "top":
		return pate.Top
	case "top-trailing":
		return pate.TopTrailing
	case "leading":
		return pate.Leading
	case "center":
		return pate.Center
	case "trailing":
		return pate.Trailing
	case "bottom-leading":
		return pate.BottomLeading
	case "bottom":
		return pate.Bottom
	case "bottom-trailing":
		return pate.BottomTrailing
	default:
		return pate.TopLeading
	}
}

// --- text helpers ---

func textAlignStr(a pate.TextAlign) string {
	switch a {
	case pate.TextAlignCenter:
		return "center"
	case pate.TextAlignRight:
		return "right"
	default:
		return "" // left is default; omit
	}
}

func parseTextAlign(s string) pate.TextAlign {
	switch s {
	case "center":
		return pate.TextAlignCenter
	case "right":
		return pate.TextAlignRight
	default:
		return pate.TextAlignLeft
	}
}

func overflowStr(o pate.TextOverflow) string {
	switch o {
	case pate.TextOverflowClip:
		return "clip"
	case pate.TextOverflowEllipsis:
		return "ellipsis"
	default:
		return "" // wrap is default; omit
	}
}

func parseOverflow(s string) pate.TextOverflow {
	switch s {
	case "clip":
		return pate.TextOverflowClip
	case "ellipsis":
		return pate.TextOverflowEllipsis
	default:
		return pate.TextOverflowWrap
	}
}

// --- font weight helpers ---

func weightStr(w pate.FontStyle) string {
	italic := w&pate.FontItalic != 0
	var s string
	switch w &^ pate.FontItalic {
	case pate.FontThin:
		s = "thin"
	case pate.FontExtraLight:
		s = "extra-light"
	case pate.FontLight:
		s = "light"
	case pate.FontMedium:
		s = "medium"
	case pate.FontSemiBold:
		s = "semi-bold"
	case pate.FontBold:
		s = "bold"
	case pate.FontExtraBold:
		s = "extra-bold"
	case pate.FontBlack:
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

func parseWeight(s string) pate.FontStyle {
	italic := strings.HasSuffix(s, "-italic") || s == "italic"
	base := strings.TrimSuffix(s, "-italic")
	var w pate.FontStyle
	switch base {
	case "thin":
		w = pate.FontThin
	case "extra-light":
		w = pate.FontExtraLight
	case "light":
		w = pate.FontLight
	case "medium":
		w = pate.FontMedium
	case "semi-bold":
		w = pate.FontSemiBold
	case "bold":
		w = pate.FontBold
	case "extra-bold":
		w = pate.FontExtraBold
	case "black":
		w = pate.FontBlack
	default:
		w = pate.FontRegular
	}
	if italic {
		return w | pate.FontItalic
	}
	return w
}

// --- image scale helpers ---

func scaleStr(s pate.ImageScale) string {
	switch s {
	case pate.ImageFill:
		return "fill"
	case pate.ImageStretch:
		return "stretch"
	default:
		return "" // fit is default; omit
	}
}

func parseScale(s string) pate.ImageScale {
	switch s {
	case "fill":
		return pate.ImageFill
	case "stretch":
		return pate.ImageStretch
	default:
		return pate.ImageFit
	}
}
