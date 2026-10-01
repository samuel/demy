package demy

import (
	"errors"
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
	color        color.Gray      // 0 or 255
	clip         image.Rectangle // physical pixels; empty means unclipped
}

// blitGlyph writes each set mask pixel of the glyph as a solid scale×scale
// block, leaving unset pixels untouched.
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
// effective scale is in [0.5, 64] (Scale 0 selects it automatically), and
// ColorMode is Unset or a supported mode.
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
	return &Canvas{
		cfg:          cfg,
		physicalDPMM: cfg.PhysicalDPMM(),
		dpmm:         cfg.DPMM(),
		scale:        scale,
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
// dithering are dithered to pal while drawing; nil pal leaves them as is.
func (c *Canvas) draw(root Node, pal []color.Gray) (*cv.Canvas, []textBlit, error) {
	// Size the canvas from the pixel dimensions so millimeters map exactly onto
	// whole pixels; PhysicalHeightMM need not match the width-derived density.
	wMM := float64(c.cfg.PixelWidth) / c.physicalDPMM
	hMM := float64(c.cfg.PixelHeight) / c.physicalDPMM
	cvs := cv.New(wMM, hMM)
	ctx := cv.NewContext(cvs)
	ctx.SetCoordSystem(cv.CartesianIV)

	internal, err := buildNode(root, &c.cfg, dirVertical, 0)
	if err != nil {
		return nil, nil, err
	}

	wLogical := float64(c.cfg.PixelWidth) / c.scale
	hLogical := float64(c.cfg.PixelHeight) / c.scale

	mctx := &measureCtx{
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
		ctx:     ctx,
		dpmm:    c.dpmm,
		scale:   c.scale,
		roles:   c.cfg.Roles,
		fonts:   c.cfg.Fonts,
		palette: pal,
		clip:    image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight),
	}
	internal.draw(dctx)
	if dctx.err != nil {
		return nil, nil, dctx.err
	}
	return cvs, dctx.textBlits, nil
}

// rasterize converts a rendered cv.Canvas into a pixel image.
func (c *Canvas) rasterize(cvs *cv.Canvas) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight))
	ras := newVectorRenderer(img, c.physicalDPMM)
	cvs.RenderTo(ras)
	return img
}

// Render lays out and rasterizes root. Unset mode uses Config.ColorMode, and
// RGBA is used if that is also Unset. algo selects the dithering applied when
// quantizing (default NoDithering); an ImageNode with its own Dither is
// dithered with that algorithm instead, so images can be dithered while text
// stays crisp.
func (c *Canvas) Render(root Node, mode ColorMode, algo ...Dither) (image.Image, error) {
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
	pal := palette(mode)
	cvs, blits, err := c.draw(root, pal)
	if err != nil {
		return nil, err
	}
	rgba := c.rasterize(cvs)
	if mode == RGBA {
		applyBlits(rgba, blits)
		return rgba, nil
	}

	gray := toGray(rgba)
	if pal == nil {
		applyBlits(gray, blits)
		return gray, nil
	}

	out := applyDither(gray, pal, algorithm)
	applyBlits(out, blits)
	return out, nil
}

// palette returns the gray levels mode quantizes to, or nil when mode does
// not quantize.
func palette(mode ColorMode) []color.Gray {
	switch mode {
	case OneBit:
		return []color.Gray{{Y: 0}, {Y: 255}}
	case TwoBit:
		return []color.Gray{{Y: 0}, {Y: 85}, {Y: 170}, {Y: 255}}
	case FourBit:
		pal := make([]color.Gray, 16)
		for i := range 16 {
			pal[i] = color.Gray{Y: uint8(i * 17)}
		}
		return pal
	default:
		return nil
	}
}

// check reports an error for an unknown mode or one that isn't implemented.
func (m ColorMode) check() error {
	switch m {
	case RGBA, Grayscale, OneBit, TwoBit, FourBit:
		return nil
	case BWRed:
		return errors.New("demy: color mode BWRed is not implemented")
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
