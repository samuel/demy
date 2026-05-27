package demy

import (
	"image"

	cv "github.com/tdewolff/canvas"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/vector"
)

// vectorRenderer implements cv.Renderer using golang.org/x/image/vector instead of
// srwiley/scanx. The scanx rasterizer has a platform-specific infinite-loop bug
// triggered by certain path geometries on Linux.
type vectorRenderer struct {
	img           *image.RGBA
	resolution    cv.Resolution
	width, height float64
	ras           *vector.Rasterizer
}

func newVectorRenderer(img *image.RGBA, dpmm float64) *vectorRenderer {
	b := img.Bounds().Size()
	return &vectorRenderer{
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
	b := r.img.Bounds().Size()
	if style.HasFill() && style.Fill.IsColor() {
		fill := path.Copy().Transform(m)
		r.ras.Reset(b.X, b.Y)
		fill.ToVectorRasterizer(r.ras, r.resolution)
		r.ras.Draw(r.img, r.img.Bounds(), image.NewUniform(style.Fill.Color), image.Point{})
	}
	if style.HasStroke() && style.Stroke.IsColor() {
		tolerance := cv.PixelTolerance / r.resolution.DPMM()
		stroke := path
		if len(style.Dashes) > 0 {
			dashOffset, dashes := cv.ScaleDash(style.StrokeWidth, style.DashOffset, style.Dashes)
			stroke = stroke.Dash(dashOffset, dashes...)
		}
		stroke = stroke.Stroke(style.StrokeWidth, style.StrokeCapper, style.StrokeJoiner, tolerance)
		stroke = stroke.Transform(m)
		r.ras.Reset(b.X, b.Y)
		stroke.ToVectorRasterizer(r.ras, r.resolution)
		r.ras.Draw(r.img, r.img.Bounds(), image.NewUniform(style.Stroke.Color), image.Point{})
	}
}

func (r *vectorRenderer) RenderText(text *cv.Text, m cv.Matrix) {
	text.RenderAsPath(r, m, r.resolution)
}

func (r *vectorRenderer) RenderImage(img image.Image, m cv.Matrix) {
	dpmm := r.resolution.DPMM()
	origin := m.Dot(cv.Point{X: 0, Y: float64(img.Bounds().Size().Y)}).Mul(dpmm)
	m = m.Scale(dpmm, dpmm)
	h := float64(r.img.Bounds().Size().Y)
	aff3 := f64.Aff3{m[0][0], -m[0][1], origin.X, -m[1][0], m[1][1], h - origin.Y}
	xdraw.CatmullRom.Transform(r.img, aff3, img, img.Bounds(), xdraw.Over, nil)
}
