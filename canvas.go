package demy

import (
	"errors"
	"image"
	"image/color"
	"image/draw"

	cv "github.com/tdewolff/canvas"
)

// imageDitherRegion records a pixel region and its requested dithering algorithm,
// collected during the draw pass from ImageNodes that have a non-NoDithering Dither field.
type imageDitherRegion struct {
	bounds image.Rectangle
	algo   Dither
}

// textBlit records one bitmap glyph to composite onto the final image, in
// physical pixel coordinates. Bitmap text must bypass the antialiasing
// rasterizer and dithering to stay crisp, so blits are collected during the
// draw pass (like imageDitherRegion) and stamped on last.
type textBlit struct {
	glyph        bitmapGlyph
	destX, destY int             // physical-pixel top-left of the glyph mask
	scale        int             // integer nearest-neighbor upscale (>= 1)
	color        color.Gray      // palette extreme (0 or 255)
	clip         image.Rectangle // physical-pixel clip; empty means unclipped
}

// blitGlyph stamps a bitmap glyph onto dst, expanding each set mask pixel into a
// scale×scale block. No antialiasing or blending: set pixels are written in the
// glyph's solid color, the rest show through.
func blitGlyph(dst draw.Image, b textBlit) {
	g := b.glyph
	if g.width == 0 || g.height == 0 || len(g.bits) == 0 || b.scale < 1 {
		return
	}
	clip := dst.Bounds()
	if !b.clip.Empty() {
		clip = clip.Intersect(b.clip)
	}
	for row := range g.height {
		for col := range g.width {
			if !g.pixelSet(col, row) {
				continue
			}
			px0 := b.destX + col*b.scale
			py0 := b.destY + row*b.scale
			for dy := range b.scale {
				y := py0 + dy
				for dx := range b.scale {
					x := px0 + dx
					if image.Pt(x, y).In(clip) {
						dst.Set(x, y, b.color)
					}
				}
			}
		}
	}
}

// applyBlits stamps all collected bitmap glyphs onto dst.
func applyBlits(dst draw.Image, blits []textBlit) {
	for _, b := range blits {
		blitGlyph(dst, b)
	}
}

// Canvas is the demy rendering context. Build a Node tree using the constructor
// functions (VStack, HStack, Text, …), then call Render or EncodePNG.
//
// Layout dimensions are in logical pixels; font sizes are in points.
// The scale factor (Config.Scale) maps logical pixels to physical pixels.
// The origin is top-left (0,0); Y increases downward.
type Canvas struct {
	cfg          Config
	physicalDPMM float64 // raw display pixel density; used only by rasterizer
	dpmm         float64 // logical DPMM (physical / scale); used for all layout
	scale        float64
	Roles        Roles
}

// New creates a Canvas from the given Config.
// Returns an error if the Config has zero physical or pixel dimensions.
func New(cfg Config) (*Canvas, error) {
	if cfg.PhysicalWidthMM == 0 || cfg.PhysicalHeightMM == 0 {
		return nil, errors.New("demy: Config must have non-zero physical dimensions")
	}
	if cfg.PixelWidth == 0 || cfg.PixelHeight == 0 {
		return nil, errors.New("demy: Config must have non-zero pixel dimensions")
	}
	scale := cfg.EffectiveScale()
	return &Canvas{
		cfg:          cfg,
		physicalDPMM: cfg.PhysicalDPMM(),
		dpmm:         cfg.DPMM(),
		scale:        scale,
		Roles:        cfg.Roles,
	}, nil
}

// Scale returns the display scale factor (logical pixels to physical pixels).
func (c *Canvas) Scale() float64 { return c.scale }

// LogicalWidth returns the canvas width in logical pixels (physical pixels / Scale).
func (c *Canvas) LogicalWidth() float64 { return float64(c.cfg.PixelWidth) / c.scale }

// LogicalHeight returns the canvas height in logical pixels (physical pixels / Scale).
func (c *Canvas) LogicalHeight() float64 { return float64(c.cfg.PixelHeight) / c.scale }

// MustNew creates a Canvas and panics on error.
func MustNew(cfg Config) *Canvas {
	c, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}

// draw builds the internal node tree from root, runs the three-pass layout, and
// returns a rendered cv.Canvas plus any per-region dither requests and bitmap
// glyph blits collected during drawing.
func (c *Canvas) draw(root Node) (*cv.Canvas, []imageDitherRegion, []textBlit, error) {
	cvs := cv.New(c.cfg.PhysicalWidthMM, c.cfg.PhysicalHeightMM)
	ctx := cv.NewContext(cvs)
	ctx.SetCoordSystem(cv.CartesianIV)

	internal, err := buildNode(root, &c.cfg, dirVertical)
	if err != nil {
		return nil, nil, nil, err
	}

	wLogical := float64(c.cfg.PixelWidth) / c.scale
	hLogical := float64(c.cfg.PixelHeight) / c.scale

	mctx := &measureCtx{
		fonts:  c.cfg.Fonts,
		dpmm:   c.dpmm,
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
	ctx.DrawPath(0, 0, cv.Rectangle(c.cfg.PhysicalWidthMM, c.cfg.PhysicalHeightMM))

	dctx := &drawCtx{
		ctx:   ctx,
		dpmm:  c.dpmm,
		scale: c.scale,
		roles: c.Roles,
		fonts: c.cfg.Fonts,
	}
	internal.draw(dctx)
	return cvs, dctx.ditherRegions, dctx.textBlits, nil
}

// rasterize converts a rendered cv.Canvas into a pixel image.
func (c *Canvas) rasterize(cvs *cv.Canvas) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight))
	ras := newVectorRenderer(img, c.physicalDPMM)
	cvs.RenderTo(ras)
	return img
}

// Render lays out and rasterizes root with the given color mode and optional
// dithering algorithm (default NoDithering). ImageNodes with a non-NoDithering
// Dither field will have their pixel region re-dithered after global quantization,
// allowing selective dithering of images while keeping text crisp.
func (c *Canvas) Render(root Node, mode ColorMode, algo ...Dither) (image.Image, error) {
	if mode == Unset {
		mode = c.cfg.ColorMode
	}
	cvs, regions, blits, err := c.draw(root)
	if err != nil {
		return nil, err
	}
	rgba := c.rasterize(cvs)
	if mode == RGBA || mode == Unset {
		applyBlits(rgba, blits)
		return rgba, nil
	}

	gray := toGray(rgba)

	algorithm := NoDithering
	if len(algo) > 0 {
		algorithm = algo[0]
	}

	var pal []color.Gray
	var out *image.Gray
	switch mode {
	case OneBit:
		pal = []color.Gray{{Y: 0}, {Y: 255}}
	case TwoBit:
		pal = []color.Gray{{Y: 0}, {Y: 85}, {Y: 170}, {Y: 255}}
	case FourBit:
		pal = make([]color.Gray, 16)
		for i := range 16 {
			pal[i] = color.Gray{Y: uint8(i * 17)}
		}
	case BWRed:
		// TODO: two-pass black+red encoder
		applyBlits(gray, blits)
		return gray, nil
	case Grayscale:
		applyBlits(gray, blits)
		return gray, nil
	default:
		applyBlits(gray, blits)
		return gray, nil
	}

	out = applyDither(gray, pal, algorithm)

	// Re-dither image regions that requested a specific algorithm, starting from
	// the original 8-bit grayscale so error diffusion has the full tonal range.
	for _, r := range regions {
		sub := subGray(gray, r.bounds)
		ditheredSub := applyDither(sub, pal, r.algo)
		copyGrayRegion(out, ditheredSub, r.bounds.Min)
	}

	// Stamp crisp bitmap text last so it is never antialiased or re-dithered.
	applyBlits(out, blits)

	return out, nil
}

// applyDither quantizes a grayscale image to pal using the given algorithm.
func applyDither(src *image.Gray, pal []color.Gray, algo Dither) *image.Gray {
	switch algo {
	case FloydSteinberg:
		return ditherFloydSteinberg(src, pal)
	case Atkinson:
		return ditherAtkinson(src, pal)
	default:
		return ditherNone(src, pal)
	}
}

// subGray returns a copy of the sub-region of src defined by r.
func subGray(src *image.Gray, r image.Rectangle) *image.Gray {
	r = r.Intersect(src.Bounds())
	dst := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		for x := range r.Dx() {
			dst.SetGray(x, y, src.GrayAt(r.Min.X+x, r.Min.Y+y))
		}
	}
	return dst
}

// copyGrayRegion copies src into dst starting at offset pt.
func copyGrayRegion(dst, src *image.Gray, pt image.Point) {
	for y := range src.Bounds().Dy() {
		for x := range src.Bounds().Dx() {
			dst.SetGray(pt.X+x, pt.Y+y, src.GrayAt(x, y))
		}
	}
}
