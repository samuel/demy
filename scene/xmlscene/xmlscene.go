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
// Decoding is strict: unknown elements and attributes, unknown enum names,
// malformed numbers (including NaN, Inf, and hex floats), children or text in
// elements that don't take them, nesting deeper than 1000 elements, and content
// after the root element are errors.
//
// # Physical units
//
// Dimension attributes (width, height, padding, gap, etc.) accept an optional
// unit suffix:
//
//   - Bare number or "px" → logical pixels (e.g. "240" or "240px")
//   - "mm" → millimeters (e.g. "50mm")
//   - "pt" → points, 1/72 inch (e.g. "12pt")
//
// Font sizes (the size attribute of <text> and <style>) are in points: a bare
// number or "pt" is points, and "mm" and "px" are converted to points.
// line-height is a unitless multiplier.
//
// "mm" and "pt" dimensions and "px" font sizes are converted using Config.DPMM and
// require Config.PhysicalWidthMM to be set; otherwise they are errors. Marshal
// always writes bare values.
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
	"text/template/parse"

	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/codec"
)

// Marshal serializes a Node tree to indented XML.
func Marshal(n demy.Node) ([]byte, error) {
	if codec.IsNilNode(n) {
		return nil, fmt.Errorf("xmlscene: %w", codec.ErrNilRoot)
	}
	return encodeDoc(func(enc *xml.Encoder) error { return marshalNode(enc, n) })
}

// Unmarshal deserializes XML to a Node tree. Pass a demy.Config with physical
// dimensions set to enable "mm" and "pt" unit suffixes in dimension attributes.
func Unmarshal(data []byte, cfg demy.Config) (demy.Node, error) {
	n, styles, err := unmarshalDoc(data, cfg, false)
	if err != nil {
		return nil, err
	}
	_ = styles // always nil without a <scene> wrapper
	return n, nil
}

// Execute executes a text/template that produces XML and unmarshals the result
// with Unmarshal. The output of every template action is XML-escaped, so data
// can't inject markup; pipe a value to raw (e.g. {{.Snippet | raw}}) to insert
// it unescaped.
func Execute(tmplSrc []byte, data any, cfg demy.Config) (demy.Node, error) {
	out, err := executeTemplate(tmplSrc, data)
	if err != nil {
		return nil, err
	}
	return Unmarshal(out, cfg)
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
	if codec.IsNilNode(n) {
		return nil, fmt.Errorf("xmlscene: %w", codec.ErrNilRoot)
	}
	return encodeDoc(func(enc *xml.Encoder) error {
		sceneElem := xml.StartElement{Name: xml.Name{Local: "scene"}}
		if err := enc.EncodeToken(sceneElem); err != nil {
			return err
		}
		if len(styles) > 0 {
			stylesElem := xml.StartElement{Name: xml.Name{Local: "styles"}}
			if err := enc.EncodeToken(stylesElem); err != nil {
				return err
			}
			for _, k := range slices.Sorted(maps.Keys(styles)) {
				b := newElem("style")
				b.str("name", k)
				b.style(styles[k])
				if b.err != nil {
					return fmt.Errorf("style %q: %w", k, b.err)
				}
				if err := encodeEmpty(enc, b.start); err != nil {
					return err
				}
			}
			if err := enc.EncodeToken(stylesElem.End()); err != nil {
				return err
			}
		}
		if err := marshalNode(enc, n); err != nil {
			return err
		}
		return enc.EncodeToken(sceneElem.End())
	})
}

// UnmarshalScene deserializes XML to a Node tree and style definitions.
// The document may be a bare node element or a <scene> wrapper containing an
// optional <styles> element and exactly one root element. Merge the returned
// styles into Config.Styles before rendering so that class references resolve.
// Pass a demy.Config with physical dimensions set to enable "mm" and "pt" unit
// suffixes.
func UnmarshalScene(data []byte, cfg demy.Config) (demy.Node, map[string]demy.Style, error) {
	return unmarshalDoc(data, cfg, true)
}

// ExecuteScene executes a text/template that produces XML and unmarshals the result
// with UnmarshalScene. Action output is XML-escaped as in Execute.
func ExecuteScene(tmplSrc []byte, data any, cfg demy.Config) (demy.Node, map[string]demy.Style, error) {
	out, err := executeTemplate(tmplSrc, data)
	if err != nil {
		return nil, nil, err
	}
	return UnmarshalScene(out, cfg)
}

// --- templates ---

const (
	escapeFunc = "xmlscene_escape"
	rawFunc    = "raw"
)

// rawXML marks a value that the template inserts without escaping.
type rawXML string

var templateFuncs = template.FuncMap{
	escapeFunc: func(args ...any) string {
		if len(args) == 1 {
			if r, ok := args[0].(rawXML); ok {
				return string(r)
			}
		}
		var buf bytes.Buffer
		_ = xml.EscapeText(&buf, []byte(fmt.Sprint(args...))) //nolint:errcheck // writes to a bytes.Buffer can't fail
		return buf.String()
	},
	rawFunc: func(v any) rawXML { return rawXML(fmt.Sprint(v)) },
}

func executeTemplate(src []byte, data any) ([]byte, error) {
	t, err := template.New("scene").Funcs(templateFuncs).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("xmlscene: template parse: %w", err)
	}
	for _, tt := range t.Templates() {
		if tt.Tree != nil {
			escapeActions(tt.Tree, tt.Root)
		}
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("xmlscene: template execute: %w", err)
	}
	return buf.Bytes(), nil
}

// escapeActions appends the escape function to the pipeline of every action
// that produces output, the way html/template rewrites its templates. Actions
// that declare variables print nothing and are left alone. A value already
// marked raw passes through the escape function unchanged.
func escapeActions(tree *parse.Tree, n parse.Node) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			escapeActions(tree, c)
		}
	case *parse.ActionNode:
		if len(n.Pipe.Decl) > 0 {
			return
		}
		id := parse.NewIdentifier(escapeFunc).SetTree(tree).SetPos(n.Pos)
		n.Pipe.Cmds = append(n.Pipe.Cmds, &parse.CommandNode{
			NodeType: parse.NodeCommand,
			Pos:      n.Pos,
			Args:     []parse.Node{id},
		})
	case *parse.IfNode:
		escapeActions(tree, n.List)
		escapeActions(tree, n.ElseList)
	case *parse.RangeNode:
		escapeActions(tree, n.List)
		escapeActions(tree, n.ElseList)
	case *parse.WithNode:
		escapeActions(tree, n.List)
		escapeActions(tree, n.ElseList)
	}
}

// --- marshal ---

func encodeDoc(write func(enc *xml.Encoder) error) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "\t")
	if err := write(enc); err != nil {
		_ = enc.Close() //nolint:errcheck // the write error is the one to report
		return nil, fmt.Errorf("xmlscene: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("xmlscene: %w", err)
	}
	return buf.Bytes(), nil
}

// elem builds a start element's attributes, recording the first encoding error.
type elem struct {
	start xml.StartElement
	err   error
}

func newElem(name string) *elem {
	return &elem{start: xml.StartElement{Name: xml.Name{Local: name}}}
}

func (b *elem) str(name, value string) {
	if value != "" {
		b.start.Attr = append(b.start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
	}
}

func (b *elem) float(name string, v float64) {
	if v != 0 {
		b.str(name, strconv.FormatFloat(v, 'f', -1, 64))
	}
}

func (b *elem) int(name string, v int) {
	if v != 0 {
		b.str(name, strconv.Itoa(v))
	}
}

func (b *elem) bool(name string, v bool) {
	if v {
		b.str(name, "true")
	}
}

func (b *elem) color(name string, c color.Color) {
	b.str(name, codec.EncodeColor(c, &b.err))
}

func (b *elem) style(s demy.Style) {
	b.str("font", s.Font)
	b.float("size", s.Size)
	b.str("weight", codec.EncodeWeight(s.Weight, &b.err))
	b.color("color", s.Color)
	b.str("align", codec.TextAlign.Encode(s.Align, &b.err))
	b.float("line-height", s.LineHeight)
	b.int("max-lines", s.MaxLines)
	b.str("overflow", codec.Overflow.Encode(s.Overflow, &b.err))
}

func encodeEmpty(enc *xml.Encoder, start xml.StartElement) error {
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	return enc.EncodeToken(start.End())
}

func marshalNode(enc *xml.Encoder, n demy.Node) error {
	var (
		b        *elem
		children []demy.Node
		text     string
		grid     *demy.GridNode
	)
	switch v := n.(type) {
	case *demy.VStackNode:
		b = newElem("vstack")
		b.float("gap", v.Gap)
		b.float("padding", v.Padding)
		b.str("align", codec.Align.Encode(v.Align, &b.err))
		children = v.Children
	case *demy.HStackNode:
		b = newElem("hstack")
		b.float("gap", v.Gap)
		b.float("padding", v.Padding)
		b.str("align", codec.Align.Encode(v.Align, &b.err))
		children = v.Children
	case *demy.ZStackNode:
		b = newElem("zstack")
		b.str("align", codec.ZAlign.Encode(v.Align, &b.err))
		children = v.Children
	case *demy.BoxNode:
		b = newElem("box")
		b.float("padding", v.Padding)
		b.float("border-width", v.BorderWidth)
		b.color("border-color", v.BorderColor)
		b.color("background", v.Background)
		b.float("corner-radius", v.CornerRadius)
		b.bool("clip", v.Clip)
		children = v.Children
	case *demy.FrameNode:
		b = newElem("frame")
		b.float("width", v.Width)
		b.float("height", v.Height)
		children = v.Children
	case *demy.SpacerNode:
		b = newElem("spacer")
		b.float("weight", v.Weight)
	case *demy.WeightedNode:
		b = newElem("weighted")
		b.float("weight", v.Weight)
		children = []demy.Node{v.Child}
	case *demy.DividerNode:
		b = newElem("divider")
		b.float("size", v.Size)
		b.color("color", v.Color)
	case *demy.TextNode:
		b = newElem("text")
		b.str("class", string(v.Class))
		b.style(v.Style)
		text = v.Text
	case *demy.ImageNode:
		if err := codec.CheckImage(v); err != nil {
			return err
		}
		b = newElem("image")
		b.str("key", v.Key)
		b.str("scale", codec.ImageScale.Encode(v.Scale, &b.err))
		b.str("dither", codec.Dither.Encode(v.Dither, &b.err))
	case *demy.BarcodeNode:
		b = newElem("barcode")
		b.str("content", v.Content)
		b.str("type", codec.BarcodeType.Encode(v.Type, &b.err))
		b.str("ec-level", codec.ECLevel.Encode(v.ECLevel, &b.err))
	case *demy.GridNode:
		b = newElem("grid")
		b.float("padding", v.Padding)
		b.float("row-gap", v.RowGap)
		b.float("col-gap", v.ColGap)
		children = v.Children
		grid = v
	case *demy.GridRowNode:
		b = newElem("row")
		children = v.Children
	default:
		return fmt.Errorf("unknown node type %T", n)
	}
	if b.err != nil {
		return fmt.Errorf("<%s>: %w", b.start.Name.Local, b.err)
	}

	if err := enc.EncodeToken(b.start); err != nil {
		return err
	}
	if text != "" {
		if err := enc.EncodeToken(xml.CharData(text)); err != nil {
			return err
		}
	}
	if grid != nil {
		if err := marshalTracks(enc, "columns", grid.Columns); err != nil {
			return err
		}
		if err := marshalTracks(enc, "rows", grid.Rows); err != nil {
			return err
		}
	}
	for _, child := range children {
		// Nil children are skipped when rendering, so they aren't written.
		if codec.IsNilNode(child) {
			continue
		}
		if err := marshalNode(enc, child); err != nil {
			return err
		}
	}
	return enc.EncodeToken(b.start.End())
}

func marshalTracks(enc *xml.Encoder, name string, tracks []demy.GridTrack) error {
	if len(tracks) == 0 {
		return nil
	}
	start := xml.StartElement{Name: xml.Name{Local: name}}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, t := range tracks {
		b := newElem(codec.TrackKind(t))
		switch b.start.Name.Local {
		case codec.TrackFixed:
			b.float("size", t.Size())
		case codec.TrackFr:
			b.float("weight", t.Weight())
		}
		if err := encodeEmpty(enc, b.start); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

// --- unmarshal context ---

// unmarshalCtx carries per-parse state. dpmm is 0 when the Config lacks
// physical dimensions.
type unmarshalCtx struct {
	dec   *xml.Decoder
	dpmm  float64
	depth int // element nesting depth
}

// styleAttrs lists the attributes shared by <text> and <style>.
var styleAttrs = []string{"font", "size", "weight", "color", "align", "line-height", "max-lines", "overflow"}

// allowedAttrs lists the attributes each element accepts.
var allowedAttrs = map[string][]string{
	"scene":    nil,
	"styles":   nil,
	"style":    append([]string{"name"}, styleAttrs...),
	"vstack":   {"gap", "padding", "align"},
	"hstack":   {"gap", "padding", "align"},
	"zstack":   {"align"},
	"box":      {"padding", "border-width", "border-color", "background", "corner-radius", "clip"},
	"frame":    {"width", "height"},
	"spacer":   {"weight"},
	"weighted": {"weight"},
	"divider":  {"size", "color"},
	"text":     append([]string{"class"}, styleAttrs...),
	"image":    {"key", "scale", "dither"},
	"barcode":  {"content", "type", "ec-level"},
	"grid":     {"padding", "row-gap", "col-gap"},
	"row":      nil,
	"columns":  nil,
	"rows":     nil,
	"auto":     nil,
	"fixed":    {"size"},
	"fr":       {"weight"},
}

// attrs reads an element's attributes, recording the first parse error.
type attrs struct {
	m    map[string]string
	dpmm float64
	err  error
}

func readAttrs(start xml.StartElement, dpmm float64) (*attrs, error) {
	name := start.Name.Local
	allowed, ok := allowedAttrs[name]
	if !ok {
		return nil, fmt.Errorf("unknown element <%s>", name)
	}
	if start.Name.Space != "" {
		return nil, fmt.Errorf("namespaced element <%s:%s> is not supported", start.Name.Space, name)
	}
	m := make(map[string]string, len(start.Attr))
	for _, a := range start.Attr {
		if a.Name.Space != "" {
			return nil, fmt.Errorf("<%s>: namespaced attribute %s:%s is not supported", name, a.Name.Space, a.Name.Local)
		}
		if !slices.Contains(allowed, a.Name.Local) {
			return nil, fmt.Errorf("<%s>: unknown attribute %q", name, a.Name.Local)
		}
		if _, dup := m[a.Name.Local]; dup {
			return nil, fmt.Errorf("<%s>: duplicate attribute %q", name, a.Name.Local)
		}
		m[a.Name.Local] = a.Value
	}
	return &attrs{m: m, dpmm: dpmm}, nil
}

func (a *attrs) fail(name string, err error) {
	if a.err == nil {
		a.err = fmt.Errorf("attribute %q: %w", name, err)
	}
}

func (a *attrs) str(name string) string { return a.m[name] }

func (a *attrs) float(name string) float64 {
	s := a.m[name]
	if s == "" {
		return 0
	}
	v, err := parseNumber(s)
	if err != nil {
		a.fail(name, fmt.Errorf("invalid number %q", s))
	}
	return v
}

// parseNumber parses a finite decimal number. strconv.ParseFloat also accepts
// NaN, infinities, hex floats, and underscores, which scenes reject.
func parseNumber(s string) (float64, error) {
	if strings.ContainsFunc(s, func(r rune) bool { return !strings.ContainsRune("0123456789+-.eE", r) }) {
		return 0, errors.New("not a decimal number")
	}
	return strconv.ParseFloat(s, 64) // rejects out-of-range values
}

func (a *attrs) int(name string) int {
	s := a.m[name]
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		a.fail(name, fmt.Errorf("invalid integer %q", s))
	}
	return v
}

func (a *attrs) bool(name string) bool {
	switch s := a.m[name]; s {
	case "", "false":
		return false
	case "true":
		return true
	default:
		a.fail(name, fmt.Errorf("invalid boolean %q (want true or false)", s))
		return false
	}
}

func (a *attrs) color(name string) color.Color {
	c, err := codec.ParseColor(a.m[name])
	if err != nil {
		a.fail(name, err)
	}
	return c
}

// splitUnit separates a numeric value from its unit suffix.
func (a *attrs) splitUnit(name string) (float64, string, bool) {
	s := a.m[name]
	if s == "" {
		return 0, "", false
	}
	num, unit := s, ""
	for _, u := range []string{"mm", "pt", "px"} {
		if n, ok := strings.CutSuffix(s, u); ok {
			num, unit = n, u
			break
		}
	}
	v, err := parseNumber(num)
	if err != nil {
		a.fail(name, fmt.Errorf("invalid dimension %q", s))
		return 0, "", false
	}
	return v, unit, true
}

// needDPMM records an error when unit can't be converted without physical
// dimensions.
func (a *attrs) needDPMM(name, unit string) bool {
	if a.dpmm == 0 {
		a.fail(name, fmt.Errorf("unit %q in %q needs Config physical dimensions", unit, a.m[name]))
		return false
	}
	return true
}

// dim parses a dimension in logical pixels.
func (a *attrs) dim(name string) float64 {
	v, unit, ok := a.splitUnit(name)
	if !ok {
		return 0
	}
	switch unit {
	case "mm":
		if !a.needDPMM(name, unit) {
			return 0
		}
		return v * a.dpmm
	case "pt":
		if !a.needDPMM(name, unit) {
			return 0
		}
		return v * a.dpmm * 25.4 / 72
	default:
		return v
	}
}

// fontSize parses a font size in points.
func (a *attrs) fontSize(name string) float64 {
	v, unit, ok := a.splitUnit(name)
	if !ok {
		return 0
	}
	switch unit {
	case "mm":
		return v * 72 / 25.4
	case "px":
		if !a.needDPMM(name, unit) {
			return 0
		}
		return v / a.dpmm * 72 / 25.4
	default:
		return v
	}
}

func (a *attrs) style() demy.Style {
	var err error
	s := demy.Style{
		Font:       a.str("font"),
		Size:       a.fontSize("size"),
		Weight:     codec.DecodeWeight(a.str("weight"), &err),
		Color:      a.color("color"),
		Align:      codec.TextAlign.Decode(a.str("align"), &err),
		LineHeight: a.float("line-height"),
		MaxLines:   a.int("max-lines"),
		Overflow:   codec.Overflow.Decode(a.str("overflow"), &err),
	}
	if err != nil && a.err == nil {
		a.err = err
	}
	return s
}

// --- unmarshal ---

func unmarshalDoc(data []byte, cfg demy.Config, allowScene bool) (demy.Node, map[string]demy.Style, error) {
	uctx := &unmarshalCtx{dec: xml.NewDecoder(bytes.NewReader(data))}
	if cfg.PhysicalWidthMM != 0 {
		uctx.dpmm = cfg.DPMM()
	}
	start, err := uctx.nextStart()
	if err != nil {
		return nil, nil, fmt.Errorf("xmlscene: %w", err)
	}
	var (
		root   demy.Node
		styles map[string]demy.Style
	)
	if allowScene && start.Name.Local == "scene" {
		root, styles, err = uctx.scene(start)
	} else {
		root, err = uctx.element(start)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("xmlscene: %w", err)
	}
	if err := uctx.expectEOF(); err != nil {
		return nil, nil, fmt.Errorf("xmlscene: %w", err)
	}
	return root, styles, nil
}

// nextStart returns the first start element, skipping the prolog.
func (u *unmarshalCtx) nextStart() (xml.StartElement, error) {
	for {
		tok, err := u.dec.Token()
		if errors.Is(err, io.EOF) {
			return xml.StartElement{}, errors.New("empty document")
		}
		if err != nil {
			return xml.StartElement{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, nil
		case xml.CharData:
			if len(bytes.TrimSpace(t)) > 0 {
				return xml.StartElement{}, fmt.Errorf("unexpected text %q before the root element", t)
			}
		}
	}
}

// expectEOF reports an error for anything but whitespace, comments, and
// processing instructions after the root element.
func (u *unmarshalCtx) expectEOF() error {
	for {
		tok, err := u.dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return fmt.Errorf("unexpected element <%s> after the root element", t.Name.Local)
		case xml.CharData:
			if len(bytes.TrimSpace(t)) > 0 {
				return fmt.Errorf("unexpected text %q after the root element", t)
			}
		}
	}
}

// contents reads the tokens of the current element up to its end, calling
// onElem for each child element and onText for character data. A nil onText
// rejects non-whitespace text; a nil onElem rejects child elements.
func (u *unmarshalCtx) contents(parent string, onElem func(xml.StartElement) error, onText func([]byte)) error {
	for {
		tok, err := u.dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if onElem == nil {
				return fmt.Errorf("<%s> does not allow child elements, found <%s>", parent, t.Name.Local)
			}
			if err := onElem(t); err != nil {
				return err
			}
		case xml.CharData:
			switch {
			case onText != nil:
				onText(t)
			case len(bytes.TrimSpace(t)) > 0:
				return fmt.Errorf("<%s> does not allow text, found %q", parent, bytes.TrimSpace(t))
			}
		case xml.EndElement:
			return nil
		}
	}
}

func (u *unmarshalCtx) children(parent string) ([]demy.Node, error) {
	var out []demy.Node
	err := u.contents(parent, func(t xml.StartElement) error {
		n, err := u.element(t)
		if err != nil {
			return err
		}
		out = append(out, n)
		return nil
	}, nil)
	return out, err
}

func (u *unmarshalCtx) element(start xml.StartElement) (demy.Node, error) {
	u.depth++
	defer func() { u.depth-- }()
	if u.depth > codec.MaxDepth {
		return nil, fmt.Errorf("elements nested deeper than %d", codec.MaxDepth)
	}
	name := start.Name.Local
	a, err := readAttrs(start, u.dpmm)
	if err != nil {
		return nil, err
	}

	var n demy.Node
	switch name {
	case "vstack", "hstack":
		gap, padding := a.dim("gap"), a.dim("padding")
		align := codec.Align.Decode(a.str("align"), &err)
		children, cerr := u.children(name)
		if cerr != nil {
			return nil, cerr
		}
		if name == "vstack" {
			n = &demy.VStackNode{Gap: gap, Padding: padding, Align: align, Children: children}
		} else {
			n = &demy.HStackNode{Gap: gap, Padding: padding, Align: align, Children: children}
		}
	case "zstack":
		align := codec.ZAlign.Decode(a.str("align"), &err)
		children, cerr := u.children(name)
		if cerr != nil {
			return nil, cerr
		}
		n = &demy.ZStackNode{Align: align, Children: children}
	case "box":
		b := &demy.BoxNode{
			Padding:      a.dim("padding"),
			BorderWidth:  a.dim("border-width"),
			BorderColor:  a.color("border-color"),
			Background:   a.color("background"),
			CornerRadius: a.dim("corner-radius"),
			Clip:         a.bool("clip"),
		}
		if b.Children, err = u.children(name); err != nil {
			return nil, err
		}
		n = b
	case "frame":
		f := &demy.FrameNode{Width: a.dim("width"), Height: a.dim("height")}
		if f.Children, err = u.children(name); err != nil {
			return nil, err
		}
		n = f
	case "spacer":
		n = &demy.SpacerNode{Weight: a.float("weight")}
		if err := u.contents(name, nil, nil); err != nil {
			return nil, err
		}
	case "weighted":
		w := &demy.WeightedNode{Weight: a.float("weight")}
		children, cerr := u.children(name)
		if cerr != nil {
			return nil, cerr
		}
		switch len(children) {
		case 0:
		case 1:
			w.Child = children[0]
		default:
			return nil, fmt.Errorf("<weighted> has %d children, want at most 1", len(children))
		}
		n = w
	case "divider":
		n = &demy.DividerNode{Color: a.color("color"), Size: a.dim("size")}
		if err := u.contents(name, nil, nil); err != nil {
			return nil, err
		}
	case "text":
		t := &demy.TextNode{Class: demy.StyleClass(a.str("class")), Style: a.style()}
		var sb strings.Builder
		if err := u.contents(name, nil, func(b []byte) { sb.Write(b) }); err != nil {
			return nil, err
		}
		t.Text = sb.String()
		n = t
	case "image":
		n = &demy.ImageNode{
			Key:    a.str("key"),
			Scale:  codec.ImageScale.Decode(a.str("scale"), &err),
			Dither: codec.Dither.Decode(a.str("dither"), &err),
		}
		if err := u.contents(name, nil, nil); err != nil {
			return nil, err
		}
	case "barcode":
		n = &demy.BarcodeNode{
			Content: a.str("content"),
			Type:    codec.BarcodeType.Decode(a.str("type"), &err),
			ECLevel: codec.ECLevel.Decode(a.str("ec-level"), &err),
		}
		if err := u.contents(name, nil, nil); err != nil {
			return nil, err
		}
	case "grid":
		g := &demy.GridNode{Padding: a.dim("padding"), RowGap: a.dim("row-gap"), ColGap: a.dim("col-gap")}
		if gerr := u.grid(g); gerr != nil {
			return nil, gerr
		}
		n = g
	case "row":
		r := &demy.GridRowNode{}
		if r.Children, err = u.children(name); err != nil {
			return nil, err
		}
		n = r
	default:
		return nil, fmt.Errorf("<%s> is not a node element", name)
	}
	if a.err != nil {
		err = a.err
	}
	if err != nil {
		return nil, fmt.Errorf("<%s>: %w", name, err)
	}
	return n, nil
}

// grid reads a <grid> element's <columns>, <rows>, and child nodes.
func (u *unmarshalCtx) grid(g *demy.GridNode) error {
	var haveColumns, haveRows bool
	return u.contents("grid", func(t xml.StartElement) error {
		var err error
		switch t.Name.Local {
		case "columns":
			if haveColumns {
				return errors.New("<grid> has more than one <columns>")
			}
			haveColumns = true
			g.Columns, err = u.tracks(t)
		case "rows":
			if haveRows {
				return errors.New("<grid> has more than one <rows>")
			}
			haveRows = true
			g.Rows, err = u.tracks(t)
		default:
			var n demy.Node
			if n, err = u.element(t); err == nil {
				g.Children = append(g.Children, n)
			}
		}
		return err
	}, nil)
}

// tracks reads <auto/>, <fixed size="..."/>, or <fr weight="..."/> elements.
func (u *unmarshalCtx) tracks(start xml.StartElement) ([]demy.GridTrack, error) {
	if _, err := readAttrs(start, u.dpmm); err != nil {
		return nil, err
	}
	parent := start.Name.Local
	var tracks []demy.GridTrack
	err := u.contents(parent, func(t xml.StartElement) error {
		kind := t.Name.Local
		if kind != codec.TrackAuto && kind != codec.TrackFixed && kind != codec.TrackFr {
			return fmt.Errorf("<%s>: unknown track <%s> (want auto, fixed, or fr)", parent, kind)
		}
		a, err := readAttrs(t, u.dpmm)
		if err != nil {
			return err
		}
		track, err := codec.ParseTrack(kind, a.dim("size"), a.float("weight"))
		if err == nil {
			err = a.err
		}
		if err != nil {
			return fmt.Errorf("<%s>: %w", kind, err)
		}
		tracks = append(tracks, track)
		return u.contents(kind, nil, nil)
	}, nil)
	return tracks, err
}

func (u *unmarshalCtx) styles(start xml.StartElement) (map[string]demy.Style, error) {
	if _, err := readAttrs(start, u.dpmm); err != nil {
		return nil, err
	}
	styles := make(map[string]demy.Style)
	err := u.contents("styles", func(t xml.StartElement) error {
		if t.Name.Local != "style" {
			return fmt.Errorf("<styles> allows only <style>, found <%s>", t.Name.Local)
		}
		a, err := readAttrs(t, u.dpmm)
		if err != nil {
			return err
		}
		name := a.str("name")
		if name == "" {
			return errors.New("<style> is missing a name")
		}
		if _, dup := styles[name]; dup {
			return fmt.Errorf("duplicate style %q", name)
		}
		styles[name] = a.style()
		if a.err != nil {
			return fmt.Errorf("style %q: %w", name, a.err)
		}
		return u.contents("style", nil, nil)
	}, nil)
	return styles, err
}

func (u *unmarshalCtx) scene(start xml.StartElement) (demy.Node, map[string]demy.Style, error) {
	if _, err := readAttrs(start, u.dpmm); err != nil {
		return nil, nil, err
	}
	var (
		root       demy.Node
		styles     map[string]demy.Style
		haveStyles bool
	)
	err := u.contents("scene", func(t xml.StartElement) error {
		if t.Name.Local == "styles" {
			if haveStyles {
				return errors.New("<scene> has more than one <styles>")
			}
			haveStyles = true
			var err error
			styles, err = u.styles(t)
			return err
		}
		if root != nil {
			return fmt.Errorf("<scene> has more than one root element, found <%s>", t.Name.Local)
		}
		n, err := u.element(t)
		root = n
		return err
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	if root == nil {
		return nil, nil, errors.New("<scene> has no root element")
	}
	return root, styles, nil
}
