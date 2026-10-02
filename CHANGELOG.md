# Changelog

All notable changes to demy are listed here. Until v1.0.0, minor releases may include breaking
changes; they are called out under each release.

## Unreleased

- Layout time grows linearly with nesting depth. Nested `HStack`s and `Grid`s used to measure
  their descendants at exponentially many sizes.
- Breaking: an `HStack` sizes children from their natural width, limited to the stack's width,
  instead of measuring them at the full width first. An `Image` in an `HStack` now keeps its
  natural size rather than filling the row; wrap it in `Weighted` to fill. Children in `Grid`
  Auto columns are sized the same way, so an image in an Auto column no longer grows to the
  column's share of the width.
- Breaking: a node tree may have at most 16384 nodes, 128 clipped nodes (`Frame`, and `Box`
  with `Clip`), and 16 barcodes, counting each use of a shared node. Larger trees fail to
  render. XML and JSON scenes with more than 16384 nodes fail to decode.
- Breaking: a `Text` may be at most 8192 bytes of UTF-8, and all text in a tree at most 65536
  bytes. Longer text fails to render.
- Text is shaped once per render instead of in both measure and draw.
- Breaking: BDF fonts are rejected if glyph advances, offsets, or font metrics are outside
  ±4096 pixels, or if their glyph bitmaps take more than 16 MiB. Rendering fails if bitmap
  glyphs cover more than 16 times the canvas area.
- Bitmap glyphs are blitted only where visible, writing pixels directly.
- Breaking: `Canvas.Render`, `xmlscene.Execute`, and `xmlscene.ExecuteScene` take a
  `context.Context` and stop when it's done. Template output is limited to
  `xmlscene.MaxTemplateOutput` (4 MiB).

## v0.1.0 — 2026-10-01

First release.

- Declarative layout: `VStack`, `HStack`, `ZStack`, `Grid`, `Box`, `Frame`, `Spacer`,
  `Weighted`, and `Divider`, measured in logical pixels with a display `Scale`.
- Text with vector (TTF/OTF) or bitmap (BDF) fonts, named styles, wrapping, line limits, and
  ellipsis. Images with fit, fill, and stretch scaling. QR codes.
- Rendering to RGBA, grayscale, and 1-, 2-, and 4-bit images, with Floyd-Steinberg or Atkinson
  dithering for the whole scene or per image. Presets for TRMNL Original and TRMNL X.
- Strict JSON (`scene/jsonscene`) and XML (`scene/xmlscene`) scene formats; XML scenes can be
  `text/template` templates with escaped output.
- Starlark plugin host (`plugin/starlark`) with step limits, cancellation, and custom builtins.
- Invalid trees, fonts, images, and configs return errors instead of panicking. Scenes,
  templates, scripts, and fonts must still be trusted; see the README.
