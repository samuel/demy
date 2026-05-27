package demy

import (
	"errors"
	"image"
	"image/color"

	cv "github.com/tdewolff/canvas"
)

// imageDitherRegion records a pixel region and its requested dithering algorithm,
// collected during the draw pass from ImageNodes that have a non-NoDithering Dither field.
type imageDitherRegion struct {
	bounds image.Rectangle
	algo   Dither
}

// Canvas is the demy rendering context. Build a Node tree using the constructor
// functions (VStack, HStack, Text, …), then call Render or EncodePNG.
//
// All dimensions are in pixels. Font sizes are in points.
// The origin is top-left (0,0); Y increases downward.
type Canvas struct {
	cfg   Config
	dpmm  float64
	Roles Roles
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
	return &Canvas{
		cfg:   cfg,
		dpmm:  cfg.DPMM(),
		Roles: cfg.Roles,
	}, nil
}

// MustNew creates a Canvas and panics on error.
func MustNew(cfg Config) *Canvas {
	c, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}

// draw builds the internal node tree from root, runs the three-pass layout, and
// returns a rendered cv.Canvas and any per-region dither requests collected during drawing.
func (c *Canvas) draw(root Node) (*cv.Canvas, []imageDitherRegion, error) {
	cvs := cv.New(c.cfg.PhysicalWidthMM, c.cfg.PhysicalHeightMM)
	ctx := cv.NewContext(cvs)
	ctx.SetCoordSystem(cv.CartesianIV)

	wPx := float64(c.cfg.PixelWidth)
	hPx := float64(c.cfg.PixelHeight)

	internal, err := buildNode(root, &c.cfg, dirVertical)
	if err != nil {
		return nil, nil, err
	}

	mctx := &measureCtx{
		fonts:  c.cfg.Fonts,
		dpmm:   c.dpmm,
		availW: wPx,
		availH: hPx,
	}
	internal.measure(mctx)

	bounds := nodeRect{x: 0, y: 0, w: wPx, h: hPx}
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
		roles: c.Roles,
		fonts: c.cfg.Fonts,
	}
	internal.draw(dctx)
	return cvs, dctx.ditherRegions, nil
}

// rasterize converts a rendered cv.Canvas into a pixel image.
func (c *Canvas) rasterize(cvs *cv.Canvas) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight))
	ras := newVectorRenderer(img, c.dpmm)
	cvs.RenderTo(ras)
	return img
}

// Render lays out and rasterizes root with the given color mode and optional
// dithering algorithm (default NoDithering). ImageNodes with a non-NoDithering
// Dither field will have their pixel region re-dithered after global quantization,
// allowing selective dithering of images while keeping text crisp.
func (c *Canvas) Render(root Node, mode ColorMode, algo ...Dither) (image.Image, error) {
	cvs, regions, err := c.draw(root)
	if err != nil {
		return nil, err
	}
	rgba := c.rasterize(cvs)
	if mode == RGBA {
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
	case BWRed:
		// TODO: two-pass black+red encoder
		return gray, nil
	case Grayscale:
		return gray, nil
	default:
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
