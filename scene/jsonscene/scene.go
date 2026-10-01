// Package jsonscene provides JSON serialization for demy Node trees, for passing
// scenes across process boundaries (e.g. WASM plugins).
//
// Colors are encoded as role names ("foreground", "background", "accent", "muted"),
// which unmarshal to demy.RoleColor and resolve against Config.Roles at render time,
// or as "#rgb", "#rrggbb", or "#rrggbbaa" hex strings.
//
// Decoding is strict: unknown fields, unknown enum names, children on leaf
// nodes, and trailing data are errors.
//
// Two formats are supported:
//   - Bare node: {"type":"vstack",...} — Marshal/Unmarshal
//   - Scene envelope: {"styles":{...},"root":{...}} — MarshalScene/UnmarshalScene
package jsonscene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/internal/codec"
)

// GridTrackDef serializes a single GridTrack.
type GridTrackDef struct {
	Kind   string  `json:"kind"`             // "auto", "fixed", or "fr"
	Size   float64 `json:"size,omitempty"`   // "fixed" only
	Weight float64 `json:"weight,omitempty"` // "fr" only
}

// Node is the JSON form of a demy.Node. Type selects the node kind; only the
// fields relevant to that kind are set.
type Node struct {
	Type         string         `json:"type"`
	Gap          float64        `json:"gap,omitempty"`
	Padding      float64        `json:"padding,omitempty"`
	BorderWidth  float64        `json:"border_width,omitempty"`
	BorderColor  string         `json:"border_color,omitempty"`
	Background   string         `json:"background,omitempty"`
	CornerRadius float64        `json:"corner_radius,omitempty"`
	Clip         bool           `json:"clip,omitempty"`
	Width        float64        `json:"width,omitempty"`
	Height       float64        `json:"height,omitempty"`
	Weight       float64        `json:"weight,omitempty"`
	Color        string         `json:"color,omitempty"`
	Size         float64        `json:"size,omitempty"`
	Align        string         `json:"align,omitempty"`
	Columns      []GridTrackDef `json:"columns,omitempty"`
	Rows         []GridTrackDef `json:"rows,omitempty"`
	RowGap       float64        `json:"row_gap,omitempty"`
	ColGap       float64        `json:"col_gap,omitempty"`
	Text         string         `json:"text,omitempty"`
	Class        string         `json:"class,omitempty"`
	Style        *Style         `json:"style,omitempty"`
	ImageKey     string         `json:"image_key,omitempty"`
	Scale        string         `json:"scale,omitempty"`
	Dither       string         `json:"dither,omitempty"`
	Content      string         `json:"content,omitempty"`
	BarcodeType  string         `json:"barcode_type,omitempty"`
	ECLevel      string         `json:"ec_level,omitempty"`
	Children     []Node         `json:"children,omitempty"`
}

// Style is the JSON form of demy.Style.
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
	sn, err := rootToScene(n)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sn)
}

// Unmarshal deserializes bare-node JSON to a Node tree. Image keys are resolved
// against Config.Images at render time.
func Unmarshal(data []byte) (demy.Node, error) {
	var sn Node
	if err := decodeStrict(data, &sn); err != nil {
		return nil, err
	}
	n, err := sceneToNode(sn)
	if err != nil {
		return nil, fmt.Errorf("jsonscene: %w", err)
	}
	return n, nil
}

// MarshalScene serializes a Node tree and named style definitions to a JSON Scene
// envelope.
func MarshalScene(n demy.Node, styles map[string]demy.Style) ([]byte, error) {
	root, err := rootToScene(n)
	if err != nil {
		return nil, err
	}
	s := Scene{Root: root}
	if len(styles) > 0 {
		s.Styles = make(map[string]Style, len(styles))
		for k, v := range styles {
			ss, err := styleToScene(v)
			if err != nil {
				return nil, fmt.Errorf("jsonscene: style %q: %w", k, err)
			}
			s.Styles[k] = ss
		}
	}
	return json.Marshal(s)
}

// UnmarshalScene deserializes a JSON Scene envelope, returning the root node and
// style definitions. Merge the returned styles into Config.Styles before rendering
// so that class references in the node tree resolve correctly.
func UnmarshalScene(data []byte) (demy.Node, map[string]demy.Style, error) {
	var s Scene
	if err := decodeStrict(data, &s); err != nil {
		return nil, nil, err
	}
	n, err := sceneToNode(s.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("jsonscene: %w", err)
	}
	var outStyles map[string]demy.Style
	if len(s.Styles) > 0 {
		outStyles = make(map[string]demy.Style, len(s.Styles))
		for k, ss := range s.Styles {
			parsed, err := parseStyle(&ss)
			if err != nil {
				return nil, nil, fmt.Errorf("jsonscene: style %q: %w", k, err)
			}
			outStyles[k] = parsed
		}
	}
	return n, outStyles, nil
}

// decodeStrict decodes a single JSON value, rejecting unknown fields and
// trailing data.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("jsonscene: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("jsonscene: unexpected data after the top-level value")
	}
	return nil
}

// --- marshal helpers ---

func rootToScene(n demy.Node) (Node, error) {
	if codec.IsNilNode(n) {
		return Node{}, fmt.Errorf("jsonscene: %w", codec.ErrNilRoot)
	}
	sn, err := nodeToScene(n)
	if err != nil {
		return Node{}, fmt.Errorf("jsonscene: %w", err)
	}
	return sn, nil
}

// childrenToScene encodes children, skipping nil entries as rendering does.
func childrenToScene(children []demy.Node) ([]Node, error) {
	var out []Node
	for _, child := range children {
		if codec.IsNilNode(child) {
			continue
		}
		cs, err := nodeToScene(child)
		if err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, nil
}

func nodeToScene(n demy.Node) (Node, error) {
	// Field encoders below record the first error in err.
	var (
		err      error
		sn       Node
		children []demy.Node
	)
	switch v := n.(type) {
	case *demy.VStackNode:
		sn = Node{Type: "vstack", Gap: v.Gap, Padding: v.Padding, Align: codec.Align.Encode(v.Align, &err)}
		children = v.Children
	case *demy.HStackNode:
		sn = Node{Type: "hstack", Gap: v.Gap, Padding: v.Padding, Align: codec.Align.Encode(v.Align, &err)}
		children = v.Children
	case *demy.ZStackNode:
		sn = Node{Type: "zstack", Align: codec.ZAlign.Encode(v.Align, &err)}
		children = v.Children
	case *demy.BoxNode:
		sn = Node{
			Type:         "box",
			Padding:      v.Padding,
			BorderWidth:  v.BorderWidth,
			BorderColor:  codec.EncodeColor(v.BorderColor, &err),
			Background:   codec.EncodeColor(v.Background, &err),
			CornerRadius: v.CornerRadius,
			Clip:         v.Clip,
		}
		children = v.Children
	case *demy.FrameNode:
		sn = Node{Type: "frame", Width: v.Width, Height: v.Height}
		children = v.Children
	case *demy.SpacerNode:
		sn = Node{Type: "spacer", Weight: v.Weight}
	case *demy.WeightedNode:
		sn = Node{Type: "weighted", Weight: v.Weight}
		children = []demy.Node{v.Child}
	case *demy.DividerNode:
		sn = Node{Type: "divider", Color: codec.EncodeColor(v.Color, &err), Size: v.Size}
	case *demy.TextNode:
		sn = Node{Type: "text", Text: v.Text, Class: string(v.Class)}
		if v.Style != (demy.Style{}) {
			var st Style
			st, err = styleToScene(v.Style)
			sn.Style = &st
		}
	case *demy.ImageNode:
		if err := codec.CheckImage(v); err != nil {
			return Node{}, err
		}
		sn = Node{
			Type:     "image",
			ImageKey: v.Key,
			Scale:    codec.ImageScale.Encode(v.Scale, &err),
			Dither:   codec.Dither.Encode(v.Dither, &err),
		}
	case *demy.BarcodeNode:
		sn = Node{
			Type:        "barcode",
			Content:     v.Content,
			BarcodeType: codec.BarcodeType.Encode(v.Type, &err),
			ECLevel:     codec.ECLevel.Encode(v.ECLevel, &err),
		}
	case *demy.GridNode:
		sn = Node{
			Type:    "grid",
			Padding: v.Padding,
			RowGap:  v.RowGap,
			ColGap:  v.ColGap,
			Columns: trackDefsFromTracks(v.Columns),
			Rows:    trackDefsFromTracks(v.Rows),
		}
		children = v.Children
	case *demy.GridRowNode:
		sn = Node{Type: "row"}
		children = v.Children
	default:
		return Node{}, fmt.Errorf("unknown node type %T", n)
	}
	if err != nil {
		return Node{}, fmt.Errorf("%s: %w", sn.Type, err)
	}
	cs, err := childrenToScene(children)
	if err != nil {
		return Node{}, err
	}
	sn.Children = cs
	return sn, nil
}

func styleToScene(s demy.Style) (Style, error) {
	var err error
	st := Style{
		Font:       s.Font,
		Size:       s.Size,
		Weight:     codec.EncodeWeight(s.Weight, &err),
		Color:      codec.EncodeColor(s.Color, &err),
		Align:      codec.TextAlign.Encode(s.Align, &err),
		LineHeight: s.LineHeight,
		MaxLines:   s.MaxLines,
		Overflow:   codec.Overflow.Encode(s.Overflow, &err),
	}
	return st, err
}

// --- unmarshal helpers ---

func childrenFromScene(children []Node) ([]demy.Node, error) {
	var out []demy.Node
	for _, child := range children {
		c, err := sceneToNode(child)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func sceneToNode(sn Node) (demy.Node, error) {
	var leaf bool
	switch sn.Type {
	case "spacer", "divider", "text", "image", "barcode":
		leaf = true
	}
	if leaf && len(sn.Children) > 0 {
		return nil, fmt.Errorf("%s: children are not allowed", sn.Type)
	}
	if sn.Type == "weighted" && len(sn.Children) > 1 {
		return nil, fmt.Errorf("weighted: has %d children, want at most 1", len(sn.Children))
	}
	children, err := childrenFromScene(sn.Children)
	if err != nil {
		return nil, err
	}

	// Field decoders below record the first error in err.
	var n demy.Node
	switch sn.Type {
	case "vstack":
		n = &demy.VStackNode{Gap: sn.Gap, Padding: sn.Padding, Align: codec.Align.Decode(sn.Align, &err), Children: children}
	case "hstack":
		n = &demy.HStackNode{Gap: sn.Gap, Padding: sn.Padding, Align: codec.Align.Decode(sn.Align, &err), Children: children}
	case "zstack":
		n = &demy.ZStackNode{Align: codec.ZAlign.Decode(sn.Align, &err), Children: children}
	case "box":
		n = &demy.BoxNode{
			Padding:      sn.Padding,
			BorderWidth:  sn.BorderWidth,
			BorderColor:  codec.DecodeColor(sn.BorderColor, &err),
			Background:   codec.DecodeColor(sn.Background, &err),
			CornerRadius: sn.CornerRadius,
			Clip:         sn.Clip,
			Children:     children,
		}
	case "frame":
		n = &demy.FrameNode{Width: sn.Width, Height: sn.Height, Children: children}
	case "spacer":
		n = &demy.SpacerNode{Weight: sn.Weight}
	case "weighted":
		w := &demy.WeightedNode{Weight: sn.Weight}
		if len(children) == 1 {
			w.Child = children[0]
		}
		n = w
	case "divider":
		n = &demy.DividerNode{Color: codec.DecodeColor(sn.Color, &err), Size: sn.Size}
	case "text":
		var style demy.Style
		style, err = parseStyle(sn.Style)
		n = &demy.TextNode{Text: sn.Text, Class: demy.StyleClass(sn.Class), Style: style}
	case "image":
		n = &demy.ImageNode{
			Key:    sn.ImageKey,
			Scale:  codec.ImageScale.Decode(sn.Scale, &err),
			Dither: codec.Dither.Decode(sn.Dither, &err),
		}
	case "barcode":
		n = &demy.BarcodeNode{
			Content: sn.Content,
			Type:    codec.BarcodeType.Decode(sn.BarcodeType, &err),
			ECLevel: codec.ECLevel.Decode(sn.ECLevel, &err),
		}
	case "grid":
		g := &demy.GridNode{
			Padding:  sn.Padding,
			RowGap:   sn.RowGap,
			ColGap:   sn.ColGap,
			Children: children,
		}
		if g.Columns, err = tracksFromDefs(sn.Columns); err == nil {
			g.Rows, err = tracksFromDefs(sn.Rows)
		}
		n = g
	case "row":
		n = &demy.GridRowNode{Children: children}
	default:
		return nil, fmt.Errorf("unknown node type %q", sn.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sn.Type, err)
	}
	return n, nil
}

func parseStyle(ss *Style) (demy.Style, error) {
	if ss == nil {
		return demy.Style{}, nil
	}
	var err error
	st := demy.Style{
		Font:       ss.Font,
		Size:       ss.Size,
		Weight:     codec.DecodeWeight(ss.Weight, &err),
		Color:      codec.DecodeColor(ss.Color, &err),
		Align:      codec.TextAlign.Decode(ss.Align, &err),
		LineHeight: ss.LineHeight,
		MaxLines:   ss.MaxLines,
		Overflow:   codec.Overflow.Decode(ss.Overflow, &err),
	}
	return st, err
}

// --- grid track helpers ---

func trackDefsFromTracks(tracks []demy.GridTrack) []GridTrackDef {
	if len(tracks) == 0 {
		return nil
	}
	defs := make([]GridTrackDef, len(tracks))
	for i, t := range tracks {
		defs[i] = GridTrackDef{Kind: codec.TrackKind(t)}
		switch defs[i].Kind {
		case codec.TrackFixed:
			defs[i].Size = t.Size()
		case codec.TrackFr:
			defs[i].Weight = t.Weight()
		}
	}
	return defs
}

func tracksFromDefs(defs []GridTrackDef) ([]demy.GridTrack, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	tracks := make([]demy.GridTrack, len(defs))
	for i, d := range defs {
		t, err := codec.ParseTrack(d.Kind, d.Size, d.Weight)
		if err != nil {
			return nil, err
		}
		tracks[i] = t
	}
	return tracks, nil
}
