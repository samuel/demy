package demy

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	cv "github.com/tdewolff/canvas"
)

// imageDitherRegion is a physical-pixel region of an image that requested its
// own dithering algorithm.
type imageDitherRegion struct {
	bounds image.Rectangle
	algo   Dither
}

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

// New creates a Canvas for cfg. It returns an error unless the pixel dimensions
// are in [1, 16384], the physical dimensions are finite and positive, and Scale
// is finite and non-negative (0 selects it automatically).
func New(cfg Config) (*Canvas, error) {
	if cfg.PixelWidth <= 0 || cfg.PixelHeight <= 0 || cfg.PixelWidth > maxPixelDimension || cfg.PixelHeight > maxPixelDimension {
		return nil, fmt.Errorf("demy: Config pixel dimensions %dx%d out of range [1, %d]", cfg.PixelWidth, cfg.PixelHeight, maxPixelDimension)
	}
	if !positiveFinite(cfg.PhysicalWidthMM) || !positiveFinite(cfg.PhysicalHeightMM) {
		return nil, fmt.Errorf("demy: Config physical dimensions %vx%v mm must be positive", cfg.PhysicalWidthMM, cfg.PhysicalHeightMM)
	}
	if cfg.Scale < 0 || math.IsNaN(cfg.Scale) || math.IsInf(cfg.Scale, 0) {
		return nil, fmt.Errorf("demy: Config.Scale %v must be finite and non-negative", cfg.Scale)
	}
	scale := cfg.EffectiveScale()
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
// plus the dither regions and bitmap glyphs to apply after rasterization.
func (c *Canvas) draw(root Node) (*cv.Canvas, []imageDitherRegion, []textBlit, error) {
	// Size the canvas from the pixel dimensions so millimeters map exactly onto
	// whole pixels; PhysicalHeightMM need not match the width-derived density.
	wMM := float64(c.cfg.PixelWidth) / c.physicalDPMM
	hMM := float64(c.cfg.PixelHeight) / c.physicalDPMM
	cvs := cv.New(wMM, hMM)
	ctx := cv.NewContext(cvs)
	ctx.SetCoordSystem(cv.CartesianIV)

	internal, err := buildNode(root, &c.cfg, dirVertical, 0)
	if err != nil {
		return nil, nil, nil, err
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
		ctx:   ctx,
		dpmm:  c.dpmm,
		scale: c.scale,
		roles: c.cfg.Roles,
		fonts: c.cfg.Fonts,
		clip:  image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight),
	}
	internal.draw(dctx)
	if dctx.err != nil {
		return nil, nil, nil, dctx.err
	}
	return cvs, dctx.ditherRegions, dctx.textBlits, nil
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
		// TODO: black+red two-layer encoder
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

	// Dither from the unquantized grayscale so error diffusion has the full range.
	for _, r := range regions {
		b := r.bounds.Intersect(gray.Bounds())
		if b.Empty() {
			continue
		}
		ditheredSub := applyDither(subGray(gray, b), pal, r.algo)
		copyGrayRegion(out, ditheredSub, b.Min)
	}

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

// subGray returns a copy of the region r of src, with origin (0,0). r must lie
// within src's bounds.
func subGray(src *image.Gray, r image.Rectangle) *image.Gray {
	dst := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		i := src.PixOffset(r.Min.X, r.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:], src.Pix[i:i+r.Dx()])
	}
	return dst
}

// copyGrayRegion copies src into dst with src's origin at pt. The destination
// region must lie within dst's bounds.
func copyGrayRegion(dst, src *image.Gray, pt image.Point) {
	w := src.Bounds().Dx()
	for y := range src.Bounds().Dy() {
		i := src.PixOffset(src.Bounds().Min.X, src.Bounds().Min.Y+y)
		copy(dst.Pix[dst.PixOffset(pt.X, pt.Y+y):], src.Pix[i:i+w])
	}
}
