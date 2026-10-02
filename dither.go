package demy

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
)

// maxPaletteSize is the most entries an *image.Paletted can index.
const maxPaletteSize = 256

// quantizer maps colors to the entries of an output palette. Palettes of only
// grays are matched by luminance; others by distance in OKLab.
type quantizer struct {
	palette color.Palette // Config.Palette as given
	colors  []color.RGBA  // opaque entries
	gray    bool          // every entry is gray
	levels  []color.Gray  // luminance of each entry (gray palettes)
	lab     [][3]float64  // OKLab of each entry (color palettes)
	cache   []uint8       // color palettes: nearest entry per 5-bit-per-channel RGB
}

// newQuantizer validates pal and prepares it for quantizing. A palette needs
// 2 to 256 opaque, concrete colors.
func newQuantizer(pal []color.Color) (*quantizer, error) {
	if len(pal) < 2 || len(pal) > maxPaletteSize {
		return nil, fmt.Errorf("demy: Config.Palette has %d colors, want 2 to %d", len(pal), maxPaletteSize)
	}
	q := &quantizer{palette: color.Palette(slices.Clone(pal)), gray: true}
	for i, c := range pal {
		if c == nil {
			return nil, fmt.Errorf("demy: Config.Palette[%d] is nil", i)
		}
		if _, ok := c.(RoleColor); ok {
			return nil, fmt.Errorf("demy: Config.Palette[%d] is a role; palettes need concrete colors", i)
		}
		r, g, b, a := c.RGBA()
		if a != 0xffff {
			return nil, fmt.Errorf("demy: Config.Palette[%d] is not opaque", i)
		}
		rgba := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff}
		q.colors = append(q.colors, rgba)
		if rgba.R != rgba.G || rgba.G != rgba.B {
			q.gray = false
		}
	}
	if q.gray {
		for _, c := range q.colors {
			q.levels = append(q.levels, color.Gray{Y: c.R})
		}
		return q, nil
	}
	for _, c := range q.colors {
		q.lab = append(q.lab, okLab(c.R, c.G, c.B))
	}
	q.cache = make([]uint8, 1<<15)
	for r := range 32 {
		for g := range 32 {
			for b := range 32 {
				// Bucket centers.
				q.cache[r<<10|g<<5|b] = q.nearestLab(okLab(uint8(r<<3|4), uint8(g<<3|4), uint8(b<<3|4)))
			}
		}
	}
	// Palette colors always map to themselves.
	for i, c := range q.colors {
		q.cache[cacheKey(c.R, c.G, c.B)] = uint8(i)
	}
	return q, nil
}

var errNoPalette = errors.New("demy: Paletted mode requires Config.Palette")

func cacheKey(r, g, b uint8) int { return int(r>>3)<<10 | int(g>>3)<<5 | int(b>>3) }

// nearest returns the index of the entry closest to c.
func (q *quantizer) nearest(c color.RGBA) uint8 {
	if q.gray {
		return uint8(nearestPalette(float64(luminance(c.R, c.G, c.B)), q.levels))
	}
	return q.nearestLab(okLab(c.R, c.G, c.B))
}

// chromaWeight scales chroma differences against lightness when matching
// colors. Without it, mid grays (such as antialiased edges of black text)
// match a saturated color of similar lightness, like red, rather than black
// or white.
const chromaWeight = 4

func (q *quantizer) nearestLab(c [3]float64) uint8 {
	var nearest int
	minDist := math.Inf(1)
	for i, p := range q.lab {
		dl, da, db := c[0]-p[0], c[1]-p[1], c[2]-p[2]
		if d := dl*dl + chromaWeight*(da*da+db*db); d < minDist {
			minDist = d
			nearest = i
		}
	}
	return uint8(nearest)
}

// srgbToLinear maps an 8-bit sRGB component to linear light.
var srgbToLinear = func() (t [256]float64) {
	for i := range t {
		v := float64(i) / 255
		if v <= 0.04045 {
			t[i] = v / 12.92
		} else {
			t[i] = math.Pow((v+0.055)/1.055, 2.4)
		}
	}
	return t
}()

// okLab converts an sRGB color to OKLab.
func okLab(r8, g8, b8 uint8) [3]float64 {
	r, g, b := srgbToLinear[r8], srgbToLinear[g8], srgbToLinear[b8]
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// luminance returns the BT.601 luminance of an sRGB color.
func luminance(r, g, b uint8) uint8 {
	return uint8((19595*uint32(r) + 38470*uint32(g) + 7471*uint32(b) + 1<<15) >> 16)
}

// toGray converts an RGBA image to 8-bit grayscale using BT.601 luminance.
// Translucent pixels are composited over white, matching an unlit e-ink panel.
func toGray(rgba *image.RGBA) *image.Gray {
	bounds := rgba.Bounds()
	gray := image.NewGray(bounds)
	for y := range bounds.Dy() {
		src := rgba.Pix[y*rgba.Stride : y*rgba.Stride+4*bounds.Dx()]
		dst := gray.Pix[y*gray.Stride : y*gray.Stride+bounds.Dx()]
		for x := range dst {
			p := src[4*x : 4*x+4 : 4*x+4]
			// Pix is premultiplied, so adding the uncovered fraction of white
			// composites over a white backdrop.
			white := 255 - uint32(p[3])
			r, g, b := uint32(p[0])+white, uint32(p[1])+white, uint32(p[2])+white
			dst[x] = uint8((19595*r + 38470*g + 7471*b + 1<<15) >> 16)
		}
	}
	return gray
}

// compositeOverWhite makes every pixel of rgba opaque by compositing it over
// white, as toGray does.
func compositeOverWhite(rgba *image.RGBA) {
	bounds := rgba.Bounds()
	for y := range bounds.Dy() {
		row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+4*bounds.Dx()]
		for x := range bounds.Dx() {
			p := row[4*x : 4*x+4 : 4*x+4]
			white := 255 - p[3]
			p[0], p[1], p[2], p[3] = p[0]+white, p[1]+white, p[2]+white, 0xff
		}
	}
}

// paletted quantizes rgba, whose pixels may be translucent, to an image of
// palette indices. rgba is modified.
func (q *quantizer) paletted(rgba *image.RGBA, algo Dither) *image.Paletted {
	var pix []uint8
	if q.gray {
		pix = q.grayIndices(toGray(rgba), algo)
	} else {
		compositeOverWhite(rgba)
		pix = q.colorIndices(rgba, rgba.Bounds(), algo)
	}
	b := rgba.Bounds()
	return &image.Paletted{Pix: pix, Stride: b.Dx(), Rect: b, Palette: slices.Clone(q.palette)}
}

// nearestPalette returns the index of the closest palette entry to v.
func nearestPalette(v float64, palette []color.Gray) int {
	v = min(max(v, 0), 255)
	nearest := 0
	minDist := 256.0
	for i, p := range palette {
		d := v - float64(p.Y)
		if d < 0 {
			d = -d
		}
		if d < minDist {
			minDist = d
			nearest = i
		}
	}
	return nearest
}

// diffusion is one neighbor of an error-diffusion kernel: an offset from the
// current pixel and the fraction of the error it receives.
type diffusion struct {
	dx, dy int
	weight float64
}

var (
	floydSteinbergKernel = []diffusion{{1, 0, 7.0 / 16}, {-1, 1, 3.0 / 16}, {0, 1, 5.0 / 16}, {1, 1, 1.0 / 16}}
	atkinsonKernel       = []diffusion{{1, 0, 1.0 / 8}, {2, 0, 1.0 / 8}, {-1, 1, 1.0 / 8}, {0, 1, 1.0 / 8}, {1, 1, 1.0 / 8}, {0, 2, 1.0 / 8}}
)

// kernelRows is the number of rows a kernel reaches, counting the current one.
const kernelRows = 3

func ditherKernel(algo Dither) []diffusion {
	switch algo {
	case FloydSteinberg:
		return floydSteinbergKernel
	case Atkinson:
		return atkinsonKernel
	default:
		return nil
	}
}

// grayIndices quantizes src by luminance, returning palette indices with a
// stride of src's width.
func (q *quantizer) grayIndices(src *image.Gray, algo Dither) []uint8 {
	if kernel := ditherKernel(algo); kernel != nil {
		return diffuse(src, q.levels, kernel)
	}
	return ditherNone(src, q.levels)
}

// ditherNone maps each pixel of src to the index of its nearest palette entry.
func ditherNone(src *image.Gray, palette []color.Gray) []uint8 {
	var lut [256]uint8
	for v := range lut {
		lut[v] = uint8(nearestPalette(float64(v), palette))
	}
	bounds := src.Bounds()
	w := bounds.Dx()
	out := make([]uint8, w*bounds.Dy())
	for y := range bounds.Dy() {
		in := src.Pix[y*src.Stride : y*src.Stride+w]
		o := out[y*w : (y+1)*w]
		for x, v := range in {
			o[x] = lut[v]
		}
	}
	return out
}

// diffuse quantizes src to palette indices, spreading each pixel's
// quantization error to its neighbors according to kernel. Error falling
// outside the image is dropped.
func diffuse(src *image.Gray, palette []color.Gray, kernel []diffusion) []uint8 {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	vals := make([]float64, w*h)
	for y := range h {
		for x, v := range src.Pix[y*src.Stride : y*src.Stride+w] {
			vals[y*w+x] = float64(v)
		}
	}

	out := make([]uint8, w*h)
	for y := range h {
		o := out[y*w : (y+1)*w]
		for x := range w {
			old := vals[y*w+x]
			i := nearestPalette(old, palette)
			o[x] = uint8(i)
			e := old - float64(palette[i].Y)
			for _, k := range kernel {
				nx, ny := x+k.dx, y+k.dy
				if nx >= 0 && nx < w && ny < h {
					vals[ny*w+nx] += e * k.weight
				}
			}
		}
	}
	return out
}

// colorErrLimit bounds how far diffused error can push a component past
// [0, 255]. Without it, a color the palette can't approach (yellow with only
// black, white, and red) accumulates error that streaks across the image.
const colorErrLimit = 64

// colorIndices quantizes the opaque pixels of src within r to palette
// indices, with a stride of r's width. Error diffuses per sRGB component and
// is kept for only kernelRows rows at a time.
func (q *quantizer) colorIndices(src *image.RGBA, r image.Rectangle, algo Dither) []uint8 {
	w, h := r.Dx(), r.Dy()
	out := make([]uint8, w*h)
	kernel := ditherKernel(algo)
	if kernel == nil {
		for y := range h {
			i := src.PixOffset(r.Min.X, r.Min.Y+y)
			row := src.Pix[i : i+4*w]
			o := out[y*w : (y+1)*w]
			for x := range o {
				p := row[4*x : 4*x+3 : 4*x+3]
				o[x] = q.cache[cacheKey(p[0], p[1], p[2])]
			}
		}
		return out
	}

	// Rows of accumulated error, padded by 2 pixels on each side so kernel
	// offsets need no bounds checks.
	const pad = 2
	var errs [kernelRows][]float64
	for i := range errs {
		errs[i] = make([]float64, 3*(w+2*pad))
	}
	for y := range h {
		cur := errs[y%kernelRows]
		i := src.PixOffset(r.Min.X, r.Min.Y+y)
		row := src.Pix[i : i+4*w]
		o := out[y*w : (y+1)*w]
		for x := range o {
			var v [3]float64
			var c [3]uint8
			for ch := range 3 {
				v[ch] = min(max(float64(row[4*x+ch])+cur[3*(x+pad)+ch], -colorErrLimit), 255+colorErrLimit)
				c[ch] = uint8(math.Round(min(max(v[ch], 0), 255)))
			}
			idx := q.cache[cacheKey(c[0], c[1], c[2])]
			o[x] = idx
			pc := q.colors[idx]
			e := [3]float64{v[0] - float64(pc.R), v[1] - float64(pc.G), v[2] - float64(pc.B)}
			for _, k := range kernel {
				if y+k.dy >= h {
					continue
				}
				nx := x + k.dx
				if nx < 0 || nx >= w {
					continue
				}
				dst := errs[(y+k.dy)%kernelRows][3*(nx+pad):]
				for ch := range 3 {
					dst[ch] += e[ch] * k.weight
				}
			}
		}
		// This row's buffer is reused for row y+kernelRows.
		clear(cur)
	}
	return out
}

// ditherRGBA dithers the region r of img to the palette in place. Each pixel
// is dithered on its own (unpremultiplied) color and keeps its alpha, so a
// translucent image still blends with what is drawn beneath it.
func (q *quantizer) ditherRGBA(img *image.RGBA, r image.Rectangle, algo Dither) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	var idx []uint8
	if q.gray {
		gray := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
		for y := range r.Dy() {
			i := img.PixOffset(r.Min.X, r.Min.Y+y)
			src := img.Pix[i : i+4*r.Dx()]
			dst := gray.Pix[y*gray.Stride : y*gray.Stride+r.Dx()]
			for x := range dst {
				p := src[4*x : 4*x+4 : 4*x+4]
				a := uint32(p[3])
				if a == 0 {
					dst[x] = 255
					continue
				}
				lum := (19595*uint32(p[0]) + 38470*uint32(p[1]) + 7471*uint32(p[2]) + 1<<15) >> 16
				dst[x] = uint8(min((lum*255+a/2)/a, 255))
			}
		}
		idx = q.grayIndices(gray, algo)
	} else {
		straight := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		for y := range r.Dy() {
			i := img.PixOffset(r.Min.X, r.Min.Y+y)
			src := img.Pix[i : i+4*r.Dx()]
			dst := straight.Pix[y*straight.Stride : y*straight.Stride+4*r.Dx()]
			for x := range r.Dx() {
				p := src[4*x : 4*x+4 : 4*x+4]
				d := dst[4*x : 4*x+4 : 4*x+4]
				a := uint32(p[3])
				if a == 0 {
					d[0], d[1], d[2], d[3] = 255, 255, 255, 255
					continue
				}
				for ch := range 3 {
					d[ch] = uint8(min((uint32(p[ch])*255+a/2)/a, 255))
				}
				d[3] = 255
			}
		}
		idx = q.colorIndices(straight, straight.Bounds(), algo)
	}
	w := r.Dx()
	for y := range r.Dy() {
		i := img.PixOffset(r.Min.X, r.Min.Y+y)
		dst := img.Pix[i : i+4*w]
		for x, v := range idx[y*w : (y+1)*w] {
			p := dst[4*x : 4*x+4 : 4*x+4]
			a := uint32(p[3])
			c := q.colors[v]
			p[0] = uint8((uint32(c.R)*a + 127) / 255)
			p[1] = uint8((uint32(c.G)*a + 127) / 255)
			p[2] = uint8((uint32(c.B)*a + 127) / 255)
		}
	}
}
