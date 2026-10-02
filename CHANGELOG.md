# Changelog

All notable changes to demy are listed here. Until v1.0.0, minor releases may include breaking
changes; they are called out under each release.

## v0.3.0 — 2026-10-01

First public release. demy renders declarative layouts to images for e-ink and other displays
with few colors. Scenes, templates, and BDF fonts from untrusted sources can be rendered within
fixed limits; untrusted Starlark scripts also need a memory limit from outside the process (see
"Security and untrusted input" in the README). See "Limitations" in the README for known gaps.

- Declarative layout: `VStack`, `HStack`, `ZStack`, `Grid`, `Box`, `Frame`, `Spacer`,
  `Weighted`, and `Divider`, measured in logical pixels with a display `Scale`. Layout time
  grows linearly with nesting depth.
- Text with vector (TTF/OTF, shaped with go-text/typesetting) or bitmap (BDF) fonts, named
  styles, Unicode line breaking, line limits, and ellipsis. Bitmap text is never antialiased
  and snaps to the nearest palette color. Concurrent renders that share a font family shape
  text in parallel.
- Images with fit, fill, and stretch scaling. QR codes.
- Rendering to `RGBA`, `Grayscale`, or `Paletted` images. `Paletted` quantizes to
  `Config.Palette`, with Floyd-Steinberg or Atkinson dithering for the whole scene or per
  image. Gray presets (`PaletteBW`, `PaletteGray4`, `PaletteGray16`), color e-paper presets
  (`PaletteBWR`, `PaletteBWY`, `PaletteBWRY`, `PaletteSpectra6`, `PaletteACeP7`), or any
  palette of 2 to 256 colors. Display presets for TRMNL Original and TRMNL X.
- Strict JSON (`scene/jsonscene`) and XML (`scene/xmlscene`) scene formats. XML scenes can be
  `text/template` templates with escaped output; `xmlscene.ParseTemplate` parses one once for
  repeated, concurrent execution.
- Starlark plugin host (`plugin/starlark`) with step limits, cancellation, custom builtins and
  modules, and errors that start with the script position. A panic in a custom builtin fails
  the call instead of crashing.
- `Canvas.Render` and template execution take a `context.Context` and stop when it's done.
- Invalid trees, fonts, images, and configs return errors instead of panicking. Limits: 16384
  nodes, 128 clipped nodes, and 16 barcodes per tree; 8192 bytes per `Text` and 65536 bytes of
  text per tree; 4 MiB of template output; dimensions in [0, 32768]; BDF metrics within ±4096
  pixels and glyph bitmaps within 16 MiB; Starlark `Render` data at most 100 levels deep and
  262144 values, with no cycles.
