# demy

A declarative rendering library for e-ink and similar displays.

## Features

- **Declarative layout**: `VStack`, `HStack`, `ZStack`, `Grid`, `Box`, `Frame`, `Spacer`, `Weighted`, `Divider`
- **Widgets**: `Text` (shaped, wrapped, vector or bitmap fonts), `Image` (fit/fill/stretch), `Barcode` (QR)
- **E-paper output**: quantization to any palette of up to 256 colors (gray ramps, BWR, BWRY,
  Spectra 6, ACeP 7, or a panel's measured colors) as an indexed image; Floyd-Steinberg and
  Atkinson dithering, globally or per image
- **Device presets**: `TRMNLOriginal` (800×480) and `TRMNLX` (1872×1407)
- **Semantic color roles**: foreground/background/accent/muted
- **Serialization**: JSON (`scene/jsonscene`) and XML with `text/template` support (`scene/xmlscene`)
- **Scripting**: Starlark plugin host (`plugin/starlark`)

Scenes, templates, and BDF fonts from untrusted sources are rendered within fixed limits;
untrusted Starlark scripts also need a memory limit from outside the process. See
[Security and untrusted input](#security-and-untrusted-input).

## Quick start

```go
import (
    "context"
    "image/png"
    "log"
    "os"

    "golang.org/x/image/font/gofont/gobold"
    "golang.org/x/image/font/gofont/goregular"
    "samuelks.com/demy"
)

sans := demy.MustLoadFontBytes("sans", goregular.TTF)
if err := sans.AddStyle(gobold.TTF, demy.FontBold); err != nil {
    log.Fatal(err)
}

cfg := demy.TRMNLOriginal
cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
c, err := demy.New(cfg)
if err != nil {
    log.Fatal(err)
}

root := demy.VStack(demy.Gap(4), demy.Padding(8),
    demy.Text("Temperature", demy.Style{Font: "sans", Size: 10}),
    demy.HStack(demy.AlignCenter,
        demy.Text("23.4", demy.Style{Font: "sans", Size: 32, Weight: demy.FontBold}),
        demy.Text("°C", demy.Style{Font: "sans", Size: 12, Color: cfg.Roles.Muted}),
    ),
    demy.HStack(
        demy.Text("Updated 12:34", demy.Style{Font: "sans", Size: 8}),
        demy.Spacer(),
        demy.Text("72%", demy.Style{Font: "sans", Size: 8}),
    ),
)

img, err := c.Render(context.Background(), root, demy.ColorModeDefault) // the preset's 4-gray palette
if err != nil {
    log.Fatal(err)
}
f, err := os.Create("out.png")
if err != nil {
    log.Fatal(err)
}
defer f.Close()
if err := png.Encode(f, img); err != nil {
    log.Fatal(err)
}
```

## Layout

Constructors return `Node` values. Children are passed inline or via `Children(nodes...)` for
dynamic lists:

```go
var rows []demy.Node
for _, item := range items {
    rows = append(rows, demy.Text(item.Name, body))
}
demy.VStack(demy.Gap(4), demy.Children(rows...))
```

Nil children (including typed nil pointers) are skipped, so optional nodes can be passed
directly. In a `Grid` a skipped child takes no cell, so later children move up one cell.

| Constructor | Options |
|-------------|---------|
| `VStack(opts...)` / `HStack(opts...)` | `Gap`, `Padding`, `AlignStart`/`AlignCenter`/`AlignEnd`, children |
| `ZStack(opts...)` | `ZAlignTopLeading` (default), `ZAlignTop`, …, `ZAlignBottomTrailing`, children |
| `Box(opts...)` | `Padding`, `Background(color)`, `Border(width, color)` (insets children by its width), `CornerRadius`, `Clip` (rectangular, ignores corner radius), children |
| `Frame(w, h, children...)` | constrains children to exactly w×h and clips to its bounds |
| `Spacer()` / `Spacer(weight)` | fills remaining stack space |
| `Weighted(weight, child)` | gives child a proportional share of remaining stack space; its cross-axis size counts toward the stack's |
| `Divider()` | 1px rule in the muted color; orientation follows the enclosing stack, and a vertical divider spans the `HStack`'s height |
| `Grid(opts...)` | `Columns(...)` (required), `Rows(...)`, `Gap`, `RowGap`, `ColGap`, `Padding`, children |
| `GridRow(children...)` | optional grouping of one row's cells; flattened by `Grid` |

An `HStack` gives each child its natural width (its size with no width limit), limited to the
stack's width. If the children don't fit, the ones that aren't `Frame`, `Spacer`, or `Weighted`
shrink in proportion to that width. An `Image` in an `HStack` keeps its natural size (scaled to
the stack's height when it has one); wrap it in `Weighted` to fill the remaining width.

### Grid

Cells fill row by row, left to right. `Columns` is required; rendering a grid without columns
is an error. Rows default to `GridAuto()`. When `Rows` is given it sets the row count: empty
rows keep their size and children beyond the last cell are dropped (not measured or drawn).

| Track | Meaning |
|-------|---------|
| `GridFixed(px)` | exact size |
| `GridAuto()` | max content size in that column/row; an Auto column is at most an even share (with the other Auto and Fr columns) of the width left after Fixed columns |
| `GridFr(w)` | proportional share of remaining space (like CSS `fr`); Fr rows size to content when the grid's height is unbounded, such as inside a `VStack` |

```go
demy.Grid(
    demy.Columns(demy.GridFixed(120), demy.GridFr(1), demy.GridFr(2)),
    demy.Rows(demy.GridAuto(), demy.GridFixed(40)),
    demy.Gap(4),
    demy.GridRow(demy.Text("Name", label), demy.Text("Age", label), demy.Text("Email", label)),
    demy.GridRow(demy.Text("Alice", body), demy.Text("30", body), demy.Text("alice@example.com", body)),
)
```

### Units and scale

Layout is in logical pixels; font sizes are in points. `Config.Scale` maps logical to physical
pixels (like iOS points). When 0, it is computed as `max(1, round(DPMM/5))`. The presets set it
explicitly: `TRMNLOriginal` uses 1, `TRMNLX` uses 2. The effective scale must be in [0.5, 64].

```go
cfg.DPMM()         // logical dots per mm
cfg.PhysicalDPMM() // physical dots per mm
cfg.MM(1.5)        // 1.5 mm in logical pixels
cfg.Pt(12)         // 12 pt in logical pixels

c.Scale()          // effective scale
c.LogicalWidth()   // physical width / scale
c.LogicalHeight()  // physical height / scale
```

## Text and styles

```go
demy.Style{
    Font:       "sans",
    Size:       12,                        // points
    Weight:     demy.FontBold,
    Color:      demy.DarkGray,
    Align:      demy.TextAlignCenter,      // TextAlignLeft, TextAlignCenter, TextAlignRight
    LineHeight: 1.2,                       // multiplier; default 1.0
    MaxLines:   2,                         // MaxLinesUnlimited for no limit
    Overflow:   demy.TextOverflowEllipsis, // TextOverflowWrap, TextOverflowClip, TextOverflowEllipsis
}
```

Zero-valued fields inherit (`FontInherit`, `TextAlignInherit`, `TextOverflowInherit`,
`MaxLines: 0`), so any field can be set back to its default over a class:
`demy.Style{Weight: demy.FontRegular, Align: demy.TextAlignLeft, MaxLines: demy.MaxLinesUnlimited}`.

Text is sanitized before layout: each invalid UTF-8 byte, and each control character other
than tab, line feed, and carriage return, becomes U+FFFD (�). Line feeds and carriage returns
break lines.

A `Text` may be at most 8192 bytes of UTF-8 after sanitizing, and all text in a tree at most
65536 bytes, counting each use of a shared node; longer text fails to render.

Text is shaped with [go-text/typesetting](https://github.com/go-text/typesetting) (a HarfBuzz
port: kerning, ligatures, and bidirectional and complex scripts) and wraps at Unicode line
break opportunities, filling each line before starting the next. Word spacing isn't
stretched or shrunk. `MaxLines` truncates text to that many lines; with `TextOverflowEllipsis`
the last line is shortened to fit an ellipsis.

After merging, every `Text` must have a `Font` naming a loaded family in `Config.Fonts`, and
vector text must have a non-zero `Size` (bitmap fonts ignore it). Otherwise rendering returns
an error rather than drawing nothing. A `FontFamily` must come from a `Load…` function; a
zero value is an error.

`Text` with no style uses `Config.DefaultTextStyle`. A `Class` name missing from
`Config.Styles` is a render error. Named styles in `Config.Styles` are
referenced with `Class`, and inline `Style` fields override the class:

```go
cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}
cfg.Styles = map[string]demy.Style{
    "h1":      {Font: "sans", Size: 24, Weight: demy.FontBold},
    "caption": {Font: "sans", Size: 9, Color: demy.LightGray},
}

demy.Text("hello")
demy.Text("Title", demy.Class("h1"))
demy.Text("Title", demy.Class("h1"), demy.Style{Color: demy.DarkGray})
```

### Fonts

```go
ff, err := demy.LoadFontFile("sans", "Inter-Regular.ttf") // or LoadFontBytes / Must… variants
err = ff.AddStyle(boldTTF, demy.FontBold)
err = ff.AddStyle(boldItalicTTF, demy.FontBold|demy.FontItalic)
cfg.Fonts = map[string]*demy.FontFamily{"sans": ff}
```

Weights: `FontInherit` (zero value), `FontRegular`, `FontThin`, `FontExtraLight`, `FontLight`, `FontMedium`, `FontSemiBold`,
`FontBold`, `FontExtraBold`, `FontBlack`. OR with `FontItalic` for italics.

Fonts may be TrueType or OpenType files, or the first font of a collection. A style that
isn't loaded uses the closest loaded one, thickened for a heavier weight and slanted for
italics.

### Bitmap fonts

BDF pixel fonts render without antialiasing, so they stay crisp on 1-bit and 2-bit displays.
They are loaded into `Config.Fonts` and referenced by name like vector fonts:

```go
px := demy.MustLoadBDFBytes("pixel", spleenBDF) // or LoadBDFFile
err := px.AddStyleBDF(spleenBoldBDF, demy.FontBold)
cfg.Fonts = map[string]*demy.FontFamily{"pixel": px} // or add to an existing map

demy.Text("Crisp pixels", demy.Style{Font: "pixel"})
```

- `Style.Size` is ignored; glyphs render at their native size × `Scale` (nearest-neighbor).
- Color snaps to the nearest palette color when quantizing (so red text stays red on a BWR
  panel); unquantized renders use the color as is, without blending.
- Missing glyphs fall back to the font's `DEFAULT_CHAR`.
- Bitmap text is composited after everything else, so it always draws on top. Only `Frame`
  and `Box` with `Clip` clip it.
- A font is rejected if a glyph's size, offsets, or advance, or the font's ascent, descent, or
  bounding box, is outside ±4096 pixels (advances and sizes must not be negative), or if its
  glyph bitmaps, padded to their bounding boxes, take more than 16 MiB.
- Rendering fails if the visible bitmap glyphs together cover more than 16 times the canvas
  area, as they can with a font whose large glyphs barely advance.

## Colors

```go
demy.Black, demy.DarkGray, demy.LightGray, demy.White
demy.Red, demy.Yellow, demy.Green, demy.Blue, demy.Orange

cfg.Roles.Foreground, cfg.Roles.Background, cfg.Roles.Accent, cfg.Roles.Muted

// Resolved against Config.Roles at render time (used by deserialized scenes)
demy.RoleColor{Name: "muted"}
```

## Images

```go
demy.Image(img, demy.ImageFill) // ImageFit (default), ImageFill, ImageStretch
demy.ImageKey("logo")           // looks up Config.Images["logo"]
```

`ImageFit` scales the image to fit the available space, preserving aspect ratio. Along an
unbounded axis (height inside a `VStack`) it scales to the other axis.

An image must be non-nil with each side in [1, 16384] and at most 2²⁶ pixels; otherwise
rendering returns an error. Images with unbounded extents such as `image.Uniform` are
rejected. Any `image.Image` works with every scale mode; it doesn't need a `SubImage` method.

## Barcodes

```go
demy.Frame(200, 200,
    demy.Barcode("https://example.com", demy.ECLevelHigh),
)
```

A barcode fills its allocated space, scaled by a whole number of physical pixels per module and
centered, so modules stay sharp at any display scale. Content that can't be encoded, or a space
too small for one pixel per module (including no space at all), is a render error rather than a
silently missing code. A barcode inside a `Frame` or clipped `Box` that is clipped away
entirely isn't drawn, like any other clipped content. `BarcodeTypeQR` is the only (and default) type. Error
correction levels: `ECLevelLow` (~7%), `ECLevelMedium` (~15%, default), `ECLevelQuartile` (~25%),
`ECLevelHigh` (~30%).

## Rendering

```go
img, err := c.Render(ctx, root, mode)                         // mode: RGBA, Grayscale, Paletted
img, err := c.Render(ctx, root, demy.ColorModeDefault)        // use Config.ColorMode
img, err := c.Render(ctx, root, demy.Paletted, demy.Atkinson) // dither the whole scene
```

`Render` stops when `ctx` is done and returns an error wrapping the context's error, so
`errors.Is(err, context.DeadlineExceeded)` reports a timeout. It checks the context for each
node it builds, measures, lays out, and draws, and for each path and image it rasterizes.

`Render` returns an `*image.RGBA` for `RGBA`, an `*image.Gray` for `Grayscale`, and an
`*image.Paletted` for `Paletted`. Encode it with `image/png` or any other encoder; `image/png`
writes a paletted image as a 1-, 2-, 4-, or 8-bit indexed PNG. Translucent pixels (e.g. a
translucent `Roles.Background`) are composited over white when converting to grayscale or
quantizing. `Render` is safe to call from multiple goroutines, and fonts can be
shared between canvases. `New` copies the `Config` but shares its maps and slices (`Fonts`,
`Images`, `Styles`, `Palette`), so don't modify them, or call `AddStyle`/`AddStyleBDF` on a
font, after the first `Render`. `New` rejects pixel dimensions outside 1–16384, non-positive physical
dimensions, an effective scale outside [0.5, 64], an unknown `Config.ColorMode`, and an invalid
`Config.Palette`. `Render` returns an error for `Paletted` without a palette and for unknown
color modes or dithering algorithms.

A typical dashboard renders in about 6 ms on TRMNL Original and 30 ms on TRMNL X
(`go test -bench RenderDashboard`, Apple M-series).

`Render` returns an error rather than panicking on invalid trees: sizes, gaps, padding, and
weights must be finite and within `[0, 32768]`, trees may nest at most 1000 levels, and
`GridRow` must be a direct child of `Grid`. A tree may have at most 16384 nodes, 128 clipped
nodes (`Frame`, and `Box` with `Clip`), 16 barcodes, and 65536 bytes of text. A node used in several places counts
once per use, so a small tree that shares nodes can't expand into an unbounded amount of work.
XML and JSON scenes are also limited to 1000 levels of nesting and 16384 nodes, checked while
decoding. See [Security and untrusted input](#security-and-untrusted-input) for what else to
limit.

### Per-image dithering

Rendering without global dithering keeps text crisp. Images can opt into dithering of their
own pixels in `Paletted` mode. The image is dithered as it is drawn,
so text and shapes layered over it in a `ZStack` stay crisp. A translucent image is dithered
on its own colors and then blended with what lies beneath it.

```go
root := demy.VStack(
    demy.Text("Headline", title),
    demy.Image(photo, demy.Atkinson), // or demy.FloydSteinberg
)
img, err := c.Render(ctx, root, demy.Paletted)
```

### Palettes

`Paletted` mode quantizes to `Config.Palette`, a list of 2 to 256 opaque colors. The rendered
`*image.Paletted` uses the same palette in the same order, so a driver can send each pixel's
index to the panel without matching colors. Presets cover common panels:

| Preset | Colors |
| --- | --- |
| `PaletteBW` | black, white |
| `PaletteGray4` | 4 grays (`TRMNLOriginal`) |
| `PaletteGray16` | 16 grays (`TRMNLX`) |
| `PaletteBWR`, `PaletteBWY` | black, white, red or yellow |
| `PaletteBWRY` | black, white, yellow, red |
| `PaletteSpectra6` | black, white, yellow, red, blue, green |
| `PaletteACeP7` | black, white, green, blue, red, yellow, orange |

```go
cfg := demy.TRMNLOriginal
cfg.Palette = demy.PaletteBWR
cfg.Roles.Accent = demy.Red
```

Panels number their colors differently. Check the driver and list the colors in its order.
Real inks are often duller than pure sRGB colors; listing the colors a panel actually shows
(e.g. `color.RGBA{0xa0, 0x30, 0x2e, 0xff}` for a Spectra 6 red) gives closer dithering, and the
indices stay the same.

Palettes of only grays are matched by luminance. Other palettes are matched in OKLab, weighted
toward hue so grays (such as antialiased text edges) don't turn into a color of similar
lightness. Error diffusion is limited so a color the palette can't approach doesn't streak
across the image.

## JSON scenes (`scene/jsonscene`)

```go
import "samuelks.com/demy/scene/jsonscene"

data, err := jsonscene.Marshal(root)
node, err := jsonscene.Unmarshal(data)

// With embedded style definitions
data, err = jsonscene.MarshalScene(root, cfg.Styles)
node, styles, err := jsonscene.UnmarshalScene(data) // merge styles into cfg.Styles before rendering

// Marshal options are passed to encoding/json/v2
data, err = jsonscene.Marshal(root, jsontext.WithIndent("  "))
```

Colors serialize as role names (`"muted"`) or `"#rrggbb"`/`"#rrggbbaa"` (straight alpha).
Images serialize by key, resolved against `Config.Images`; an image with no key, a nil root,
or an unknown role color is a marshal error. Nil children are omitted.

Decoding is strict: unknown or duplicate fields, field names that differ in case, unknown
enum names (`"align":"right"` on a stack), children on leaf nodes, more than one child in
`weighted`, invalid UTF-8, and trailing data are errors. Style keys are marshaled in sorted
order.

## XML scenes and templates (`scene/xmlscene`)

Element names are node types and attributes are properties. Dimensions accept `px` (default),
`mm`, and `pt` suffixes; `mm` and `pt` need the `demy.Config` argument to have physical
dimensions set. Font sizes (`size` on `<text>` and `<style>`) are in points: a bare number or
`pt` is points, and `mm`/`px` are converted. `line-height` is a unitless multiplier.

Numbers are plain decimals (`4`, `-0.5`, `1e2`); `NaN`, `Inf`, hex floats, and underscores are
rejected.

Decoding is strict: unknown elements or attributes, unknown enum names, malformed numbers,
booleans other than `true`/`false`, children or text in elements that don't take them, and
anything but whitespace or comments after the root element are errors.

```go
import "samuelks.com/demy/scene/xmlscene"

data, err := xmlscene.Marshal(root)
node, err := xmlscene.Unmarshal(data, cfg)

const tmpl = `<vstack gap="4">
  <text color="foreground">{{.Title}}</text>
  {{range .Items}}
  <hstack padding="4">
    <text size="12">{{.Name}}</text>
    <spacer/>
    <text size="12">{{.Value}}</text>
  </hstack>
  {{end}}
</vstack>`
node, err = xmlscene.Execute(ctx, []byte(tmpl), data, cfg)
```

`Execute` parses the template on every call. To render a template repeatedly, parse it once
with `ParseTemplate` and execute the returned `*Template`, which is safe for concurrent use:

```go
t, err := xmlscene.ParseTemplate([]byte(tmpl))
node, err = t.Execute(ctx, data, cfg)
```

Template output is XML-escaped automatically, so data like `AT&T` or `</text>` is inserted as
text and can't inject markup. Pipe a value to `raw` (`{{.Snippet | raw}}`) to insert XML as is.

Execution stops when `ctx` is done, checked as output is written, on each `range` iteration,
and each time a template starts, so loops and recursion that print nothing still stop. Output
is limited to `xmlscene.MaxTemplateOutput` (4 MiB).

A document can also be wrapped in `<scene>` with a `<styles>` block of
`<style name="…" font="…" size="…"/>` entries. Use `UnmarshalScene`, `ExecuteScene`, or
`MarshalScene` to read or write it.

## Starlark plugins (`plugin/starlark`)

A script defines `render(data)` returning a node tree. It may also define a module-level
`styles` dict, which the host reads with `Plugin.Styles()` and merges into `Config.Styles`
before rendering; `Class("name")` references those styles at render time.

```python
def render(data):
    rows = [Text(item["name"], Body) for item in data["items"]]
    return VStack(Gap(8), Padding(12),
        Text(data["city"], H1),
        Text("{}°".format(int(data["temp"])), Style(font="sans", size=40, weight="bold")),
        Divider(),
        Children(rows),
    )
```

```go
import starlarkplugin "samuelks.com/demy/plugin/starlark"

env := &starlarkplugin.Environment{}
p, err := env.Load(ctx, "weather.star", cfg, loc) // loc sets the zone for now(); nil = UTC

node, err := p.Render(ctx, map[string]any{"city": "Portland", "temp": 72.4, "items": items})
img, err := c.Render(ctx, node, demy.ColorModeDefault)
```

Use `Environment.AddBuiltin`, `AddValue`, and `AddModule` to expose Go functions to scripts.
`ContextFromThread` retrieves the `Render` context inside a builtin. Values added with
`AddValue` are frozen because every script and render shares them. `Render` data accepts
`map[string]any`/`[]any` plus typed slices, string-keyed maps, all integer and float kinds,
pointers, `json.Number`, and `time.Time` (as a `now()`-style time value); map keys are sorted
so iteration order is stable. Data may nest at most 100 levels and hold at most 262144 values
(a value reached through several references counts once per reference); cyclic data is an
error. `ToStarlark` applies the same limits. Set `Environment.Now` to fix the time `now()` returns.

Builtins reject unknown keyword arguments, wrong argument types, unknown enum names (such as
`weight="semibold"`), numbers outside [0, 32768] (every numeric argument is a size, gap, or
weight), and `Style(base=)` names missing from `Config.Styles`. Errors from `Load` and
`Render` start with the script position that failed, such as `t.star:2:17: Frame: w +Inf out
of range [0, 32768]`. A panic in a builtin registered with `AddBuiltin` or `AddModule` fails
that call with an error instead of crashing; panics in goroutines the builtin starts are not
recovered. The returned tree
may share nodes between renders, so treat it as read-only.

Module execution and each `Render` call are limited to `Environment.MaxSteps` Starlark
execution steps (`DefaultMaxSteps`, 10 million, when zero) and stop when their context is
cancelled. Neither bounds memory; see [Security and untrusted input](#security-and-untrusted-input)
before running untrusted scripts. Script `print()` output goes to `Environment.Logger` (default `slog.Default()`).
Each message is truncated to 4 KiB, and after 64 KiB of output in one `Load` or `Render`,
later output is dropped with a warning.

Configure an `Environment` (fields and `Add…` calls) before the first `Load`. Each `Plugin`
keeps the settings and globals from its `Load`, so later changes affect only later loads.
`Plugin.Render` may be called concurrently, and `Plugin.Styles` returns a copy. Nodes passed to
`WrapNode` must not be modified afterwards, since scripts can share them between trees and
renders.

Globals:

| Category | Names |
|----------|-------|
| Layout | `VStack`, `HStack`, `ZStack`, `Box`, `Frame`, `Spacer`, `Weighted(weight, child)`, `Divider(size=, color=)`, `Grid`, `GridRow` |
| Widgets | `Text(s, style_or_class...)`, `ImageKey`, `Barcode` |
| Style | `Style(base=, font=, size=, weight=, color=, align=, line_height=, max_lines=, overflow=)` (`base=` copies a `Config.Styles` entry for the other fields to override; `max_lines=-1` is unlimited); `Class(name)` |
| Options | `Gap`, `Padding`, `Background`, `Border`, `CornerRadius`, `Children`, `Clip`, `Columns`, `Rows`, `RowGap`, `ColGap` |
| Grid tracks | `GridFixed`, `GridAuto`, `GridFr` |
| Alignment | `AlignStart`, `AlignCenter`, `AlignEnd`; `ZAlignTopLeading` … `ZAlignBottomTrailing` |
| Images | `ImageFit`, `ImageFill`, `ImageStretch`, `FloydSteinberg`, `Atkinson` |
| Barcodes | `BarcodeTypeQR`, `ECLevelLow`, `ECLevelMedium`, `ECLevelQuartile`, `ECLevelHigh` |
| Colors | `Foreground`, `Accent`, `Muted` (roles); `Black`, `DarkGray`, `LightGray`, `White`, `Red`, `Yellow`, `Green`, `Blue`, `Orange` |
| Time | `now()` with `.year` `.month` `.day` `.hour` `.minute` `.second` `.weekday` (0=Sun) `.weekday_name` `.timezone` `.unix` `.format(layout)` (Go layout, e.g. `"Mon Jan 2 15:04"`) |
| Styles | each `Config.Styles` entry, capitalized (`"h1"` → `H1`), unless the name isn't an identifier or would shadow a builtin; use `Class(name)` for those |

## Tutorial

```sh
go run ./cmd/demy-tutorial/
```

Serves an interactive tutorial at http://127.0.0.1:8081. Lessons are XML scenes edited and
rendered live in the browser. Use `-addr` to change the listen
address.

## Limitations

- **No font fallback.** Each `Text` uses one font family, and glyphs the font lacks (emoji,
  CJK, symbols) render as its `.notdef` glyph, usually an empty box. Bitmap fonts fall back to
  the font's `DEFAULT_CHAR`, then to a blank advance. Load a font that covers the text you render.
- **Paragraphs are left to right.** Right-to-left text (Arabic, Hebrew) is shaped right to
  left within a run, but the base direction of every paragraph is left to right. A
  right-to-left paragraph with more than one run (numbers, Latin text, punctuation) puts its
  runs in the wrong visual order, and `TextAlignInherit` aligns it left.
- **Some inputs must be trusted.** Vector fonts are parsed without extra checks, and Starlark
  scripts' memory isn't limited; see [Security and untrusted input](#security-and-untrusted-input).

## Stability

demy follows semantic versioning. Until v1.0.0, a minor release (v0.x.0) may make breaking
changes; patch releases (v0.x.y) only fix bugs. [CHANGELOG.md](CHANGELOG.md) lists breaking
changes for each release.

## Security and untrusted input

demy validates everything it renders and returns errors instead of panicking. The table lists
which inputs can come from untrusted sources and what bounds them.

| Input | Untrusted? | Bounded by |
|-------|------------|------------|
| Node trees (`Render`) | Yes | Node, clipped-node, barcode, text, nesting, and dimension limits (see [Rendering](#rendering)); the `Render` context |
| JSON scenes (`jsonscene`) | Yes, with an input size limit | Nesting and node limits, checked after the whole document is parsed, so cap the input size first (e.g. `http.MaxBytesReader`) |
| XML scenes (`xmlscene`) | Yes, with an input size limit | Nesting and node limits, checked while decoding |
| XML templates (`xmlscene.Execute`, `Template.Execute`) | Yes, with plain data | The context and `MaxTemplateOutput` (4 MiB). Templates can call exported methods on the data and its fields, so pass maps, slices, and plain structs |
| BDF fonts | Yes | Glyph size, metric, and bitmap size limits; blitted glyph area per render |
| Starlark scripts (`plugin/starlark`) | Only with memory isolation | `MaxSteps` and the context bound time, not memory |
| `Config` (vector fonts, images, styles), template data, and builtins added with `AddBuiltin`/`AddModule` | No | These are the host's: vector fonts are parsed by go-text/typesetting without extra checks (and render only the glyphs they contain; see [Limitations](#limitations)), and builtins run with the host's privileges |

Apply a deadline to every untrusted render with the context passed to `Render`, `Execute`, and
`Plugin.Render`. A render's memory grows with the canvas size, since the canvas and each
dithered image are rasterized to an image of at most the canvas size.

**Starlark memory isn't limited.** `Environment.MaxSteps` counts execution steps, and one step
can allocate a lot: `"x" * (1 << 29)` is a single step that allocates 512 MiB. Go can't limit
the memory of one goroutine, and `debug.SetMemoryLimit` only makes the garbage collector work
harder. Run untrusted scripts where the operating system or a runtime enforces a memory limit:
a separate process in a cgroup (`memory.max`) or container, a process with `RLIMIT_AS`, or a
WASM build of the host in a runtime with a memory cap. Send the result back with
`jsonscene.Marshal`, and kill the process when its deadline passes. [Limitations](#limitations)
lists the other known gaps.

## Dependencies

demy depends on [go-text/typesetting](https://github.com/go-text/typesetting) for font parsing
and text shaping, `golang.org/x/image` for rasterization and image scaling,
[boombuler/barcode](https://github.com/boombuler/barcode) for QR codes, and
[go.starlark.net](https://github.com/google/starlark-go) for the plugin host.

## Development

CI (`.github/workflows/ci.yml`) runs on pushes to `main` and on pull requests. Tests run on
Linux (amd64) and macOS (arm64), so pixel checks catch floating-point differences between
architectures:

```sh
go build ./...
go vet ./...
go test -race ./...
golangci-lint run ./...
govulncheck ./...
go test -run '^$' -fuzz '^FuzzRoundTrip$' -fuzztime 30s ./scene/jsonscene  # and each other fuzz target
```

The fuzz targets are `FuzzParseBDF` (root), `FuzzUnmarshalRender` and `FuzzRoundTrip` (both
scene packages), `FuzzRoundTripJSON` (XML→JSON→XML, in `scene/xmlscene`), and `FuzzColor` and
`FuzzWeight` (`internal/codec`). Round-trip targets check that a decoded scene encodes back to
the same tree and the same bytes. Failing fuzz inputs are uploaded as a workflow artifact; add them to the package's
`testdata/fuzz/` to reproduce.

## License

This code is MIT licensed. See LICENSE.
