package demy

import (
	"image"
	"image/color"
	"math"

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
	availW float64
	availH float64
}

// drawCtx carries context for the draw pass.
type drawCtx struct {
	ctx           *cv.Context
	dpmm          float64
	roles         Roles
	fonts         map[string]*FontFamily
	ditherRegions []imageDitherRegion
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
		s := child.measure(&measureCtx{
			fonts:  mctx.fonts,
			dpmm:   mctx.dpmm,
			availW: innerW,
			availH: math.MaxFloat32,
		})
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
	innerW := bounds.w - 2*n.padding
	innerH := bounds.h - 2*n.padding

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
		n.measuredSizes[i] = child.measure(&measureCtx{
			fonts:  mctx.fonts,
			dpmm:   mctx.dpmm,
			availW: innerW,
			availH: innerH,
		})
	}

	// Pass 2: if variable-width children collectively overflow the available
	// space (after reserving gaps and fixed-width children like frameNode),
	// shrink them proportionally and re-measure so heights are correct.
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
				n.measuredSizes[i] = child.measure(&measureCtx{
					fonts:  mctx.fonts,
					dpmm:   mctx.dpmm,
					availW: newW,
					availH: innerH,
				})
			}
		}
	}

	// Accumulate totals from (possibly updated) measured sizes.
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
	innerW := bounds.w - 2*n.padding
	innerH := bounds.h - 2*n.padding

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

func (n *boxNode) measure(mctx *measureCtx) nodeSize {
	innerW := math.Max(0, mctx.availW-2*n.padding)
	innerH := math.Max(0, mctx.availH-2*n.padding)
	n.measuredSizes = make([]nodeSize, len(n.children))
	var maxW, maxH float64
	for i, child := range n.children {
		s := child.measure(&measureCtx{
			fonts:  mctx.fonts,
			dpmm:   mctx.dpmm,
			availW: innerW,
			availH: innerH,
		})
		n.measuredSizes[i] = s
		if s.w > maxW {
			maxW = s.w
		}
		if s.h > maxH {
			maxH = s.h
		}
	}
	return nodeSize{w: maxW + 2*n.padding, h: maxH + 2*n.padding}
}

func (n *boxNode) layout(bounds nodeRect) {
	n.bounds = bounds
	innerX := bounds.x + n.padding
	innerY := bounds.y + n.padding
	innerW := bounds.w - 2*n.padding
	innerH := bounds.h - 2*n.padding
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
		child.measure(&measureCtx{
			fonts:  mctx.fonts,
			dpmm:   mctx.dpmm,
			availW: n.w,
			availH: n.h,
		})
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
	dpmm := dctx.dpmm
	wPx := int(math.Round(n.w))
	hPx := int(math.Round(n.h))
	if wPx <= 0 || hPx <= 0 {
		return
	}
	wMM := n.w / dpmm
	hMM := n.h / dpmm
	xMM := n.bounds.x / dpmm
	yMM := n.bounds.y / dpmm

	subCvs := cv.New(wMM, hMM)
	subCtx := cv.NewContext(subCvs)
	subCtx.SetCoordSystem(cv.CartesianIV)
	subCtx.Translate(-xMM, -yMM)
	subDctx := &drawCtx{
		ctx:   subCtx,
		dpmm:  dpmm,
		roles: dctx.roles,
		fonts: dctx.fonts,
	}
	for _, child := range n.children {
		child.draw(subDctx)
	}

	subImg := image.NewRGBA(image.Rect(0, 0, wPx, hPx))
	subRas := newVectorRenderer(subImg, dpmm)
	subCvs.RenderTo(subRas)

	dctx.ctx.DrawImage(xMM, yMM, subImg, cv.DPMM(dpmm))
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
	// retained from measure for re-measuring the child with actual allocated dims.
	fonts  map[string]*FontFamily
	dpmm   float64
	bounds nodeRect
}

func (n *weightedNode) measure(mctx *measureCtx) nodeSize {
	n.fonts = mctx.fonts
	n.dpmm = mctx.dpmm
	n.child.measure(mctx)
	return nodeSize{}
}

func (n *weightedNode) layout(bounds nodeRect) {
	n.bounds = bounds
	n.child.measure(&measureCtx{
		fonts:  n.fonts,
		dpmm:   n.dpmm,
		availW: bounds.w,
		availH: bounds.h,
	})
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
	return nodeSize{w: sz, h: mctx.availH}
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
	if ff == nil || n.style.Size == 0 {
		return nodeSize{}
	}
	clr := n.style.Color
	if clr == nil {
		clr = Black
	}
	face := ff.family.Face(n.style.Size, clr, cv.FontStyle(n.style.Weight))
	wMM := mctx.availW / mctx.dpmm
	var heightMM float64
	if n.style.MaxLines > 0 {
		lh := face.LineHeight()
		lineStretch := n.style.LineHeight
		if lineStretch == 0 {
			lineStretch = 1.0
		}
		heightMM = lh * float64(n.style.MaxLines) * lineStretch
	}
	opts := textOptions(n.style)
	tb := cv.NewTextBox(face, n.text, wMM, heightMM, cvHAlign(n.style.Align), cv.Top, opts)
	return nodeSize{w: tb.Width * mctx.dpmm, h: tb.Height * mctx.dpmm}
}

func (n *textNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *textNode) draw(dctx *drawCtx) {
	ff := dctx.fonts[n.style.Font]
	if ff == nil || n.style.Size == 0 {
		return
	}
	clr := n.style.Color
	if clr == nil {
		clr = dctx.roles.Foreground
	}
	if clr == nil {
		clr = Black
	}
	face := ff.family.Face(n.style.Size, clr, cv.FontStyle(n.style.Weight))
	wMM := n.bounds.w / dctx.dpmm
	var heightMM float64
	if n.style.MaxLines > 0 {
		lh := face.LineHeight()
		lineStretch := n.style.LineHeight
		if lineStretch == 0 {
			lineStretch = 1.0
		}
		heightMM = lh * float64(n.style.MaxLines) * lineStretch
	}
	opts := textOptions(n.style)
	tb := cv.NewTextBox(face, n.text, wMM, heightMM, cvHAlign(n.style.Align), cv.Top, opts)
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

// --- imageNode ---

type imageNode struct {
	img    image.Image
	scale  ImageScale
	clip   bool
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
		if h > 1e9 {
			h = ih
		}
		if w > 1e9 {
			w = iw
		}
		return nodeSize{w: w, h: h}
	default: // ImageFit
		if availH > 1e9 {
			availH = ih
		}
		scale := math.Min(availW/iw, availH/ih)
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
	var fit cv.ImageFit
	switch n.scale {
	case ImageFill:
		fit = cv.ImageCover
	case ImageStretch:
		fit = cv.ImageFill
	default: // ImageFit
		fit = cv.ImageContain
	}
	dctx.ctx.FitImage(n.img, rect, fit)
	if n.dither != NoDithering {
		px := image.Rect(
			int(n.bounds.x),
			int(n.bounds.y),
			int(n.bounds.x+bw),
			int(n.bounds.y+bh),
		)
		dctx.ditherRegions = append(dctx.ditherRegions, imageDitherRegion{bounds: px, algo: n.dither})
	}
}

// --- barcodeNode ---

type barcodeNode struct {
	content string
	btype   BarcodeType
	ecLevel ECLevel
	bounds  nodeRect
}

func (n *barcodeNode) measure(mctx *measureCtx) nodeSize {
	w := mctx.availW
	h := mctx.availH
	if h > 1e9 {
		h = w
	}
	if w > 1e9 {
		w = h
	}
	return nodeSize{w: w, h: h}
}

func (n *barcodeNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *barcodeNode) draw(dctx *drawCtx) {
	w := int(math.Round(n.bounds.w))
	h := int(math.Round(n.bounds.h))
	if w <= 0 || h <= 0 {
		return
	}
	var (
		bc  barcode.Barcode
		err error
	)
	switch n.btype {
	default: // BarcodeTypeQR
		bc, err = qr.Encode(n.content, qr.ErrorCorrectionLevel(n.ecLevel), qr.Auto)
	}
	if err != nil {
		return
	}
	scaled, err := barcode.Scale(bc, w, h)
	if err != nil {
		return
	}
	dpmm := dctx.dpmm
	dctx.ctx.DrawImage(n.bounds.x/dpmm, n.bounds.y/dpmm, scaled, cv.DPMM(dpmm))
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

	// retained from measure for deferred re-measure of children in Fr rows
	measureFonts map[string]*FontFamily
	measureDPMM  float64
}

func (n *gridNode) measure(mctx *measureCtx) nodeSize {
	if len(n.columns) == 0 {
		return nodeSize{}
	}
	n.measureFonts = mctx.fonts
	n.measureDPMM = mctx.dpmm
	n.numCols = len(n.columns)
	numChildren := len(n.children)
	n.numRows = (numChildren + n.numCols - 1) / n.numCols
	if n.numRows == 0 {
		return nodeSize{}
	}
	// Cap numRows to explicit row definitions when provided.
	if len(n.rows) > 0 && len(n.rows) < n.numRows {
		n.numRows = len(n.rows)
	}

	innerW := math.Max(0, mctx.availW-2*n.padding)
	innerH := math.Max(0, mctx.availH-2*n.padding)
	totalColGaps := math.Max(0, float64(n.numCols-1)) * n.colGap
	availForCols := math.Max(0, innerW-totalColGaps)

	// Phase 1: compute rough column widths for first-pass child measurement.
	roughColWidths := n.computeColWidths(availForCols, nil)

	// Phase 2: measure all children with rough column widths.
	total := n.numRows * n.numCols
	n.cellSizes = make([]nodeSize, total)
	for i, child := range n.children {
		if i >= total {
			break
		}
		col := i % n.numCols
		n.cellSizes[i] = child.measure(&measureCtx{
			fonts:  mctx.fonts,
			dpmm:   mctx.dpmm,
			availW: roughColWidths[col],
			availH: math.MaxFloat32,
		})
	}

	// Phase 3: finalize column widths using measured content for auto tracks.
	n.colWidths = n.computeColWidths(availForCols, n.cellSizes)

	// Phase 4: re-measure children whose column width changed noticeably,
	// so row heights reflect the final layout (e.g. text re-wrapping).
	for i, child := range n.children {
		if i >= total {
			break
		}
		col := i % n.numCols
		if math.Abs(n.colWidths[col]-roughColWidths[col]) > 0.5 {
			n.cellSizes[i] = child.measure(&measureCtx{
				fonts:  mctx.fonts,
				dpmm:   mctx.dpmm,
				availW: n.colWidths[col],
				availH: math.MaxFloat32,
			})
		}
	}

	// Phase 5: compute row heights (Fr rows contribute 0 here; finalized in layout).
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

	// Re-derive column widths with actual bounds (parent may have given us a
	// different width than what measure was called with).
	n.colWidths = n.computeColWidths(availForCols, n.cellSizes)

	// Finalize Fr row heights using actual available height.
	n.rowHeights = n.finalizeRowHeights(availForRows, n.cellSizes)

	total := n.numRows * n.numCols

	// Re-measure children in Fr rows — they now have a definite height.
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
			n.cellSizes[idx] = n.children[idx].measure(&measureCtx{
				fonts:  n.measureFonts,
				dpmm:   n.measureDPMM,
				availW: n.colWidths[col],
				availH: n.rowHeights[row],
			})
		}
	}

	// Compute cumulative column and row offsets.
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

	// Lay out children in their cells.
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
func (n *gridNode) rowTrack(row int) GridTrack {
	if row < len(n.rows) {
		return n.rows[row]
	}
	return GridTrack{kind: gridTrackAuto}
}

// computeColWidths calculates column widths for the given available width.
// If cellSizes is nil, auto columns are estimated by splitting remaining space evenly.
func (n *gridNode) computeColWidths(availForCols float64, cellSizes []nodeSize) []float64 {
	widths := make([]float64, n.numCols)

	// Sum fixed widths and count non-fixed tracks.
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

	// Pass 1: assign fixed tracks.
	for c, col := range n.columns {
		if col.kind == gridTrackFixed {
			widths[c] = col.size
		}
	}

	// Pass 2: auto tracks — use measured content max if available, else split evenly.
	var autoFixedW float64 // fixed + final-auto contribution
	for c, col := range n.columns {
		if col.kind != gridTrackAuto {
			continue
		}
		if cellSizes == nil {
			// Rough estimate: split remaining evenly among non-fixed tracks.
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

	// Pass 3: distribute remaining width to Fr tracks.
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

// computeRowHeights calculates row heights from measured cell sizes.
// Fr rows return 0 (deferred to finalizeRowHeights with actual bounds).
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

// finalizeRowHeights is like computeRowHeights but always distributes Fr rows
// using the actual available height from layout bounds.
func (n *gridNode) finalizeRowHeights(availForRows float64, cellSizes []nodeSize) []float64 {
	return n.computeRowHeights(availForRows, cellSizes)
}

// --- Build bridge: exported Node tree → internal node tree ---

// buildNode converts an exported Node into the internal node used by the
// measure/layout/draw pipeline. parentDir is the layout direction of the
// enclosing container, used to infer DividerNode orientation.
func buildNode(n Node, cfg *Config, parentDir layoutDir) (node, error) {
	if n == nil {
		return &spacerNode{weight: 0}, nil
	}
	switch v := n.(type) {
	case *VStackNode:
		children, err := buildNodes(v.Children, cfg, dirVertical)
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
		children, err := buildNodes(v.Children, cfg, dirHorizontal)
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
		children, err := buildNodes(v.Children, cfg, dirVertical)
		if err != nil {
			return nil, err
		}
		return &zstackNode{align: v.Align, children: children}, nil
	case *BoxNode:
		children, err := buildNodes(v.Children, cfg, dirVertical)
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
		children, err := buildNodes(v.Children, cfg, dirVertical)
		if err != nil {
			return nil, err
		}
		return &frameNode{w: v.Width, h: v.Height, children: children}, nil
	case *SpacerNode:
		w := v.Weight
		if w == 0 {
			w = 1
		}
		return &spacerNode{weight: w}, nil
	case *WeightedNode:
		w := v.Weight
		if w == 0 {
			w = 1
		}
		var child node
		if v.Child != nil {
			var err error
			child, err = buildNode(v.Child, cfg, parentDir)
			if err != nil {
				return nil, err
			}
		} else {
			child = &spacerNode{}
		}
		return &weightedNode{weight: w, child: child}, nil
	case *DividerNode:
		return &dividerNode{isHorizontal: parentDir == dirVertical, color: v.Color, size: v.Size}, nil
	case *TextNode:
		var style Style
		if cfg != nil {
			style = cfg.DefaultTextStyle
		}
		if v.Class != "" && cfg != nil {
			if classStyle, ok := cfg.Styles[string(v.Class)]; ok {
				style = mergeStyle(style, classStyle)
			}
		}
		style = mergeStyle(style, v.Style)
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
		return &imageNode{img: img, scale: v.Scale, clip: v.Clip, dither: v.Dither}, nil
	case *BarcodeNode:
		return &barcodeNode{content: v.Content, btype: v.Type, ecLevel: v.ECLevel}, nil
	case *GridNode:
		flat := make([]Node, 0, len(v.Children))
		for _, child := range v.Children {
			if row, ok := child.(*GridRowNode); ok {
				flat = append(flat, row.Children...)
			} else {
				flat = append(flat, child)
			}
		}
		children, err := buildNodes(flat, cfg, dirVertical)
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
	default:
		panic("demy: unknown node type")
	}
}

// buildNodes converts a slice of exported Nodes, skipping nil entries.
func buildNodes(nodes []Node, cfg *Config, parentDir layoutDir) ([]node, error) {
	result := make([]node, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		built, err := buildNode(n, cfg, parentDir)
		if err != nil {
			return nil, err
		}
		result = append(result, built)
	}
	return result, nil
}

// imageKeyError is a named error constructor to avoid fmt import in this file.
type imageKeyError string

func (e imageKeyError) Error() string {
	return "demy: image key " + string(e) + " not found in Config.Images"
}
