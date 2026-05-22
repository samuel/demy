package pate

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"

	cv "github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

// stackEntry tracks the current container being built and its layout direction.
type stackEntry struct {
	children *[]node
	dir      layoutDir
}

type layoutDir int

const (
	dirVertical layoutDir = iota
	dirHorizontal
)

// Canvas is the pate rendering context. Build a layout tree by calling methods
// such as VStack, HStack, Text, and Image, then call Render or EncodePNG.
//
// All dimensions are in pixels. Font sizes are in points.
// The origin is top-left (0,0); Y increases downward.
type Canvas struct {
	cv       *cv.Canvas
	ctx      *cv.Context
	cfg      Config
	dpmm     float64
	Roles    Roles
	root     []node
	stack    []stackEntry
	rendered bool
}

// New creates a Canvas from the given Config.
// Returns an error if the Config has zero physical dimensions.
func New(cfg Config) (*Canvas, error) {
	if cfg.PhysicalWidthMM == 0 || cfg.PhysicalHeightMM == 0 {
		return nil, errors.New("pate: Config must have non-zero physical dimensions")
	}
	if cfg.PixelWidth == 0 || cfg.PixelHeight == 0 {
		return nil, errors.New("pate: Config must have non-zero pixel dimensions")
	}
	dpmm := float64(cfg.PixelWidth) / cfg.PhysicalWidthMM
	c := cv.New(cfg.PhysicalWidthMM, cfg.PhysicalHeightMM)
	ctx := cv.NewContext(c)
	ctx.SetCoordSystem(cv.CartesianIV)
	return &Canvas{
		cv:    c,
		ctx:   ctx,
		cfg:   cfg,
		dpmm:  dpmm,
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

// Pt converts n points to pixels based on the canvas PPI.
func (c *Canvas) Pt(n float64) float64 {
	return n * c.dpmm * 25.4 / 72
}

// MM converts n millimetres to pixels based on the canvas PPI.
func (c *Canvas) MM(n float64) float64 {
	return n * c.dpmm
}

// rasterize renders the canvas into an image of exactly PixelWidth×PixelHeight pixels.
func (c *Canvas) rasterize() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.cfg.PixelWidth, c.cfg.PixelHeight))
	ras := rasterizer.FromImage(img, cv.DPMM(c.dpmm), cv.DefaultColorSpace)
	c.cv.RenderTo(ras)
	ras.Close()
	return img
}

// Render returns the fully composed grayscale image. The layout and draw
// passes run automatically on the first call.
func (c *Canvas) Render() image.Image {
	c.ensureRendered()
	return c.rasterize()
}

// EncodePNG encodes the canvas as a PNG with the given color mode and optional
// dithering algorithm (default FloydSteinberg).
func (c *Canvas) EncodePNG(w io.Writer, mode ColorMode, algo ...Dither) error {
	c.ensureRendered()
	rgba := c.rasterize()
	gray := toGray(rgba)

	algorithm := Atkinson
	if len(algo) > 0 {
		algorithm = algo[0]
	}

	var out image.Image
	switch mode {
	case OneBit:
		pal := []color.Gray{{Y: 0}, {Y: 255}}
		if algorithm == FloydSteinberg {
			out = ditherFloydSteinberg(gray, pal)
		} else {
			out = ditherAtkinson(gray, pal)
		}
	case TwoBit:
		pal := []color.Gray{{Y: 0}, {Y: 85}, {Y: 170}, {Y: 255}}
		if algorithm == FloydSteinberg {
			out = ditherFloydSteinberg(gray, pal)
		} else {
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

// addNode adds n to the current container or to the root if no container is open.
func (c *Canvas) addNode(n node) {
	if len(c.stack) == 0 {
		c.root = append(c.root, n)
		return
	}
	top := c.stack[len(c.stack)-1].children
	*top = append(*top, n)
}

// withChildren opens a new child-collection scope.
func (c *Canvas) withChildren(children *[]node, dir layoutDir, fn func()) {
	c.stack = append(c.stack, stackEntry{children: children, dir: dir})
	fn()
	c.stack = c.stack[:len(c.stack)-1]
}

// currentDir returns the layout direction of the innermost container.
func (c *Canvas) currentDir() layoutDir {
	if len(c.stack) > 0 {
		return c.stack[len(c.stack)-1].dir
	}
	return dirVertical
}

// ensureRendered runs the layout and draw passes if not yet done.
func (c *Canvas) ensureRendered() {
	if c.rendered {
		return
	}
	c.rendered = true

	wPx := float64(c.cfg.PixelWidth)
	hPx := float64(c.cfg.PixelHeight)
	mctx := &measureCtx{
		fonts:  c.cfg.Fonts,
		dpmm:   c.dpmm,
		availW: wPx,
		availH: hPx,
	}
	for _, n := range c.root {
		n.measure(mctx)
	}
	bounds := nodeRect{x: 0, y: 0, w: wPx, h: hPx}
	for _, n := range c.root {
		n.layout(bounds)
	}

	// Draw background.
	bg := c.cfg.Roles.Background
	if bg == nil {
		bg = White
	}
	c.ctx.SetFillColor(bg)
	c.ctx.SetStrokeColor(color.Transparent)
	c.ctx.DrawPath(0, 0, cv.Rectangle(c.cfg.PhysicalWidthMM, c.cfg.PhysicalHeightMM))

	dctx := &drawCtx{
		ctx:   c.ctx,
		dpmm:  c.dpmm,
		roles: c.Roles,
		fonts: c.cfg.Fonts,
	}
	for _, n := range c.root {
		n.draw(dctx)
	}
}

// VStack lays children out vertically. Options: Gap, Padding, Alignment.
// The last argument must be a func() that adds children.
func (c *Canvas) VStack(args ...any) {
	n := &vstackNode{}
	var fn func()
	for _, arg := range args {
		switch v := arg.(type) {
		case gapOpt:
			n.gap = float64(v)
		case paddingOpt:
			n.padding = float64(v)
		case Alignment:
			n.align = v
		case func():
			fn = v
		}
	}
	if fn != nil {
		c.withChildren(&n.children, dirVertical, fn)
	}
	c.addNode(n)
}

// HStack lays children out horizontally. Options: Gap, Padding, Alignment.
func (c *Canvas) HStack(args ...any) {
	n := &hstackNode{}
	var fn func()
	for _, arg := range args {
		switch v := arg.(type) {
		case gapOpt:
			n.gap = float64(v)
		case paddingOpt:
			n.padding = float64(v)
		case Alignment:
			n.align = v
		case func():
			fn = v
		}
	}
	if fn != nil {
		c.withChildren(&n.children, dirHorizontal, fn)
	}
	c.addNode(n)
}

// ZStack layers children on top of each other. Options: ZAlignment.
// Default alignment is TopLeading.
func (c *Canvas) ZStack(args ...any) {
	n := &zstackNode{align: TopLeading}
	var fn func()
	for _, arg := range args {
		switch v := arg.(type) {
		case ZAlignment:
			n.align = v
		case func():
			fn = v
		}
	}
	if fn != nil {
		c.withChildren(&n.children, dirVertical, fn)
	}
	c.addNode(n)
}

// Box is a container with optional background, border, corner radius, padding, and clipping.
// Options: Padding, Background, Border, CornerRadius, Clip.
func (c *Canvas) Box(args ...any) {
	n := &boxNode{}
	var fn func()
	for _, arg := range args {
		switch v := arg.(type) {
		case paddingOpt:
			n.padding = float64(v)
		case borderOpt:
			n.borderWidth = v.width
			n.borderColor = v.color
		case backgroundOpt:
			n.background = v.color
		case cornerRadiusOpt:
			n.cornerRadius = float64(v)
		case clipOpt:
			n.clip = true
		case func():
			fn = v
		}
	}
	if fn != nil {
		c.withChildren(&n.children, dirVertical, fn)
	}
	c.addNode(n)
}

// Frame constrains its children to exactly w×h pixels.
func (c *Canvas) Frame(w, h float64, fn func()) {
	n := &frameNode{w: w, h: h}
	if fn != nil {
		c.withChildren(&n.children, dirVertical, fn)
	}
	c.addNode(n)
}

// Spacer fills remaining space in a stack. Optional weight (default 1).
func (c *Canvas) Spacer(weight ...float64) {
	w := 1.0
	if len(weight) > 0 {
		w = weight[0]
	}
	c.addNode(&spacerNode{weight: w})
}

// Divider draws a 1px rule; orientation is inferred from the enclosing stack.
func (c *Canvas) Divider() {
	horizontal := c.currentDir() == dirVertical
	c.addNode(&dividerNode{isHorizontal: horizontal})
}

// Text renders a string with the given style.
func (c *Canvas) Text(s string, style Style) {
	c.addNode(&textNode{text: s, style: style})
}

// Image renders an image. Options: ImageScale (ImageFit, ImageFill, ImageStretch), Clip.
func (c *Canvas) Image(img image.Image, opts ...any) {
	n := &imageNode{img: img}
	for _, opt := range opts {
		switch v := opt.(type) {
		case ImageScale:
			n.scale = v
		case clipOpt:
			n.clip = true
		}
	}
	c.addNode(n)
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
