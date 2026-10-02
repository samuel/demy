package demy

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
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

// renderState is shared by every pass of one render.
type renderState struct {
	ctx context.Context
	err error // the context's error once it's done
}

// stopped reports whether the render's context is done. A nil renderState
// never stops.
func (s *renderState) stopped() bool {
	if s == nil {
		return false
	}
	if s.err == nil {
		s.err = s.ctx.Err()
	}
	return s.err != nil
}

// stopErr returns the error for a stopped render.
func (s *renderState) stopErr() error {
	return fmt.Errorf("demy: render stopped: %w", s.err)
}

// measureCtx carries context for the measure pass.
type measureCtx struct {
	rs     *renderState
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

// with returns a copy of mctx with the given available size. Unbounded sizes
// are normalized so they compare equal in measureMemo.
func (mctx *measureCtx) with(availW, availH float64) *measureCtx {
	if isUnbounded(availW) {
		availW = unbounded
	}
	if isUnbounded(availH) {
		availH = unbounded
	}
	c := *mctx
	c.availW = availW
	c.availH = availH
	return &c
}

// drawCtx carries context for the draw pass.
type drawCtx struct {
	rs        *renderState
	p         *painter
	dpmm      float64 // logical DPMM (physical DPMM / scale)
	scale     float64
	roles     Roles
	fonts     map[string]*FontFamily
	quant     *quantizer // output palette for per-image dithering; nil when not quantizing
	textBlits []textBlit
	clip      image.Rectangle // physical-pixel clip for bitmap text and clipped sub-canvases
	err       error           // first error encountered while drawing
}

// setErr records err if no earlier draw error has been recorded.
func (dctx *drawCtx) setErr(err error) {
	if dctx.err == nil {
		dctx.err = err
	}
}

// memoSize is the number of measurements measureMemo keeps per node.
const memoSize = 4

// measureMemo remembers a node's recent measurements along with the
// measure-time state its layout reads (S). Measuring again at a remembered
// available size returns the remembered result and restores that state.
// Restoring a node doesn't restore its descendants, so containers re-measure
// each child at its recorded size before laying it out (childSizes.layout).
type measureMemo[S any] struct {
	entries [memoSize]memoEntry[S]
	n       int // valid entries
	next    int // entry to replace next
}

type memoEntry[S any] struct {
	availW, availH float64
	size           nodeSize
	state          S
}

// do returns the node's size at mctx's available size, calling measure on a
// miss. measure must store fresh values in *state rather than modifying the
// remembered ones in place.
func (m *measureMemo[S]) do(mctx *measureCtx, state *S, measure func(*measureCtx) nodeSize) nodeSize {
	for i := range m.n {
		if e := &m.entries[i]; e.availW == mctx.availW && e.availH == mctx.availH {
			*state = e.state
			return e.size
		}
	}
	size := measure(mctx)
	m.entries[m.next] = memoEntry[S]{availW: mctx.availW, availH: mctx.availH, size: size, state: *state}
	m.next = (m.next + 1) % memoSize
	m.n = min(m.n+1, memoSize)
	return size
}

// childSizes records how a container measured its children: the available
// size each child was last measured with and the result.
type childSizes struct {
	mctx  *measureCtx
	avail []nodeSize
	sizes []nodeSize
}

func newChildSizes(mctx *measureCtx, n int) childSizes {
	return childSizes{mctx: mctx, avail: make([]nodeSize, n), sizes: make([]nodeSize, n)}
}

// measure measures child i at the given available size and records the
// result. Once the render is stopped, children are no longer measured.
func (c *childSizes) measure(i int, child node, availW, availH float64) nodeSize {
	if c.mctx.rs.stopped() {
		return nodeSize{}
	}
	mctx := c.mctx.with(availW, availH)
	c.avail[i] = nodeSize{w: mctx.availW, h: mctx.availH}
	c.sizes[i] = child.measure(mctx)
	return c.sizes[i]
}

// layout lays out child i. It first re-measures the child at its recorded size,
// which hits the child's memo and restores the state from that measurement.
// The child may have been measured at other sizes since.
func (c *childSizes) layout(i int, child node, bounds nodeRect) {
	if c.mctx.rs.stopped() {
		return
	}
	child.measure(c.mctx.with(c.avail[i].w, c.avail[i].h))
	child.layout(bounds)
}

// clone returns a copy that can be modified without changing remembered state.
func (c childSizes) clone() childSizes {
	return childSizes{mctx: c.mctx, avail: slices.Clone(c.avail), sizes: slices.Clone(c.sizes)}
}

// node is the interface all layout and widget nodes implement.
type node interface {
	measure(mctx *measureCtx) nodeSize
	layout(bounds nodeRect)
	draw(dctx *drawCtx)
}

// --- vstackNode ---

type vstackNode struct {
	memo     measureMemo[childSizes]
	m        childSizes
	gap      float64
	padding  float64
	align    Alignment
	children []node
	bounds   nodeRect
}

func (n *vstackNode) measure(mctx *measureCtx) nodeSize {
	return n.memo.do(mctx, &n.m, n.measureUncached)
}

func (n *vstackNode) measureUncached(mctx *measureCtx) nodeSize {
	innerW := math.Max(0, mctx.availW-2*n.padding)
	n.m = newChildSizes(mctx, len(n.children))
	var totalH, maxW float64
	for i, child := range n.children {
		s := n.m.measure(i, child, innerW, unbounded)
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
			fixedH += n.m.sizes[i].h
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
			childH = n.m.sizes[i].h
			childW = n.m.sizes[i].w
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
		n.m.layout(i, child, nodeRect{x: childX, y: y, w: childLayoutW, h: childH})
		y += childH
	}
}

func (n *vstackNode) draw(dctx *drawCtx) {
	drawChildren(dctx, n.children)
}

// --- hstackNode ---

type hstackNode struct {
	memo     measureMemo[childSizes]
	m        childSizes
	gap      float64
	padding  float64
	align    Alignment
	children []node
	bounds   nodeRect
}

func (n *hstackNode) measure(mctx *measureCtx) nodeSize {
	return n.memo.do(mctx, &n.m, n.measureUncached)
}

// measureUncached measures children at their natural width first. That
// measurement doesn't depend on the stack's own size, so it is shared by every
// size the stack is measured at, which keeps nested stacks from measuring their
// descendants at exponentially many sizes.
func (n *hstackNode) measureUncached(mctx *measureCtx) nodeSize {
	innerH := math.Max(0, mctx.availH-2*n.padding)
	innerW := math.Max(0, mctx.availW-2*n.padding)
	n.m = newChildSizes(mctx, len(n.children))

	// Pass 1: measure children other than weighted ones at their natural width.
	for i, child := range n.children {
		if _, ok := child.(*weightedNode); !ok {
			n.m.measure(i, child, unbounded, innerH)
		}
	}

	// Pass 2: limit variable-width children to the stack's width and, if they
	// overflow the space left after gaps and frameNodes, shrink them
	// proportionally. Re-measure the ones whose width changed so heights are
	// correct.
	if !isUnbounded(innerW) {
		totalGaps := math.Max(0, float64(len(n.children)-1)) * n.gap
		var fixedSizeW, variableSizeW float64
		for i, child := range n.children {
			switch child.(type) {
			case *spacerNode, *weightedNode:
			case *frameNode:
				fixedSizeW += n.m.sizes[i].w
			default:
				variableSizeW += math.Min(n.m.sizes[i].w, innerW)
			}
		}
		shrinkScale := 1.0
		if availForVar := innerW - totalGaps - fixedSizeW; variableSizeW > availForVar && variableSizeW > 0 {
			shrinkScale = math.Max(0, availForVar) / variableSizeW
		}
		for i, child := range n.children {
			switch child.(type) {
			case *spacerNode, *weightedNode, *frameNode:
			default:
				if w := math.Min(n.m.sizes[i].w, innerW) * shrinkScale; w != n.m.sizes[i].w {
					n.m.measure(i, child, w, innerH)
				}
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
			usedW += n.m.sizes[i].w
		}
	}
	if totalWeight > 0 {
		totalGaps := math.Max(0, float64(len(n.children)-1)) * n.gap
		unitW := math.Max(0, innerW-totalGaps-usedW) / totalWeight
		for i, child := range n.children {
			if v, ok := child.(*weightedNode); ok {
				n.m.measure(i, child, unitW*v.weight, innerH)
			}
		}
	}

	var totalW, maxH float64
	for i, child := range n.children {
		s := n.m.sizes[i]
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
			fixedW += n.m.sizes[i].w
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
			childW = n.m.sizes[i].w
			fillCrossAxis = true
		default:
			childW = n.m.sizes[i].w
			childH = math.Min(n.m.sizes[i].h, innerH)
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
		n.m.layout(i, child, nodeRect{x: x, y: childY, w: childW, h: childH})
		x += childW
	}
}

func (n *hstackNode) draw(dctx *drawCtx) {
	drawChildren(dctx, n.children)
}

// --- zstackNode ---

type zstackNode struct {
	memo     measureMemo[childSizes]
	m        childSizes
	align    ZAlignment
	children []node
	bounds   nodeRect
}

func (n *zstackNode) measure(mctx *measureCtx) nodeSize {
	return n.memo.do(mctx, &n.m, n.measureUncached)
}

func (n *zstackNode) measureUncached(mctx *measureCtx) nodeSize {
	n.m = newChildSizes(mctx, len(n.children))
	var maxW, maxH float64
	for i, child := range n.children {
		s := n.m.measure(i, child, mctx.availW, mctx.availH)
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
		s := n.m.sizes[i]
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
		n.m.layout(i, child, nodeRect{x: cx, y: cy, w: s.w, h: s.h})
	}
}

func (n *zstackNode) draw(dctx *drawCtx) {
	drawChildren(dctx, n.children)
}

// --- boxNode ---

type boxNode struct {
	memo         measureMemo[childSizes]
	m            childSizes
	padding      float64
	borderWidth  float64
	borderColor  color.Color
	background   color.Color
	cornerRadius float64
	clip         bool
	children     []node
	bounds       nodeRect
}

// inset returns the distance from the box edge to its content.
func (n *boxNode) inset() float64 {
	if n.borderColor == nil {
		return n.padding
	}
	return n.padding + n.borderWidth
}

func (n *boxNode) measure(mctx *measureCtx) nodeSize {
	return n.memo.do(mctx, &n.m, n.measureUncached)
}

func (n *boxNode) measureUncached(mctx *measureCtx) nodeSize {
	innerW := math.Max(0, mctx.availW-2*n.inset())
	innerH := math.Max(0, mctx.availH-2*n.inset())
	n.m = newChildSizes(mctx, len(n.children))
	var maxW, maxH float64
	for i, child := range n.children {
		s := n.m.measure(i, child, innerW, innerH)
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
	for i, child := range n.children {
		n.m.layout(i, child, nodeRect{x: innerX, y: innerY, w: innerW, h: innerH})
	}
}

func (n *boxNode) draw(dctx *drawCtx) {
	s := dctx.scale
	x, y, w, h, r := n.bounds.x*s, n.bounds.y*s, n.bounds.w*s, n.bounds.h*s, n.cornerRadius*s
	if n.background != nil {
		dctx.p.fillRect(x, y, w, h, r, n.background)
	}
	if n.borderWidth > 0 && n.borderColor != nil {
		dctx.p.strokeRect(x, y, w, h, r, n.borderWidth*s, n.borderColor)
	}

	if n.clip {
		drawClipped(dctx, n.bounds, n.children)
		return
	}
	drawChildren(dctx, n.children)
}

// --- frameNode ---

type frameNode struct {
	w, h     float64
	m        childSizes
	children []node
	bounds   nodeRect
}

func (n *frameNode) measure(mctx *measureCtx) nodeSize {
	n.m = newChildSizes(mctx, len(n.children))
	for i, child := range n.children {
		n.m.measure(i, child, n.w, n.h)
	}
	return nodeSize{w: n.w, h: n.h}
}

func (n *frameNode) layout(bounds nodeRect) {
	n.bounds = bounds
	for i, child := range n.children {
		n.m.layout(i, child, nodeRect{x: bounds.x, y: bounds.y, w: n.w, h: n.h})
	}
}

func (n *frameNode) draw(dctx *drawCtx) {
	drawClipped(dctx, nodeRect{x: n.bounds.x, y: n.bounds.y, w: n.w, h: n.h}, n.children)
}

// drawChildren draws each child until the render is stopped.
func drawChildren(dctx *drawCtx, children []node) {
	for _, child := range children {
		if dctx.rs.stopped() {
			return
		}
		child.draw(dctx)
	}
}

// drawClipped draws children with a painter limited to r, rounded to whole
// physical pixels, confining their content to r.
func drawClipped(dctx *drawCtx, r nodeRect, children []node) {
	// Bitmap glyphs apply to the root image after quantization, so they are
	// clipped to the same physical-pixel bounds.
	clip := image.Rect(
		int(math.Round(r.x*dctx.scale)),
		int(math.Round(r.y*dctx.scale)),
		int(math.Round((r.x+r.w)*dctx.scale)),
		int(math.Round((r.y+r.h)*dctx.scale)),
	).Intersect(dctx.clip)
	if clip.Empty() {
		return
	}

	subDctx := &drawCtx{
		rs:    dctx.rs,
		p:     dctx.p.sub(clip),
		dpmm:  dctx.dpmm,
		scale: dctx.scale,
		roles: dctx.roles,
		fonts: dctx.fonts,
		quant: dctx.quant,
		clip:  clip,
	}
	drawChildren(subDctx, children)
	if subDctx.err != nil {
		dctx.setErr(subDctx.err)
	}
	dctx.textBlits = append(dctx.textBlits, subDctx.textBlits...)
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
	var xPx, yPx float64
	if n.isHorizontal {
		xPx = n.bounds.x
		yPx = math.Round(n.bounds.y)
	} else {
		xPx = math.Round(n.bounds.x)
		yPx = n.bounds.y
	}
	s := dctx.scale
	dctx.p.fillRect(xPx*s, yPx*s, n.bounds.w*s, n.bounds.h*s, 0, col)
}

// --- textNode ---

type textNode struct {
	memo   measureMemo[textLayout]
	lay    textLayout
	text   string
	style  Style // Color is always set
	bounds nodeRect
}

func (n *textNode) measure(mctx *measureCtx) nodeSize {
	return n.memo.do(mctx, &n.lay, n.measureUncached)
}

// measureUncached measures the text. buildNode guarantees a loaded font and,
// for vector fonts, a non-zero size.
func (n *textNode) measureUncached(mctx *measureCtx) nodeSize {
	ff := mctx.fonts[n.style.Font]
	if ff.isBitmap() {
		n.lay = textLayout{}
		return n.measureBitmap(mctx, ff)
	}
	// Points to logical pixels.
	size := n.style.Size * mctx.dpmm * 25.4 / 72
	n.lay = ff.layoutText(n.text, n.style, size, mctx.availW)
	return nodeSize{w: n.lay.width, h: n.lay.height}
}

func (n *textNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *textNode) draw(dctx *drawCtx) {
	ff := dctx.fonts[n.style.Font]
	if ff.isBitmap() {
		n.drawBitmap(dctx, ff)
		return
	}
	ff.drawText(dctx.p, &n.lay, n.style, n.bounds.x, n.bounds.y, n.bounds.w, dctx.scale)
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
	col := opaqueRGBA(n.style.Color)

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

// opaqueRGBA returns c without its alpha. Bitmap text is drawn without
// blending, and Render snaps it to the palette when quantizing.
func opaqueRGBA(c color.Color) color.RGBA {
	n := color.NRGBAModel.Convert(c).(color.NRGBA) //nolint:forcetypeassert // NRGBAModel returns color.NRGBA
	return color.RGBA{R: n.R, G: n.G, B: n.B, A: 0xff}
}

// --- imageNode ---

type imageNode struct {
	img    image.Image
	scale  ImageScale
	dither Dither
	bounds nodeRect
}

// measure sizes the image. buildNode guarantees a non-empty image.
func (n *imageNode) measure(mctx *measureCtx) nodeSize {
	iw := float64(n.img.Bounds().Dx())
	ih := float64(n.img.Bounds().Dy())
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
	if n.bounds.w == 0 || n.bounds.h == 0 {
		return
	}
	sc := dctx.scale
	x, y, w, h := n.bounds.x*sc, n.bounds.y*sc, n.bounds.w*sc, n.bounds.h*sc
	img := n.img
	ib := img.Bounds()
	iw, ih := float64(ib.Dx()), float64(ib.Dy())
	xres, yres := iw/w, ih/h // source pixels per destination pixel
	switch n.scale {
	case ImageFill:
		// Crop the source to the destination's aspect ratio, centered.
		crop := ib
		if xres < yres {
			dy := int((ih-h*xres)/2 + 0.5)
			crop.Min.Y += dy
			crop.Max.Y -= dy
		} else {
			dx := int((iw-w*yres)/2 + 0.5)
			crop.Min.X += dx
			crop.Max.X -= dx
		}
		si, ok := img.(subImager)
		if !ok {
			si = imageView{img, ib}
		}
		img = si.SubImage(crop)
	case ImageStretch:
	default: // ImageFit
		// Letterbox within the bounds, centered.
		if xres < yres {
			dx := (w - iw/yres) / 2
			x += dx
			w -= 2 * dx
		} else {
			dy := (h - ih/xres) / 2
			y += dy
			h -= 2 * dy
		}
	}
	if n.dither == NoDithering || dctx.quant == nil {
		dctx.p.drawImage(img, x, y, w, h)
		return
	}
	n.drawDithered(dctx, img, x, y, w, h)
}

type subImager interface {
	SubImage(image.Rectangle) image.Image
}

// imageView adds SubImage to an image that lacks it. A sub-image is a view of
// the same pixels, so nothing is copied.
type imageView struct {
	image.Image
	r image.Rectangle
}

func (v imageView) Bounds() image.Rectangle { return v.r }

func (v imageView) SubImage(r image.Rectangle) image.Image {
	return imageView{v.Image, r.Intersect(v.r)}
}

// drawDithered draws the image alone onto a scratch image on whole physical
// pixels, dithers it to the output palette, and draws the result 1:1.
// Dithering in the draw pass leaves content drawn over the image untouched.
func (n *imageNode) drawDithered(dctx *drawCtx, img image.Image, x, y, w, h float64) {
	// Limiting the scratch image to the visible region bounds its allocation
	// by the root canvas regardless of the node's own size.
	px := image.Rect(
		int(math.Floor(x)),
		int(math.Floor(y)),
		int(math.Ceil(x+w)),
		int(math.Ceil(y+h)),
	).Intersect(dctx.clip)
	if px.Empty() {
		return
	}
	scratch := image.NewRGBA(px)
	newPainter(scratch, dctx.rs).drawImage(img, x, y, w, h)

	// Dither only the area the image covers, not letterboxing around it.
	area := image.Rect(
		int(math.Round(x)),
		int(math.Round(y)),
		int(math.Round(x+w)),
		int(math.Round(y+h)),
	).Intersect(px)
	dctx.quant.ditherRGBA(scratch, area, n.dither)
	dctx.p.drawImage(scratch, float64(px.Min.X), float64(px.Min.Y), float64(px.Dx()), float64(px.Dy()))
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
	// A barcode that can't be drawn fails the render rather than being
	// skipped, since a missing code breaks the display as much as a bad one.
	if px.Empty() {
		dctx.setErr(fmt.Errorf("demy: barcode has no space (%dx%d pixels)", px.Dx(), px.Dy()))
		return
	}
	scaled, err := barcode.Scale(n.code, px.Dx(), px.Dy())
	if err != nil {
		dctx.setErr(fmt.Errorf("demy: barcode does not fit %dx%d pixels: %w", px.Dx(), px.Dy(), err))
		return
	}
	sb := scaled.Bounds()
	dctx.p.drawImage(scaled, float64(px.Min.X), float64(px.Min.Y), float64(sb.Dx()), float64(sb.Dy()))
}

// --- gridNode ---

type gridNode struct {
	memo     measureMemo[gridMeasure]
	m        gridMeasure
	columns  []GridTrack
	rows     []GridTrack
	rowGap   float64
	colGap   float64
	padding  float64
	children []node

	numCols    int
	numRows    int
	colWidths  []float64 // computed in layout
	rowHeights []float64 // computed in layout
	bounds     nodeRect
}

// gridMeasure is the measure-time state a grid's layout reads.
type gridMeasure struct {
	cells childSizes // len == numRows*numCols; zero for empty slots
	// frRowsAuto is set when measured with unbounded height; Fr rows then size
	// to content like Auto rows.
	frRowsAuto bool
}

func (n *gridNode) measure(mctx *measureCtx) nodeSize {
	return n.memo.do(mctx, &n.m, n.measureUncached)
}

// measureUncached measures children in Auto columns at their natural width
// first, which, as in hstackNode, keeps the measurement shared by every size
// the grid is measured at.
func (n *gridNode) measureUncached(mctx *measureCtx) nodeSize {
	n.numCols = len(n.columns)
	// Explicit row definitions fix the row count; buildNode drops children
	// beyond the last cell.
	n.numRows = (len(n.children) + n.numCols - 1) / n.numCols
	if len(n.rows) > 0 {
		n.numRows = len(n.rows)
	}
	n.m = gridMeasure{cells: newChildSizes(mctx, n.numRows*n.numCols), frRowsAuto: isUnbounded(mctx.availH)}
	if n.numRows == 0 {
		return nodeSize{}
	}
	cells := &n.m.cells

	innerW := math.Max(0, mctx.availW-2*n.padding)
	innerH := math.Max(0, mctx.availH-2*n.padding)
	totalColGaps := math.Max(0, float64(n.numCols-1)) * n.colGap
	availForCols := math.Max(0, innerW-totalColGaps)

	// Step 1: measure children in Auto columns at their natural width.
	for i, child := range n.children {
		if n.columns[i%n.numCols].kind == gridTrackAuto {
			cells.measure(i, child, unbounded, unbounded)
		}
	}

	// Step 2: limit children in Auto columns to an even share of the width
	// left after Fixed columns, re-measuring the ones that are wider.
	shares := n.computeColWidths(availForCols, nil)
	for i, child := range n.children {
		col := i % n.numCols
		if n.columns[col].kind == gridTrackAuto && cells.sizes[i].w > shares[col] {
			cells.measure(i, child, shares[col], unbounded)
		}
	}

	// Step 3: size Auto columns to their content and measure the other
	// children at their column's width.
	colWidths := n.computeColWidths(availForCols, cells.sizes)
	for i, child := range n.children {
		col := i % n.numCols
		if n.columns[col].kind != gridTrackAuto {
			cells.measure(i, child, colWidths[col], unbounded)
		}
	}

	// Step 4: compute row heights. Fr rows are recomputed in layout with the
	// actual bounds.
	totalRowGaps := math.Max(0, float64(n.numRows-1)) * n.rowGap
	availForRows := math.Max(0, innerH-totalRowGaps)
	rowHeights := n.computeRowHeights(availForRows, cells.sizes)

	totalW := totalColGaps
	for _, w := range colWidths {
		totalW += w
	}
	totalH := totalRowGaps
	for _, h := range rowHeights {
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
	n.colWidths = n.computeColWidths(availForCols, n.m.cells.sizes)

	// Distribute Fr rows using the actual available height.
	n.rowHeights = n.computeRowHeights(availForRows, n.m.cells.sizes)

	// Re-measure children in Fr rows with their definite height. The cells are
	// copied first since the memo shares them.
	n.m.cells = n.m.cells.clone()
	for row := range n.numRows {
		if n.rowTrack(row).kind != gridTrackFr {
			continue
		}
		for col := range n.numCols {
			idx := row*n.numCols + col
			if idx >= len(n.children) {
				break
			}
			n.m.cells.measure(idx, n.children[idx], n.colWidths[col], n.rowHeights[row])
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
		col := i % n.numCols
		row := i / n.numCols
		n.m.cells.layout(i, child, nodeRect{
			x: originX + colOffsets[col],
			y: originY + rowOffsets[row],
			w: n.colWidths[col],
			h: n.rowHeights[row],
		})
	}
}

func (n *gridNode) draw(dctx *drawCtx) {
	drawChildren(dctx, n.children)
}

// rowTrack returns the GridTrack for the given row, defaulting to gridTrackAuto.
// Fr rows are reported as Auto when the grid's height is unbounded.
func (n *gridNode) rowTrack(row int) GridTrack {
	if row < len(n.rows) && (n.rows[row].kind != gridTrackFr || !n.m.frRowsAuto) {
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

// --- Build bridge: exported Node tree → internal node tree ---

// buildNode converts an exported Node tree into the internal nodes used by
// the measure/layout/draw pipeline.
func buildNode(n Node, cfg *Config, rs *renderState) (node, error) {
	b := &builder{cfg: cfg, rs: rs}
	return b.build(n, dirVertical, 0)
}

// builder converts a Node tree and counts what it builds. A Node used in
// several places is built, and counted, once per use.
type builder struct {
	cfg       *Config
	rs        *renderState
	nodes     int // all nodes, bounded by maxNodes
	clipped   int // Frames and Boxes with Clip, bounded by maxClippedNodes
	barcodes  int // bounded by maxBarcodes
	textBytes int // bounded by maxTreeTextBytes
}

// count adds one to *counter and reports an error past limit.
func count(counter *int, limit int, what string) error {
	*counter++
	if *counter > limit {
		return fmt.Errorf("demy: node tree has more than %d %s, counting each use of a shared node", limit, what)
	}
	return nil
}

// build converts n. parentDir is the layout direction of the enclosing
// container, used to infer DividerNode orientation. depth is the nesting depth
// of n, bounded by maxNodeDepth.
func (b *builder) build(n Node, parentDir layoutDir, depth int) (node, error) {
	if depth > maxNodeDepth {
		return nil, fmt.Errorf("demy: node tree nested deeper than %d", maxNodeDepth)
	}
	if err := count(&b.nodes, maxNodes, "nodes"); err != nil {
		return nil, err
	}
	if b.rs.stopped() {
		return nil, b.rs.stopErr()
	}
	if isNilNode(n) {
		return &spacerNode{weight: 0}, nil
	}
	cfg := b.cfg
	switch v := n.(type) {
	case *VStackNode:
		if err := checkDims("VStack", dim{"gap", v.Gap}, dim{"padding", v.Padding}); err != nil {
			return nil, err
		}
		children, err := b.buildChildren(v.Children, dirVertical, depth+1)
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
		children, err := b.buildChildren(v.Children, dirHorizontal, depth+1)
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
		children, err := b.buildChildren(v.Children, dirVertical, depth+1)
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
		if v.Clip {
			if err := count(&b.clipped, maxClippedNodes, "clipped Frames and Boxes"); err != nil {
				return nil, err
			}
		}
		children, err := b.buildChildren(v.Children, dirVertical, depth+1)
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
		if err := count(&b.clipped, maxClippedNodes, "clipped Frames and Boxes"); err != nil {
			return nil, err
		}
		children, err := b.buildChildren(v.Children, dirVertical, depth+1)
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
			child, err = b.build(v.Child, parentDir, depth+1)
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
		if err := checkTextFont(style, cfg); err != nil {
			return nil, err
		}
		if cfg != nil {
			style.Color = resolveColor(style.Color, cfg.Roles)
			if style.Color == nil {
				style.Color = cfg.Roles.Foreground
			}
		}
		if style.Color == nil {
			style.Color = Black
		}
		text, err := b.text(v.Text)
		if err != nil {
			return nil, err
		}
		return &textNode{text: text, style: style}, nil
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
		if err := checkImage(img); err != nil {
			return nil, err
		}
		if err := v.Dither.check(); err != nil {
			return nil, err
		}
		return &imageNode{img: img, scale: v.Scale, dither: v.Dither}, nil
	case *BarcodeNode:
		if err := count(&b.barcodes, maxBarcodes, "barcodes"); err != nil {
			return nil, err
		}
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
		if len(v.Columns) == 0 {
			return nil, errors.New("demy: Grid has no columns")
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
				if err := count(&b.nodes, maxNodes, "nodes"); err != nil {
					return nil, err
				}
				flat = append(flat, row.Children...)
			} else {
				flat = append(flat, child)
			}
		}
		children, err := b.buildChildren(flat, dirVertical, depth+1)
		if err != nil {
			return nil, err
		}
		// With explicit rows, children beyond the last cell get no cell and are
		// dropped, so measure, layout, and draw never see them.
		if len(v.Rows) > 0 {
			if cells := len(v.Rows) * len(v.Columns); len(children) > cells {
				children = children[:cells]
			}
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

// text sanitizes s and counts it against the text limits.
func (b *builder) text(s string) (string, error) {
	// Sanitizing only grows text, so longer input fails before it's copied.
	if len(s) <= maxTextBytes {
		s = sanitizeText(s)
	}
	if len(s) > maxTextBytes {
		return "", fmt.Errorf("demy: Text has %d bytes, more than %d", len(s), maxTextBytes)
	}
	b.textBytes += len(s)
	if b.textBytes > maxTreeTextBytes {
		return "", fmt.Errorf("demy: node tree has more than %d bytes of text, counting each use of a shared node", maxTreeTextBytes)
	}
	return s, nil
}

// sanitizeText replaces each invalid UTF-8 byte, and each control character
// other than tab, line feed, and carriage return, with U+FFFD. Vector text
// shaping panics on invalid UTF-8, and control characters would otherwise
// render differently in vector and bitmap fonts.
func sanitizeText(s string) string {
	replace := func(r rune) bool {
		return r == utf8.RuneError || unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r'
	}
	if !strings.ContainsFunc(s, replace) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s { // invalid bytes decode as utf8.RuneError, one per byte
		if replace(r) {
			r = utf8.RuneError
		}
		b.WriteRune(r)
	}
	return b.String()
}

// checkTextFont reports an error unless style names a loaded font in
// cfg.Fonts and, for a vector font, a non-zero size.
func checkTextFont(style Style, cfg *Config) error {
	if style.Font == "" {
		return errors.New("demy: Text has no font; set Style.Font or Config.DefaultTextStyle.Font")
	}
	var ff *FontFamily
	if cfg != nil {
		ff = cfg.Fonts[style.Font]
	}
	if ff == nil {
		return fmt.Errorf("demy: font %q not found in Config.Fonts", style.Font)
	}
	if !ff.isLoaded() {
		return fmt.Errorf("font %q: %w", style.Font, errFontNotLoaded)
	}
	if !ff.isBitmap() && style.Size == 0 {
		return fmt.Errorf("demy: Text in font %q has no size; set Style.Size or Config.DefaultTextStyle.Size", style.Font)
	}
	return nil
}

// maxImagePixels bounds the pixel count of an image. Resampling cost grows with
// the source size, so an image with huge bounds but no pixel data, such as
// image.Uniform, could otherwise stall a render.
const maxImagePixels = 1 << 26

// checkImage reports an error for a missing image, or one whose bounds are
// empty, wider or taller than maxPixelDimension, or larger than maxImagePixels.
func checkImage(img image.Image) error {
	if v := reflect.ValueOf(img); !v.IsValid() || v.Kind() == reflect.Pointer && v.IsNil() {
		return errors.New("demy: Image has no image or key")
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > maxPixelDimension || h > maxPixelDimension || w*h > maxImagePixels {
		return fmt.Errorf("demy: Image bounds %v out of range: each side must be in [1, %d] and the area at most %d pixels",
			b, maxPixelDimension, maxImagePixels)
	}
	return nil
}

// maxDimension bounds every size, gap, and weight in a node tree so untrusted
// scenes can't request unbounded allocations or overflow layout math.
const maxDimension = 1 << 15

// maxNodeDepth bounds node tree nesting so recursion in measure, layout, and
// draw can't exhaust the stack.
const maxNodeDepth = 1000

// maxNodes bounds the number of nodes in a tree, counting each use of a
// shared node, so a small tree that reuses nodes can't expand into an
// unbounded amount of layout work.
const maxNodes = 1 << 14

// maxClippedNodes bounds Frames and Boxes with Clip, each of which rasterizes
// and composites a sub-canvas up to the size of the whole canvas.
const maxClippedNodes = 128

// maxBarcodes bounds barcodes, which are encoded while building the tree.
const maxBarcodes = 16

// maxTextBytes bounds the UTF-8 length of one Text. Line breaking time grows
// faster than linearly with paragraph length.
const maxTextBytes = 1 << 13

// maxTreeTextBytes bounds the total UTF-8 length of all Text in a tree,
// counting each use of a shared node.
const maxTreeTextBytes = 1 << 16

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

// buildChildren converts a slice of exported Nodes, skipping nil entries.
func (b *builder) buildChildren(nodes []Node, parentDir layoutDir, depth int) ([]node, error) {
	result := make([]node, 0, len(nodes))
	for _, n := range nodes {
		if isNilNode(n) {
			continue
		}
		built, err := b.build(n, parentDir, depth)
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
