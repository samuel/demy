// Package starlarkplugin exposes pate layout constructors as Starlark builtins.
// A plugin script must define render(data) returning a pate Node tree.
//
// Exposed globals:
//
//	Constructors: VStack, HStack, ZStack, Box, Frame, Spacer, Divider, Text, ImageKey
//	Style:        Style(font=, size=, weight=, color=, align=, line_height=, max_lines=, overflow=)
//	Options:      Gap, Padding, Background, Border, CornerRadius, Children, Clip
//	Alignment:    AlignStart, AlignCenter, AlignEnd
//	ZAlignment:   TopLeading, Top, TopTrailing, Leading, Center, Trailing,
//	              BottomLeading, Bottom, BottomTrailing
//	ImageScale:   ImageFit, ImageFill, ImageStretch
//	Colors:       Foreground, Accent, Muted (role strings), Black, DarkGray, LightGray,
//	              White, Red, Yellow (hex strings)
//
// Named styles from Config.Styles are injected as capitalized globals (e.g. "h1" → H1).
package starlarkplugin

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"unicode"

	sl "go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/samuel/pate"
)

// Plugin is a compiled Starlark script that exports a render(data) function.
type Plugin struct {
	globals sl.StringDict
}

// Load compiles the Starlark script at path. cfg injects named styles and color
// constants into the script's global environment.
func Load(path string, cfg pate.Config) (*Plugin, error) {
	return execFile(path, nil, cfg)
}

// LoadBytes compiles src as a Starlark script named filename.
func LoadBytes(filename string, src []byte, cfg pate.Config) (*Plugin, error) {
	return execFile(filename, src, cfg)
}

func execFile(filename string, src any, cfg pate.Config) (*Plugin, error) {
	thread := &sl.Thread{Name: filename}
	globals, err := sl.ExecFileOptions(&syntax.FileOptions{}, thread, filename, src, newEnv(cfg))
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: load %s: %w", filename, err)
	}
	if _, ok := globals["render"]; !ok {
		return nil, fmt.Errorf("starlarkplugin: %s must define render(data)", filename)
	}
	return &Plugin{globals: globals}, nil
}

// Render calls the script's render(data) and returns the root Node tree.
func (p *Plugin) Render(data map[string]any) (pate.Node, error) {
	d, err := dataToStarlark(data)
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: %w", err)
	}
	thread := &sl.Thread{Name: "render"}
	result, err := sl.Call(thread, p.globals["render"], sl.Tuple{d}, nil)
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: render: %w", err)
	}
	nv, ok := result.(*nodeValue)
	if !ok {
		return nil, fmt.Errorf("starlarkplugin: render must return a Node, got %s", result.Type())
	}
	return nv.n, nil
}

// --- Starlark value types ---

type nodeValue struct{ n pate.Node }

func (v *nodeValue) String() string        { return fmt.Sprintf("<%T>", v.n) }
func (v *nodeValue) Type() string          { return "node" }
func (v *nodeValue) Freeze()               {}
func (v *nodeValue) Truth() sl.Bool        { return sl.True }
func (v *nodeValue) Hash() (uint32, error) { return 0, errors.New("unhashable: node") }

type optValue struct{ o pate.Option }

func (v *optValue) String() string        { return fmt.Sprintf("<%T>", v.o) }
func (v *optValue) Type() string          { return "option" }
func (v *optValue) Freeze()               {}
func (v *optValue) Truth() sl.Bool        { return sl.True }
func (v *optValue) Hash() (uint32, error) { return 0, errors.New("unhashable: option") }

type styleValue struct{ s pate.Style }

func (v *styleValue) String() string        { return "<style>" }
func (v *styleValue) Type() string          { return "style" }
func (v *styleValue) Freeze()               {}
func (v *styleValue) Truth() sl.Bool        { return sl.True }
func (v *styleValue) Hash() (uint32, error) { return 0, errors.New("unhashable: style") }

// --- Environment ---

func newEnv(cfg pate.Config) sl.StringDict {
	d := sl.StringDict{
		// Layout constructors
		"VStack":  sl.NewBuiltin("VStack", builtinVStack),
		"HStack":  sl.NewBuiltin("HStack", builtinHStack),
		"ZStack":  sl.NewBuiltin("ZStack", builtinZStack),
		"Box":     sl.NewBuiltin("Box", builtinBox),
		"Frame":   sl.NewBuiltin("Frame", builtinFrame),
		"Spacer":  sl.NewBuiltin("Spacer", builtinSpacer),
		"Divider": sl.NewBuiltin("Divider", builtinDivider),
		// Widget constructors
		"Text":     sl.NewBuiltin("Text", builtinText),
		"ImageKey": sl.NewBuiltin("ImageKey", builtinImageKey),
		// Style constructor
		"Style": sl.NewBuiltin("Style", builtinStyle),
		// Option constructors
		"Gap":          sl.NewBuiltin("Gap", builtinGap),
		"Padding":      sl.NewBuiltin("Padding", builtinPadding),
		"Background":   sl.NewBuiltin("Background", builtinBackground),
		"Border":       sl.NewBuiltin("Border", builtinBorder),
		"CornerRadius": sl.NewBuiltin("CornerRadius", builtinCornerRadius),
		"Children":     sl.NewBuiltin("Children", builtinChildren),
		// Clip constant
		"Clip": &optValue{pate.Clip},
		// Cross-axis alignment (VStack/HStack)
		"AlignStart":  &optValue{pate.AlignStart},
		"AlignCenter": &optValue{pate.AlignCenter},
		"AlignEnd":    &optValue{pate.AlignEnd},
		// ZStack positioning
		"TopLeading":     &optValue{pate.TopLeading},
		"Top":            &optValue{pate.Top},
		"TopTrailing":    &optValue{pate.TopTrailing},
		"Leading":        &optValue{pate.Leading},
		"Center":         &optValue{pate.Center},
		"Trailing":       &optValue{pate.Trailing},
		"BottomLeading":  &optValue{pate.BottomLeading},
		"Bottom":         &optValue{pate.Bottom},
		"BottomTrailing": &optValue{pate.BottomTrailing},
		// Image scale
		"ImageFit":     &optValue{pate.ImageFit},
		"ImageFill":    &optValue{pate.ImageFill},
		"ImageStretch": &optValue{pate.ImageStretch},
		// Color role constants (accepted by Style(color=...) and Background(...))
		"Foreground": sl.String("foreground"),
		"Accent":     sl.String("accent"),
		"Muted":      sl.String("muted"),
		// Palette color constants (hex strings)
		"Black":     colorHex(pate.Black),
		"DarkGray":  colorHex(pate.DarkGray),
		"LightGray": colorHex(pate.LightGray),
		"White":     colorHex(pate.White),
		"Red":       colorHex(pate.Red),
		"Yellow":    colorHex(pate.Yellow),
	}
	// Named styles from cfg.Styles are injected as capitalized globals.
	// "h1" → H1, "body" → Body, "caption" → Caption, etc.
	for k, v := range cfg.Styles {
		d[capitalizeFirst(k)] = &styleValue{v}
	}
	return d
}

// --- Builtin implementations ---

func builtinVStack(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("vstack: %w", err)
	}
	return &nodeValue{pate.VStack(opts...)}, nil
}

func builtinHStack(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("hstack: %w", err)
	}
	return &nodeValue{pate.HStack(opts...)}, nil
}

func builtinZStack(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("zstack: %w", err)
	}
	return &nodeValue{pate.ZStack(opts...)}, nil
}

func builtinBox(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("box: %w", err)
	}
	return &nodeValue{pate.Box(opts...)}, nil
}

func builtinFrame(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) < 2 {
		return nil, errors.New("frame: expected at least 2 arguments (w, h)")
	}
	w, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("frame: w must be a number, got %s", args[0].Type())
	}
	h, ok := asFloat(args[1])
	if !ok {
		return nil, fmt.Errorf("frame: h must be a number, got %s", args[1].Type())
	}
	opts, err := argsToOptions(args[2:])
	if err != nil {
		return nil, fmt.Errorf("frame: %w", err)
	}
	return &nodeValue{pate.Frame(w, h, opts...)}, nil
}

func builtinSpacer(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	switch len(args) {
	case 0:
		return &nodeValue{pate.Spacer()}, nil
	case 1:
		w, ok := asFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("spacer: weight must be a number, got %s", args[0].Type())
		}
		return &nodeValue{pate.Spacer(w)}, nil
	default:
		return nil, errors.New("spacer: expected at most 1 argument")
	}
}

func builtinDivider(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 0 {
		return nil, errors.New("divider: takes no arguments")
	}
	return &nodeValue{pate.Divider()}, nil
}

func builtinText(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("text: expected at least 1 argument")
	}
	s, ok := sl.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("text: first argument must be a string, got %s", args[0].Type())
	}
	var textOpts []pate.TextOption
	for _, arg := range args[1:] {
		sv, ok := arg.(*styleValue)
		if !ok {
			return nil, fmt.Errorf("text: expected Style, got %s", arg.Type())
		}
		textOpts = append(textOpts, sv.s)
	}
	return &nodeValue{pate.Text(s, textOpts...)}, nil
}

func builtinImageKey(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("imageKey: expected at least 1 argument (key)")
	}
	key, ok := sl.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("imageKey: key must be a string, got %s", args[0].Type())
	}
	opts, err := argsToOptions(args[1:])
	if err != nil {
		return nil, fmt.Errorf("imageKey: %w", err)
	}
	return &nodeValue{pate.ImageKey(key, opts...)}, nil
}

func builtinStyle(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, kwargs []sl.Tuple) (sl.Value, error) {
	if len(args) > 0 {
		return nil, errors.New("style: takes keyword arguments only")
	}
	var s pate.Style
	for _, kv := range kwargs {
		key, _ := sl.AsString(kv[0])
		val := kv[1]
		switch key {
		case "font":
			s.Font, _ = sl.AsString(val)
		case "size":
			s.Size, _ = asFloat(val)
		case "weight":
			w, ok := sl.AsString(val)
			if !ok {
				return nil, errors.New("style: weight must be a string")
			}
			s.Weight = parseWeight(w)
		case "color":
			cs, ok := sl.AsString(val)
			if !ok {
				return nil, errors.New("style: color must be a string")
			}
			c, err := parseColorStr(cs)
			if err != nil {
				return nil, fmt.Errorf("style: %w", err)
			}
			s.Color = c
		case "align":
			a, ok := sl.AsString(val)
			if !ok {
				return nil, errors.New("style: align must be a string")
			}
			s.Align = parseTextAlign(a)
		case "line_height":
			s.LineHeight, _ = asFloat(val)
		case "max_lines":
			n, err := sl.AsInt32(val)
			if err != nil {
				return nil, fmt.Errorf("style: max_lines: %w", err)
			}
			s.MaxLines = int(n)
		case "overflow":
			o, ok := sl.AsString(val)
			if !ok {
				return nil, errors.New("style: overflow must be a string")
			}
			s.Overflow = parseOverflow(o)
		default:
			return nil, fmt.Errorf("style: unknown field %q", key)
		}
	}
	return &styleValue{s}, nil
}

func builtinGap(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("gap: expected 1 argument")
	}
	px, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("gap: expected number, got %s", args[0].Type())
	}
	return &optValue{pate.Gap(px)}, nil
}

func builtinPadding(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("padding: expected 1 argument")
	}
	px, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("padding: expected number, got %s", args[0].Type())
	}
	return &optValue{pate.Padding(px)}, nil
}

// Background(color_string) — accepts role names ("muted", "foreground", …) or "#rrggbb" hex.
func builtinBackground(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("background: expected 1 argument")
	}
	s, ok := sl.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("background: expected string, got %s", args[0].Type())
	}
	c, err := parseColorStr(s)
	if err != nil {
		return nil, fmt.Errorf("background: %w", err)
	}
	return &optValue{pate.Background(c)}, nil
}

// Border(width, color_string) — color accepts role names or "#rrggbb" hex.
func builtinBorder(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 2 {
		return nil, errors.New("border: expected 2 arguments (width, color)")
	}
	w, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("border: width must be a number, got %s", args[0].Type())
	}
	cs, ok := sl.AsString(args[1])
	if !ok {
		return nil, fmt.Errorf("border: color must be a string, got %s", args[1].Type())
	}
	c, err := parseColorStr(cs)
	if err != nil {
		return nil, fmt.Errorf("border: %w", err)
	}
	return &optValue{pate.Border(w, c)}, nil
}

func builtinCornerRadius(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("cornerRadius: expected 1 argument")
	}
	px, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("cornerRadius: expected number, got %s", args[0].Type())
	}
	return &optValue{pate.CornerRadius(px)}, nil
}

// Children accepts individual nodes or a single Starlark list of nodes.
func builtinChildren(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	var nodes []pate.Node
	for _, arg := range args {
		switch v := arg.(type) {
		case *nodeValue:
			nodes = append(nodes, v.n)
		case *sl.List:
			for i := range v.Len() {
				item := v.Index(i)
				nv, ok := item.(*nodeValue)
				if !ok {
					return nil, fmt.Errorf("children: list item must be a node, got %s", item.Type())
				}
				nodes = append(nodes, nv.n)
			}
		default:
			return nil, fmt.Errorf("children: expected node or list, got %s", arg.Type())
		}
	}
	return &optValue{pate.Children(nodes...)}, nil
}

// --- Helpers ---

// argsToOptions converts Starlark args to pate.Option values.
// Accepts *nodeValue (all node types implement Option) and *optValue.
func argsToOptions(args sl.Tuple) ([]pate.Option, error) {
	opts := make([]pate.Option, 0, len(args))
	for _, arg := range args {
		switch v := arg.(type) {
		case *nodeValue:
			opt, ok := v.n.(pate.Option)
			if !ok {
				return nil, fmt.Errorf("node %T is not an Option", v.n)
			}
			opts = append(opts, opt)
		case *optValue:
			opts = append(opts, v.o)
		default:
			return nil, fmt.Errorf("expected node or option, got %s", arg.Type())
		}
	}
	return opts, nil
}

func asFloat(v sl.Value) (float64, bool) {
	switch t := v.(type) {
	case sl.Float:
		return float64(t), true
	case sl.Int:
		n, ok := t.Int64()
		return float64(n), ok
	}
	return 0, false
}

func colorHex(c color.Color) sl.String {
	r, g, b, _ := c.RGBA()
	return sl.String(fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8))
}

func parseColorStr(s string) (color.Color, error) {
	switch s {
	case "foreground", "background", "accent", "muted":
		return pate.RoleColor{Name: s}, nil
	}
	if !strings.HasPrefix(s, "#") {
		return nil, fmt.Errorf("unknown color %q (use a role name or #rrggbb hex)", s)
	}
	hex := s[1:]
	switch len(hex) {
	case 6:
		rv, e1 := strconv.ParseUint(hex[0:2], 16, 8)
		gv, e2 := strconv.ParseUint(hex[2:4], 16, 8)
		bv, e3 := strconv.ParseUint(hex[4:6], 16, 8)
		if e1 != nil || e2 != nil || e3 != nil {
			return nil, fmt.Errorf("invalid color %q", s)
		}
		return color.RGBA{R: uint8(rv), G: uint8(gv), B: uint8(bv), A: 255}, nil
	case 8:
		rv, e1 := strconv.ParseUint(hex[0:2], 16, 8)
		gv, e2 := strconv.ParseUint(hex[2:4], 16, 8)
		bv, e3 := strconv.ParseUint(hex[4:6], 16, 8)
		av, e4 := strconv.ParseUint(hex[6:8], 16, 8)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return nil, fmt.Errorf("invalid color %q", s)
		}
		return color.RGBA{R: uint8(rv), G: uint8(gv), B: uint8(bv), A: uint8(av)}, nil
	}
	return nil, fmt.Errorf("invalid color %q (expected #rrggbb or #rrggbbaa)", s)
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

func capitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// --- Data conversion: Go map → Starlark dict ---

func dataToStarlark(data map[string]any) (*sl.Dict, error) {
	d := new(sl.Dict)
	for k, v := range data {
		sv, err := anyToStarlark(v)
		if err != nil {
			return nil, fmt.Errorf("data[%q]: %w", k, err)
		}
		if err := d.SetKey(sl.String(k), sv); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func anyToStarlark(v any) (sl.Value, error) {
	switch t := v.(type) {
	case nil:
		return sl.None, nil
	case bool:
		return sl.Bool(t), nil
	case int:
		return sl.MakeInt(t), nil
	case int32:
		return sl.MakeInt(int(t)), nil
	case int64:
		return sl.MakeInt64(t), nil
	case float32:
		return sl.Float(float64(t)), nil
	case float64:
		return sl.Float(t), nil
	case string:
		return sl.String(t), nil
	case map[string]any:
		return dataToStarlark(t)
	case []any:
		items := make([]sl.Value, len(t))
		for i, item := range t {
			sv, err := anyToStarlark(item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			items[i] = sv
		}
		return sl.NewList(items), nil
	default:
		return nil, fmt.Errorf("unsupported type %T", v)
	}
}
