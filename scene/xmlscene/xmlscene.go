// Package xmlscene provides XML serialization and template rendering for pate Node trees.
//
// The format maps element tag names to node types, attributes to layout/style
// properties, and child elements to child nodes — no "type" discriminator field,
// no "children" wrapper. This makes it suitable for Go's text/template:
//
//	<vstack gap="4" padding="8">
//	  <text size="{{.TitleSize}}" color="foreground">{{.Title}}</text>
//	  {{range .Items}}
//	  <hstack padding="4">
//	    <text size="12">{{.Name}}</text>
//	    <spacer/>
//	    <text size="12" align="right">{{.Value}}</text>
//	  </hstack>
//	  {{end}}
//	</vstack>
//
// Color encoding matches the scene package: role names ("foreground", "background",
// "accent", "muted") are stored as-is and resolved at render time via pate.RoleColor;
// other colors are "#rrggbb" or "#rrggbbaa" hex strings.
package xmlscene

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"io"
	"strconv"
	"strings"
	"text/template"

	"github.com/samuel/pate"
)

// Marshal serializes a Node tree to indented XML.
func Marshal(n pate.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "\t")
	if err := marshalNode(enc, n); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("xmlscene: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("xmlscene: %w", err)
	}
	return buf.Bytes(), nil
}

// Unmarshal deserializes XML to a Node tree.
func Unmarshal(data []byte) (pate.Node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("xmlscene: empty document")
		}
		if err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return unmarshalElement(dec, start)
		}
	}
}

// Execute renders a text/template that produces XML and unmarshals the result.
func Execute(tmplSrc []byte, data any) (pate.Node, error) {
	t, err := template.New("scene").Parse(string(tmplSrc))
	if err != nil {
		return nil, fmt.Errorf("xmlscene: template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("xmlscene: template execute: %w", err)
	}
	return Unmarshal(buf.Bytes())
}

// --- marshal ---

func marshalNode(enc *xml.Encoder, n pate.Node) error {
	if n == nil {
		start := xml.StartElement{Name: xml.Name{Local: "spacer"}}
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	}
	switch v := n.(type) {
	case *pate.VStackNode:
		start := xml.StartElement{Name: xml.Name{Local: "vstack"}}
		appendFloat(&start, "gap", v.Gap)
		appendFloat(&start, "padding", v.Padding)
		appendStr(&start, "align", alignStr(v.Align))
		return marshalContainer(enc, start, v.Children)
	case *pate.HStackNode:
		start := xml.StartElement{Name: xml.Name{Local: "hstack"}}
		appendFloat(&start, "gap", v.Gap)
		appendFloat(&start, "padding", v.Padding)
		appendStr(&start, "align", alignStr(v.Align))
		return marshalContainer(enc, start, v.Children)
	case *pate.ZStackNode:
		start := xml.StartElement{Name: xml.Name{Local: "zstack"}}
		appendStr(&start, "align", zAlignStr(v.Align))
		return marshalContainer(enc, start, v.Children)
	case *pate.BoxNode:
		start := xml.StartElement{Name: xml.Name{Local: "box"}}
		appendFloat(&start, "padding", v.Padding)
		appendFloat(&start, "border-width", v.BorderWidth)
		appendStr(&start, "border-color", colorStr(v.BorderColor))
		appendStr(&start, "background", colorStr(v.Background))
		appendFloat(&start, "corner-radius", v.CornerRadius)
		appendBool(&start, "clip", v.Clip)
		return marshalContainer(enc, start, v.Children)
	case *pate.FrameNode:
		start := xml.StartElement{Name: xml.Name{Local: "frame"}}
		appendFloat(&start, "width", v.Width)
		appendFloat(&start, "height", v.Height)
		return marshalContainer(enc, start, v.Children)
	case *pate.SpacerNode:
		start := xml.StartElement{Name: xml.Name{Local: "spacer"}}
		appendFloat(&start, "weight", v.Weight)
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	case *pate.WeightedNode:
		start := xml.StartElement{Name: xml.Name{Local: "weighted"}}
		appendFloat(&start, "weight", v.Weight)
		var children []pate.Node
		if v.Child != nil {
			children = []pate.Node{v.Child}
		}
		return marshalContainer(enc, start, children)
	case *pate.DividerNode:
		start := xml.StartElement{Name: xml.Name{Local: "divider"}}
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	case *pate.TextNode:
		start := xml.StartElement{Name: xml.Name{Local: "text"}}
		appendStyleAttrs(&start, v.Style)
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		if v.Text != "" {
			if err := enc.EncodeToken(xml.CharData(v.Text)); err != nil {
				return err
			}
		}
		return enc.EncodeToken(start.End())
	case *pate.ImageNode:
		start := xml.StartElement{Name: xml.Name{Local: "image"}}
		appendStr(&start, "key", v.Key)
		appendStr(&start, "scale", scaleStr(v.Scale))
		appendBool(&start, "clip", v.Clip)
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	default:
		return fmt.Errorf("xmlscene: unknown node type %T", n)
	}
}

func marshalContainer(enc *xml.Encoder, start xml.StartElement, children []pate.Node) error {
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, child := range children {
		if err := marshalNode(enc, child); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

func appendFloat(start *xml.StartElement, name string, v float64) {
	if v == 0 {
		return
	}
	start.Attr = append(start.Attr, xml.Attr{
		Name:  xml.Name{Local: name},
		Value: strconv.FormatFloat(v, 'f', -1, 64),
	})
}

func appendStr(start *xml.StartElement, name, value string) {
	if value == "" {
		return
	}
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func appendBool(start *xml.StartElement, name string, value bool) {
	if !value {
		return
	}
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: "true"})
}

func appendInt(start *xml.StartElement, name string, v int) {
	if v == 0 {
		return
	}
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: strconv.Itoa(v)})
}

func appendStyleAttrs(start *xml.StartElement, s pate.Style) {
	appendStr(start, "font", s.Font)
	appendFloat(start, "size", s.Size)
	appendStr(start, "weight", weightStr(s.Weight))
	appendStr(start, "color", colorStr(s.Color))
	appendStr(start, "align", textAlignStr(s.Align))
	appendFloat(start, "line-height", s.LineHeight)
	appendInt(start, "max-lines", s.MaxLines)
	appendStr(start, "overflow", overflowStr(s.Overflow))
}

// --- unmarshal ---

func unmarshalElement(dec *xml.Decoder, start xml.StartElement) (pate.Node, error) {
	attrs := attrMap(start.Attr)
	switch start.Name.Local {
	case "vstack":
		n := &pate.VStackNode{
			Gap:     floatAttr(attrs, "gap"),
			Padding: floatAttr(attrs, "padding"),
			Align:   parseAlign(attrs["align"]),
		}
		children, err := unmarshalChildren(dec)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "hstack":
		n := &pate.HStackNode{
			Gap:     floatAttr(attrs, "gap"),
			Padding: floatAttr(attrs, "padding"),
			Align:   parseAlign(attrs["align"]),
		}
		children, err := unmarshalChildren(dec)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "zstack":
		n := &pate.ZStackNode{Align: parseZAlign(attrs["align"])}
		children, err := unmarshalChildren(dec)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "box":
		bc, err := parseColor(attrs["border-color"])
		if err != nil {
			return nil, err
		}
		bg, err := parseColor(attrs["background"])
		if err != nil {
			return nil, err
		}
		n := &pate.BoxNode{
			Padding:      floatAttr(attrs, "padding"),
			BorderWidth:  floatAttr(attrs, "border-width"),
			BorderColor:  bc,
			Background:   bg,
			CornerRadius: floatAttr(attrs, "corner-radius"),
			Clip:         boolAttr(attrs, "clip"),
		}
		children, err := unmarshalChildren(dec)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "frame":
		n := &pate.FrameNode{
			Width:  floatAttr(attrs, "width"),
			Height: floatAttr(attrs, "height"),
		}
		children, err := unmarshalChildren(dec)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "spacer":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		return &pate.SpacerNode{Weight: floatAttr(attrs, "weight")}, nil
	case "weighted":
		children, err := unmarshalChildren(dec)
		if err != nil {
			return nil, err
		}
		var child pate.Node
		if len(children) > 0 {
			child = children[0]
		}
		return &pate.WeightedNode{Weight: floatAttr(attrs, "weight"), Child: child}, nil
	case "divider":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		return &pate.DividerNode{}, nil
	case "text":
		text, err := unmarshalTextContent(dec)
		if err != nil {
			return nil, err
		}
		style, err := styleFromAttrs(attrs)
		if err != nil {
			return nil, err
		}
		return &pate.TextNode{Text: text, Style: style}, nil
	case "image":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		return &pate.ImageNode{
			Key:   attrs["key"],
			Scale: parseScale(attrs["scale"]),
			Clip:  boolAttr(attrs, "clip"),
		}, nil
	default:
		return nil, fmt.Errorf("xmlscene: unknown element %q", start.Name.Local)
	}
}

func unmarshalChildren(dec *xml.Decoder) ([]pate.Node, error) {
	var children []pate.Node
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := unmarshalElement(dec, t)
			if err != nil {
				return nil, err
			}
			children = append(children, child)
		case xml.EndElement:
			return children, nil
		}
	}
}

func unmarshalTextContent(dec *xml.Decoder) (string, error) {
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("xmlscene: %w", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			return sb.String(), nil
		case xml.StartElement:
			if err := dec.Skip(); err != nil {
				return "", fmt.Errorf("xmlscene: %w", err)
			}
		}
	}
}

func attrMap(attrs []xml.Attr) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Name.Local] = a.Value
	}
	return m
}

func floatAttr(attrs map[string]string, name string) float64 {
	s := attrs[name]
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func boolAttr(attrs map[string]string, name string) bool {
	return attrs[name] == "true"
}

func styleFromAttrs(attrs map[string]string) (pate.Style, error) {
	clr, err := parseColor(attrs["color"])
	if err != nil {
		return pate.Style{}, err
	}
	var maxLines int
	if s := attrs["max-lines"]; s != "" {
		maxLines, _ = strconv.Atoi(s)
	}
	return pate.Style{
		Font:       attrs["font"],
		Size:       floatAttr(attrs, "size"),
		Weight:     parseWeight(attrs["weight"]),
		Color:      clr,
		Align:      parseTextAlign(attrs["align"]),
		LineHeight: floatAttr(attrs, "line-height"),
		MaxLines:   maxLines,
		Overflow:   parseOverflow(attrs["overflow"]),
	}, nil
}

// --- color ---

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
	return nil, fmt.Errorf("xmlscene: unknown color %q", s)
}

// --- alignment ---

func alignStr(a pate.Alignment) string {
	switch a {
	case pate.AlignCenter:
		return "center"
	case pate.AlignEnd:
		return "end"
	default:
		return ""
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
		return ""
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

// --- text style ---

func textAlignStr(a pate.TextAlign) string {
	switch a {
	case pate.TextAlignCenter:
		return "center"
	case pate.TextAlignRight:
		return "right"
	default:
		return ""
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
		return ""
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

// --- font weight ---

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
		s = ""
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

// --- image scale ---

func scaleStr(s pate.ImageScale) string {
	switch s {
	case pate.ImageFill:
		return "fill"
	case pate.ImageStretch:
		return "stretch"
	default:
		return ""
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
