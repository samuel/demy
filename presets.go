package demy

import "image/color"

// TRMNLOriginal is the preset Config for the TRMNL Original display (800×480 px, ~163×98 mm).
var TRMNLOriginal = Config{
	PixelWidth:       800,
	PixelHeight:      480,
	PhysicalWidthMM:  163,
	PhysicalHeightMM: 98,
	Scale:            1,
	ColorMode:        TwoBit,
	Palette:          []color.Color{Black, DarkGray, LightGray, White},
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
	ColorMode:        FourBit,
	Palette: []color.Color{
		color.Gray{Y: 0x00}, color.Gray{Y: 0x11}, color.Gray{Y: 0x22}, color.Gray{Y: 0x33},
		color.Gray{Y: 0x44}, color.Gray{Y: 0x55}, color.Gray{Y: 0x66}, color.Gray{Y: 0x77},
		color.Gray{Y: 0x88}, color.Gray{Y: 0x99}, color.Gray{Y: 0xAA}, color.Gray{Y: 0xBB},
		color.Gray{Y: 0xCC}, color.Gray{Y: 0xDD}, color.Gray{Y: 0xEE}, color.Gray{Y: 0xFF},
	},
	Roles: Roles{
		Foreground: Black,
		Background: White,
		Accent:     DarkGray,
		Muted:      LightGray,
	},
}
