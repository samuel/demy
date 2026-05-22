package pate

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"

	cv "github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

// Canvas is the pate rendering context. Build a Node tree using the constructor
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
		return nil, errors.New("pate: Config must have non-zero physical dimensions")
	}
	if cfg.PixelWidth == 0 || cfg.PixelHeight == 0 {
		return nil, errors.New("pate: Config must have non-zero pixel dimensions")
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
// returns a rendered cv.Canvas ready for rasterization.
func (c *Canvas) draw(root Node) (*cv.Canvas, error) {
	cvs := cv.New(c.cfg.PhysicalWidthMM, c.cfg.PhysicalHeightMM)
	ctx := cv.NewContext(cvs)
	ctx.SetCoordSystem(cv.CartesianIV)

	wPx := float64(c.cfg.PixelWidth)
	hPx := float64(c.cfg.PixelHeight)

	internal, err := buildNode(root, &c.cfg, dirVertical)
	if err != nil {
		return nil, err
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
	return cvs, nil
}

// rasterize converts a rendered cv.Canvas into a pixel image.
func (c *Canvas) rasterize(cvs *cv.Canvas) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight))
	ras := rasterizer.FromImage(img, cv.DPMM(c.dpmm), cv.DefaultColorSpace)
	cvs.RenderTo(ras)
	ras.Close()
	return img
}

// Render lays out and rasterizes root, returning the full grayscale image.
func (c *Canvas) Render(root Node) (image.Image, error) {
	cvs, err := c.draw(root)
	if err != nil {
		return nil, err
	}
	return c.rasterize(cvs), nil
}

// EncodePNG renders root and writes a PNG with the given color mode and optional
// dithering algorithm (default FloydSteinberg).
func (c *Canvas) EncodePNG(root Node, w io.Writer, mode ColorMode, algo ...Dither) error {
	cvs, err := c.draw(root)
	if err != nil {
		return err
	}
	rgba := c.rasterize(cvs)
	gray := toGray(rgba)

	algorithm := NoDithering
	if len(algo) > 0 {
		algorithm = algo[0]
	}

	var out image.Image
	switch mode {
	case OneBit:
		pal := []color.Gray{{Y: 0}, {Y: 255}}
		switch algorithm {
		case NoDithering:
			out = ditherNone(gray, pal)
		case FloydSteinberg:
			out = ditherFloydSteinberg(gray, pal)
		case Atkinson:
			out = ditherAtkinson(gray, pal)
		}
	case TwoBit:
		pal := []color.Gray{{Y: 0}, {Y: 85}, {Y: 170}, {Y: 255}}
		switch algorithm {
		case NoDithering:
			out = ditherNone(gray, pal)
		case FloydSteinberg:
			out = ditherFloydSteinberg(gray, pal)
		case Atkinson:
			out = ditherAtkinson(gray, pal)
		}
	case BWRed:
		// TODO: two-pass black+red encoder
		out = gray
	default: // Grayscale
		out = gray
	}
	return png.Encode(w, out)
}
