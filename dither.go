package demy

import (
	"image"
	"image/color"
	"math"
)

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// toGray converts an RGBA image to 8-bit grayscale using luminance weights.
func toGray(rgba *image.RGBA) *image.Gray {
	bounds := rgba.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := rgba.RGBAAt(x, y)
			// ITU-R BT.601 luminance
			lum := (19595*uint32(c.R) + 38470*uint32(c.G) + 7471*uint32(c.B) + 1<<15) >> 16
			gray.SetGray(x, y, color.Gray{Y: uint8(lum)})
		}
	}
	return gray
}

// nearestPalette returns the index of the closest palette entry to val (0–255).
func nearestPalette(val float64, palette []color.Gray) int {
	v := clamp(val, 0, 255)
	nearest := 0
	minDist := math.Abs(v - float64(palette[0].Y))
	for i := 1; i < len(palette); i++ {
		d := math.Abs(v - float64(palette[i].Y))
		if d < minDist {
			minDist = d
			nearest = i
		}
	}
	return nearest
}

func ditherNone(src *image.Gray, palette []color.Gray) *image.Gray {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	out := image.NewGray(bounds)
	for y := range h {
		for x := range w {
			old := src.Pix[y*w+x]
			idx := nearestPalette(float64(old), palette)
			out.SetGray(bounds.Min.X+x, bounds.Min.Y+y, palette[idx])
		}
	}
	return out
}

// ditherFloydSteinberg applies Floyd-Steinberg error diffusion to src using palette.
func ditherFloydSteinberg(src *image.Gray, palette []color.Gray) *image.Gray {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	errs := make([]float64, w*h)
	for i, v := range src.Pix {
		errs[i] = float64(v)
	}

	out := image.NewGray(bounds)
	for y := range h {
		for x := range w {
			old := errs[y*w+x]
			idx := nearestPalette(old, palette)
			out.SetGray(bounds.Min.X+x, bounds.Min.Y+y, palette[idx])
			e := old - float64(palette[idx].Y)
			if x+1 < w {
				errs[y*w+x+1] += e * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					errs[(y+1)*w+x-1] += e * 3 / 16
				}
				errs[(y+1)*w+x] += e * 5 / 16
				if x+1 < w {
					errs[(y+1)*w+x+1] += e * 1 / 16
				}
			}
		}
	}
	return out
}

// ditherAtkinson applies Atkinson error diffusion to src using palette.
// Atkinson retains more white space, often looking sharper at 800×480.
func ditherAtkinson(src *image.Gray, palette []color.Gray) *image.Gray {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	errs := make([]float64, w*h)
	for i, v := range src.Pix {
		errs[i] = float64(v)
	}

	out := image.NewGray(bounds)
	for y := range h {
		for x := range w {
			old := errs[y*w+x]
			idx := nearestPalette(old, palette)
			out.SetGray(bounds.Min.X+x, bounds.Min.Y+y, palette[idx])
			e := (old - float64(palette[idx].Y)) / 8
			if x+1 < w {
				errs[y*w+x+1] += e
			}
			if x+2 < w {
				errs[y*w+x+2] += e
			}
			if y+1 < h {
				if x > 0 {
					errs[(y+1)*w+x-1] += e
				}
				errs[(y+1)*w+x] += e
				if x+1 < w {
					errs[(y+1)*w+x+1] += e
				}
			}
			if y+2 < h {
				errs[(y+2)*w+x] += e
			}
		}
	}
	return out
}
