package main

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/samuel/demy"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestRenderXKCD(t *testing.T) {
	// Synthetic grayscale image standing in for a real xkcd comic.
	comicImg := image.NewGray(image.Rect(0, 0, 400, 200))
	for y := range 200 {
		for x := range 400 {
			v := uint8((x + y) % 256)
			comicImg.SetGray(x, y, color.Gray{Y: v})
		}
	}
	// f, err := os.Open("flag_design_2x.png")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// defer f.Close()
	// comicImg, err := png.Decode(f)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		t.Fatal(err)
	}
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	cfg.Images = map[string]image.Image{"comic": comicImg}

	comic := &xkcdComic{
		title:    "Dependency",
		altText:  "Someday ImageMagick will finally break for good and we'll have a long period of scrambling as everyone tries to show that their project is the one that gets to replace it.",
		comicNum: "2347",
		imageURL: "",
	}

	b := &bytes.Buffer{}
	if err := renderXKCD(b, comic, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("xkcd.png", b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}
