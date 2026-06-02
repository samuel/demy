package builtin

import (
	"image"
	"image/color"
	"math"
)

const iconSz = 64

// loadWeatherIcons populates m with generated monochrome weather condition icons.
// Each icon is iconSz×iconSz pixels, black on white.
func loadWeatherIcons(m map[string]image.Image) {
	m["weather-sunny"] = iconSunny()
	m["weather-clear-night"] = iconClearNight()
	m["weather-partlycloudy"] = iconPartlyCloudy()
	m["weather-cloudy"] = iconCloudy()
	m["weather-fog"] = iconFog()
	m["weather-rainy"] = iconRainy()
	m["weather-snowy"] = iconSnowy()
	m["weather-hail"] = iconHail()
	m["weather-lightning"] = iconLightning()
	m["weather-windy"] = iconWindy()
	m["weather-exceptional"] = iconExceptional()
}

// ic is a simple pixel canvas for drawing weather icons.
type ic struct {
	img *image.NRGBA
}

func newIc() *ic {
	img := image.NewNRGBA(image.Rect(0, 0, iconSz, iconSz))
	for i := range img.Pix {
		img.Pix[i] = 0xff // opaque white (R=G=B=A=255)
	}
	return &ic{img: img}
}

func (c *ic) set(x, y int) {
	if x >= 0 && x < iconSz && y >= 0 && y < iconSz {
		c.img.SetNRGBA(x, y, color.NRGBA{A: 0xff})
	}
}

func (c *ic) unset(x, y int) {
	if x >= 0 && x < iconSz && y >= 0 && y < iconSz {
		c.img.SetNRGBA(x, y, color.NRGBA{0xff, 0xff, 0xff, 0xff})
	}
}

func (c *ic) filledCircle(cx, cy, r int) {
	r2 := r * r
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r2 {
				c.set(cx+dx, cy+dy)
			}
		}
	}
}

func (c *ic) eraseCircle(cx, cy, r int) {
	r2 := r * r
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r2 {
				c.unset(cx+dx, cy+dy)
			}
		}
	}
}

func (c *ic) circleOutline(cx, cy, r, t int) {
	outer2 := r * r
	inner := r - t
	inner2 := inner * inner
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			d2 := dx*dx + dy*dy
			if d2 <= outer2 && d2 >= inner2 {
				c.set(cx+dx, cy+dy)
			}
		}
	}
}

func (c *ic) filledRect(x0, y0, x1, y1 int) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			c.set(x, y)
		}
	}
}

// line draws a thick line from (x0,y0) to (x1,y1) with given thickness t.
func (c *ic) line(x0, y0, x1, y1, t int) {
	dx := float64(x1 - x0)
	dy := float64(y1 - y0)
	length := math.Sqrt(dx*dx + dy*dy)
	if length < 0.5 {
		c.filledCircle(x0, y0, t/2)
		return
	}
	nx := -dy / length
	ny := dx / length
	steps := int(length*2) + 2
	for i := range steps {
		frac := float64(i) / float64(steps-1)
		px := float64(x0) + frac*dx
		py := float64(y0) + frac*dy
		for j := -t / 2; j <= t/2; j++ {
			c.set(int(px+nx*float64(j)+0.5), int(py+ny*float64(j)+0.5))
		}
	}
}

// cloud draws a 2-bump cloud shape centered at cx, starting at topY.
func (c *ic) cloud(cx, topY, width, height int) {
	bumpR := max(width/4, 4)
	bumpCY := topY + bumpR
	leftCX := cx - bumpR
	rightCX := cx + bumpR

	bodyLeft := leftCX - bumpR
	bodyRight := rightCX + bumpR
	bodyTop := bumpCY
	bodyBottom := topY + height

	c.filledRect(bodyLeft, bodyTop, bodyRight, bodyBottom)
	c.filledCircle(leftCX, bumpCY, bumpR)
	c.filledCircle(rightCX, bumpCY, bumpR)
}

func iconSunny() image.Image {
	c := newIc()
	c.circleOutline(32, 32, 15, 4)
	for i := range 8 {
		angle := float64(i) * math.Pi / 4
		x0 := 32 + int(math.Cos(angle)*20+0.5)
		y0 := 32 + int(math.Sin(angle)*20+0.5)
		x1 := 32 + int(math.Cos(angle)*27+0.5)
		y1 := 32 + int(math.Sin(angle)*27+0.5)
		c.line(x0, y0, x1, y1, 3)
	}
	return c.img
}

func iconClearNight() image.Image {
	c := newIc()
	c.filledCircle(28, 32, 20)
	// Erase a shifted circle to carve out a crescent.
	c.eraseCircle(36, 26, 16)
	return c.img
}

func iconPartlyCloudy() image.Image {
	c := newIc()
	// Small sun in upper-right.
	c.circleOutline(46, 20, 10, 3)
	for i := range 4 {
		angle := float64(i) * math.Pi / 2
		x0 := 46 + int(math.Cos(angle)*14+0.5)
		y0 := 20 + int(math.Sin(angle)*14+0.5)
		x1 := 46 + int(math.Cos(angle)*18+0.5)
		y1 := 20 + int(math.Sin(angle)*18+0.5)
		c.line(x0, y0, x1, y1, 2)
	}
	// Cloud in lower-center, overlapping the sun.
	c.cloud(28, 30, 46, 26)
	return c.img
}

func iconCloudy() image.Image {
	c := newIc()
	c.cloud(32, 10, 54, 36)
	return c.img
}

func iconFog() image.Image {
	c := newIc()
	c.line(10, 18, 54, 18, 4)
	c.line(14, 28, 50, 28, 4)
	c.line(10, 38, 54, 38, 4)
	c.line(14, 48, 50, 48, 4)
	return c.img
}

func iconRainy() image.Image {
	c := newIc()
	c.cloud(32, 6, 50, 22)
	for i, x := range []int{12, 20, 28, 36, 44, 52} {
		y := 36 + (i%2)*4
		c.line(x, y, x-4, y+10, 2)
	}
	return c.img
}

func iconSnowy() image.Image {
	c := newIc()
	c.cloud(32, 6, 50, 22)
	for _, d := range [][2]int{{12, 40}, {22, 46}, {32, 40}, {42, 46}, {52, 40}} {
		c.filledCircle(d[0], d[1], 3)
	}
	return c.img
}

func iconHail() image.Image {
	c := newIc()
	c.cloud(32, 6, 50, 22)
	for _, d := range [][2]int{{16, 42}, {28, 48}, {40, 42}, {52, 48}} {
		c.filledCircle(d[0], d[1], 5)
	}
	return c.img
}

func iconLightning() image.Image {
	c := newIc()
	c.cloud(32, 4, 48, 22)
	// Zigzag lightning bolt.
	pts := [][2]int{{36, 30}, {28, 44}, {35, 44}, {26, 60}}
	for i := 1; i < len(pts); i++ {
		c.line(pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], 4)
	}
	return c.img
}

func iconWindy() image.Image {
	c := newIc()
	c.line(8, 20, 52, 20, 3)
	c.line(52, 20, 57, 14, 3)
	c.line(8, 32, 46, 32, 3)
	c.line(46, 32, 51, 26, 3)
	c.line(8, 44, 52, 44, 3)
	c.line(52, 44, 57, 38, 3)
	return c.img
}

func iconExceptional() image.Image {
	c := newIc()
	c.circleOutline(32, 32, 28, 3)
	c.filledRect(29, 14, 35, 36)
	c.filledCircle(32, 44, 4)
	return c.img
}
