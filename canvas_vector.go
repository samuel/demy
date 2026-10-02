package demy

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	cv "github.com/tdewolff/canvas"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/vector"
)

// vectorRenderer implements cv.Renderer using golang.org/x/image/vector. The
// default srwiley/scanx rasterizer can loop forever on some path geometries
// on Linux.
type vectorRenderer struct {
	rs            *renderState // paths and images aren't drawn once the render is stopped
	img           *image.RGBA
	resolution    cv.Resolution
	width, height float64
	ras           *vector.Rasterizer
}

func newVectorRenderer(img *image.RGBA, dpmm float64, rs *renderState) *vectorRenderer {
	b := img.Bounds().Size()
	return &vectorRenderer{
		rs:         rs,
		img:        img,
		resolution: cv.DPMM(dpmm),
		width:      float64(b.X) / dpmm,
		height:     float64(b.Y) / dpmm,
		ras:        vector.NewRasterizer(b.X, b.Y),
	}
}

func (r *vectorRenderer) Size() (float64, float64) {
	return r.width, r.height
}

func (r *vectorRenderer) RenderPath(path *cv.Path, style cv.Style, m cv.Matrix) {
	if r.rs.stopped() {
		return
	}
	if style.HasFill() && style.Fill.IsColor() {
		r.fill(path.Copy().Transform(m), style.Fill.Color)
	}
	if style.HasStroke() && style.Stroke.IsColor() {
		tolerance := cv.PixelTolerance / r.resolution.DPMM()
		stroke := path
		if len(style.Dashes) > 0 {
			dashOffset, dashes := cv.ScaleDash(style.StrokeWidth, style.DashOffset, style.Dashes)
			stroke = stroke.Dash(dashOffset, dashes...)
		}
		stroke = stroke.Stroke(style.StrokeWidth, style.StrokeCapper, style.StrokeJoiner, tolerance)
		r.fill(stroke.Transform(m), style.Stroke.Color)
	}
}

// minRasterizerWidth exceeds x/image/vector's fixed-point threshold (512).
const minRasterizerWidth = 513

// fill rasterizes path (in millimeters, Y up) with c. The rasterizer covers
// only the path's pixel bounding box, so the cost scales with the path's size
// rather than the canvas's.
func (r *vectorRenderer) fill(path *cv.Path, c color.Color) {
	dpmm := r.resolution.DPMM()
	h := float64(r.img.Bounds().Dy())
	b := path.FastBounds()
	px := image.Rect(
		int(math.Floor(b.X0*dpmm))-1,
		int(math.Floor(h-b.Y1*dpmm))-1,
		int(math.Ceil(b.X1*dpmm))+1,
		int(math.Ceil(h-b.Y0*dpmm))+1,
	).Intersect(r.img.Bounds())
	if px.Empty() {
		return
	}
	// ToVectorRasterizer maps x to x*dpmm and y to rasterizerHeight-y*dpmm;
	// shift the path so the bounding box lands at the rasterizer's origin.
	path = path.Translate(-float64(px.Min.X)/dpmm, (float64(px.Dy())-h+float64(px.Min.Y))/dpmm)
	// The rasterizer switches to lower-precision fixed-point math when both
	// dimensions are at most 512; keep it wider so every path is rasterized
	// with the same floating-point math regardless of its size. Draw only
	// reads the px-sized corner.
	r.ras.Reset(max(px.Dx(), minRasterizerWidth), px.Dy())
	path.ToVectorRasterizer(r.ras, r.resolution)
	r.ras.Draw(r.img, px, image.NewUniform(c), image.Point{})
}

func (r *vectorRenderer) RenderText(text *cv.Text, m cv.Matrix) {
	text.RenderTo(r, m, r.resolution)
}

func (r *vectorRenderer) RenderImage(img image.Image, m cv.Matrix) {
	if r.rs.stopped() {
		return
	}
	dpmm := r.resolution.DPMM()
	sb := img.Bounds()
	origin := m.Dot(cv.Point{X: 0, Y: float64(sb.Dy())}).Mul(dpmm)
	m = m.Scale(dpmm, dpmm)
	h := float64(r.img.Bounds().Dy())
	tx, ty := origin.X, h-origin.Y

	// An unscaled image on whole pixels is copied directly, which is exact and
	// avoids resampling (clipped sub-canvases and barcodes are drawn this way).
	const eps = 1e-6
	if math.Abs(m[0][0]-1) < eps && math.Abs(m[1][1]-1) < eps &&
		math.Abs(m[0][1]) < eps && math.Abs(m[1][0]) < eps &&
		math.Abs(tx-math.Round(tx)) < eps && math.Abs(ty-math.Round(ty)) < eps {
		pt := image.Pt(int(math.Round(tx)), int(math.Round(ty)))
		draw.Draw(r.img, sb.Sub(sb.Min).Add(pt), img, sb.Min, draw.Over)
		return
	}

	// aff3 maps source pixels relative to the source origin; shift it so it
	// accepts the absolute coordinates of images whose bounds don't start at
	// (0,0), such as cover-cropped SubImages.
	a, b, d, e := m[0][0], -m[0][1], -m[1][0], m[1][1]
	aff3 := f64.Aff3{
		a, b, tx - a*float64(sb.Min.X) - b*float64(sb.Min.Y),
		d, e, ty - d*float64(sb.Min.X) - e*float64(sb.Min.Y),
	}
	xdraw.CatmullRom.Transform(r.img, aff3, img, sb, xdraw.Over, nil)
}
