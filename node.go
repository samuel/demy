package pate

import (
	"image"
	"image/color"
	"math"

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
	ctx   *cv.Context
	dpmm  float64
	roles Roles
	fonts map[string]*FontFamily
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
		if _, ok := child.(*spacerNode); !ok {
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

	// Compute fixed height and total spacer weight.
	fixedH := -n.gap // account for no gap before first child
	totalWeight := 0.0
	for i, child := range n.children {
		fixedH += n.gap
		if s, ok := child.(*spacerNode); ok {
			totalWeight += s.weight
		} else {
			fixedH += n.measuredSizes[i].h
		}
	}
	spacerH := 0.0
	if totalWeight > 0 {
		spacerH = math.Max(0, innerH-fixedH) / totalWeight
	}

	y := innerY
	for i, child := range n.children {
		if i > 0 {
			y += n.gap
		}
		var childW, childH float64
		if sp, ok := child.(*spacerNode); ok {
			childH = spacerH * sp.weight
			childW = 0
		} else {
			childH = n.measuredSizes[i].h
			childW = n.measuredSizes[i].w
		}
		childX := innerX
		if childW < innerW {
			switch n.align {
			case AlignCenter:
				childX = innerX + (innerW-childW)/2
			case AlignEnd:
				childX = innerX + innerW - childW
			}
		}
		child.layout(nodeRect{x: childX, y: y, w: innerW, h: childH})
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
	var totalW, maxH float64
	for i, child := range n.children {
		s := child.measure(&measureCtx{
			fonts:  mctx.fonts,
			dpmm:   mctx.dpmm,
			availW: innerW,
			availH: innerH,
		})
		n.measuredSizes[i] = s
		if i > 0 {
			totalW += n.gap
		}
		if _, ok := child.(*spacerNode); !ok {
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
		if s, ok := child.(*spacerNode); ok {
			totalWeight += s.weight
		} else {
			fixedW += n.measuredSizes[i].w
		}
	}
	spacerW := 0.0
	if totalWeight > 0 {
		spacerW = math.Max(0, innerW-fixedW) / totalWeight
	}

	x := innerX
	for i, child := range n.children {
		if i > 0 {
			x += n.gap
		}
		var childW, childH float64
		if sp, ok := child.(*spacerNode); ok {
			childW = spacerW * sp.weight
			childH = innerH
		} else {
			childW = n.measuredSizes[i].w
			childH = n.measuredSizes[i].h
		}
		childY := innerY
		if childH < innerH {
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
	for _, child := range n.children {
		child.draw(dctx)
	}
}

// --- spacerNode ---

type spacerNode struct {
	weight float64
	bounds nodeRect
}

func (n *spacerNode) measure(_ *measureCtx) nodeSize { return nodeSize{} }

func (n *spacerNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *spacerNode) draw(_ *drawCtx) {}

// --- dividerNode ---

type dividerNode struct {
	isHorizontal bool
	bounds       nodeRect
}

func (n *dividerNode) measure(mctx *measureCtx) nodeSize {
	if n.isHorizontal {
		return nodeSize{w: mctx.availW, h: 1}
	}
	return nodeSize{w: 1, h: mctx.availH}
}

func (n *dividerNode) layout(bounds nodeRect) { n.bounds = bounds }

func (n *dividerNode) draw(dctx *drawCtx) {
	col := dctx.roles.Muted
	if col == nil {
		col = dctx.roles.Foreground
	}
	if col == nil {
		col = Black
	}
	dctx.ctx.SetFillColor(col)
	dctx.ctx.SetStrokeColor(color.Transparent)
	xMM := n.bounds.x / dctx.dpmm
	yMM := n.bounds.y / dctx.dpmm
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
	case *DividerNode:
		return &dividerNode{isHorizontal: parentDir == dirVertical}, nil
	case *TextNode:
		style := v.Style
		if style.Font == "" && cfg != nil {
			style = cfg.DefaultTextStyle
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
		return &imageNode{img: img, scale: v.Scale, clip: v.Clip}, nil
	default:
		panic("pate: unknown node type")
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
	return "pate: image key " + string(e) + " not found in Config.Images"
}
