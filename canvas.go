package demy

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	cv "github.com/tdewolff/canvas"
)

// textBlit is one bitmap glyph to stamp onto the final image after
// quantization, bypassing antialiasing and dithering.
type textBlit struct {
	glyph        bitmapGlyph
	destX, destY int             // physical-pixel top-left of the glyph mask
	scale        int             // nearest-neighbor upscale factor
	color        color.RGBA      // opaque text color
	clip         image.Rectangle // physical pixels; empty means unclipped
}

// visible returns the part of the scaled glyph inside bounds and the clip.
func (b textBlit) visible(bounds image.Rectangle) image.Rectangle {
	if !b.clip.Empty() {
		bounds = bounds.Intersect(b.clip)
	}
	g := b.glyph
	return image.Rect(b.destX, b.destY, b.destX+g.width*b.scale, b.destY+g.height*b.scale).Intersect(bounds)
}

// maxBlitCoverage bounds the total visible area of bitmap glyphs as a multiple
// of the canvas area. Blitting time is proportional to it, and a font whose
// glyphs are large but advance little could otherwise stack thousands of
// canvas-sized glyphs.
const maxBlitCoverage = 16

// checkBlits reports an error if the glyphs' visible area within bounds
// exceeds maxBlitCoverage times the area of bounds.
func checkBlits(blits []textBlit, bounds image.Rectangle) error {
	limit := maxBlitCoverage * bounds.Dx() * bounds.Dy()
	var area int
	for _, b := range blits {
		r := b.visible(bounds)
		if area += r.Dx() * r.Dy(); area > limit {
			return fmt.Errorf("demy: bitmap glyphs cover more than %d times the canvas area", maxBlitCoverage)
		}
	}
	return nil
}

// blitGlyph writes each set mask pixel of the glyph as a solid scale×scale
// block, leaving unset pixels untouched. Only the part of the glyph inside the
// clip is visited. A *image.Gray gets the color's luminance, and a
// *image.Paletted gets the palette index idx.
func blitGlyph(dst draw.Image, b textBlit, idx uint8) {
	g := b.glyph
	if g.width == 0 || g.height == 0 || len(g.bits) == 0 || b.scale < 1 {
		return
	}
	r := b.visible(dst.Bounds())
	if r.Empty() {
		return
	}
	var plot func(x, y int)
	switch d := dst.(type) {
	case *image.Paletted:
		plot = func(x, y int) { d.Pix[d.PixOffset(x, y)] = idx }
	case *image.Gray:
		lum := luminance(b.color.R, b.color.G, b.color.B)
		plot = func(x, y int) { d.Pix[d.PixOffset(x, y)] = lum }
	case *image.RGBA:
		plot = func(x, y int) {
			i := d.PixOffset(x, y)
			d.Pix[i], d.Pix[i+1], d.Pix[i+2], d.Pix[i+3] = b.color.R, b.color.G, b.color.B, 0xff
		}
	default:
		plot = func(x, y int) { dst.Set(x, y, b.color) }
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := (y - b.destY) / b.scale
		for x := r.Min.X; x < r.Max.X; x++ {
			if g.pixelSet((x-b.destX)/b.scale, row) {
				plot(x, y)
			}
		}
	}
}

// applyBlits stamps all collected bitmap glyphs onto dst. On a
// *image.Paletted, each glyph takes the palette entry nearest its color.
func applyBlits(dst draw.Image, blits []textBlit, q *quantizer) {
	for _, b := range blits {
		var idx uint8
		if q != nil {
			idx = q.nearest(b.color)
		}
		blitGlyph(dst, b, idx)
	}
}

// Canvas renders Node trees for a display Config. It holds no per-render state,
// so Render may be called from multiple goroutines, including with fonts shared
// between Canvases.
//
// Layout dimensions are in logical pixels and font sizes in points; the scale
// factor maps logical to physical pixels. The origin is the top-left corner
// and Y increases downward.
type Canvas struct {
	cfg          Config
	physicalDPMM float64 // used only by the rasterizer
	dpmm         float64 // logical; used for layout
	scale        float64
	quant        *quantizer // nil without Config.Palette
}

// maxPixelDimension bounds Config.PixelWidth and PixelHeight.
const maxPixelDimension = 1 << 14

// minScale and maxScale bound the display scale. At minScale the largest
// canvas is maxPixelDimension/minScale = maxDimension logical pixels, so a
// logical size never reaches the unbounded threshold. maxScale bounds the
// per-pixel work of scaled bitmap glyphs.
const (
	minScale = 0.5
	maxScale = 64
)

// New creates a Canvas for cfg. It returns an error unless the pixel dimensions
// are in [1, 16384], the physical dimensions are finite and positive, the
// effective scale is in [0.5, 64] (Scale 0 selects it automatically),
// ColorMode is Unset or a supported mode, and Config.Palette, if set or
// required by ColorMode Paletted, has 2 to 256 opaque, concrete colors.
func New(cfg Config) (*Canvas, error) {
	if cfg.PixelWidth <= 0 || cfg.PixelHeight <= 0 || cfg.PixelWidth > maxPixelDimension || cfg.PixelHeight > maxPixelDimension {
		return nil, fmt.Errorf("demy: Config pixel dimensions %dx%d out of range [1, %d]", cfg.PixelWidth, cfg.PixelHeight, maxPixelDimension)
	}
	if !positiveFinite(cfg.PhysicalWidthMM) || !positiveFinite(cfg.PhysicalHeightMM) {
		return nil, fmt.Errorf("demy: Config physical dimensions %vx%v mm must be positive", cfg.PhysicalWidthMM, cfg.PhysicalHeightMM)
	}
	if cfg.Scale != 0 && !(cfg.Scale >= minScale && cfg.Scale <= maxScale) {
		return nil, fmt.Errorf("demy: Config.Scale %v out of range [%v, %v] (0 selects it automatically)", cfg.Scale, minScale, maxScale)
	}
	scale := cfg.EffectiveScale()
	if scale > maxScale {
		return nil, fmt.Errorf("demy: automatic scale %v from %v pixels per mm exceeds %v; check the physical dimensions or set Config.Scale",
			scale, cfg.PhysicalDPMM(), maxScale)
	}
	if cfg.ColorMode != Unset {
		if err := cfg.ColorMode.check(); err != nil {
			return nil, err
		}
	}
	var q *quantizer
	if len(cfg.Palette) > 0 || cfg.ColorMode == Paletted {
		var err error
		if q, err = newQuantizer(cfg.Palette); err != nil {
			return nil, err
		}
	}
	return &Canvas{
		cfg:          cfg,
		physicalDPMM: cfg.PhysicalDPMM(),
		dpmm:         cfg.DPMM(),
		scale:        scale,
		quant:        q,
	}, nil
}

func positiveFinite(v float64) bool { return v > 0 && !math.IsInf(v, 1) }

// Scale returns the display scale factor (logical pixels to physical pixels).
func (c *Canvas) Scale() float64 { return c.scale }

// LogicalWidth returns the canvas width in logical pixels.
func (c *Canvas) LogicalWidth() float64 { return float64(c.cfg.PixelWidth) / c.scale }

// LogicalHeight returns the canvas height in logical pixels.
func (c *Canvas) LogicalHeight() float64 { return float64(c.cfg.PixelHeight) / c.scale }

// MustNew creates a Canvas and panics on error.
func MustNew(cfg Config) *Canvas {
	c, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}

// draw builds, measures, lays out, and draws root, returning the vector canvas
// plus the bitmap glyphs to apply after quantization. Images that request
// dithering are dithered with q while drawing; nil q leaves them as is.
func (c *Canvas) draw(rs *renderState, root Node, q *quantizer) (*cv.Canvas, []textBlit, error) {
	// Size the canvas from the pixel dimensions so millimeters map exactly onto
	// whole pixels; PhysicalHeightMM need not match the width-derived density.
	wMM := float64(c.cfg.PixelWidth) / c.physicalDPMM
	hMM := float64(c.cfg.PixelHeight) / c.physicalDPMM
	cvs := cv.New(wMM, hMM)
	ctx := cv.NewContext(cvs)
	ctx.SetCoordSystem(cv.CartesianIV)

	internal, err := buildNode(root, &c.cfg, rs)
	if err != nil {
		return nil, nil, err
	}

	wLogical := float64(c.cfg.PixelWidth) / c.scale
	hLogical := float64(c.cfg.PixelHeight) / c.scale

	mctx := &measureCtx{
		rs:     rs,
		fonts:  c.cfg.Fonts,
		dpmm:   c.dpmm,
		scale:  c.scale,
		availW: wLogical,
		availH: hLogical,
	}
	internal.measure(mctx)

	bounds := nodeRect{x: 0, y: 0, w: wLogical, h: hLogical}
	internal.layout(bounds)

	bg := c.cfg.Roles.Background
	if bg == nil {
		bg = White
	}
	ctx.SetFillColor(bg)
	ctx.SetStrokeColor(color.Transparent)
	ctx.DrawPath(0, 0, cv.Rectangle(wMM, hMM))

	dctx := &drawCtx{
		rs:    rs,
		ctx:   ctx,
		dpmm:  c.dpmm,
		scale: c.scale,
		roles: c.cfg.Roles,
		fonts: c.cfg.Fonts,
		quant: q,
		clip:  image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight),
	}
	internal.draw(dctx)
	if rs.stopped() {
		return nil, nil, rs.stopErr()
	}
	if dctx.err != nil {
		return nil, nil, dctx.err
	}
	return cvs, dctx.textBlits, nil
}

// rasterize converts a rendered cv.Canvas into a pixel image.
func (c *Canvas) rasterize(rs *renderState, cvs *cv.Canvas) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight))
	ras := newVectorRenderer(img, c.physicalDPMM, rs)
	cvs.RenderTo(ras)
	return img
}

// Render lays out and rasterizes root. Unset mode uses Config.ColorMode, and
// RGBA is used if that is also Unset. Paletted mode quantizes to
// Config.Palette and returns an *image.Paletted whose palette is
// Config.Palette in the same order. algo selects the dithering applied when
// quantizing (default NoDithering); an ImageNode with its own Dither is
// dithered with that algorithm instead, so images can be dithered while text
// stays crisp.
//
// Rendering stops early when ctx is done, returning an error that wraps the
// context's error.
func (c *Canvas) Render(ctx context.Context, root Node, mode ColorMode, algo ...Dither) (image.Image, error) {
	if mode == Unset {
		mode = c.cfg.ColorMode
	}
	if mode == Unset {
		mode = RGBA
	}
	if err := mode.check(); err != nil {
		return nil, err
	}
	algorithm := NoDithering
	if len(algo) > 0 {
		algorithm = algo[0]
	}
	if err := algorithm.check(); err != nil {
		return nil, err
	}
	var q *quantizer
	if mode == Paletted {
		if c.quant == nil {
			return nil, errNoPalette
		}
		q = c.quant
	}
	rs := &renderState{ctx: ctx}
	cvs, blits, err := c.draw(rs, root, q)
	if err != nil {
		return nil, err
	}
	if err := checkBlits(blits, image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight)); err != nil {
		return nil, err
	}
	rgba := c.rasterize(rs, cvs)
	if rs.stopped() {
		return nil, rs.stopErr()
	}
	var out draw.Image
	switch mode {
	case RGBA:
		out = rgba
	case Grayscale:
		out = toGray(rgba)
	default:
		out = q.paletted(rgba, algorithm)
	}
	applyBlits(out, blits, q)
	return out, nil
}

// check reports an error for an unknown mode.
func (m ColorMode) check() error {
	switch m {
	case RGBA, Grayscale, Paletted:
		return nil
	default:
		return fmt.Errorf("demy: unknown color mode %d", int(m))
	}
}

// check reports an error for an unknown dithering algorithm.
func (d Dither) check() error {
	switch d {
	case NoDithering, FloydSteinberg, Atkinson:
		return nil
	default:
		return fmt.Errorf("demy: unknown dithering algorithm %d", int(d))
	}
}
