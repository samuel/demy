// Package starlarkplugin runs Starlark scripts that build demy Node trees.
// A script must define render(data), which receives the map passed to
// Plugin.Render as a dict and returns a node.
//
// Scripts may declare a module-level "styles" dict of named styles. The host
// retrieves it with Plugin.Styles and merges it into Config.Styles before
// rendering; Class(name) references resolve against the merged styles at render
// time:
//
//	styles = {
//	    "h1":   Style(font="sans", size=24),
//	    "body": Style(font="sans", size=12),
//	}
//
//	def render(data):
//	    return VStack(Text(data["title"], Class("h1")))
//
// To expose custom Go functions to scripts, register them on an Environment
// before loading the script:
//
//	env := &starlarkplugin.Environment{}
//	env.AddModule("svc", map[string]starlarkplugin.BuiltinFunc{
//	    "fetch": func(thread *starlark.Thread, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
//	        ctx := starlarkplugin.ContextFromThread(thread)
//	        return fetch(ctx, args) // fetch calls a remote service
//	    },
//	})
//	p, err := env.Load(ctx, "display.star", cfg, time.Local)
//	node, err := p.Render(r.Context(), data)
//	img, err := canvas.Render(r.Context(), node, demy.ColorModeDefault)
//
// Module execution and each Render call are limited to Environment.MaxSteps
// Starlark execution steps (DefaultMaxSteps when zero) and are cancelled when
// their context is done. print() output goes to Environment.Logger, truncated
// to 4 KiB per message and 64 KiB per Load or Render.
//
// A panic in a builtin registered with AddBuiltin or AddModule fails the call
// with an error. Numeric arguments must be in [0, 32768], and Render data may
// nest at most 100 levels, hold at most 262144 values, and not be cyclic.
// Errors from Load and Render start with the script position that failed.
//
// Configure an Environment before the first Load and don't modify it while a
// Load is running. A Plugin keeps the settings and globals the Environment had
// when it was loaded, so later changes apply only to later Loads.
//
// Exposed globals:
//
//	Containers:   VStack(...), HStack(...), ZStack(...), Box(...), Grid(...),
//	              Frame(w, h, ...), GridRow(nodes...), Weighted(weight, child)
//	Nodes:        Text(s, style_or_class...), ImageKey(key, ...), Barcode(content, ...),
//	              Spacer(weight?), Divider(size=, color=)
//	Style:        Style(class=, font=, size=, weight=, color=, align=, line_height=,
//	              max_lines=, overflow=)
//	              class= copies a named style from Config.Styles (it must exist);
//	              other fields override it. weight is "regular", "thin" … "black",
//	              optionally suffixed with "-italic", or "italic"; align is "left",
//	              "center", or "right"; overflow is "wrap", "clip", or "ellipsis";
//	              max_lines=-1 is unlimited. size= is ignored for bitmap fonts.
//	Class:        Class(name) references a named style resolved at render time,
//	              including styles from the script's own styles dict.
//	Options:      Gap(px), Padding(px), Background(color), Border(width, color),
//	              CornerRadius(px), Children(nodes or list), Clip,
//	              Columns(tracks or list), Rows(tracks or list), RowGap(px), ColGap(px)
//	Grid tracks:  GridFixed(px), GridAuto(), GridFr(weight)
//	Alignment:    AlignStart, AlignCenter, AlignEnd
//	ZAlignment:   ZAlignTopLeading, ZAlignTop, ZAlignTopTrailing, ZAlignLeading,
//	              ZAlignCenter, ZAlignTrailing, ZAlignBottomLeading, ZAlignBottom,
//	              ZAlignBottomTrailing
//	ImageScale:   ImageFit, ImageFill, ImageStretch
//	Dither:       FloydSteinberg, Atkinson (options to ImageKey)
//	BarcodeType:  BarcodeTypeQR
//	ECLevel:      ECLevelLow, ECLevelMedium, ECLevelQuartile, ECLevelHigh
//	Colors:       Foreground, Accent, Muted (role names), Black, DarkGray, LightGray,
//	              White, Red, Yellow, Green, Blue, Orange (hex strings)
//	Time:         now() → time value with .year .month .day .hour .minute .second
//	              .weekday (0=Sun) .weekday_name .timezone .unix and .format(layout),
//	              where layout is a Go time layout such as "Mon Jan 2 15:04"
//
// Builtins reject unknown keyword arguments, wrong argument types, and unknown
// enum names. Color arguments accept role names ("foreground", "background",
// "accent", "muted"; there is no Background color constant because the name is
// the Background option) or "#rgb", "#rrggbb", "#rrggbbaa" hex strings. Named
// styles from Config.Styles are also exposed as capitalized globals (e.g. "h1" →
// H1) when the name is an identifier that doesn't shadow a builtin; use
// Class(name) for the rest. Data passed to Render is converted with sorted map
// keys, so iteration order is stable.
//
// # Untrusted scripts
//
// MaxSteps and the context bound a script's execution time, but nothing bounds
// its memory: one step can allocate hundreds of megabytes ("x" * (1 << 29)),
// and Go can't limit one goroutine's memory. Run untrusted scripts in a
// process or runtime with an enforced memory limit, such as a separate process
// in a cgroup or container or with RLIMIT_AS, or a WASM build of the host in a
// runtime with a memory cap, and send the node tree back with jsonscene.
// Builtins added with AddBuiltin and AddModule run with the host's privileges.
package starlarkplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	sl "go.starlark.net/starlark"
	slstruct "go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/codec"
)

// BuiltinFunc is the signature for Go functions exposed as Starlark builtins.
// Use ContextFromThread(thread) to retrieve the context.Context passed to Render.
type BuiltinFunc func(thread *sl.Thread, args sl.Tuple, kwargs []sl.Tuple) (sl.Value, error)

// DefaultMaxSteps is the execution step limit used when Environment.MaxSteps is zero.
const DefaultMaxSteps = 10_000_000

// Environment holds user-registered globals merged into the Starlark execution
// environment when a script is compiled. Its zero value is valid (no extra globals).
//
// Configure it before the first Load. Load copies the fields and globals into
// the Plugin, so changing the Environment afterwards doesn't affect existing
// Plugins. Load may be called concurrently once the Environment is no longer
// being modified.
type Environment struct {
	// MaxSteps limits Starlark execution steps for module execution and for each
	// Render call. Zero means DefaultMaxSteps.
	MaxSteps uint64
	// Logger receives script print() output. Nil means slog.Default().
	Logger *slog.Logger
	// Now returns the current time for the now() builtin. Nil means time.Now;
	// set it for deterministic previews and tests.
	Now func() time.Time

	extras sl.StringDict
}

func (e *Environment) initExtras() {
	if e.extras == nil {
		e.extras = make(sl.StringDict)
	}
}

// AddBuiltin registers a Go function as a named Starlark global.
// If the name conflicts with a demy built-in it will shadow it.
func (e *Environment) AddBuiltin(name string, fn BuiltinFunc) *Environment {
	e.initExtras()
	e.extras[name] = sl.NewBuiltin(name, recoverCall(fn))
	return e
}

// recoverCall wraps fn so that a panic becomes an error, which the script
// sees as a failed call at the line that made it.
func recoverCall(fn BuiltinFunc) func(*sl.Thread, *sl.Builtin, sl.Tuple, []sl.Tuple) (sl.Value, error) {
	return func(t *sl.Thread, b *sl.Builtin, args sl.Tuple, kwargs []sl.Tuple) (v sl.Value, err error) {
		defer func() {
			if r := recover(); r != nil {
				v, err = nil, fmt.Errorf("%s: panic: %v", b.Name(), r)
			}
		}()
		return fn(t, args, kwargs)
	}
}

// withPosition prefixes a Starlark evaluation error with the innermost script
// position in its call stack, so the error names the line that failed.
func withPosition(err error) error {
	var evalErr *sl.EvalError
	if !errors.As(err, &evalErr) {
		return err
	}
	for _, frame := range slices.Backward(evalErr.CallStack) {
		if frame.Pos.IsValid() && frame.Pos.Filename() != "<builtin>" {
			return fmt.Errorf("%s: %w", frame.Pos, err)
		}
	}
	return err
}

// recoverError sets *err to an error describing a recovered panic, if any.
// Deferred by Load and Render as a backstop for panics outside builtins.
func recoverError(err *error, what string) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("starlarkplugin: %s: panic: %v", what, r)
	}
}

// AddValue registers an arbitrary Starlark value as a named global. The value
// is frozen, since every script and render shares it.
func (e *Environment) AddValue(name string, v sl.Value) *Environment {
	e.initExtras()
	v.Freeze()
	e.extras[name] = v
	return e
}

// AddModule registers a group of functions as a named Starlark module, accessible
// in scripts as name.fn(args).
func (e *Environment) AddModule(name string, fns map[string]BuiltinFunc) *Environment {
	e.initExtras()
	members := make(sl.StringDict, len(fns))
	for fnName, fn := range fns {
		members[fnName] = sl.NewBuiltin(fnName, recoverCall(fn))
	}
	e.extras[name] = &slstruct.Module{Name: name, Members: members}
	return e
}

// Load compiles and executes the Starlark script at path.
// loc sets the timezone used by the now() builtin; nil defaults to UTC.
func (e *Environment) Load(ctx context.Context, path string, cfg demy.Config, loc *time.Location) (*Plugin, error) {
	return e.execFile(ctx, path, nil, cfg, loc)
}

// LoadBytes compiles and executes src as a Starlark script named filename.
// loc sets the timezone used by the now() builtin; nil defaults to UTC.
func (e *Environment) LoadBytes(ctx context.Context, filename string, src []byte, cfg demy.Config, loc *time.Location) (*Plugin, error) {
	return e.execFile(ctx, filename, src, cfg, loc)
}

// threadConfig holds the Environment settings that a Plugin captures at Load,
// with defaults applied.
type threadConfig struct {
	maxSteps uint64
	logger   *slog.Logger
}

func (e *Environment) threadConfig() threadConfig {
	tc := threadConfig{maxSteps: e.MaxSteps, logger: e.Logger}
	if tc.maxSteps == 0 {
		tc.maxSteps = DefaultMaxSteps
	}
	if tc.logger == nil {
		tc.logger = slog.Default()
	}
	return tc
}

// newThread returns a thread limited to the step budget whose print() output
// goes to the logger. The returned stop function must be called when execution
// finishes; until then, ctx being done cancels the thread.
func (tc threadConfig) newThread(ctx context.Context, name string) (*sl.Thread, func() bool) {
	logger := tc.logger
	var printed int // bytes logged by this thread
	thread := &sl.Thread{
		Name: name,
		Print: func(t *sl.Thread, msg string) {
			ctx := ContextFromThread(t)
			if printed >= maxPrintTotal {
				return
			}
			msg = truncateUTF8(msg, min(maxPrintBytes, maxPrintTotal-printed))
			printed += len(msg)
			logger.InfoContext(ctx, msg, "script", name)
			if printed >= maxPrintTotal {
				logger.WarnContext(ctx, "print output limit reached; further output is dropped", "script", name, "limit", maxPrintTotal)
			}
		},
	}
	thread.SetMaxExecutionSteps(tc.maxSteps)
	thread.SetLocal(contextKey, ctx)
	stop := context.AfterFunc(ctx, func() {
		thread.Cancel(context.Cause(ctx).Error())
	})
	return thread, stop
}

// maxPrintBytes bounds one print() message; longer messages are truncated.
const maxPrintBytes = 4096

// maxPrintTotal bounds print() output per Load or Render; later output is
// dropped.
const maxPrintTotal = 1 << 16

// truncateUTF8 shortens s to at most n bytes without splitting a character,
// marking a shortened string with "…" within the n bytes.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const ellipsis = "…"
	n = max(n-len(ellipsis), 0)
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + ellipsis
}

// contextKey is the thread-local key holding the context.Context passed to Render.
const contextKey = "demy/ctx"

// ContextFromThread retrieves the context.Context set by Render.
// Returns context.Background() if no context has been set.
func ContextFromThread(t *sl.Thread) context.Context {
	if v := t.Local(contextKey); v != nil {
		return v.(context.Context) //nolint:forcetypeassert
	}
	return context.Background()
}

// ToStarlark converts a Go value to a Starlark value. It accepts nil, Starlark
// values, bools, strings, all integer and float kinds, json.Number, time.Time,
// pointers, slices, arrays, and string-keyed maps of these. It rejects cyclic
// data, nesting deeper than 100 levels, and more than 262144 values.
func ToStarlark(v any) (sl.Value, error) {
	return anyToStarlark(v)
}

// WrapNode wraps a demy.Node as a Starlark value that can be returned from a
// BuiltinFunc and passed to layout constructors in scripts. Don't modify n
// after wrapping it: scripts can place it in several trees and keep it across
// renders, and the trees Render returns hold n itself, not a copy.
func WrapNode(n demy.Node) sl.Value {
	return &nodeValue{n}
}

// UnwrapNode extracts a demy.Node from a Starlark value produced by WrapNode or
// a layout constructor. Returns false if v is not a node value.
func UnwrapNode(v sl.Value) (demy.Node, bool) {
	nv, ok := v.(*nodeValue)
	if !ok {
		return nil, false
	}
	return nv.n, true
}

// Plugin is a compiled Starlark script that exports a render(data) function.
// Its module globals are frozen after Load, so Render may be called
// concurrently.
type Plugin struct {
	threads      threadConfig
	name         string
	globals      sl.StringDict
	scriptStyles map[string]demy.Style
}

func (e *Environment) execFile(ctx context.Context, filename string, src any, cfg demy.Config, loc *time.Location) (_ *Plugin, err error) {
	defer recoverError(&err, "load "+filename)
	if ctx == nil {
		ctx = context.Background()
	}
	if loc == nil {
		loc = time.UTC
	}
	now := e.Now
	if now == nil {
		now = time.Now
	}
	env := newEnv(cfg, loc, now)
	maps.Copy(env, e.extras)
	threads := e.threadConfig()
	thread, stop := threads.newThread(ctx, filename)
	globals, err := sl.ExecFileOptions(&syntax.FileOptions{}, thread, filename, src, env)
	stop()
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: load %s: %w", filename, withPosition(err))
	}
	if _, ok := globals["render"]; !ok {
		return nil, fmt.Errorf("starlarkplugin: %s must define render(data)", filename)
	}
	p := &Plugin{threads: threads, name: filename, globals: globals}
	if sv, ok := globals["styles"]; ok {
		d, ok := sv.(*sl.Dict)
		if !ok {
			return nil, fmt.Errorf("starlarkplugin: %s: styles must be a dict, got %s", filename, sv.Type())
		}
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
	return p, nil
}

// Styles returns a copy of the named styles declared in the script's
// module-level "styles" dict, or nil if there are none. Merge these into
// Config.Styles before rendering so that Class(name) references in the returned
// node tree resolve.
func (p *Plugin) Styles() map[string]demy.Style {
	return maps.Clone(p.scriptStyles)
}

// Render calls the script's render(data) and returns the root Node tree.
// ctx is propagated to user-provided builtins via ContextFromThread, and
// cancels the script when done. The returned tree may share nodes with
// module-level values and other renders, so treat it as read-only.
func (p *Plugin) Render(ctx context.Context, data map[string]any) (_ demy.Node, err error) {
	defer recoverError(&err, "render")
	if ctx == nil {
		ctx = context.Background()
	}
	d, err := dataToStarlark(data)
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: %w", err)
	}
	thread, stop := p.threads.newThread(ctx, p.name)
	defer stop()
	result, err := sl.Call(thread, p.globals["render"], sl.Tuple{d}, nil)
	if err != nil {
		return nil, fmt.Errorf("starlarkplugin: render: %w", withPosition(err))
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

type trackValue struct{ t demy.GridTrack }

func (v *trackValue) String() string        { return "<track>" }
func (v *trackValue) Type() string          { return "track" }
func (v *trackValue) Freeze()               {}
func (v *trackValue) Truth() sl.Bool        { return sl.True }
func (v *trackValue) Hash() (uint32, error) { return 0, errors.New("unhashable: track") }

// classValue is a StyleClass reference produced by Class(name).
type classValue struct{ c demy.StyleClass }

func (v *classValue) String() string        { return fmt.Sprintf("Class(%q)", string(v.c)) }
func (v *classValue) Type() string          { return "class" }
func (v *classValue) Freeze()               {}
func (v *classValue) Truth() sl.Bool        { return sl.True }
func (v *classValue) Hash() (uint32, error) { return 0, errors.New("unhashable: class") }

// --- Environment ---

func newEnv(cfg demy.Config, loc *time.Location, now func() time.Time) sl.StringDict {
	d := sl.StringDict{
		// Layout constructors
		"VStack":   positional("VStack", containerOf(demy.VStack)),
		"HStack":   positional("HStack", containerOf(demy.HStack)),
		"ZStack":   positional("ZStack", containerOf(demy.ZStack)),
		"Box":      positional("Box", containerOf(demy.Box)),
		"Grid":     positional("Grid", containerOf(demy.Grid)),
		"Frame":    positional("Frame", builtinFrame),
		"Spacer":   positional("Spacer", builtinSpacer),
		"Weighted": positional("Weighted", builtinWeighted),
		"Divider":  sl.NewBuiltin("Divider", builtinDivider),
		"GridRow":  positional("GridRow", builtinGridRow),
		// Grid track constructors
		"GridFixed": positional("GridFixed", trackOf(demy.GridFixed, "px")),
		"GridAuto":  positional("GridAuto", builtinGridAuto),
		"GridFr":    positional("GridFr", trackOf(demy.GridFr, "weight")),
		// Grid layout options
		"Columns": positional("Columns", tracksOpt(demy.Columns)),
		"Rows":    positional("Rows", tracksOpt(demy.Rows)),
		"RowGap":  positional("RowGap", numberOpt(demy.RowGap)),
		"ColGap":  positional("ColGap", numberOpt(demy.ColGap)),
		// Widget constructors
		"Text":     positional("Text", builtinText),
		"Class":    positional("Class", builtinClass),
		"ImageKey": positional("ImageKey", builtinImageKey),
		"Barcode":  positional("Barcode", builtinBarcode),
		// Style constructor
		"Style": sl.NewBuiltin("Style", func(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, kwargs []sl.Tuple) (sl.Value, error) {
			v, err := builtinStyle(args, kwargs, cfg)
			if err != nil {
				return nil, builtinError("Style", err)
			}
			return v, nil
		}),
		// Option constructors
		"Gap":          positional("Gap", numberOpt(demy.Gap)),
		"Padding":      positional("Padding", numberOpt(demy.Padding)),
		"CornerRadius": positional("CornerRadius", numberOpt(demy.CornerRadius)),
		"Background":   positional("Background", builtinBackground),
		"Border":       positional("Border", builtinBorder),
		"Children":     positional("Children", builtinChildren),
		// Clip constant
		"Clip": &optValue{demy.Clip},
		// Cross-axis alignment (VStack/HStack)
		"AlignStart":  &optValue{demy.AlignStart},
		"AlignCenter": &optValue{demy.AlignCenter},
		"AlignEnd":    &optValue{demy.AlignEnd},
		// ZStack positioning
		"ZAlignTopLeading":     &optValue{demy.ZAlignTopLeading},
		"ZAlignTop":            &optValue{demy.ZAlignTop},
		"ZAlignTopTrailing":    &optValue{demy.ZAlignTopTrailing},
		"ZAlignLeading":        &optValue{demy.ZAlignLeading},
		"ZAlignCenter":         &optValue{demy.ZAlignCenter},
		"ZAlignTrailing":       &optValue{demy.ZAlignTrailing},
		"ZAlignBottomLeading":  &optValue{demy.ZAlignBottomLeading},
		"ZAlignBottom":         &optValue{demy.ZAlignBottom},
		"ZAlignBottomTrailing": &optValue{demy.ZAlignBottomTrailing},
		// Image scale
		"ImageFit":     &optValue{demy.ImageFit},
		"ImageFill":    &optValue{demy.ImageFill},
		"ImageStretch": &optValue{demy.ImageStretch},
		// Image dither algorithms
		"FloydSteinberg": &optValue{demy.FloydSteinberg},
		"Atkinson":       &optValue{demy.Atkinson},
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
		"Green":     colorHex(demy.Green),
		"Blue":      colorHex(demy.Blue),
		"Orange":    colorHex(demy.Orange),
		// Time
		"now": positional("now", func(args sl.Tuple) (sl.Value, error) {
			if len(args) != 0 {
				return nil, errors.New("takes no arguments")
			}
			return &timeValue{t: now().In(loc)}, nil
		}),
	}
	// Named styles are exposed as capitalized globals ("h1" → H1) when the
	// result is an identifier that doesn't shadow a builtin; every style is
	// also reachable with Class(name).
	for k, v := range cfg.Styles {
		name := capitalizeFirst(k)
		if _, taken := d[name]; !taken && isIdent(name) {
			d[name] = &styleValue{v}
		}
	}
	return d
}

// positional wraps a builtin that takes only positional arguments, rejecting
// keyword arguments and prefixing errors with the builtin's name.
func positional(name string, fn func(args sl.Tuple) (sl.Value, error)) *sl.Builtin {
	return sl.NewBuiltin(name, func(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, kwargs []sl.Tuple) (sl.Value, error) {
		if len(kwargs) > 0 {
			return nil, builtinError(name, fmt.Errorf("unexpected keyword argument %s", kwargs[0][0]))
		}
		v, err := fn(args)
		if err != nil {
			return nil, builtinError(name, err)
		}
		return v, nil
	})
}

// builtinError prefixes err with the name of the builtin that returned it.
func builtinError(name string, err error) error {
	return fmt.Errorf("%s: %w", name, err)
}

// --- Builtin implementations ---

func containerOf[N demy.Node](ctor func(...demy.Option) N) func(sl.Tuple) (sl.Value, error) {
	return func(args sl.Tuple) (sl.Value, error) {
		opts, err := argsToOptions(args)
		if err != nil {
			return nil, err
		}
		return &nodeValue{ctor(opts...)}, nil
	}
}

func numberOpt(ctor func(float64) demy.Option) func(sl.Tuple) (sl.Value, error) {
	return func(args sl.Tuple) (sl.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 argument, got %d", len(args))
		}
		v, err := toFloat("argument", args[0])
		if err != nil {
			return nil, err
		}
		return &optValue{ctor(v)}, nil
	}
}

func trackOf(ctor func(float64) demy.GridTrack, what string) func(sl.Tuple) (sl.Value, error) {
	return func(args sl.Tuple) (sl.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("expected 1 argument (%s), got %d", what, len(args))
		}
		v, err := toFloat(what, args[0])
		if err != nil {
			return nil, err
		}
		return &trackValue{ctor(v)}, nil
	}
}

func tracksOpt(ctor func(...demy.GridTrack) demy.Option) func(sl.Tuple) (sl.Value, error) {
	return func(args sl.Tuple) (sl.Value, error) {
		tracks, err := parseTracks(args)
		if err != nil {
			return nil, err
		}
		return &optValue{ctor(tracks...)}, nil
	}
}

func builtinFrame(args sl.Tuple) (sl.Value, error) {
	if len(args) < 2 {
		return nil, errors.New("expected at least 2 arguments (w, h)")
	}
	w, err := toFloat("w", args[0])
	if err != nil {
		return nil, err
	}
	h, err := toFloat("h", args[1])
	if err != nil {
		return nil, err
	}
	opts, err := argsToOptions(args[2:])
	if err != nil {
		return nil, err
	}
	return &nodeValue{demy.Frame(w, h, opts...)}, nil
}

func builtinSpacer(args sl.Tuple) (sl.Value, error) {
	switch len(args) {
	case 0:
		return &nodeValue{demy.Spacer()}, nil
	case 1:
		w, err := toFloat("weight", args[0])
		if err != nil {
			return nil, err
		}
		return &nodeValue{demy.Spacer(w)}, nil
	default:
		return nil, errors.New("expected at most 1 argument (weight)")
	}
}

func builtinWeighted(args sl.Tuple) (sl.Value, error) {
	if len(args) != 2 {
		return nil, errors.New("expected 2 arguments (weight, child)")
	}
	w, err := toFloat("weight", args[0])
	if err != nil {
		return nil, err
	}
	nv, ok := args[1].(*nodeValue)
	if !ok {
		return nil, fmt.Errorf("child must be a node, got %s", args[1].Type())
	}
	return &nodeValue{demy.Weighted(w, nv.n)}, nil
}

func builtinDivider(_ *sl.Thread, _ *sl.Builtin, args sl.Tuple, kwargs []sl.Tuple) (sl.Value, error) {
	if len(args) != 0 {
		return nil, builtinError("Divider", errors.New("takes keyword arguments only (size, color)"))
	}
	n := demy.Divider()
	for _, kv := range kwargs {
		key, _ := sl.AsString(kv[0])
		var err error
		switch key {
		case "size":
			n.Size, err = toFloat("size", kv[1])
		case "color":
			n.Color, err = toColor("color", kv[1])
		default:
			err = fmt.Errorf("unknown keyword argument %q", key)
		}
		if err != nil {
			return nil, builtinError("Divider", err)
		}
	}
	return &nodeValue{n}, nil
}

func builtinText(args sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("expected at least 1 argument")
	}
	s, err := toString("text", args[0])
	if err != nil {
		return nil, err
	}
	var textOpts []demy.TextOption
	for _, arg := range args[1:] {
		switch v := arg.(type) {
		case *styleValue:
			textOpts = append(textOpts, v.s)
		case *classValue:
			textOpts = append(textOpts, v.c)
		default:
			return nil, fmt.Errorf("expected Style or Class, got %s", arg.Type())
		}
	}
	return &nodeValue{demy.Text(s, textOpts...)}, nil
}

// builtinClass references a named style resolved at render time against
// Config.Styles, which may include the script's own styles dict.
func builtinClass(args sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("expected 1 argument (name)")
	}
	name, err := toString("name", args[0])
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errors.New("name must not be empty")
	}
	return &classValue{demy.Class(name)}, nil
}

func builtinImageKey(args sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("expected at least 1 argument (key)")
	}
	key, err := toString("key", args[0])
	if err != nil {
		return nil, err
	}
	opts, err := argsToOptions(args[1:])
	if err != nil {
		return nil, err
	}
	return &nodeValue{demy.ImageKey(key, opts...)}, nil
}

func builtinBarcode(args sl.Tuple) (sl.Value, error) {
	if len(args) < 1 {
		return nil, errors.New("expected at least 1 argument (content)")
	}
	content, err := toString("content", args[0])
	if err != nil {
		return nil, err
	}
	opts, err := argsToOptions(args[1:])
	if err != nil {
		return nil, err
	}
	return &nodeValue{demy.Barcode(content, opts...)}, nil
}

// builtinStyle builds a Style from keyword arguments. class= copies a named
// style from Config.Styles as the base; the other fields override it.
func builtinStyle(args sl.Tuple, kwargs []sl.Tuple, cfg demy.Config) (sl.Value, error) {
	if len(args) > 0 {
		return nil, errors.New("takes keyword arguments only")
	}
	var s demy.Style
	// Pass 1: resolve class as base so per-field kwargs override it regardless of order.
	for _, kv := range kwargs {
		if key, _ := sl.AsString(kv[0]); key == "class" {
			name, err := toString("class", kv[1])
			if err != nil {
				return nil, err
			}
			classStyle, ok := cfg.Styles[name]
			if !ok {
				return nil, fmt.Errorf("class %q not found in Config.Styles (use Class(%q) for styles merged at render time)", name, name)
			}
			s = classStyle
			break
		}
	}
	// Pass 2: apply per-field overrides.
	for _, kv := range kwargs {
		key, _ := sl.AsString(kv[0])
		val := kv[1]
		var (
			err error
			str string
		)
		switch key {
		case "class":
			// handled in pass 1
		case "font":
			s.Font, err = toString("font", val)
		case "size":
			s.Size, err = toFloat("size", val)
		case "line_height":
			s.LineHeight, err = toFloat("line_height", val)
		case "color":
			s.Color, err = toColor("color", val)
		case "max_lines":
			s.MaxLines, err = sl.AsInt32(val)
			if err == nil && s.MaxLines < demy.MaxLinesUnlimited {
				err = fmt.Errorf("max_lines %d must be at least -1 (-1 is unlimited)", s.MaxLines)
			}
		case "weight", "align", "overflow":
			if str, err = toString(key, val); err != nil {
				break
			}
			switch key {
			case "weight":
				s.Weight, err = codec.ParseWeight(str)
			case "align":
				s.Align, err = codec.TextAlign.Parse(str)
			case "overflow":
				s.Overflow, err = codec.Overflow.Parse(str)
			}
		default:
			err = fmt.Errorf("unknown field %q", key)
		}
		if err != nil {
			return nil, err
		}
	}
	return &styleValue{s}, nil
}

func builtinBackground(args sl.Tuple) (sl.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("expected 1 argument (color)")
	}
	c, err := toColor("color", args[0])
	if err != nil {
		return nil, err
	}
	return &optValue{demy.Background(c)}, nil
}

func builtinBorder(args sl.Tuple) (sl.Value, error) {
	if len(args) != 2 {
		return nil, errors.New("expected 2 arguments (width, color)")
	}
	w, err := toFloat("width", args[0])
	if err != nil {
		return nil, err
	}
	c, err := toColor("color", args[1])
	if err != nil {
		return nil, err
	}
	return &optValue{demy.Border(w, c)}, nil
}

func builtinGridRow(args sl.Tuple) (sl.Value, error) {
	var children []demy.Node
	for _, arg := range args {
		nv, ok := arg.(*nodeValue)
		if !ok {
			return nil, fmt.Errorf("expected node, got %s", arg.Type())
		}
		children = append(children, nv.n)
	}
	return &nodeValue{demy.GridRow(children...)}, nil
}

func builtinGridAuto(args sl.Tuple) (sl.Value, error) {
	if len(args) != 0 {
		return nil, errors.New("takes no arguments")
	}
	return &trackValue{demy.GridAuto()}, nil
}

// parseTracks accepts tracks or lists of tracks.
func parseTracks(args sl.Tuple) ([]demy.GridTrack, error) {
	var tracks []demy.GridTrack
	for _, arg := range args {
		switch v := arg.(type) {
		case *trackValue:
			tracks = append(tracks, v.t)
		case *sl.List:
			for i := range v.Len() {
				item := v.Index(i)
				tv, ok := item.(*trackValue)
				if !ok {
					return nil, fmt.Errorf("list item must be a track, got %s", item.Type())
				}
				tracks = append(tracks, tv.t)
			}
		default:
			return nil, fmt.Errorf("expected track or list of tracks, got %s", arg.Type())
		}
	}
	return tracks, nil
}

// builtinChildren accepts individual nodes or Starlark lists of nodes.
func builtinChildren(args sl.Tuple) (sl.Value, error) {
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
					return nil, fmt.Errorf("list item must be a node, got %s", item.Type())
				}
				nodes = append(nodes, nv.n)
			}
		default:
			return nil, fmt.Errorf("expected node or list, got %s", arg.Type())
		}
	}
	return &optValue{demy.Children(nodes...)}, nil
}

// --- Helpers ---

// argsToOptions converts node and option arguments to demy.Option values.
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

// toFloat converts an int or float argument. Every numeric argument is a
// size, gap, or weight, so it must be in [0, codec.MaxDimension], as rendering
// requires; checking here reports the script line that passed it.
func toFloat(what string, v sl.Value) (float64, error) {
	var f float64
	switch t := v.(type) {
	case sl.Float:
		f = float64(t)
	case sl.Int:
		f = float64(t.Float())
	default:
		return 0, fmt.Errorf("%s must be a number, got %s", what, v.Type())
	}
	if math.IsNaN(f) || f < 0 || f > codec.MaxDimension {
		return 0, fmt.Errorf("%s %g out of range [0, %d]", what, f, codec.MaxDimension)
	}
	return f, nil
}

func toString(what string, v sl.Value) (string, error) {
	s, ok := sl.AsString(v)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %s", what, v.Type())
	}
	return s, nil
}

func toColor(what string, v sl.Value) (color.Color, error) {
	s, err := toString(what, v)
	if err != nil {
		return nil, err
	}
	if s == "" {
		return nil, fmt.Errorf("%s must not be empty", what)
	}
	c, err := codec.ParseColor(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return c, nil
}

func colorHex(c color.Color) sl.String {
	r, g, b, _ := c.RGBA()
	return sl.String(fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8))
}

func capitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// isIdent reports whether s is a valid Starlark identifier.
func isIdent(s string) bool {
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}

// --- now() builtin ---

// timeValue is a Starlark value wrapping time.Time, returned by now().
type timeValue struct{ t time.Time }

func (v *timeValue) String() string        { return v.t.Format(time.RFC3339) }
func (v *timeValue) Type() string          { return "time" }
func (v *timeValue) Freeze()               {}
func (v *timeValue) Truth() sl.Bool        { return sl.True }
func (v *timeValue) Hash() (uint32, error) { return 0, errors.New("unhashable: time") }

func (v *timeValue) Attr(name string) (sl.Value, error) {
	switch name {
	case "unix":
		return sl.MakeInt64(v.t.Unix()), nil
	case "year":
		return sl.MakeInt(v.t.Year()), nil
	case "month":
		return sl.MakeInt(int(v.t.Month())), nil
	case "day":
		return sl.MakeInt(v.t.Day()), nil
	case "hour":
		return sl.MakeInt(v.t.Hour()), nil
	case "minute":
		return sl.MakeInt(v.t.Minute()), nil
	case "second":
		return sl.MakeInt(v.t.Second()), nil
	case "weekday":
		return sl.MakeInt(int(v.t.Weekday())), nil
	case "weekday_name":
		return sl.String(v.t.Weekday().String()), nil
	case "timezone":
		return sl.String(v.t.Location().String()), nil
	case "format":
		t := v.t
		return positional("format", func(args sl.Tuple) (sl.Value, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("expected 1 argument (layout), got %d", len(args))
			}
			layout, err := toString("layout", args[0])
			if err != nil {
				return nil, err
			}
			return sl.String(t.Format(layout)), nil
		}), nil
	}
	return nil, nil
}

func (v *timeValue) AttrNames() []string {
	return []string{"unix", "year", "month", "day", "hour", "minute", "second", "weekday", "weekday_name", "timezone", "format"}
}

// --- Data conversion: Go values → Starlark values ---

// maxDataDepth bounds the nesting of converted data.
const maxDataDepth = 100

// maxDataValues bounds the number of values converted from one call, counting
// a value reached through several references once per reference.
const maxDataValues = 1 << 18

// converter converts Go values to Starlark values, rejecting cycles and data
// that is too deep or too large.
type converter struct {
	depth  int
	values int
	path   map[visit]bool // maps, slices, and pointers being converted
}

// visit identifies a map, slice, or pointer for cycle detection.
type visit struct {
	ptr unsafe.Pointer
	typ reflect.Type
	len int
}

func dataToStarlark(data map[string]any) (*sl.Dict, error) {
	var c converter
	v, err := c.convert(reflect.ValueOf(data))
	if err != nil {
		return nil, err
	}
	return v.(*sl.Dict), nil //nolint:forcetypeassert // a map[string]any converts to a dict
}

func anyToStarlark(v any) (sl.Value, error) {
	var c converter
	return c.convert(reflect.ValueOf(v))
}

// convert converts numbers, strings, bools, json.Number, time.Time, Starlark
// values, slices, arrays, string-keyed maps, and pointers to them.
func (c *converter) convert(rv reflect.Value) (sl.Value, error) {
	if c.values++; c.values > maxDataValues {
		return nil, fmt.Errorf("more than %d values", maxDataValues)
	}
	if !rv.IsValid() {
		return sl.None, nil
	}
	if rv.CanInterface() {
		switch t := rv.Interface().(type) {
		case sl.Value:
			return t, nil
		case json.Number:
			if i, err := t.Int64(); err == nil {
				return sl.MakeInt64(i), nil
			}
			f, err := t.Float64()
			if err != nil {
				return nil, fmt.Errorf("invalid json.Number %q", t)
			}
			return sl.Float(f), nil
		case time.Time:
			return &timeValue{t: t}, nil
		}
	}
	switch rv.Kind() {
	case reflect.Bool:
		return sl.Bool(rv.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return sl.MakeInt64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return sl.MakeUint64(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return sl.Float(rv.Float()), nil
	case reflect.String:
		return sl.String(rv.String()), nil
	case reflect.Interface:
		if rv.IsNil() {
			return sl.None, nil
		}
		return c.convert(rv.Elem())
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return c.convertContainer(rv)
	default:
		return nil, fmt.Errorf("unsupported type %s", rv.Type())
	}
}

// convertContainer converts a pointer, slice, array, or map, tracking depth
// and the containers on the current path.
func (c *converter) convertContainer(rv reflect.Value) (sl.Value, error) {
	if rv.Kind() != reflect.Array && rv.IsNil() {
		switch rv.Kind() {
		case reflect.Slice:
			return sl.NewList(nil), nil
		case reflect.Map:
			return sl.NewDict(0), nil
		default:
			return sl.None, nil
		}
	}
	if c.depth++; c.depth > maxDataDepth {
		return nil, fmt.Errorf("nested deeper than %d", maxDataDepth)
	}
	defer func() { c.depth-- }()
	if rv.Kind() != reflect.Array {
		v := visit{ptr: rv.UnsafePointer(), typ: rv.Type()}
		if rv.Kind() == reflect.Slice {
			v.len = rv.Len()
		}
		if c.path[v] {
			return nil, errors.New("cyclic data")
		}
		if c.path == nil {
			c.path = make(map[visit]bool)
		}
		c.path[v] = true
		defer delete(c.path, v)
	}

	switch rv.Kind() {
	case reflect.Pointer:
		return c.convert(rv.Elem())
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type %s", rv.Type().Key())
		}
		keys := rv.MapKeys()
		// Sorted keys give scripts a stable iteration order across renders.
		slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(a.String(), b.String()) })
		d := sl.NewDict(len(keys))
		for _, k := range keys {
			sv, err := c.convert(rv.MapIndex(k))
			if err != nil {
				return nil, fmt.Errorf("[%q]: %w", k.String(), err)
			}
			if err := d.SetKey(sl.String(k.String()), sv); err != nil {
				return nil, err
			}
		}
		return d, nil
	default: // slice or array
		items := make([]sl.Value, rv.Len())
		for i := range rv.Len() {
			sv, err := c.convert(rv.Index(i))
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			items[i] = sv
		}
		return sl.NewList(items), nil
	}
}
