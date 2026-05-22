Core: A Go rendering context built on tdewolff/canvas (already in go.mod). Initialized with
pixel dimensions and physical dimensions; origin is top-left to match display conventions.

Backend: tdewolff/canvas
- HarfBuzz-backed text shaping: word wrap, kerning, ligatures
- Pre-load fonts as *pate.FontFamily at construction time; reference by name in styles
- Renders to image.Image / PNG

Coordinate system: pate is always Y-down (top-left = 0,0). tdewolff/canvas is Cartesian
(Y-up); pate handles the flip internally — callers never deal with it.

Units:
All layout measurements (Gap, Padding, Frame dimensions, border widths) are plain float64 in
pixels. Font Size in Style is in points (pt) — the universal typography convention; the canvas
converts pt → px internally.

For reusable components that must work correctly across displays with different PPI, use the
canvas conversion helpers instead of bare pixel literals:

  c.Pt(n float64) float64   // n points → pixels, based on canvas PPI
  c.MM(n float64) float64   // n millimetres → pixels, based on canvas PPI

  c.VStack(pate.Gap(c.MM(1.5)), ...)  // 1.5mm gap — portable across displays
  c.VStack(pate.Gap(6), ...)          // 6px gap — fine for display-specific code

Device presets: exported Config values for known hardware. Each preset stores pixel dimensions,
physical dimensions, a physical Palette (what colors the display can produce), and a default
Roles mapping (semantic color aliases for portable components):

  pate.TRMNLOriginal   // 800×480 px, ~163×98 mm (~124 PPI); grayscale palette

Colors and roles:
Concrete color constants name the physical ink colors a display can produce:

  pate.Black, pate.DarkGray, pate.LightGray, pate.White   // grayscale displays
  pate.Red, pate.Yellow                                    // accent-color displays

These are the only valid inputs to dithering — arbitrary RGB values produce muddy results.

Semantic roles decouple components from concrete display colors, making them portable across
display types. Roles live in Config and presets define sensible defaults:

  type Roles struct {
      Foreground color.Color  // text, icons, borders
      Background color.Color  // canvas fill
      Accent     color.Color  // highlights, progress fills
      Muted      color.Color  // secondary text, dividers
  }

Preset defaults:
  TRMNLOriginal:   Foreground=Black, Background=White, Accent=DarkGray, Muted=LightGray
  // hypothetical red display:
  // BWRedDisplay: Foreground=Black, Background=White, Accent=Red, Muted=White

Callers override roles at construction time:
  cfg := pate.TRMNLOriginal
  cfg.Roles.Accent = pate.DarkGray  // override if needed

Portable components reference c.Roles.Accent etc. rather than concrete colors.
Display-specific code may use concrete constants directly.

Layout model: two-pass (measure then layout)
Layout requires two phases to support flex-like behavior:
1. Measure pass — each node reports its intrinsic/minimum size bottom-up
2. Layout pass — parent distributes available space top-down
Both passes run automatically before the first draw call; callers do not trigger them manually.

Layout primitives:
- HStack / VStack — flex row/column
  Options: Gap(px), Padding(px), cross-axis Align (Start/Center/End)
- ZStack — layering
  Options: child Align (TopLeading/Top/TopTrailing/Leading/Center/Trailing/BottomLeading/
  Bottom/BottomTrailing; default TopLeading); sizes to its largest child unless given a Frame
- Box — container
  Options: Padding(px), Border(width px, color), Background(color), CornerRadius(px), Clip
  Border is drawn inside padding; content is inset by padding.
  Clip crops children to box bounds (enables rounded-corner image masking when combined with CornerRadius)
- Spacer(weight ...float64) — fills remaining space in a stack
  Default weight=1; two equal Spacers split evenly; Spacer(2) takes twice the space of Spacer(1)
- Frame(w, h float64, child func()) — constrains content to exactly w×h px
  Content larger than the frame is clipped
- Divider() — a thin rule; orientation inferred from parent stack direction
  (horizontal in VStack, vertical in HStack)

Widget layer: Composable Go functions built on the layout primitives:
- Text(s string, style Style) — font reference, size (pt), color,
  alignment (Left/Center/Right; default Left), line height multiplier (default 1.0),
  max lines (0 = unlimited), overflow (Wrap/Clip/Ellipsis; default Wrap)
- Image(img image.Image, opts ...) — scaling: Fit (preserve aspect ratio), Fill (scale to
  fill frame), Stretch; optional Clip to mask to frame bounds

Font handling:
Fonts are pre-loaded at canvas construction and referenced by name in Style. This avoids
repeated disk reads and makes missing-font errors surface at construction time.

pate provides load helpers so callers never need to import tdewolff/canvas directly:

  pate.LoadFont(path string) (*pate.FontFamily, error)
  pate.MustLoadFont(path string) *pate.FontFamily  // panics on error

  cfg := pate.TRMNLOriginal
  cfg.Fonts = map[string]*pate.FontFamily{
      "sans":      pate.MustLoadFont("Inter-Regular.ttf"),
      "sans-bold": pate.MustLoadFont("Inter-Bold.ttf"),
  }

pate.FontFamily is an opaque type; callers never use canvas.FontFamily directly.
Style uses a string FontRef as the key into the Fonts map.

Construction:
  c, err := pate.New(cfg)   // returns error if a font file is missing or invalid
  c := pate.MustNew(cfg)    // panics; convenient for main() / init()

Dithering and output — first-class, not post-processing:
TRMNL expects a 1-bit or 2-bit palette PNG (see colormap-2bit.png). The renderer owns the
dithering so callers never need to post-process with ImageMagick.

  img := c.Render()                                    // full grayscale image.Image
  err := c.EncodePNG(w, pate.OneBit)                   // Floyd-Steinberg → pure black/white PNG
  err := c.EncodePNG(w, pate.TwoBit)                   // Floyd-Steinberg → 4-shade palette PNG
  err := c.EncodePNG(w, pate.TwoBit, pate.Atkinson)    // Atkinson → 4-shade palette PNG
  err := c.EncodePNG(w, pate.Grayscale)                // standard 8-bit grayscale PNG
  err := c.EncodePNG(w, pate.BWRed)                    // black+white+red display encoder

ColorMode values: Grayscale, OneBit, TwoBit (4-shade: black/dark-gray/light-gray/white), BWRed.

Grayscale, OneBit, and TwoBit use standard Floyd-Steinberg (or Atkinson) dithering to remap
pixels to the nearest palette color.

BWRed is structurally different: black and red are separate physical ink layers that cannot
mix, so it uses a two-pass encoder — one pass isolates red pixels, a second handles the
black/white layer. The exact output format (interleaved PNG, separate images, etc.) depends
on the target hardware and is TBD.

Dithering algorithm is an optional variadic parameter to EncodePNG; default is Floyd-Steinberg.
pate.Atkinson retains more white space and often looks sharper at 800×480 — offer both and
let callers choose per image.

Rough Sketch:
```go
cfg := pate.TRMNLOriginal
cfg.Fonts = map[string]*pate.FontFamily{
    "sans":      pate.MustLoadFont("Inter-Regular.ttf"),
    "sans-bold": pate.MustLoadFont("Inter-Bold.ttf"),
}
c, err := pate.New(cfg)
if err != nil {
	log.Fatal(err)
}

c.VStack(pate.Gap(4), pate.Padding(8), func() {
    c.Text("Temperature", pate.Style{Font: "sans", Size: 10})
    c.HStack(pate.AlignCenter, func() {
        c.Text("23.4", pate.Style{Font: "sans-bold", Size: 32})
        c.Text("°C", pate.Style{Font: "sans", Size: 12, Color: c.Roles.Muted})
    })
    c.ProgressBar(0.72, pate.ProgressBarStyle{
        FillColor:    c.Roles.Accent,
        TrackColor:   c.Roles.Muted,
        Height:       6,
        CornerRadius: 3,
    })
    c.HStack(func() {
        c.Text("Updated 12:34", pate.Style{Font: "sans", Size: 8})
        c.Spacer()
        c.Text("72%", pate.Style{Font: "sans", Size: 8})
    })
})

err := c.EncodePNG(w, pate.TwoBit)
```

This approach gives you HTML-like composability without the HTML complexity, a clean
pixel-default unit model with opt-in physical units for portable components, and a natural
path to add a template format later.