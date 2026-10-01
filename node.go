package demy

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"reflect"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	cv "github.com/tdewolff/canvas"
)

type layoutDir int

const (
	dirVertical layoutDir = iota
	dirHorizontal
)

// nodeSize holds a measured width and height in pixels.
type nodeSize struct{ w, h float64 }

// nodeRect holds a node's assigned bounds in pixels.
type nodeRect struct{ x, y, w, h float64 }

// measureCtx carries context for the measure pass.
type measureCtx struct {
	fonts  map[string]*FontFamily
	dpmm   float64
	scale  float64
	availW float64
	availH float64
}

// unbounded is the available size on an axis with no constraint.
const unbounded = math.MaxFloat32

// isUnbounded reports whether an available size is effectively unconstrained.
func isUnbounded(v float64) bool { return v > 1e9 }

// with returns a copy of mctx with the given available size.
func (mctx *measureCtx) with(availW, availH float64) *measureCtx {
	c := *mctx
	c.availW = availW
	c.availH = availH
	return &c
}

// drawCtx carries context for the draw pass.
type drawCtx struct {
	ctx           *cv.Context
	dpmm          float64 // logical DPMM (physical DPMM / scale)
	scale         float64
	roles         Roles
	fonts         map[string]*FontFamily
	ditherRegions []imageDitherRegion
	textBlits     []textBlit
	clip          image.Rectangle // physical-pixel clip for bitmap text and clipped sub-canvases
	err           error           // first error encountered while drawing
}

// setErr records err if no earlier draw error has been recorded.
func (dctx *drawCtx) setErr(err error) {
	if dctx.err == nil {
		dctx.err = err
	}
}

// node is the interface all layout and widget nodes implement.
type node interface {
	measure(mctx *measureCtx) nodeSize
	layout(bounds nodeRect)
	draw(dctx *drawCtx)
}

// --- vstackNode ---

type vstackNode struct {
	gap           float64
	padding       float64
	align         Alignment
	children      []node
	measuredSizes []nodeSize
	bounds        nodeRect
}

func (n *vstackNode) measure(mctx *measureCtx) nodeSize {
	innerW := math.Max(0, mctx.availW-2*n.padding)
	n.measuredSizes = make([]nodeSize, len(n.children))
	var totalH, maxW float64
	for i, child := range n.children {
		s := child.measure(mctx.with(innerW, unbounded))
		n.measuredSizes[i] = s
		if i > 0 {
			totalH += n.gap
		}
		switch child.(type) {
		case *spacerNode, *weightedNode:
		default:
			totalH += s.h
		}
		if s.w > maxW {
			maxW = s.w
		}
	}
	return nodeSize{w: maxW + 2*n.padding, h: totalH + 2*n.padding}
}

func (n *vstackNode) layout(bounds nodeRect) {
	n.bounds = bounds
	innerX := bounds.x + n.padding
	innerY := bounds.y + n.padding
	innerW := math.Max(0, bounds.w-2*n.padding)
	innerH := math.Max(0, bounds.h-2*n.padding)

	// Compute fixed height and total weight (spacers + weighted nodes share one pool).
	fixedH := -n.gap // account for no gap before first child
	totalWeight := 0.0
	for i, child := range n.children {
		fixedH += n.gap
		switch v := child.(type) {
		case *spacerNode:
			totalWeight += v.weight
		case *weightedNode:
			totalWeight += v.weight
		default:
			fixedH += n.measuredSizes[i].h
		}
	}
	unitH := 0.0
	if totalWeight > 0 {
		unitH = math.Max(0, innerH-fixedH) / totalWeight
	}

	y := innerY
	for i, child := range n.children {
		if i > 0 {
			y += n.gap
		}
		var childW, childH float64
		var fillCrossAxis bool
		switch v := child.(type) {
		case *spacerNode:
			childH = unitH * v.weight
			fillCrossAxis = true
		case *weightedNode:
			childH = unitH * v.weight
			fillCrossAxis = true
		default:
			childH = n.measuredSizes[i].h
			childW = n.measuredSizes[i].w
		}
		childX := innerX
		childLayoutW := innerW
		if !fillCrossAxis && childW < innerW {
			switch n.align {
			case AlignCenter:
				childX = innerX + (innerW-childW)/2
				childLayoutW = childW
			case AlignEnd:
				childX = innerX + innerW - childW
				childLayoutW = childW
			case AlignStart:
			}
		}
		child.layout(nodeRect{x: childX, y: y, w: childLayoutW, h: childH})
		y += childH
	}
}

func (n *vstackNode) draw(dctx *drawCtx) {
	for _, child := range n.children {
		child.draw(dctx)
	}
}

// --- hstackNode ---

type hstackNode struct {
	gap           float64
	padding       float64
	align         Alignment
	children      []node
	measuredSizes []nodeSize
	bounds        nodeRect
}

func (n *hstackNode) measure(mctx *measureCtx) nodeSize {
	innerH := math.Max(0, mctx.availH-2*n.padding)
	innerW := math.Max(0, mctx.availW-2*n.padding)
	n.measuredSizes = make([]nodeSize, len(n.children))

	// Pass 1: measure all children at full available width.
	for i, child := range n.children {
		n.measuredSizes[i] = child.measure(mctx.with(innerW, innerH))
	}

	// Pass 2: if variable-width children overflow the space left after gaps and
	// frameNodes, shrink them proportionally and re-measure so heights are correct.
	totalGaps := math.Max(0, float64(len(n.children)-1)) * n.gap
	var fixedSizeW, variableSizeW float64
	for i, child := range n.children {
		switch child.(type) {
		case *spacerNode, *weightedNode:
		case *frameNode:
			fixedSizeW += n.measuredSizes[i].w
		default:
			variableSizeW += n.measuredSizes[i].w
		}
	}
	availForVar := innerW - totalGaps - fixedSizeW
	if variableSizeW > availForVar && variableSizeW > 0 {
		shrinkScale := math.Max(0, availForVar) / variableSizeW
		for i, child := range n.children {
			switch child.(type) {
			case *spacerNode, *weightedNode, *frameNode:
			default:
				newW := n.measuredSizes[i].w * shrinkScale
				n.measuredSizes[i] = child.measure(mctx.with(newW, innerH))
			}
		}
	}

	// Pass 3: measure weighted children at their share of the remaining width so
	// their heights count toward the stack's height.
	var usedW, totalWeight float64
	for i, child := range n.children {
		switch v := child.(type) {
		case *spacerNode:
			totalWeight += v.weight
		case *weightedNode:
			totalWeight += v.weight
		default:
			usedW += n.measuredSizes[i].w
		}
	}
	if totalWeight > 0 {
		unitW := math.Max(0, innerW-totalGaps-usedW) / totalWeight
		for i, child := range n.children {
			if v, ok := child.(*weightedNode); ok {
				n.measuredSizes[i] = child.measure(mctx.with(unitW*v.weight, innerH))
			}
		}
	}

	var totalW, maxH float64
	for i, child := range n.children {
		s := n.measuredSizes[i]
		if i > 0 {
			totalW += n.gap
		}
		switch child.(type) {
		case *spacerNode, *weightedNode:
		default:
			totalW += s.w
		}
		if s.h > maxH {
			maxH = s.h
		}
	}
	return nodeSize{w: totalW + 2*n.padding, h: maxH + 2*n.padding}
}

func (n *hstackNode) layout(bounds nodeRect) {
	n.bounds = bounds
	innerX := bounds.x + n.padding
	innerY := bounds.y + n.padding
	innerW := math.Max(0, bounds.w-2*n.padding)
	innerH := math.Max(0, bounds.h-2*n.padding)

	fixedW := -n.gap
	totalWeight := 0.0
	for i, child := range n.children {
		fixedW += n.gap
		switch v := child.(type) {
		case *spacerNode:
			totalWeight += v.weight
		case *weightedNode:
			totalWeight += v.weight
		default:
			fixedW += n.measuredSizes[i].w
		}
	}
	unitW := 0.0
	if totalWeight > 0 {
		unitW = math.Max(0, innerW-fixedW) / totalWeight
	}

	x := innerX
	for i, child := range n.children {
		if i > 0 {
			x += n.gap
		}
		var childW, childH float64
		var fillCrossAxis bool
		switch v := child.(type) {
		case *spacerNode:
			childW = unitW * v.weight
			fillCrossAxis = true
		case *weightedNode:
			childW = unitW * v.weight
			fillCrossAxis = true
		case *dividerNode:
			childW = n.measuredSizes[i].w
			fillCrossAxis = true
		default:
			childW = n.measuredSizes[i].w
			childH = math.Min(n.measuredSizes[i].h, innerH)
		}
		if fillCrossAxis {
			childH = innerH
		}
		childY := innerY
		if !fillCrossAxis && childH < innerH {
			switch n.align {
			case AlignCenter:
				childY = innerY + (innerH-childH)/2
			case AlignEnd:
				childY = innerY + innerH - childH
			case AlignStart:
			}
		}
		child.layout(nodeRect{x: x, y: childY, w: childW, h: childH})
		x += childW
	}
}

func (n *hstackNode) draw(dctx *drawCtx) {
	for _, child := range n.children {
		child.draw(dctx)
	}
}

// --- zstackNode ---

type zstackNode struct {
	align         ZAlignment
	children      []node
	measuredSizes []nodeSize
	bounds        nodeRect
}

func (n *zstackNode) measure(mctx *measureCtx) nodeSize {
	n.measuredSizes = make([]nodeSize, len(n.children))
	var maxW, maxH float64
	for i, child := range n.children {
		s := child.measure(mctx)
		n.measuredSizes[i] = s
		if s.w > maxW {
			maxW = s.w
		}
		if s.h > maxH {
			maxH = s.h
		}
	}
	return nodeSize{w: maxW, h: maxH}
}

func (n *zstackNode) layout(bounds nodeRect) {
	n.bounds = bounds
	for i, child := range n.children {
		s := n.measuredSizes[i]
		// Horizontal: align%3 → 0=leading, 1=center, 2=trailing
		var cx float64
		switch int(n.align) % 3 {
		case 1:
			cx = bounds.x + (bounds.w-s.w)/2
		case 2:
			cx = bounds.x + bounds.w - s.w
		default:
			cx = bounds.x
		}
		// Vertical: align/3 → 0=top, 1=middle, 2=bottom
		var cy float64
		switch int(n.align) / 3 {
		case 1:
			cy = bounds.y + (bounds.h-s.h)/2
		case 2:
			cy = bounds.y + bounds.h - s.h
		default:
			cy = bounds.y
		}
		child.layout(nodeRect{x: cx, y: cy, w: s.w, h: s.h})
	}
}

func (n *zstackNode) draw(dctx *drawCtx) {
	for _, child := range n.children {
		child.draw(dctx)
	}
}

// --- boxNode ---

type boxNode struct {
	padding       float64
	borderWidth   float64
	borderColor   color.Color
	background    color.Color
	cornerRadius  float64
	clip          bool
	children      []node
	measuredSizes []nodeSize
	bounds        nodeRect
}

// inset returns the distance from the box edge to its content.
func (n *boxNode) inset() float64 {
	if n.borderColor == nil {
		return n.padding
	}
	return n.padding + n.borderWidth
}

func (n *boxNode) measure(mctx *measureCtx) nodeSize {
	innerW := math.Max(0, mctx.availW-2*n.inset())
	innerH := math.Max(0, mctx.availH-2*n.inset())
	n.measuredSizes = make([]nodeSize, len(n.children))
	var maxW, maxH float64
	for i, child := range n.children {
		s := child.measure(mctx.with(innerW, innerH))
		n.measuredSizes[i] = s
		if s.w > maxW {
			maxW = s.w
		}
		if s.h > maxH {
			maxH = s.h
		}
	}
	return nodeSize{w: maxW + 2*n.inset(), h: maxH + 2*n.inset()}
}

func (n *boxNode) layout(bounds nodeRect) {
	n.bounds = bounds
	innerX := bounds.x + n.inset()
	innerY := bounds.y + n.inset()
	innerW := math.Max(0, bounds.w-2*n.inset())
	innerH := math.Max(0, bounds.h-2*n.inset())
	for _, child := range n.children {
		child.layout(nodeRect{x: innerX, y: innerY, w: innerW, h: innerH})
	}
}

func (n *boxNode) draw(dctx *drawCtx) {
	dpmm := dctx.dpmm
	xMM := n.bounds.x / dpmm
	yMM := n.bounds.y / dpmm
	wMM := n.bounds.w / dpmm
	hMM := n.bounds.h / dpmm
	rMM := n.cornerRadius / dpmm

	if n.background != nil {
		dctx.ctx.SetFillColor(n.background)
		dctx.ctx.SetStrokeColor(color.Transparent)
		if n.cornerRadius > 0 {
			dctx.ctx.DrawPath(xMM, yMM, cv.RoundedRectangle(wMM, hMM, rMM))
		} else {
			dctx.ctx.DrawPath(xMM, yMM, cv.Rectangle(wMM, hMM))
		}
	}

	if n.borderWidth > 0 && n.borderColor != nil {
		bwMM := n.borderWidth / dpmm
		inset := bwMM / 2
		dctx.ctx.SetFillColor(color.Transparent)
		dctx.ctx.SetStrokeColor(n.borderColor)
		dctx.ctx.SetStrokeWidth(bwMM)
		if n.cornerRadius > 0 {
			r := math.Max(0, rMM-inset)
			dctx.ctx.DrawPath(xMM+inset, yMM+inset, cv.RoundedRectangle(wMM-2*inset, hMM-2*inset, r))
		} else {
			dctx.ctx.DrawPath(xMM+inset, yMM+inset, cv.Rectangle(wMM-2*inset, hMM-2*inset))
		}
		dctx.ctx.SetStrokeWidth(1)
	}

	if n.clip {
		drawClipped(dctx, n.bounds, n.children)
		return
	}
	for _, child := range n.children {
		child.draw(dctx)
	}
}

// --- frameNode ---

type frameNode struct {
	w, h     float64
	children []node
	bounds   nodeRect
}

func (n *frameNode) measure(mctx *measureCtx) nodeSize {
	for _, child := range n.children {
		child.measure(mctx.with(n.w, n.h))
	}
	return nodeSize{w: n.w, h: n.h}
}

func (n *frameNode) layout(bounds nodeRect) {
	n.bounds = bounds
	for _, child := range n.children {
		child.layout(nodeRect{x: bounds.x, y: bounds.y, w: n.w, h: n.h})
	}
}

func (n *frameNode) draw(dctx *drawCtx) {
	drawClipped(dctx, nodeRect{x: n.bounds.x, y: n.bounds.y, w: n.w, h: n.h}, n.children)
}

// drawClipped draws children onto a sub-canvas covering r and composites it
// onto dctx, confining their content to r. The sub-canvas is rasterized at
// physical resolution on whole physical pixels, so clipped content is as sharp
// as unclipped content.
func drawClipped(dctx *drawCtx, r nodeRect, children []node) {
	// Bitmap glyphs and dither regions apply to the root image after
	// quantization, so they are clipped to the same physical-pixel bounds.
	// Limiting the sub-canvas to the visible region bounds its allocation by
	// the root canvas regardless of the node's own size.
	clip := image.Rect(
		int(math.Round(r.x*dctx.scale)),
		int(math.Round(r.y*dctx.scale)),
		int(math.Round((r.x+r.w)*dctx.scale)),
		int(math.Round((r.y+r.h)*dctx.scale)),
	).Intersect(dctx.clip)
	if clip.Empty() {
		return
	}

	physDPMM := dctx.dpmm * dctx.scale
	xMM := float64(clip.Min.X) / physDPMM
	yMM := float64(clip.Min.Y) / physDPMM

	subCvs := cv.New(float64(clip.Dx())/physDPMM, float64(clip.Dy())/physDPMM)
	subCtx := cv.NewContext(subCvs)
	subCtx.SetCoordSystem(cv.CartesianIV)
	subCtx.Translate(-xMM, -yMM)

	subDctx := &drawCtx{
		ctx:   subCtx,
		dpmm:  dctx.dpmm,
		scale: dctx.scale,
		roles: dctx.roles,
		fonts: dctx.fonts,
		clip:  clip,
	}
	for _, child := range children {
		child.draw(subDctx)
	}
	if subDctx.err != nil {
		dctx.setErr(subDctx.err)
	}
	dctx.textBlits = append(dctx.textBlits, subDctx.textBlits...)
	for _, dr := range subDctx.ditherRegions {
		dr.bounds = dr.bounds.Intersect(clip)
		if !dr.bounds.Empty() {
			dctx.ditherRegions = append(dctx.ditherRegions, dr)
		}
	}

	subImg := image.NewRGBA(image.Rect(0, 0, clip.Dx(), clip.Dy()))
	subCvs.RenderTo(newVectorRenderer(subImg, physDPMM))
	dctx.ctx.DrawImage(xMM, yMM, subImg, cv.DPMM(physDPMM))
}

// --- spacerNode ---

type spacerNode struct {
	weight float64
	bounds nodeRect
}

func (n *spacerNode) measure(_ *measureCtx) nodeSize { return nodeSize{} }

func (n *spacerNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *spacerNode) draw(_ *drawCtx) {}

// --- weightedNode ---

type weightedNode struct {
	weight float64
	child  node
	mctx   *measureCtx // retained from measure to re-measure the child with its allocated bounds
	bounds nodeRect
}

// measure returns the child's size. Stacks ignore the main-axis component and
// use the cross-axis one.
func (n *weightedNode) measure(mctx *measureCtx) nodeSize {
	n.mctx = mctx
	return n.child.measure(mctx)
}

func (n *weightedNode) layout(bounds nodeRect) {
	n.bounds = bounds
	n.child.measure(n.mctx.with(bounds.w, bounds.h))
	n.child.layout(bounds)
}

func (n *weightedNode) draw(dctx *drawCtx) { n.child.draw(dctx) }

// --- dividerNode ---

type dividerNode struct {
	isHorizontal bool
	color        color.Color
	size         float64
	bounds       nodeRect
}

func (n *dividerNode) measure(mctx *measureCtx) nodeSize {
	sz := n.size
	if sz == 0 {
		sz = 1
	}
	if n.isHorizontal {
		return nodeSize{w: mctx.availW, h: sz}
	}
	// A vertical divider stretches to the stack's height in layout, so it
	// contributes no height of its own.
	return nodeSize{w: sz}
}

func (n *dividerNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *dividerNode) draw(dctx *drawCtx) {
	col := n.color
	if col == nil {
		col = dctx.roles.Muted
	}
	if col == nil {
		col = dctx.roles.Foreground
	}
	if col == nil {
		col = Black
	}
	dctx.ctx.SetFillColor(col)
	dctx.ctx.SetStrokeColor(color.Transparent)
	var xPx, yPx float64
	if n.isHorizontal {
		xPx = n.bounds.x
		yPx = math.Round(n.bounds.y)
	} else {
		xPx = math.Round(n.bounds.x)
		yPx = n.bounds.y
	}
	xMM := xPx / dctx.dpmm
	yMM := yPx / dctx.dpmm
	wMM := n.bounds.w / dctx.dpmm
	hMM := n.bounds.h / dctx.dpmm
	dctx.ctx.DrawPath(xMM, yMM, cv.Rectangle(wMM, hMM))
}

// --- textNode ---

type textNode struct {
	text   string
	style  Style
	bounds nodeRect
}

func (n *textNode) measure(mctx *measureCtx) nodeSize {
	ff := mctx.fonts[n.style.Font]
	if ff == nil {
		return nodeSize{}
	}
	if ff.isBitmap() {
		return n.measureBitmap(mctx, ff)
	}
	if n.style.Size == 0 {
		return nodeSize{}
	}
	clr := n.style.Color
	if clr == nil {
		clr = Black
	}
	face := ff.family.Face(n.style.Size, clr, n.style.Weight.cvStyle())
	tb := n.textBox(face, mctx.availW/mctx.dpmm)
	return nodeSize{w: tb.Width * mctx.dpmm, h: tb.Height * mctx.dpmm}
}

// textBox lays out the text in a box wMM millimeters wide, limited to MaxLines.
// With TextOverflowEllipsis, text that doesn't fit is shortened to the longest
// prefix that fits with an ellipsis appended.
func (n *textNode) textBox(face *cv.FontFace, wMM float64) *cv.Text {
	var heightMM float64
	if n.style.MaxLines > 0 {
		lineStretch := n.style.LineHeight
		if lineStretch == 0 {
			lineStretch = 1.0
		}
		heightMM = face.LineHeight() * float64(n.style.MaxLines) * lineStretch
	}
	opts := textOptions(n.style)
	box := func(text string) *cv.Text {
		return cv.NewTextBox(face, text, wMM, heightMM, cvHAlign(n.style.Align), cv.Top, opts)
	}
	tb := box(n.text)
	if heightMM == 0 || n.style.Overflow != TextOverflowEllipsis || !tb.OverflowsY {
		return tb
	}
	// Binary search for the longest rune prefix that fits with the ellipsis.
	runes := []rune(n.text)
	lo, hi := 0, len(runes)
	best := box("…")
	for lo < hi {
		mid := (lo + hi + 1) / 2
		cand := box(strings.TrimRight(string(runes[:mid]), " ") + "…")
		if cand.OverflowsY {
			hi = mid - 1
		} else {
			lo = mid
			best = cand
		}
	}
	return best
}

func (n *textNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *textNode) draw(dctx *drawCtx) {
	ff := dctx.fonts[n.style.Font]
	if ff == nil {
		return
	}
	if ff.isBitmap() {
		n.drawBitmap(dctx, ff)
		return
	}
	if n.style.Size == 0 {
		return
	}
	clr := n.style.Color
	if clr == nil {
		clr = dctx.roles.Foreground
	}
	if clr == nil {
		clr = Black
	}
	face := ff.family.Face(n.style.Size, clr, n.style.Weight.cvStyle())
	// The laid-out width is often exactly the measured width; the epsilon
	// absorbs the float round trip so lines don't break differently than measured.
	tb := n.textBox(face, n.bounds.w/dctx.dpmm+1e-6)
	xMM := n.bounds.x / dctx.dpmm
	yMM := n.bounds.y / dctx.dpmm
	dctx.ctx.DrawText(xMM, yMM, tb)
}

func cvHAlign(a TextAlign) cv.TextAlign {
	switch a {
	case TextAlignCenter:
		return cv.Center
	case TextAlignRight:
		return cv.Right
	default:
		return cv.Left
	}
}

func textOptions(s Style) *cv.TextOptions {
	if s.LineHeight != 0 && s.LineHeight != 1.0 {
		return &cv.TextOptions{LineStretch: s.LineHeight - 1.0}
	}
	return nil
}

// --- bitmap text ---

// bitmapIntScale returns the whole-number upscale applied to bitmap glyphs.
func bitmapIntScale(scale float64) int { return max(int(math.Round(scale)), 1) }

// bitmapUnit returns the logical size of one font pixel. Glyphs are drawn at a
// whole-number physical scale, so with a fractional display scale a font pixel
// is not exactly one logical pixel.
func bitmapUnit(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	return float64(bitmapIntScale(scale)) / scale
}

// measureBitmap measures text in a bitmap font.
func (n *textNode) measureBitmap(mctx *measureCtx, ff *FontFamily) nodeSize {
	bf := ff.bitmapVariant(n.style.Weight)
	if bf == nil {
		return nodeSize{}
	}
	unit := bitmapUnit(mctx.scale)
	maxW := mctx.availW
	if !isUnbounded(maxW) {
		maxW /= unit
	}
	lines := layoutBitmapText(bf, n.text, maxW, n.style)
	widest := 0
	for _, runes := range lines {
		if w := bmpLineWidth(bf, runes); w > widest {
			widest = w
		}
	}
	return nodeSize{
		w: float64(widest) * unit,
		h: float64(len(lines)) * bitmapLineHeight(bf, n.style) * unit,
	}
}

// drawBitmap collects glyph blits for text in a bitmap font. Blits are applied
// after quantization so the text is never antialiased or dithered.
func (n *textNode) drawBitmap(dctx *drawCtx, ff *FontFamily) {
	bf := ff.bitmapVariant(n.style.Weight)
	if bf == nil {
		return
	}
	clr := n.style.Color
	if clr == nil {
		clr = dctx.roles.Foreground
	}
	if clr == nil {
		clr = Black
	}
	col := snapBitmapColor(clr)

	iscale := bitmapIntScale(dctx.scale)
	lines := layoutBitmapText(bf, n.text, n.bounds.w/bitmapUnit(dctx.scale), n.style)

	baseXp := n.bounds.x * dctx.scale
	baseYp := n.bounds.y * dctx.scale
	boundsWp := n.bounds.w * dctx.scale
	lineStepP := bitmapLineHeight(bf, n.style) * float64(iscale)

	for li, runes := range lines {
		lineWp := float64(bmpLineWidth(bf, runes) * iscale)
		var startXp float64
		switch n.style.Align {
		case TextAlignCenter:
			startXp = baseXp + (boundsWp-lineWp)/2
		case TextAlignRight:
			startXp = baseXp + (boundsWp - lineWp)
		default:
			startXp = baseXp
		}
		penX := int(math.Round(startXp))
		lineTop := int(math.Round(baseYp + float64(li)*lineStepP))
		for _, r := range runes {
			if g, ok := bf.glyph(r); ok && len(g.bits) > 0 {
				dctx.textBlits = append(dctx.textBlits, textBlit{
					glyph: g,
					destX: penX + g.xOffset*iscale,
					destY: lineTop + (bf.ascent-g.height-g.yOffset)*iscale,
					scale: iscale,
					color: col,
					clip:  dctx.clip,
				})
			}
			penX += bmpRuneAdvance(bf, r) * iscale
		}
	}
}

// bitmapLineHeight returns the line height in font pixels, applying the
// style's line-height multiplier (default 1.0).
func bitmapLineHeight(bf *bitmapFont, s Style) float64 {
	stretch := s.LineHeight
	if stretch == 0 {
		stretch = 1.0
	}
	return float64(bf.lineHeight) * stretch
}

// layoutBitmapText splits text into lines, honoring line breaks, MaxLines, and
// ellipsis overflow. When maxW (in font pixels) is a finite positive width,
// words wrap greedily and words wider than maxW are broken between runes.
func layoutBitmapText(bf *bitmapFont, text string, maxW float64, style Style) [][]rune {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	wrap := maxW > 0 && !isUnbounded(maxW)
	limit := int(math.Floor(maxW))
	var lines [][]rune
	for para := range strings.SplitSeq(text, "\n") {
		if !wrap {
			lines = append(lines, []rune(para))
			continue
		}
		var cur []rune
		curW := 0
		space := bf.spaceAdvance
		for word := range strings.SplitSeq(para, " ") {
			wr := []rune(word)
			ww := bmpLineWidth(bf, wr)
			if len(cur) > 0 && curW+space+ww > limit {
				lines = append(lines, cur)
				cur = nil
				curW = 0
			}
			if len(cur) > 0 {
				cur = append(cur, ' ')
				curW += space
			}
			for _, r := range wr {
				adv := bmpRuneAdvance(bf, r)
				if len(cur) > 0 && curW+adv > limit {
					lines = append(lines, cur)
					cur = nil
					curW = 0
				}
				cur = append(cur, r)
				curW += adv
			}
		}
		lines = append(lines, cur)
	}
	if style.MaxLines > 0 && len(lines) > style.MaxLines {
		lines = lines[:style.MaxLines]
		if style.Overflow == TextOverflowEllipsis {
			i := len(lines) - 1
			lines[i] = appendEllipsis(bf, lines[i], limit, wrap)
		}
	}
	return lines
}

// appendEllipsis appends an ellipsis marker to a line, using '…' when the font
// has that glyph and "..." otherwise. When fit is set, runes are removed from
// the end of the line until the marker fits within maxW font pixels.
func appendEllipsis(bf *bitmapFont, line []rune, maxW int, fit bool) []rune {
	marker := []rune("...")
	if _, ok := bf.glyphs['…']; ok {
		marker = []rune{'…'}
	}
	if fit {
		mw := bmpLineWidth(bf, marker)
		for len(line) > 0 && bmpLineWidth(bf, line)+mw > maxW {
			line = line[:len(line)-1]
		}
	}
	return append(line, marker...)
}

// bmpRuneAdvance returns the x-advance for a rune, or the space advance when
// the font has no glyph for it.
func bmpRuneAdvance(bf *bitmapFont, r rune) int {
	if g, ok := bf.glyph(r); ok {
		return g.advance
	}
	return bf.spaceAdvance
}

// bmpLineWidth returns the total advance width of a line of runes, in font pixels.
func bmpLineWidth(bf *bitmapFont, runes []rune) int {
	w := 0
	for _, r := range runes {
		w += bmpRuneAdvance(bf, r)
	}
	return w
}

// snapBitmapColor snaps a color to black or white by luminance.
func snapBitmapColor(c color.Color) color.Gray {
	gy := color.GrayModel.Convert(c).(color.Gray)
	if gy.Y < 128 {
		return color.Gray{Y: 0}
	}
	return color.Gray{Y: 255}
}

// --- imageNode ---

type imageNode struct {
	img    image.Image
	scale  ImageScale
	dither Dither
	bounds nodeRect
}

func (n *imageNode) measure(mctx *measureCtx) nodeSize {
	if n.img == nil {
		return nodeSize{}
	}
	iw := float64(n.img.Bounds().Dx())
	ih := float64(n.img.Bounds().Dy())
	if iw == 0 || ih == 0 {
		return nodeSize{}
	}
	availW := mctx.availW
	availH := mctx.availH
	switch n.scale {
	case ImageFill, ImageStretch:
		w := availW
		h := availH
		if isUnbounded(h) {
			h = ih
		}
		if isUnbounded(w) {
			w = iw
		}
		return nodeSize{w: w, h: h}
	default: // ImageFit
		// An unbounded axis doesn't constrain the fit, so the image scales to
		// the other axis, as it does when both are bounded.
		var scale float64
		switch {
		case isUnbounded(availW) && isUnbounded(availH):
			scale = 1
		case isUnbounded(availH):
			scale = availW / iw
		case isUnbounded(availW):
			scale = availH / ih
		default:
			scale = math.Min(availW/iw, availH/ih)
		}
		return nodeSize{w: iw * scale, h: ih * scale}
	}
}

func (n *imageNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *imageNode) draw(dctx *drawCtx) {
	if n.img == nil {
		return
	}
	bw := n.bounds.w
	bh := n.bounds.h
	if bw == 0 || bh == 0 {
		return
	}
	dpmm := dctx.dpmm
	rect := cv.Rect{
		X0: n.bounds.x / dpmm,
		Y0: n.bounds.y / dpmm,
		X1: (n.bounds.x + bw) / dpmm,
		Y1: (n.bounds.y + bh) / dpmm,
	}
	img := n.img
	var fit cv.ImageFit
	switch n.scale {
	case ImageFill:
		fit = cv.ImageCover
		// The canvas crops cover images with SubImage and panics without it.
		if _, ok := img.(interface {
			SubImage(image.Rectangle) image.Image
		}); !ok {
			rgba := image.NewRGBA(img.Bounds())
			draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
			img = rgba
		}
	case ImageStretch:
		fit = cv.ImageFill
	default: // ImageFit
		fit = cv.ImageContain
	}
	drawn := dctx.ctx.FitImage(img, rect, fit)
	if n.dither != NoDithering {
		// Dither only the area the image covers, not letterboxing around it.
		physDPMM := dpmm * dctx.scale
		px := image.Rect(
			int(math.Round(drawn.X0*physDPMM)),
			int(math.Round(drawn.Y0*physDPMM)),
			int(math.Round(drawn.X1*physDPMM)),
			int(math.Round(drawn.Y1*physDPMM)),
		)
		dctx.ditherRegions = append(dctx.ditherRegions, imageDitherRegion{bounds: px, algo: n.dither})
	}
}

// --- barcodeNode ---

type barcodeNode struct {
	code   barcode.Barcode // encoded at build time
	bounds nodeRect
}

func encodeBarcode(v *BarcodeNode) (barcode.Barcode, error) {
	switch v.Type {
	default: // BarcodeTypeQR
		bc, err := qr.Encode(v.Content, qr.ErrorCorrectionLevel(v.ECLevel), qr.Auto)
		if err != nil {
			return nil, fmt.Errorf("demy: encoding QR code: %w", err)
		}
		return bc, nil
	}
}

func (n *barcodeNode) measure(mctx *measureCtx) nodeSize {
	w := mctx.availW
	h := mctx.availH
	if isUnbounded(h) {
		h = w
	}
	if isUnbounded(w) {
		w = h
	}
	return nodeSize{w: w, h: h}
}

func (n *barcodeNode) layout(bounds nodeRect) { n.bounds = bounds }

// draw scales the code by a whole number of physical pixels per module and
// places it on whole physical pixels, so modules stay sharp at any display scale.
func (n *barcodeNode) draw(dctx *drawCtx) {
	px := image.Rect(
		int(math.Round(n.bounds.x*dctx.scale)),
		int(math.Round(n.bounds.y*dctx.scale)),
		int(math.Round((n.bounds.x+n.bounds.w)*dctx.scale)),
		int(math.Round((n.bounds.y+n.bounds.h)*dctx.scale)),
	)
	if px.Empty() {
		return
	}
	scaled, err := barcode.Scale(n.code, px.Dx(), px.Dy())
	if err != nil {
		dctx.setErr(fmt.Errorf("demy: barcode does not fit %dx%d pixels: %w", px.Dx(), px.Dy(), err))
		return
	}
	physDPMM := dctx.dpmm * dctx.scale
	dctx.ctx.DrawImage(float64(px.Min.X)/physDPMM, float64(px.Min.Y)/physDPMM, scaled, cv.DPMM(physDPMM))
}

// --- gridNode ---

type gridNode struct {
	columns  []GridTrack
	rows     []GridTrack
	rowGap   float64
	colGap   float64
	padding  float64
	children []node

	// computed during measure / finalized in layout
	numCols    int
	numRows    int
	colWidths  []float64
	rowHeights []float64
	cellSizes  []nodeSize // len == numRows*numCols; zero for empty slots
	bounds     nodeRect

	mctx *measureCtx // retained from measure for deferred re-measure of children in Fr rows
	// frRowsAuto is set when measured with unbounded height; Fr rows then size
	// to content like Auto rows.
	frRowsAuto bool
}

func (n *gridNode) measure(mctx *measureCtx) nodeSize {
	if len(n.columns) == 0 {
		return nodeSize{}
	}
	n.mctx = mctx
	n.numCols = len(n.columns)
	numChildren := len(n.children)
	// Explicit row definitions fix the row count; children beyond the last
	// row are dropped.
	n.numRows = (numChildren + n.numCols - 1) / n.numCols
	if len(n.rows) > 0 {
		n.numRows = len(n.rows)
	}
	if n.numRows == 0 {
		return nodeSize{}
	}

	innerW := math.Max(0, mctx.availW-2*n.padding)
	innerH := math.Max(0, mctx.availH-2*n.padding)
	n.frRowsAuto = isUnbounded(mctx.availH)
	totalColGaps := math.Max(0, float64(n.numCols-1)) * n.colGap
	availForCols := math.Max(0, innerW-totalColGaps)

	// Step 1: estimate column widths for the first measure pass.
	roughColWidths := n.computeColWidths(availForCols, nil)

	// Step 2: measure all children with the estimated widths.
	total := n.numRows * n.numCols
	n.cellSizes = make([]nodeSize, total)
	for i, child := range n.children {
		if i >= total {
			break
		}
		col := i % n.numCols
		n.cellSizes[i] = child.measure(mctx.with(roughColWidths[col], unbounded))
	}

	// Step 3: size auto columns from measured content.
	n.colWidths = n.computeColWidths(availForCols, n.cellSizes)

	// Step 4: re-measure children whose column width changed so row heights
	// reflect re-wrapped content.
	for i, child := range n.children {
		if i >= total {
			break
		}
		col := i % n.numCols
		if math.Abs(n.colWidths[col]-roughColWidths[col]) > 0.5 {
			n.cellSizes[i] = child.measure(mctx.with(n.colWidths[col], unbounded))
		}
	}

	// Step 5: compute row heights. Fr rows are recomputed in layout with the
	// actual bounds.
	totalRowGaps := math.Max(0, float64(n.numRows-1)) * n.rowGap
	availForRows := math.Max(0, innerH-totalRowGaps)
	n.rowHeights = n.computeRowHeights(availForRows, n.cellSizes)

	totalW := totalColGaps
	for _, w := range n.colWidths {
		totalW += w
	}
	totalH := totalRowGaps
	for _, h := range n.rowHeights {
		totalH += h
	}
	return nodeSize{w: totalW + 2*n.padding, h: totalH + 2*n.padding}
}

func (n *gridNode) layout(bounds nodeRect) {
	n.bounds = bounds
	innerW := math.Max(0, bounds.w-2*n.padding)
	innerH := math.Max(0, bounds.h-2*n.padding)
	totalColGaps := math.Max(0, float64(n.numCols-1)) * n.colGap
	totalRowGaps := math.Max(0, float64(n.numRows-1)) * n.rowGap
	availForCols := math.Max(0, innerW-totalColGaps)
	availForRows := math.Max(0, innerH-totalRowGaps)

	// The parent may assign a different width than measure was given.
	n.colWidths = n.computeColWidths(availForCols, n.cellSizes)

	// Distribute Fr rows using the actual available height.
	n.rowHeights = n.finalizeRowHeights(availForRows, n.cellSizes)

	total := n.numRows * n.numCols

	// Re-measure children in Fr rows with their definite height.
	for row := range n.numRows {
		rowTrack := n.rowTrack(row)
		if rowTrack.kind != gridTrackFr {
			continue
		}
		for col := range n.numCols {
			idx := row*n.numCols + col
			if idx >= len(n.children) || idx >= total {
				break
			}
			n.cellSizes[idx] = n.children[idx].measure(n.mctx.with(n.colWidths[col], n.rowHeights[row]))
		}
	}

	colOffsets := make([]float64, n.numCols)
	x := 0.0
	for col := range n.numCols {
		colOffsets[col] = x
		x += n.colWidths[col] + n.colGap
	}
	rowOffsets := make([]float64, n.numRows)
	y := 0.0
	for row := range n.numRows {
		rowOffsets[row] = y
		y += n.rowHeights[row] + n.rowGap
	}

	originX := bounds.x + n.padding
	originY := bounds.y + n.padding
	for i, child := range n.children {
		if i >= total {
			break
		}
		col := i % n.numCols
		row := i / n.numCols
		if row >= n.numRows {
			break
		}
		child.layout(nodeRect{
			x: originX + colOffsets[col],
			y: originY + rowOffsets[row],
			w: n.colWidths[col],
			h: n.rowHeights[row],
		})
	}
}

func (n *gridNode) draw(dctx *drawCtx) {
	for _, child := range n.children {
		child.draw(dctx)
	}
}

// rowTrack returns the GridTrack for the given row, defaulting to gridTrackAuto.
// Fr rows are reported as Auto when the grid's height is unbounded.
func (n *gridNode) rowTrack(row int) GridTrack {
	if row < len(n.rows) && (n.rows[row].kind != gridTrackFr || !n.frRowsAuto) {
		return n.rows[row]
	}
	return GridTrack{kind: gridTrackAuto}
}

// computeColWidths calculates column widths for the given available width.
// Auto columns use the widest cell in cellSizes; if cellSizes is nil, they get
// an even share of the space left after fixed columns.
func (n *gridNode) computeColWidths(availForCols float64, cellSizes []nodeSize) []float64 {
	widths := make([]float64, n.numCols)

	var fixedW float64
	var nonFixedCount int
	var totalFrWeight float64
	for _, col := range n.columns {
		switch col.kind {
		case gridTrackFixed:
			fixedW += col.size
		case gridTrackAuto, gridTrackFr:
			nonFixedCount++
			if col.kind == gridTrackFr {
				totalFrWeight += col.weight
			}
		}
	}

	for c, col := range n.columns {
		if col.kind == gridTrackFixed {
			widths[c] = col.size
		}
	}

	var autoFixedW float64 // total width of auto columns
	for c, col := range n.columns {
		if col.kind != gridTrackAuto {
			continue
		}
		if cellSizes == nil {
			remaining := math.Max(0, availForCols-fixedW)
			if nonFixedCount > 0 {
				widths[c] = remaining / float64(nonFixedCount)
			}
		} else {
			var maxW float64
			for row := range n.numRows {
				idx := row*n.numCols + c
				if idx < len(cellSizes) {
					if w := cellSizes[idx].w; w > maxW {
						maxW = w
					}
				}
			}
			widths[c] = maxW
		}
		autoFixedW += widths[c]
	}
	fixedTotal := fixedW + autoFixedW

	// Fr columns share the remaining width.
	if totalFrWeight > 0 {
		remaining := math.Max(0, availForCols-fixedTotal)
		for c, col := range n.columns {
			if col.kind == gridTrackFr {
				widths[c] = remaining * col.weight / totalFrWeight
			}
		}
	}
	return widths
}

// computeRowHeights calculates row heights from measured cell sizes. Fr rows
// share the height left after fixed and auto rows.
func (n *gridNode) computeRowHeights(availForRows float64, cellSizes []nodeSize) []float64 {
	heights := make([]float64, n.numRows)
	var fixedAutoH float64
	var totalFrWeight float64
	for row := range n.numRows {
		track := n.rowTrack(row)
		switch track.kind {
		case gridTrackFixed:
			heights[row] = track.size
			fixedAutoH += track.size
		case gridTrackAuto:
			var maxH float64
			for col := range n.numCols {
				idx := row*n.numCols + col
				if idx < len(cellSizes) {
					if h := cellSizes[idx].h; h > maxH {
						maxH = h
					}
				}
			}
			heights[row] = maxH
			fixedAutoH += maxH
		case gridTrackFr:
			totalFrWeight += track.weight
		}
	}
	if totalFrWeight > 0 {
		remaining := math.Max(0, availForRows-fixedAutoH)
		for row := range n.numRows {
			if n.rowTrack(row).kind == gridTrackFr {
				heights[row] = remaining * n.rowTrack(row).weight / totalFrWeight
			}
		}
	}
	return heights
}

// finalizeRowHeights computes row heights for the final layout bounds.
func (n *gridNode) finalizeRowHeights(availForRows float64, cellSizes []nodeSize) []float64 {
	return n.computeRowHeights(availForRows, cellSizes)
}

// --- Build bridge: exported Node tree → internal node tree ---

// buildNode converts an exported Node into the internal node used by the
// measure/layout/draw pipeline. parentDir is the layout direction of the
// enclosing container, used to infer DividerNode orientation. depth is the
// nesting depth of n, bounded by maxNodeDepth.
func buildNode(n Node, cfg *Config, parentDir layoutDir, depth int) (node, error) {
	if depth > maxNodeDepth {
		return nil, fmt.Errorf("demy: node tree nested deeper than %d", maxNodeDepth)
	}
	if isNilNode(n) {
		return &spacerNode{weight: 0}, nil
	}
	switch v := n.(type) {
	case *VStackNode:
		if err := checkDims("VStack", dim{"gap", v.Gap}, dim{"padding", v.Padding}); err != nil {
			return nil, err
		}
		children, err := buildNodes(v.Children, cfg, dirVertical, depth+1)
		if err != nil {
			return nil, err
		}
		return &vstackNode{
			gap:      v.Gap,
			padding:  v.Padding,
			align:    v.Align,
			children: children,
		}, nil
	case *HStackNode:
		if err := checkDims("HStack", dim{"gap", v.Gap}, dim{"padding", v.Padding}); err != nil {
			return nil, err
		}
		children, err := buildNodes(v.Children, cfg, dirHorizontal, depth+1)
		if err != nil {
			return nil, err
		}
		return &hstackNode{
			gap:      v.Gap,
			padding:  v.Padding,
			align:    v.Align,
			children: children,
		}, nil
	case *ZStackNode:
		children, err := buildNodes(v.Children, cfg, dirVertical, depth+1)
		if err != nil {
			return nil, err
		}
		return &zstackNode{align: v.Align, children: children}, nil
	case *BoxNode:
		if err := checkDims("Box",
			dim{"padding", v.Padding},
			dim{"border width", v.BorderWidth},
			dim{"corner radius", v.CornerRadius},
		); err != nil {
			return nil, err
		}
		children, err := buildNodes(v.Children, cfg, dirVertical, depth+1)
		if err != nil {
			return nil, err
		}
		bc := v.BorderColor
		bg := v.Background
		if cfg != nil {
			bc = resolveColor(bc, cfg.Roles)
			bg = resolveColor(bg, cfg.Roles)
		}
		return &boxNode{
			padding:      v.Padding,
			borderWidth:  v.BorderWidth,
			borderColor:  bc,
			background:   bg,
			cornerRadius: v.CornerRadius,
			clip:         v.Clip,
			children:     children,
		}, nil
	case *FrameNode:
		if err := checkDims("Frame", dim{"width", v.Width}, dim{"height", v.Height}); err != nil {
			return nil, err
		}
		children, err := buildNodes(v.Children, cfg, dirVertical, depth+1)
		if err != nil {
			return nil, err
		}
		return &frameNode{w: v.Width, h: v.Height, children: children}, nil
	case *SpacerNode:
		if err := checkDims("Spacer", dim{"weight", v.Weight}); err != nil {
			return nil, err
		}
		w := v.Weight
		if w == 0 {
			w = 1
		}
		return &spacerNode{weight: w}, nil
	case *WeightedNode:
		if err := checkDims("Weighted", dim{"weight", v.Weight}); err != nil {
			return nil, err
		}
		w := v.Weight
		if w == 0 {
			w = 1
		}
		var child node
		if v.Child != nil {
			var err error
			child, err = buildNode(v.Child, cfg, parentDir, depth+1)
			if err != nil {
				return nil, err
			}
		} else {
			child = &spacerNode{}
		}
		return &weightedNode{weight: w, child: child}, nil
	case *DividerNode:
		if err := checkDims("Divider", dim{"size", v.Size}); err != nil {
			return nil, err
		}
		clr := v.Color
		if cfg != nil {
			clr = resolveColor(clr, cfg.Roles)
		}
		return &dividerNode{isHorizontal: parentDir == dirVertical, color: clr, size: v.Size}, nil
	case *TextNode:
		var style Style
		if cfg != nil {
			style = cfg.DefaultTextStyle
		}
		if v.Class != "" && cfg != nil {
			classStyle, ok := cfg.Styles[string(v.Class)]
			if !ok {
				return nil, fmt.Errorf("demy: style class %q not found in Config.Styles", v.Class)
			}
			style = mergeStyle(style, classStyle)
		}
		style = mergeStyle(style, v.Style)
		if err := checkDims("Text", dim{"size", style.Size}, dim{"line height", style.LineHeight}); err != nil {
			return nil, err
		}
		switch {
		case style.MaxLines == MaxLinesUnlimited:
			style.MaxLines = 0
		case style.MaxLines < 0:
			return nil, fmt.Errorf("demy: Text max lines %d must not be negative", style.MaxLines)
		}
		if style.Font != "" && cfg != nil && cfg.Fonts[style.Font] == nil {
			return nil, fmt.Errorf("demy: font %q not found in Config.Fonts", style.Font)
		}
		if cfg != nil {
			style.Color = resolveColor(style.Color, cfg.Roles)
		}
		return &textNode{text: v.Text, style: style}, nil
	case *ImageNode:
		img := v.Image
		if img == nil && v.Key != "" {
			if cfg != nil && cfg.Images != nil {
				img = cfg.Images[v.Key]
			}
			if img == nil {
				return nil, imageKeyError(v.Key)
			}
		}
		return &imageNode{img: img, scale: v.Scale, dither: v.Dither}, nil
	case *BarcodeNode:
		bc, err := encodeBarcode(v)
		if err != nil {
			return nil, err
		}
		return &barcodeNode{code: bc}, nil
	case *GridNode:
		if err := checkDims("Grid",
			dim{"row gap", v.RowGap},
			dim{"column gap", v.ColGap},
			dim{"padding", v.Padding},
		); err != nil {
			return nil, err
		}
		if err := checkTracks("column", v.Columns); err != nil {
			return nil, err
		}
		if err := checkTracks("row", v.Rows); err != nil {
			return nil, err
		}
		flat := make([]Node, 0, len(v.Children))
		for _, child := range v.Children {
			if row, ok := child.(*GridRowNode); ok && row != nil {
				flat = append(flat, row.Children...)
			} else {
				flat = append(flat, child)
			}
		}
		children, err := buildNodes(flat, cfg, dirVertical, depth+1)
		if err != nil {
			return nil, err
		}
		return &gridNode{
			columns:  v.Columns,
			rows:     v.Rows,
			rowGap:   v.RowGap,
			colGap:   v.ColGap,
			padding:  v.Padding,
			children: children,
		}, nil
	case *GridRowNode:
		return nil, errors.New("demy: GridRow must be a direct child of Grid")
	default:
		return nil, fmt.Errorf("demy: unknown node type %T", n)
	}
}

// maxDimension bounds every size, gap, and weight in a node tree so untrusted
// scenes can't request unbounded allocations or overflow layout math.
const maxDimension = 1 << 15

// maxNodeDepth bounds node tree nesting so recursion in measure, layout, and
// draw can't exhaust the stack.
const maxNodeDepth = 1000

type dim struct {
	name string
	v    float64
}

// checkDims reports an error if any value is NaN, infinite, negative, or
// larger than maxDimension.
func checkDims(kind string, dims ...dim) error {
	for _, d := range dims {
		if math.IsNaN(d.v) || d.v < 0 || d.v > maxDimension {
			return fmt.Errorf("demy: %s %s %v out of range [0, %d]", kind, d.name, d.v, maxDimension)
		}
	}
	return nil
}

func checkTracks(kind string, tracks []GridTrack) error {
	for _, t := range tracks {
		if err := checkDims("Grid", dim{kind + " size", t.size}, dim{kind + " weight", t.weight}); err != nil {
			return err
		}
	}
	return nil
}

// isNilNode reports whether n is nil or a typed nil pointer.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// buildNodes converts a slice of exported Nodes, skipping nil entries.
func buildNodes(nodes []Node, cfg *Config, parentDir layoutDir, depth int) ([]node, error) {
	result := make([]node, 0, len(nodes))
	for _, n := range nodes {
		if isNilNode(n) {
			continue
		}
		built, err := buildNode(n, cfg, parentDir, depth)
		if err != nil {
			return nil, err
		}
		result = append(result, built)
	}
	return result, nil
}

// imageKeyError reports an ImageNode key missing from Config.Images.
type imageKeyError string

func (e imageKeyError) Error() string {
	return "demy: image key " + string(e) + " not found in Config.Images"
}
