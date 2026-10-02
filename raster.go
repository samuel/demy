package demy

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/vector"
)

// pathOp is one path command in physical pixels, Y down.
type pathOp struct {
	op  uint8
	pts [3][2]float32
}

const (
	opMove uint8 = iota
	opLine
	opQuad
	opCube
	opClose
)

// path is a list of closed contours with their bounding box. Contours with
// opposite windings cancel, so a reversed inner contour cuts a hole.
type path struct {
	ops                    []pathOp
	minX, minY, maxX, maxY float32
}

func (p *path) extend(x, y float32) {
	if len(p.ops) == 0 {
		p.minX, p.minY, p.maxX, p.maxY = x, y, x, y
		return
	}
	p.minX, p.minY = min(p.minX, x), min(p.minY, y)
	p.maxX, p.maxY = max(p.maxX, x), max(p.maxY, y)
}

func (p *path) add(op uint8, pts ...[2]float32) {
	for _, pt := range pts {
		p.extend(pt[0], pt[1])
	}
	var o pathOp
	o.op = op
	copy(o.pts[:], pts)
	p.ops = append(p.ops, o)
}

func (p *path) moveTo(x, y float32) { p.add(opMove, [2]float32{x, y}) }
func (p *path) lineTo(x, y float32) { p.add(opLine, [2]float32{x, y}) }
func (p *path) close()              { p.ops = append(p.ops, pathOp{op: opClose}) }
func (p *path) empty() bool         { return len(p.ops) == 0 }
func (p *path) quadTo(bx, by, cx, cy float32) {
	p.add(opQuad, [2]float32{bx, by}, [2]float32{cx, cy})
}

func (p *path) cubeTo(bx, by, cx, cy, dx, dy float32) {
	p.add(opCube, [2]float32{bx, by}, [2]float32{cx, cy}, [2]float32{dx, dy})
}

// arcK places cubic control points that approximate a quarter circle.
const arcK = 0.5522847498

// roundedRect adds a rectangle with corner radius r, limited to half the
// shorter side. reverse winds it the other way, to cut a hole.
func (p *path) roundedRect(x, y, w, h, r float64, reverse bool) {
	if w <= 0 || h <= 0 {
		return
	}
	r = min(r, w/2, h/2)
	if r <= 0 {
		if reverse {
			p.moveTo(f32(x), f32(y))
			p.lineTo(f32(x), f32(y+h))
			p.lineTo(f32(x+w), f32(y+h))
			p.lineTo(f32(x+w), f32(y))
		} else {
			p.moveTo(f32(x), f32(y))
			p.lineTo(f32(x+w), f32(y))
			p.lineTo(f32(x+w), f32(y+h))
			p.lineTo(f32(x), f32(y+h))
		}
		p.close()
		return
	}
	k := r * arcK
	x1, y1 := x+w, y+h
	if reverse {
		p.moveTo(f32(x+r), f32(y))
		p.cubeTo(f32(x+r-k), f32(y), f32(x), f32(y+r-k), f32(x), f32(y+r))
		p.lineTo(f32(x), f32(y1-r))
		p.cubeTo(f32(x), f32(y1-r+k), f32(x+r-k), f32(y1), f32(x+r), f32(y1))
		p.lineTo(f32(x1-r), f32(y1))
		p.cubeTo(f32(x1-r+k), f32(y1), f32(x1), f32(y1-r+k), f32(x1), f32(y1-r))
		p.lineTo(f32(x1), f32(y+r))
		p.cubeTo(f32(x1), f32(y+r-k), f32(x1-r+k), f32(y), f32(x1-r), f32(y))
	} else {
		p.moveTo(f32(x+r), f32(y))
		p.lineTo(f32(x1-r), f32(y))
		p.cubeTo(f32(x1-r+k), f32(y), f32(x1), f32(y+r-k), f32(x1), f32(y+r))
		p.lineTo(f32(x1), f32(y1-r))
		p.cubeTo(f32(x1), f32(y1-r+k), f32(x1-r+k), f32(y1), f32(x1-r), f32(y1))
		p.lineTo(f32(x+r), f32(y1))
		p.cubeTo(f32(x+r-k), f32(y1), f32(x), f32(y1-r+k), f32(x), f32(y1-r))
		p.lineTo(f32(x), f32(y+r))
		p.cubeTo(f32(x), f32(y+r-k), f32(x+r-k), f32(y), f32(x+r), f32(y))
	}
	p.close()
}

func f32(v float64) float32 { return float32(v) }

// painter draws onto img in physical pixels. A painter on a SubImage clips
// everything it draws to the sub-image's bounds.
type painter struct {
	rs  *renderState // nothing is drawn once the render is stopped
	img *image.RGBA
	ras *vector.Rasterizer
}

func newPainter(img *image.RGBA, rs *renderState) *painter {
	return &painter{rs: rs, img: img, ras: vector.NewRasterizer(0, 0)}
}

// sub returns a painter that draws only within r, sharing p's pixels.
func (p *painter) sub(r image.Rectangle) *painter {
	return &painter{rs: p.rs, img: p.img.SubImage(r).(*image.RGBA), ras: p.ras} //nolint:forcetypeassert // RGBA.SubImage returns *image.RGBA
}

// minRasterizerWidth exceeds x/image/vector's fixed-point threshold (512).
const minRasterizerWidth = 513

// fill rasterizes pa with c. The rasterizer covers only the path's pixel
// bounding box, so the cost scales with the path's size rather than the
// canvas's.
func (p *painter) fill(pa *path, c color.Color) {
	if p.rs.stopped() || pa.empty() {
		return
	}
	px := image.Rect(
		int(math.Floor(float64(pa.minX)))-1,
		int(math.Floor(float64(pa.minY)))-1,
		int(math.Ceil(float64(pa.maxX)))+1,
		int(math.Ceil(float64(pa.maxY)))+1,
	).Intersect(p.img.Bounds())
	if px.Empty() {
		return
	}
	// The rasterizer switches to lower-precision fixed-point math when both
	// dimensions are at most 512; keep it wider so every path is rasterized
	// with the same floating-point math regardless of its size. Draw only
	// reads the px-sized corner.
	p.ras.Reset(max(px.Dx(), minRasterizerWidth), px.Dy())
	ox, oy := float32(px.Min.X), float32(px.Min.Y)
	open := false
	for _, o := range pa.ops {
		a, b, c := o.pts[0], o.pts[1], o.pts[2]
		switch o.op {
		case opMove:
			if open {
				p.ras.ClosePath()
			}
			p.ras.MoveTo(a[0]-ox, a[1]-oy)
			open = true
		case opLine:
			p.ras.LineTo(a[0]-ox, a[1]-oy)
		case opQuad:
			p.ras.QuadTo(a[0]-ox, a[1]-oy, b[0]-ox, b[1]-oy)
		case opCube:
			p.ras.CubeTo(a[0]-ox, a[1]-oy, b[0]-ox, b[1]-oy, c[0]-ox, c[1]-oy)
		case opClose:
			p.ras.ClosePath()
			open = false
		}
	}
	if open {
		p.ras.ClosePath()
	}
	p.ras.Draw(p.img, px, image.NewUniform(c), image.Point{})
}

// fillRect fills the rectangle (x, y, w, h), in physical pixels, with corner
// radius r.
func (p *painter) fillRect(x, y, w, h, r float64, c color.Color) {
	var pa path
	pa.roundedRect(x, y, w, h, r, false)
	p.fill(&pa, c)
}

// strokeRect draws a border of width bw inside the rectangle (x, y, w, h)
// with corner radius r. The border is centered on a rectangle inset by bw/2
// with radius r-bw/2, so a radius under bw/2 gives square corners.
func (p *painter) strokeRect(x, y, w, h, r, bw float64, c color.Color) {
	mid := max(0, r-bw/2)
	var outer float64
	if mid > 0 {
		outer = mid + bw/2
	}
	var pa path
	pa.roundedRect(x, y, w, h, outer, false)
	pa.roundedRect(x+bw, y+bw, w-2*bw, h-2*bw, max(0, mid-bw/2), true)
	p.fill(&pa, c)
}

// drawImage draws src scaled to the rectangle (x, y, w, h) in physical
// pixels. An unscaled image on whole pixels is copied directly, which is exact
// and avoids resampling (clipped content, dithered images, and barcodes are
// drawn this way).
func (p *painter) drawImage(src image.Image, x, y, w, h float64) {
	if p.rs.stopped() {
		return
	}
	sb := src.Bounds()
	if sb.Empty() || w <= 0 || h <= 0 {
		return
	}
	sx, sy := w/float64(sb.Dx()), h/float64(sb.Dy())
	const eps = 1e-6
	if math.Abs(sx-1) < eps && math.Abs(sy-1) < eps &&
		math.Abs(x-math.Round(x)) < eps && math.Abs(y-math.Round(y)) < eps {
		pt := image.Pt(int(math.Round(x)), int(math.Round(y)))
		draw.Draw(p.img, sb.Sub(sb.Min).Add(pt), src, sb.Min, draw.Over)
		return
	}
	// aff3 maps source pixels to the destination; it accepts the absolute
	// coordinates of images whose bounds don't start at (0,0), such as
	// cover-cropped SubImages.
	aff3 := f64.Aff3{
		sx, 0, x - sx*float64(sb.Min.X),
		0, sy, y - sy*float64(sb.Min.Y),
	}
	xdraw.CatmullRom.Transform(p.img, aff3, src, sb, xdraw.Over, nil)
}
