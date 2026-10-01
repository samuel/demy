# demy

A declarative rendering library for e-ink and similar displays.

[Quoin](https://github.com/samuel/quoin), a multi-user TRMNL display server, is built on demy.

## Features

- **Declarative layout**: `VStack`, `HStack`, `ZStack`, `Grid`, `Box`, `Frame`, `Spacer`, `Weighted`, `Divider`
- **Widgets**: `Text` (shaped, wrapped, vector or bitmap fonts), `Image` (fit/fill/stretch), `Barcode` (QR)
- **E-ink output**: 1-bit, 2-bit, 4-bit, and grayscale quantization; Floyd-Steinberg and Atkinson
  dithering, globally or per image
- **Device presets**: `TRMNLOriginal` (800×480) and `TRMNLX` (1872×1407)
- **Semantic color roles**: foreground/background/accent/muted
- **Serialization**: JSON (`scene/jsonscene`) and XML with `text/template` support (`scene/xmlscene`)
- **Scripting**: Starlark plugin host (`plugin/starlark`)

## Quick start

```go
import (
    "image/png"
    "os"

    "github.com/samuel/demy"
    "golang.org/x/image/font/gofont/gobold"
    "golang.org/x/image/font/gofont/goregular"
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

img, err := c.Render(root, demy.TwoBit)
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

| Constructor | Options |
|-------------|---------|
| `VStack(opts...)` / `HStack(opts...)` | `Gap`, `Padding`, `AlignStart`/`AlignCenter`/`AlignEnd`, children |
| `ZStack(opts...)` | `TopLeading` (default), `Top`, …, `BottomTrailing`, children |
| `Box(opts...)` | `Padding`, `Background(color)`, `Border(width, color)`, `CornerRadius`, `Clip` (rectangular, ignores corner radius), children |
| `Frame(w, h, children...)` | constrains children to exactly w×h and clips to its bounds |
| `Spacer()` / `Spacer(weight)` | fills remaining stack space |
| `Weighted(weight, child)` | gives child a proportional share of remaining stack space |
| `Divider()` | 1px rule in the muted color; orientation follows the enclosing stack |
| `Grid(opts...)` | `Columns(...)` (required), `Rows(...)`, `Gap`, `RowGap`, `ColGap`, `Padding`, children |
| `GridRow(children...)` | optional grouping of one row's cells; flattened by `Grid` |

### Grid

Cells fill row by row, left to right. Rows default to `GridAuto()`.

| Track | Meaning |
|-------|---------|
| `GridFixed(px)` | exact size |
| `GridAuto()` | max content size in that column/row |
| `GridFr(w)` | proportional share of remaining space (like CSS `fr`) |

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
explicitly: `TRMNLOriginal` uses 1, `TRMNLX` uses 2.

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
    LineHeight: 1.2,                       // multiplier; 0 = 1.0
    MaxLines:   2,                         // 0 = unlimited
    Overflow:   demy.TextOverflowEllipsis, // TextOverflowWrap, TextOverflowClip, TextOverflowEllipsis
}
```

`Text` with no style uses `Config.DefaultTextStyle`. Named styles in `Config.Styles` are
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

Weights: `FontRegular`, `FontThin`, `FontExtraLight`, `FontLight`, `FontMedium`, `FontSemiBold`,
`FontBold`, `FontExtraBold`, `FontBlack`. OR with `FontItalic` for italics.

### Bitmap fonts

BDF pixel fonts render without antialiasing, so they stay crisp on 1-bit and 2-bit displays.
They are loaded into `Config.Fonts` and referenced by name like vector fonts:

```go
px := demy.MustLoadBitmapFontBDF("pixel", spleenBDF) // or LoadBitmapFontBDFFile
err := px.AddStyleBDF(spleenBoldBDF, demy.FontBold)
cfg.Fonts["pixel"] = px

demy.Text("Crisp pixels", demy.Style{Font: "pixel"})
```

- `Style.Size` is ignored; glyphs render at their native size × `Scale` (nearest-neighbor).
- Color snaps to black or white by luminance.
- Missing glyphs fall back to the font's `DEFAULT_CHAR`.
- Bitmap text is composited after everything else, so it always draws on top. Only `Frame`
  and `Box` with `Clip` clip it.

## Colors

```go
demy.Black, demy.DarkGray, demy.LightGray, demy.White, demy.Red, demy.Yellow

cfg.Roles.Foreground, cfg.Roles.Background, cfg.Roles.Accent, cfg.Roles.Muted

// Resolved against Config.Roles at render time (used by deserialized scenes)
demy.RoleColor{Name: "muted"}
```

## Images

```go
demy.Image(img, demy.ImageFill) // ImageFit (default), ImageFill, ImageStretch
demy.ImageKey("logo")           // looks up Config.Images["logo"]
```

## Barcodes

```go
demy.Frame(200, 200,
    demy.Barcode("https://example.com", demy.ECLevelHigh),
)
```

A barcode fills its allocated space. `BarcodeTypeQR` is the only (and default) type. Error
correction levels: `ECLevelLow` (~7%), `ECLevelMedium` (~15%, default), `ECLevelQuartile` (~25%),
`ECLevelHigh` (~30%).

## Rendering

```go
img, err := c.Render(root, mode)                // mode: RGBA, Grayscale, OneBit, TwoBit, FourBit
img, err := c.Render(root, demy.Unset)          // use Config.ColorMode
img, err := c.Render(root, demy.TwoBit, demy.Atkinson) // dither the whole scene
```

`Render` returns an `*image.RGBA` for `RGBA` and an `*image.Gray` otherwise, with quantized modes
snapped to the palette's gray levels. Encode it with `image/png` or any other encoder.

`Render` returns an error rather than panicking on invalid trees, so scenes from untrusted
sources are safe to render: sizes, gaps, padding, and weights must be finite and within
`[0, 32768]`, trees may nest at most 1000 levels, and `GridRow` must be a direct child of
`Grid`. XML scenes are also limited to 1000 levels of element nesting.

### Per-image dithering

Rendering without global dithering keeps text crisp. Images can opt into dithering for their
own region, applied after quantization in `OneBit`, `TwoBit`, and `FourBit` modes:

```go
root := demy.VStack(
    demy.Text("Headline", title),
    demy.Image(photo, demy.Atkinson), // or demy.FloydSteinberg
)
img, err := c.Render(root, demy.TwoBit)
```

## JSON scenes (`scene/jsonscene`)

```go
import "github.com/samuel/demy/scene/jsonscene"

data, err := jsonscene.Marshal(root)
node, err := jsonscene.Unmarshal(data)

// With embedded style definitions
data, err = jsonscene.MarshalScene(root, cfg.Styles)
node, styles, err := jsonscene.UnmarshalScene(data) // merge styles into cfg.Styles before rendering
```

Colors serialize as role names (`"muted"`) or `"#rrggbb"`. Images serialize by key, resolved
against `Config.Images`.

## XML scenes and templates (`scene/xmlscene`)

Element names are node types and attributes are properties. Dimensions accept `px` (default),
`mm`, and `pt` suffixes, so the `demy.Config` argument must have physical dimensions set.

```go
import "github.com/samuel/demy/scene/xmlscene"

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
node, err = xmlscene.Execute([]byte(tmpl), data, cfg)
```

A document can also be wrapped in `<scene>` with a `<styles>` block of
`<style name="…" font="…" size="…"/>` entries. Use `UnmarshalScene`, `ExecuteScene`, or
`MarshalScene` to read or write it.

## Starlark plugins (`plugin/starlark`)

A script defines `render(data)` returning a node tree. It may also define a module-level
`styles` dict, which the host reads with `Plugin.Styles()`.

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
import starlarkplugin "github.com/samuel/demy/plugin/starlark"

env := &starlarkplugin.Environment{}
p, err := env.Load(ctx, "weather.star", cfg, loc) // loc sets the zone for now(); nil = UTC

node, err := p.Render(ctx, map[string]any{"city": "Portland", "temp": 72.4, "items": items})
img, err := c.Render(node, demy.Unset)
```

Use `Environment.AddBuiltin`, `AddValue`, and `AddModule` to expose Go functions to scripts.
`ContextFromThread` retrieves the `Render` context inside a builtin.

Module execution and each `Render` call are limited to `Environment.MaxSteps` Starlark
execution steps (`DefaultMaxSteps`, 10 million, when zero) and stop when their context is
cancelled. Script `print()` output goes to `Environment.Logger` (default `slog.Default()`).

Globals:

| Category | Names |
|----------|-------|
| Layout | `VStack`, `HStack`, `ZStack`, `Box`, `Frame`, `Spacer`, `Divider(size=, color=)`, `Grid`, `GridRow` |
| Widgets | `Text`, `ImageKey`, `Barcode` |
| Style | `Style(class=, font=, size=, weight=, color=, align=, line_height=, max_lines=, overflow=)` |
| Options | `Gap`, `Padding`, `Background`, `Border`, `CornerRadius`, `Children`, `Clip`, `Columns`, `Rows`, `RowGap`, `ColGap` |
| Grid tracks | `GridFixed`, `GridAuto`, `GridFr` |
| Alignment | `AlignStart`, `AlignCenter`, `AlignEnd`; `TopLeading` … `BottomTrailing` |
| Images | `ImageFit`, `ImageFill`, `ImageStretch`, `FloydSteinberg`, `Atkinson` |
| Barcodes | `BarcodeTypeQR`, `ECLevelLow`, `ECLevelMedium`, `ECLevelQuartile`, `ECLevelHigh` |
| Colors | `Foreground`, `Accent`, `Muted` (roles); `Black`, `DarkGray`, `LightGray`, `White`, `Red`, `Yellow` |
| Time | `now()` with `.year` `.month` `.day` `.hour` `.minute` `.second` `.weekday` (0=Sun) `.weekday_name` `.timezone` `.unix` `.format(layout)` |
| Styles | each `Config.Styles` entry, capitalized (`"h1"` → `H1`) |

## Tutorial

```sh
go run ./cmd/demy-tutorial/
```

Serves an interactive tutorial at http://127.0.0.1:8081. Use `-addr` to change the listen
address.
