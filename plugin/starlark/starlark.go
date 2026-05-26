// Package starlarkplugin exposes demy layout constructors as Starlark builtins.
// A plugin script must define render(data) returning a demy Node tree.
//
// Scripts may optionally declare a module-level "styles" dict to define named styles.
// The host retrieves these via Plugin.Styles() and merges them into Config.Styles
// before rendering:
//
//	styles = {
//	    "h1":   Style(font="sans", size=24),
//	    "body": Style(font="sans", size=12),
//	}
//
//	def render(data):
//	    return VStack(Text(data["title"], styles["h1"]))
//
// Exposed globals:
//
//	Constructors: VStack, HStack, ZStack, Box, Frame, Spacer, Divider, Text, ImageKey, Barcode
//	Style:        Style(font=, size=, weight=, color=, align=, line_height=, max_lines=, overflow=)
//	Options:      Gap, Padding, Background, Border, CornerRadius, Children, Clip
//	Alignment:    AlignStart, AlignCenter, AlignEnd
//	ZAlignment:   TopLeading, Top, TopTrailing, Leading, Center, Trailing,
//	              BottomLeading, Bottom, BottomTrailing
//	ImageScale:   ImageFit, ImageFill, ImageStretch
//	BarcodeType:  BarcodeTypeQR
//	ECLevel:      ECLevelLow, ECLevelMedium, ECLevelQuartile, ECLevelHigh
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

	"github.com/samuel/demy"
)

// Plugin is a compiled Starlark script that exports a render(data) function.
type Plugin struct {
	globals      sl.StringDict
	scriptStyles map[string]demy.Style
}

// Load compiles the Starlark script at path. cfg injects named styles and color
// constants into the script's global environment.
func Load(path string, cfg demy.Config) (*Plugin, error) {
	return execFile(path, nil, cfg)
}

// LoadBytes compiles src as a Starlark script named filename.
func LoadBytes(filename string, src []byte, cfg demy.Config) (*Plugin, error) {
	return execFile(filename, src, cfg)
}

func execFile(filename string, src any, cfg demy.Config) (*Plugin, error) {
	thread := &sl.Thread{Name: filename}
	globals, err := sl.ExecFileOptions(&syntax.FileOptions{}, thread, filename, src, newEnv(cfg))
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: load %s: %w", filename, err)
	}
	if _, ok := globals["render"]; !ok {
		return nil, fmt.Errorf("starlarkplugin: %s must define render(data)", filename)
	}
	p := &Plugin{globals: globals}
	if sv, ok := globals["styles"]; ok {
		if d, ok := sv.(*sl.Dict); ok {
			scriptStyles := make(map[string]demy.Style, d.Len())
			for _, item := range d.Items() {
				k, ok := sl.AsString(item[0])
				if !ok {
					return nil, fmt.Errorf("starlarkplugin: styles key must be a string, got %s", item[0].Type())
				}
				sv, ok := item[1].(*styleValue)
				if !ok {
					return nil, fmt.Errorf("starlarkplugin: styles[%q] must be a Style value, got %s", k, item[1].Type())
				}
				scriptStyles[k] = sv.s
			}
			if len(scriptStyles) > 0 {
				p.scriptStyles = scriptStyles
			}
		}
	}
	return p, nil
}

// Styles returns named style definitions declared in the script's module-level
// "styles" dict. Merge these into Config.Styles before calling Canvas.Render so
// that Class("h1") references in the returned node tree resolve correctly.
func (p *Plugin) Styles() map[string]demy.Style {
	return p.scriptStyles
}

// Render calls the script's render(data) and returns the root Node tree.
func (p *Plugin) Render(data map[string]any) (demy.Node, error) {
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

type nodeValue struct{ n demy.Node }

func (v *nodeValue) String() string        { return fmt.Sprintf("<%T>", v.n) }
func (v *nodeValue) Type() string          { return "node" }
func (v *nodeValue) Freeze()               {}
func (v *nodeValue) Truth() sl.Bool        { return sl.True }
func (v *nodeValue) Hash() (uint32, error) { return 0, errors.New("unhashable: node") }

type optValue struct{ o demy.Option }

func (v *optValue) String() string        { return fmt.Sprintf("<%T>", v.o) }
func (v *optValue) Type() string          { return "option" }
func (v *optValue) Freeze()               {}
func (v *optValue) Truth() sl.Bool        { return sl.True }
func (v *optValue) Hash() (uint32, error) { return 0, errors.New("unhashable: option") }

type styleValue struct{ s demy.Style }

func (v *styleValue) String() string        { return "<style>" }
func (v *styleValue) Type() string          { return "style" }
func (v *styleValue) Freeze()               {}
func (v *styleValue) Truth() sl.Bool        { return sl.True }
func (v *styleValue) Hash() (uint32, error) { return 0, errors.New("unhashable: style") }

// --- Environment ---

func newEnv(cfg demy.Config) sl.StringDict {
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
		"Barcode":  sl.NewBuiltin("Barcode", builtinBarcode),
		// Style constructor
		"Style": sl.NewBuiltin("Style", func(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, kwargs []sl.Tuple) (sl.Value, error) {
			return builtinStyleWithClass(args, kwargs, cfg)
		}),
		// Option constructors
		"Gap":          sl.NewBuiltin("Gap", builtinGap),
		"Padding":      sl.NewBuiltin("Padding", builtinPadding),
		"Background":   sl.NewBuiltin("Background", builtinBackground),
		"Border":       sl.NewBuiltin("Border", builtinBorder),
		"CornerRadius": sl.NewBuiltin("CornerRadius", builtinCornerRadius),
		"Children":     sl.NewBuiltin("Children", builtinChildren),
		// Clip constant
		"Clip": &optValue{demy.Clip},
		// Cross-axis alignment (VStack/HStack)
		"AlignStart":  &optValue{demy.AlignStart},
		"AlignCenter": &optValue{demy.AlignCenter},
		"AlignEnd":    &optValue{demy.AlignEnd},
		// ZStack positioning
		"TopLeading":     &optValue{demy.TopLeading},
		"Top":            &optValue{demy.Top},
		"TopTrailing":    &optValue{demy.TopTrailing},
		"Leading":        &optValue{demy.Leading},
		"Center":         &optValue{demy.Center},
		"Trailing":       &optValue{demy.Trailing},
		"BottomLeading":  &optValue{demy.BottomLeading},
		"Bottom":         &optValue{demy.Bottom},
		"BottomTrailing": &optValue{demy.BottomTrailing},
		// Image scale
		"ImageFit":     &optValue{demy.ImageFit},
		"ImageFill":    &optValue{demy.ImageFill},
		"ImageStretch": &optValue{demy.ImageStretch},
		// Barcode type
		"BarcodeTypeQR": &optValue{demy.BarcodeTypeQR},
		// QR error correction level
		"ECLevelLow":      &optValue{demy.ECLevelLow},
		"ECLevelMedium":   &optValue{demy.ECLevelMedium},
		"ECLevelQuartile": &optValue{demy.ECLevelQuartile},
		"ECLevelHigh":     &optValue{demy.ECLevelHigh},
		// Color role constants (accepted by Style(color=...) and Background(...))
		"Foreground": sl.String("foreground"),
		"Accent":     sl.String("accent"),
		"Muted":      sl.String("muted"),
		// Palette color constants (hex strings)
		"Black":     colorHex(demy.Black),
		"DarkGray":  colorHex(demy.DarkGray),
		"LightGray": colorHex(demy.LightGray),
		"White":     colorHex(demy.White),
		"Red":       colorHex(demy.Red),
		"Yellow":    colorHex(demy.Yellow),
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
	return &nodeValue{demy.VStack(opts...)}, nil
}

func builtinHStack(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("hstack: %w", err)
	}
	return &nodeValue{demy.HStack(opts...)}, nil
}

func builtinZStack(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("zstack: %w", err)
	}
	return &nodeValue{demy.ZStack(opts...)}, nil
}

func builtinBox(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	opts, err := argsToOptions(args)
	if err != nil {
		return nil, fmt.Errorf("box: %w", err)
	}
	return &nodeValue{demy.Box(opts...)}, nil
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
	return &nodeValue{demy.Frame(w, h, opts...)}, nil
}

func builtinSpacer(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	switch len(args) {
	case 0:
		return &nodeValue{demy.Spacer()}, nil
	case 1:
		w, ok := asFloat(args[0])
		if !ok {
			return nil, fmt.Errorf("spacer: weight must be a number, got %s", args[0].Type())
		}
		return &nodeValue{demy.Spacer(w)}, nil
	default:
		return nil, errors.New("spacer: expected at most 1 argument")
	}
}

func builtinDivider(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 0 {
		return nil, errors.New("divider: takes no arguments")
	}
	return &nodeValue{demy.Divider()}, nil
}

func builtinText(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("text: expected at least 1 argument")
	}
	s, ok := sl.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("text: first argument must be a string, got %s", args[0].Type())
	}
	var textOpts []demy.TextOption
	for _, arg := range args[1:] {
		sv, ok := arg.(*styleValue)
		if !ok {
			return nil, fmt.Errorf("text: expected Style, got %s", arg.Type())
		}
		textOpts = append(textOpts, sv.s)
	}
	return &nodeValue{demy.Text(s, textOpts...)}, nil
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
	return &nodeValue{demy.ImageKey(key, opts...)}, nil
}

func builtinBarcode(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("barcode: expected at least 1 argument (content)")
	}
	content, ok := sl.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("barcode: content must be a string, got %s", args[0].Type())
	}
	opts, err := argsToOptions(args[1:])
	if err != nil {
		return nil, fmt.Errorf("barcode: %w", err)
	}
	return &nodeValue{demy.Barcode(content, opts...)}, nil
}

func builtinStyleWithClass(args sl.Tuple, kwargs []sl.Tuple, cfg demy.Config) (sl.Value, error) {
	if len(args) > 0 {
		return nil, errors.New("style: takes keyword arguments only")
	}
	var s demy.Style
	// Pass 1: resolve class as base so per-field kwargs override it regardless of order.
	for _, kv := range kwargs {
		if key, _ := sl.AsString(kv[0]); key == "class" {
			name, ok := sl.AsString(kv[1])
			if !ok {
				return nil, errors.New("style: class must be a string")
			}
			if classStyle, ok := cfg.Styles[name]; ok {
				s = classStyle
			}
			break
		}
	}
	// Pass 2: apply per-field overrides.
	for _, kv := range kwargs {
		key, _ := sl.AsString(kv[0])
		val := kv[1]
		switch key {
		case "class":
			// handled in pass 1
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
			s.MaxLines = n
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
	return &optValue{demy.Gap(px)}, nil
}

func builtinPadding(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("padding: expected 1 argument")
	}
	px, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("padding: expected number, got %s", args[0].Type())
	}
	return &optValue{demy.Padding(px)}, nil
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
	return &optValue{demy.Background(c)}, nil
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
	return &optValue{demy.Border(w, c)}, nil
}

func builtinCornerRadius(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("cornerRadius: expected 1 argument")
	}
	px, ok := asFloat(args[0])
	if !ok {
		return nil, fmt.Errorf("cornerRadius: expected number, got %s", args[0].Type())
	}
	return &optValue{demy.CornerRadius(px)}, nil
}

// Children accepts individual nodes or a single Starlark list of nodes.
func builtinChildren(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, _ []sl.Tuple) (sl.Value, error) {
	var nodes []demy.Node
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
	return &optValue{demy.Children(nodes...)}, nil
}

// --- Helpers ---

// argsToOptions converts Starlark args to demy.Option values.
// Accepts *nodeValue (all node types implement Option) and *optValue.
func argsToOptions(args sl.Tuple) ([]demy.Option, error) {
	opts := make([]demy.Option, 0, len(args))
	for _, arg := range args {
		switch v := arg.(type) {
		case *nodeValue:
			opt, ok := v.n.(demy.Option)
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
		return demy.RoleColor{Name: s}, nil
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
		return sl.Float(t), nil
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
