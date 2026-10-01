# Changelog

All notable changes to demy are listed here. Until v1.0.0, minor releases may include breaking
changes; they are called out under each release.

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
