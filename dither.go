package demy

import (
	"image"
	"image/color"
)

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

// ditherNone maps each pixel of src to its nearest palette entry.
func ditherNone(src *image.Gray, palette []color.Gray) *image.Gray {
	var lut [256]uint8
	for v := range lut {
		lut[v] = palette[nearestPalette(float64(v), palette)].Y
	}
	bounds := src.Bounds()
	out := image.NewGray(bounds)
	for y := range bounds.Dy() {
		in := src.Pix[y*src.Stride : y*src.Stride+bounds.Dx()]
		o := out.Pix[y*out.Stride : y*out.Stride+bounds.Dx()]
		for x, v := range in {
			o[x] = lut[v]
		}
	}
	return out
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

// ditherFloydSteinberg applies Floyd-Steinberg error diffusion to src using palette.
func ditherFloydSteinberg(src *image.Gray, palette []color.Gray) *image.Gray {
	return diffuse(src, palette, floydSteinbergKernel)
}

// ditherAtkinson applies Atkinson error diffusion to src using palette.
func ditherAtkinson(src *image.Gray, palette []color.Gray) *image.Gray {
	return diffuse(src, palette, atkinsonKernel)
}

// diffuse quantizes src to palette, spreading each pixel's quantization error
// to its neighbors according to kernel. Error falling outside the image is
// dropped.
func diffuse(src *image.Gray, palette []color.Gray, kernel []diffusion) *image.Gray {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	vals := make([]float64, w*h)
	for y := range h {
		for x, v := range src.Pix[y*src.Stride : y*src.Stride+w] {
			vals[y*w+x] = float64(v)
		}
	}

	out := image.NewGray(bounds)
	for y := range h {
		o := out.Pix[y*out.Stride : y*out.Stride+w]
		for x := range w {
			old := vals[y*w+x]
			q := palette[nearestPalette(old, palette)].Y
			o[x] = q
			e := old - float64(q)
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
