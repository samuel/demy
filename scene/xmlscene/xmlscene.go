// Package xmlscene provides XML serialization and template rendering for demy Node trees.
//
// Element names map to node types, attributes to layout and style properties, and
// child elements to child nodes, which makes the format convenient to generate with
// text/template (see Execute):
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
// Colors are role names ("foreground", "background", "accent", "muted"), which
// resolve against Config.Roles at render time, or "#rgb", "#rrggbb", or "#rrggbbaa"
// hex strings.
//
// # Physical units
//
// Dimension attributes (width, height, padding, gap, size, etc.) accept an optional
// unit suffix:
//
//   - Bare number or "px" → logical pixels (e.g. "240" or "240px")
//   - "mm" → millimeters (e.g. "50mm")
//   - "pt" → points, 1/72 inch (e.g. "12pt")
//
// "mm" and "pt" are converted using Config.DPMM and require Config.PhysicalWidthMM
// to be set; otherwise they resolve to 0. Marshal always writes bare pixel values.
package xmlscene

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/samuel/demy"
)

// Marshal serializes a Node tree to indented XML.
func Marshal(n demy.Node) ([]byte, error) {
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

// Unmarshal deserializes XML to a Node tree. Pass a demy.Config with physical
// dimensions set to enable "mm" and "pt" unit suffixes in dimension attributes.
func Unmarshal(data []byte, cfg demy.Config) (demy.Node, error) {
	uctx := newUnmarshalCtx(cfg)
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
			return unmarshalElement(dec, start, uctx)
		}
	}
}

// Execute executes a text/template that produces XML and unmarshals the result
// with Unmarshal. Template output is not XML-escaped; data containing "<" or "&"
// must be escaped by the template (e.g. with the html function).
func Execute(tmplSrc []byte, data any, cfg demy.Config) (demy.Node, error) {
	t, err := template.New("scene").Parse(string(tmplSrc))
	if err != nil {
		return nil, fmt.Errorf("xmlscene: template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("xmlscene: template execute: %w", err)
	}
	return Unmarshal(buf.Bytes(), cfg)
}

// MarshalScene serializes a Node tree and named style definitions to an indented
// <scene> envelope:
//
//	<scene>
//	  <styles>
//	    <style name="h1" font="sans" size="24" weight="bold"/>
//	  </styles>
//	  <vstack>...</vstack>
//	</scene>
func MarshalScene(n demy.Node, styles map[string]demy.Style) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "\t")

	sceneElem := xml.StartElement{Name: xml.Name{Local: "scene"}}
	if err := enc.EncodeToken(sceneElem); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("xmlscene: %w", err)
	}

	if len(styles) > 0 {
		stylesElem := xml.StartElement{Name: xml.Name{Local: "styles"}}
		if err := enc.EncodeToken(stylesElem); err != nil {
			_ = enc.Close()
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		for _, k := range slices.Sorted(maps.Keys(styles)) {
			styleElem := xml.StartElement{Name: xml.Name{Local: "style"}}
			appendStr(&styleElem, "name", k)
			appendStyleAttrs(&styleElem, styles[k])
			if err := enc.EncodeToken(styleElem); err != nil {
				_ = enc.Close()
				return nil, fmt.Errorf("xmlscene: %w", err)
			}
			if err := enc.EncodeToken(styleElem.End()); err != nil {
				_ = enc.Close()
				return nil, fmt.Errorf("xmlscene: %w", err)
			}
		}
		if err := enc.EncodeToken(stylesElem.End()); err != nil {
			_ = enc.Close()
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
	}

	if err := marshalNode(enc, n); err != nil {
		_ = enc.Close()
		return nil, err
	}

	if err := enc.EncodeToken(sceneElem.End()); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("xmlscene: %w", err)
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

// UnmarshalScene deserializes XML to a Node tree and style definitions.
// The document may be a bare node element or a <scene> wrapper containing
// <styles> and a root element. Merge the returned styles into Config.Styles before
// rendering so that class references resolve. Pass a demy.Config with physical
// dimensions set to enable "mm" and "pt" unit suffixes.
func UnmarshalScene(data []byte, cfg demy.Config) (demy.Node, map[string]demy.Style, error) {
	uctx := newUnmarshalCtx(cfg)
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil, nil, errors.New("xmlscene: empty document")
		}
		if err != nil {
			return nil, nil, fmt.Errorf("xmlscene: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local == "scene" {
				return unmarshalScene(dec, uctx)
			}
			n, err := unmarshalElement(dec, start, uctx)
			return n, nil, err
		}
	}
}

// ExecuteScene executes a text/template that produces XML and unmarshals the result
// with UnmarshalScene.
func ExecuteScene(tmplSrc []byte, data any, cfg demy.Config) (demy.Node, map[string]demy.Style, error) {
	t, err := template.New("scene").Parse(string(tmplSrc))
	if err != nil {
		return nil, nil, fmt.Errorf("xmlscene: template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, nil, fmt.Errorf("xmlscene: template execute: %w", err)
	}
	return UnmarshalScene(buf.Bytes(), cfg)
}

// --- marshal ---

func marshalNode(enc *xml.Encoder, n demy.Node) error {
	if n == nil {
		start := xml.StartElement{Name: xml.Name{Local: "spacer"}}
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	}
	switch v := n.(type) {
	case *demy.VStackNode:
		start := xml.StartElement{Name: xml.Name{Local: "vstack"}}
		appendFloat(&start, "gap", v.Gap)
		appendFloat(&start, "padding", v.Padding)
		appendStr(&start, "align", alignStr(v.Align))
		return marshalContainer(enc, start, v.Children)
	case *demy.HStackNode:
		start := xml.StartElement{Name: xml.Name{Local: "hstack"}}
		appendFloat(&start, "gap", v.Gap)
		appendFloat(&start, "padding", v.Padding)
		appendStr(&start, "align", alignStr(v.Align))
		return marshalContainer(enc, start, v.Children)
	case *demy.ZStackNode:
		start := xml.StartElement{Name: xml.Name{Local: "zstack"}}
		appendStr(&start, "align", zAlignStr(v.Align))
		return marshalContainer(enc, start, v.Children)
	case *demy.BoxNode:
		start := xml.StartElement{Name: xml.Name{Local: "box"}}
		appendFloat(&start, "padding", v.Padding)
		appendFloat(&start, "border-width", v.BorderWidth)
		appendStr(&start, "border-color", colorStr(v.BorderColor))
		appendStr(&start, "background", colorStr(v.Background))
		appendFloat(&start, "corner-radius", v.CornerRadius)
		appendBool(&start, "clip", v.Clip)
		return marshalContainer(enc, start, v.Children)
	case *demy.FrameNode:
		start := xml.StartElement{Name: xml.Name{Local: "frame"}}
		appendFloat(&start, "width", v.Width)
		appendFloat(&start, "height", v.Height)
		return marshalContainer(enc, start, v.Children)
	case *demy.SpacerNode:
		start := xml.StartElement{Name: xml.Name{Local: "spacer"}}
		appendFloat(&start, "weight", v.Weight)
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	case *demy.WeightedNode:
		start := xml.StartElement{Name: xml.Name{Local: "weighted"}}
		appendFloat(&start, "weight", v.Weight)
		var children []demy.Node
		if v.Child != nil {
			children = []demy.Node{v.Child}
		}
		return marshalContainer(enc, start, children)
	case *demy.DividerNode:
		start := xml.StartElement{Name: xml.Name{Local: "divider"}}
		appendFloat(&start, "size", v.Size)
		appendStr(&start, "color", colorStr(v.Color))
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	case *demy.TextNode:
		start := xml.StartElement{Name: xml.Name{Local: "text"}}
		appendStr(&start, "class", string(v.Class))
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
	case *demy.ImageNode:
		start := xml.StartElement{Name: xml.Name{Local: "image"}}
		appendStr(&start, "key", v.Key)
		appendStr(&start, "scale", scaleStr(v.Scale))
		appendStr(&start, "dither", ditherStr(v.Dither))
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	case *demy.BarcodeNode:
		start := xml.StartElement{Name: xml.Name{Local: "barcode"}}
		appendStr(&start, "content", v.Content)
		appendStr(&start, "type", barcodeTypeStr(v.Type))
		appendStr(&start, "ec-level", ecLevelStr(v.ECLevel))
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		return enc.EncodeToken(start.End())
	case *demy.GridNode:
		start := xml.StartElement{Name: xml.Name{Local: "grid"}}
		appendFloat(&start, "padding", v.Padding)
		appendFloat(&start, "row-gap", v.RowGap)
		appendFloat(&start, "col-gap", v.ColGap)
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		if len(v.Columns) > 0 {
			colsElem := xml.StartElement{Name: xml.Name{Local: "columns"}}
			if err := enc.EncodeToken(colsElem); err != nil {
				return err
			}
			for _, t := range v.Columns {
				if err := marshalTrack(enc, t); err != nil {
					return err
				}
			}
			if err := enc.EncodeToken(colsElem.End()); err != nil {
				return err
			}
		}
		if len(v.Rows) > 0 {
			rowsElem := xml.StartElement{Name: xml.Name{Local: "rows"}}
			if err := enc.EncodeToken(rowsElem); err != nil {
				return err
			}
			for _, t := range v.Rows {
				if err := marshalTrack(enc, t); err != nil {
					return err
				}
			}
			if err := enc.EncodeToken(rowsElem.End()); err != nil {
				return err
			}
		}
		for _, child := range v.Children {
			if err := marshalNode(enc, child); err != nil {
				return err
			}
		}
		return enc.EncodeToken(start.End())
	case *demy.GridRowNode:
		return marshalContainer(enc, xml.StartElement{Name: xml.Name{Local: "row"}}, v.Children)
	default:
		return fmt.Errorf("xmlscene: unknown node type %T", n)
	}
}

func marshalTrack(enc *xml.Encoder, t demy.GridTrack) error {
	var elem xml.StartElement
	switch {
	case t.IsFixed():
		elem = xml.StartElement{Name: xml.Name{Local: "fixed"}}
		appendFloat(&elem, "size", t.Size())
	case t.IsFr():
		elem = xml.StartElement{Name: xml.Name{Local: "fr"}}
		appendFloat(&elem, "weight", t.Weight())
	default:
		elem = xml.StartElement{Name: xml.Name{Local: "auto"}}
	}
	if err := enc.EncodeToken(elem); err != nil {
		return err
	}
	return enc.EncodeToken(elem.End())
}

func marshalContainer(enc *xml.Encoder, start xml.StartElement, children []demy.Node) error {
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

func appendStyleAttrs(start *xml.StartElement, s demy.Style) {
	appendStr(start, "font", s.Font)
	appendFloat(start, "size", s.Size)
	appendStr(start, "weight", weightStr(s.Weight))
	appendStr(start, "color", colorStr(s.Color))
	appendStr(start, "align", textAlignStr(s.Align))
	appendFloat(start, "line-height", s.LineHeight)
	appendInt(start, "max-lines", s.MaxLines)
	appendStr(start, "overflow", overflowStr(s.Overflow))
}

// --- unmarshal context ---

// unmarshalCtx carries per-parse state for resolving physical unit suffixes.
// dpmm is 0 when the Config lacks physical dimensions.
// maxDepth bounds element nesting so deeply nested input can't exhaust the stack.
const maxDepth = 1000

type unmarshalCtx struct {
	dpmm  float64
	depth int // element nesting depth
}

func newUnmarshalCtx(cfg demy.Config) unmarshalCtx {
	if cfg.PhysicalWidthMM == 0 {
		return unmarshalCtx{}
	}
	return unmarshalCtx{dpmm: cfg.DPMM()}
}

func (uctx unmarshalCtx) dim(attrs map[string]string, name string) float64 {
	return parseDim(attrs[name], uctx.dpmm)
}

// parseDim converts a dimension string with an optional "mm", "pt", or "px"
// suffix to pixels.
func parseDim(s string, dpmm float64) float64 {
	if s == "" {
		return 0
	}
	switch {
	case strings.HasSuffix(s, "mm"):
		v, _ := strconv.ParseFloat(s[:len(s)-2], 64)
		return v * dpmm
	case strings.HasSuffix(s, "pt"):
		v, _ := strconv.ParseFloat(s[:len(s)-2], 64)
		return v * dpmm * 25.4 / 72
	case strings.HasSuffix(s, "px"):
		v, _ := strconv.ParseFloat(s[:len(s)-2], 64)
		return v
	default:
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
}

// --- unmarshal ---

func unmarshalElement(dec *xml.Decoder, start xml.StartElement, uctx unmarshalCtx) (demy.Node, error) {
	uctx.depth++
	if uctx.depth > maxDepth {
		return nil, fmt.Errorf("xmlscene: elements nested deeper than %d", maxDepth)
	}
	attrs := attrMap(start.Attr)
	switch start.Name.Local {
	case "vstack":
		n := &demy.VStackNode{
			Gap:     uctx.dim(attrs, "gap"),
			Padding: uctx.dim(attrs, "padding"),
			Align:   parseAlign(attrs["align"]),
		}
		children, err := unmarshalChildren(dec, uctx)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "hstack":
		n := &demy.HStackNode{
			Gap:     uctx.dim(attrs, "gap"),
			Padding: uctx.dim(attrs, "padding"),
			Align:   parseAlign(attrs["align"]),
		}
		children, err := unmarshalChildren(dec, uctx)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "zstack":
		n := &demy.ZStackNode{Align: parseZAlign(attrs["align"])}
		children, err := unmarshalChildren(dec, uctx)
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
		n := &demy.BoxNode{
			Padding:      uctx.dim(attrs, "padding"),
			BorderWidth:  uctx.dim(attrs, "border-width"),
			BorderColor:  bc,
			Background:   bg,
			CornerRadius: uctx.dim(attrs, "corner-radius"),
			Clip:         boolAttr(attrs, "clip"),
		}
		children, err := unmarshalChildren(dec, uctx)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "frame":
		n := &demy.FrameNode{
			Width:  uctx.dim(attrs, "width"),
			Height: uctx.dim(attrs, "height"),
		}
		children, err := unmarshalChildren(dec, uctx)
		if err != nil {
			return nil, err
		}
		n.Children = children
		return n, nil
	case "spacer":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		return &demy.SpacerNode{Weight: floatAttr(attrs, "weight")}, nil
	case "weighted":
		children, err := unmarshalChildren(dec, uctx)
		if err != nil {
			return nil, err
		}
		var child demy.Node
		if len(children) > 0 {
			child = children[0]
		}
		return &demy.WeightedNode{Weight: floatAttr(attrs, "weight"), Child: child}, nil
	case "divider":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		clr, err := parseColor(attrs["color"])
		if err != nil {
			return nil, fmt.Errorf("xmlscene divider color: %w", err)
		}
		return &demy.DividerNode{Color: clr, Size: uctx.dim(attrs, "size")}, nil
	case "text":
		text, err := unmarshalTextContent(dec)
		if err != nil {
			return nil, err
		}
		style, err := styleFromAttrs(attrs, uctx)
		if err != nil {
			return nil, err
		}
		return &demy.TextNode{Text: text, Class: demy.StyleClass(attrs["class"]), Style: style}, nil
	case "image":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		return &demy.ImageNode{
			Key:    attrs["key"],
			Scale:  parseScale(attrs["scale"]),
			Dither: parseDither(attrs["dither"]),
		}, nil
	case "barcode":
		if err := dec.Skip(); err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		return &demy.BarcodeNode{
			Content: attrs["content"],
			Type:    parseBarcodeType(attrs["type"]),
			ECLevel: parseECLevel(attrs["ec-level"]),
		}, nil
	case "grid":
		n := &demy.GridNode{
			Padding: uctx.dim(attrs, "padding"),
			RowGap:  uctx.dim(attrs, "row-gap"),
			ColGap:  uctx.dim(attrs, "col-gap"),
		}
		var err error
		n.Columns, n.Rows, n.Children, err = unmarshalGridContents(dec, uctx)
		if err != nil {
			return nil, err
		}
		return n, nil
	case "row":
		children, err := unmarshalChildren(dec, uctx)
		if err != nil {
			return nil, err
		}
		return &demy.GridRowNode{Children: children}, nil
	default:
		return nil, fmt.Errorf("xmlscene: unknown element %q", start.Name.Local)
	}
}

// unmarshalGridContents reads the children of a <grid> element, separating
// <columns> and <rows> sub-elements from child nodes.
func unmarshalGridContents(dec *xml.Decoder, uctx unmarshalCtx) (columns, rows []demy.GridTrack, children []demy.Node, err error) {
	for {
		tok, tokErr := dec.Token()
		if tokErr != nil {
			return nil, nil, nil, fmt.Errorf("xmlscene: %w", tokErr)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "columns":
				columns, err = unmarshalTracks(dec, uctx)
				if err != nil {
					return nil, nil, nil, err
				}
			case "rows":
				rows, err = unmarshalTracks(dec, uctx)
				if err != nil {
					return nil, nil, nil, err
				}
			default:
				child, childErr := unmarshalElement(dec, t, uctx)
				if childErr != nil {
					return nil, nil, nil, childErr
				}
				children = append(children, child)
			}
		case xml.EndElement:
			return columns, rows, children, nil
		}
	}
}

// unmarshalTracks reads <auto/>, <fixed size="..."/>, or <fr weight="..."/> elements.
func unmarshalTracks(dec *xml.Decoder, uctx unmarshalCtx) ([]demy.GridTrack, error) {
	var tracks []demy.GridTrack
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			a := attrMap(t.Attr)
			switch t.Name.Local {
			case "fixed":
				tracks = append(tracks, demy.GridFixed(uctx.dim(a, "size")))
			case "fr":
				tracks = append(tracks, demy.GridFr(floatAttr(a, "weight")))
			default: // "auto" or unknown
				tracks = append(tracks, demy.GridAuto())
			}
			if err := dec.Skip(); err != nil {
				return nil, fmt.Errorf("xmlscene: %w", err)
			}
		case xml.EndElement:
			return tracks, nil
		}
	}
}

func unmarshalStyles(dec *xml.Decoder, uctx unmarshalCtx) (map[string]demy.Style, error) {
	styles := make(map[string]demy.Style)
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "style" {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("xmlscene: %w", err)
				}
				continue
			}
			attrs := attrMap(t.Attr)
			name := attrs["name"]
			if name == "" {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("xmlscene: %w", err)
				}
				continue
			}
			style, err := styleFromAttrs(attrs, uctx)
			if err != nil {
				return nil, err
			}
			styles[name] = style
			if err := dec.Skip(); err != nil {
				return nil, fmt.Errorf("xmlscene: %w", err)
			}
		case xml.EndElement:
			return styles, nil
		}
	}
}

func unmarshalScene(dec *xml.Decoder, uctx unmarshalCtx) (demy.Node, map[string]demy.Style, error) {
	var (
		root   demy.Node
		styles map[string]demy.Style
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil, fmt.Errorf("xmlscene: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "styles" {
				s, err := unmarshalStyles(dec, uctx)
				if err != nil {
					return nil, nil, err
				}
				styles = s
			} else {
				n, err := unmarshalElement(dec, t, uctx)
				if err != nil {
					return nil, nil, err
				}
				root = n
			}
		case xml.EndElement:
			return root, styles, nil
		}
	}
}

func unmarshalChildren(dec *xml.Decoder, uctx unmarshalCtx) ([]demy.Node, error) {
	var children []demy.Node
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("xmlscene: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := unmarshalElement(dec, t, uctx)
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

func styleFromAttrs(attrs map[string]string, uctx unmarshalCtx) (demy.Style, error) {
	clr, err := parseColor(attrs["color"])
	if err != nil {
		return demy.Style{}, err
	}
	var maxLines int
	if s := attrs["max-lines"]; s != "" {
		maxLines, _ = strconv.Atoi(s)
	}
	return demy.Style{
		Font:       attrs["font"],
		Size:       uctx.dim(attrs, "size"),
		Weight:     parseWeight(attrs["weight"]),
		Color:      clr,
		Align:      parseTextAlign(attrs["align"]),
		LineHeight: uctx.dim(attrs, "line-height"),
		MaxLines:   maxLines,
		Overflow:   parseOverflow(attrs["overflow"]),
	}, nil
}

// --- color ---

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
			return color.RGBA{R: uint8(rv<<4 | rv), G: uint8(gv<<4 | gv), B: uint8(bv<<4 | bv), A: 255}, nil
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

func alignStr(a demy.Alignment) string {
	switch a {
	case demy.AlignCenter:
		return "center"
	case demy.AlignEnd:
		return "end"
	default:
		return ""
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
		return ""
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

// --- text style ---

func textAlignStr(a demy.TextAlign) string {
	switch a {
	case demy.TextAlignCenter:
		return "center"
	case demy.TextAlignRight:
		return "right"
	default:
		return ""
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
		return ""
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

// --- font weight ---

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

// --- image helpers ---

func ditherStr(d demy.Dither) string {
	switch d {
	case demy.FloydSteinberg:
		return "floyd-steinberg"
	case demy.Atkinson:
		return "atkinson"
	default:
		return ""
	}
}

func parseDither(s string) demy.Dither {
	switch s {
	case "floyd-steinberg":
		return demy.FloydSteinberg
	case "atkinson":
		return demy.Atkinson
	default:
		return demy.NoDithering
	}
}

func scaleStr(s demy.ImageScale) string {
	switch s {
	case demy.ImageFill:
		return "fill"
	case demy.ImageStretch:
		return "stretch"
	default:
		return ""
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
