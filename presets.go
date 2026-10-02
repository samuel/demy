package demy

import "image/color"

// Palette presets for common e-paper panels. Colored palettes follow the
// order most panels number their colors in, but drivers differ; check the
// panel's documentation and build a palette in its order when it doesn't match.
var (
	PaletteBW     = []color.Color{Black, White}
	PaletteGray4  = []color.Color{Black, DarkGray, LightGray, White}
	PaletteGray16 = func() []color.Color {
		pal := make([]color.Color, 16)
		for i := range pal {
			pal[i] = color.Gray{Y: uint8(i * 0x11)}
		}
		return pal
	}()
	PaletteBWR      = []color.Color{Black, White, Red}
	PaletteBWY      = []color.Color{Black, White, Yellow}
	PaletteBWRY     = []color.Color{Black, White, Yellow, Red}
	PaletteSpectra6 = []color.Color{Black, White, Yellow, Red, Blue, Green}
	PaletteACeP7    = []color.Color{Black, White, Green, Blue, Red, Yellow, Orange}
)

// TRMNLOriginal is the preset Config for the TRMNL Original display (800×480 px, ~163×98 mm).
var TRMNLOriginal = Config{
	PixelWidth:       800,
	PixelHeight:      480,
	PhysicalWidthMM:  163,
	PhysicalHeightMM: 98,
	Scale:            1,
	ColorMode:        Paletted,
	Palette:          PaletteGray4,
	Roles: Roles{
		Foreground: Black,
		Background: White,
		Accent:     DarkGray,
		Muted:      LightGray,
	},
}

// TRMNLX is the preset Config for the TRMNL X display (1872×1407 px, ~209×157 mm).
var TRMNLX = Config{
	PixelWidth:       1872,
	PixelHeight:      1407,
	PhysicalWidthMM:  209,
	PhysicalHeightMM: 157,
	Scale:            2,
	ColorMode:        Paletted,
	Palette:          PaletteGray16,
	Roles: Roles{
		Foreground: Black,
		Background: White,
		Accent:     DarkGray,
		Muted:      LightGray,
	},
}
