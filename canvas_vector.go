package demy

import (
	"image"
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
	text.RenderTo(r, m, r.resolution)
}

func (r *vectorRenderer) RenderImage(img image.Image, m cv.Matrix) {
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
