package demy_test

import (
	"fmt"
	"image"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/samuel/demy"
)

func Example() {
	sans := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := sans.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		log.Fatal(err)
	}
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": sans}
	cfg.DefaultTextStyle = demy.Style{Font: "sans", Size: 12}

	root := demy.VStack(demy.Padding(10), demy.Gap(8),
		demy.Box(demy.Background(demy.Black), demy.Frame(100, 40)),
		demy.Text("Temperature", demy.Style{Weight: demy.FontBold}),
		demy.HStack(demy.Text("23.4 °C"), demy.Spacer(), demy.Text("Portland")),
	)
	img, err := demy.MustNew(cfg).Render(root, demy.Unset) // cfg.ColorMode: TwoBit
	if err != nil {
		log.Fatal(err)
	}
	gray := img.(*image.Gray) //nolint:forcetypeassert // quantized modes render *image.Gray
	fmt.Println(img.Bounds())
	fmt.Println("inside the box:", gray.GrayAt(50, 30).Y)
	fmt.Println("margin:", gray.GrayAt(5, 5).Y)
	// Output:
	// (0,0)-(800,480)
	// inside the box: 0
	// margin: 255
}

// Children spreads a slice built at runtime into a container's children.
func ExampleChildren() {
	items := []string{"Milk", "Eggs", "Bread"}
	var rows []demy.Node
	for _, item := range items {
		rows = append(rows, demy.Text(item))
	}
	list := demy.VStack(demy.Gap(4), demy.Children(rows...))
	fmt.Println(len(list.Children), "rows")
	// Output: 3 rows
}

func ExampleGrid() {
	grid := demy.Grid(
		demy.Columns(demy.GridFixed(120), demy.GridAuto(), demy.GridFr(1)),
		demy.Gap(4),
		demy.Text("Name"), demy.Text("Age"), demy.Text("Email"),
		demy.Text("Alice"), demy.Text("30"), demy.Text("alice@example.com"),
	)
	for _, c := range grid.Columns {
		switch {
		case c.IsFixed():
			fmt.Println("fixed", c.Size())
		case c.IsAuto():
			fmt.Println("auto")
		case c.IsFr():
			fmt.Println("fr", c.Weight())
		}
	}
	// Output:
	// fixed 120
	// auto
	// fr 1
}

// Unit helpers convert physical sizes to logical pixels before a Canvas exists.
func ExampleConfig_MM() {
	cfg := demy.TRMNLX
	fmt.Printf("scale %.0f, 10 mm = %.1f px, 12 pt = %.1f px\n", cfg.EffectiveScale(), cfg.MM(10), cfg.Pt(12))
	// Output: scale 2, 10 mm = 44.8 px, 12 pt = 19.0 px
}

// An image can opt into dithering while the rest of the scene, such as text,
// is quantized without it.
func ExampleDither() {
	photo := image.NewGray(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			photo.Pix[y*photo.Stride+x] = uint8(x * 4) // horizontal gradient
		}
	}
	cfg := demy.Config{PixelWidth: 64, PixelHeight: 64, PhysicalWidthMM: 64, PhysicalHeightMM: 64}
	img, err := demy.MustNew(cfg).Render(demy.Image(photo, demy.Atkinson), demy.OneBit)
	if err != nil {
		log.Fatal(err)
	}
	levels := map[uint8]bool{}
	for _, v := range img.(*image.Gray).Pix { //nolint:forcetypeassert // OneBit renders *image.Gray
		levels[v] = true
	}
	fmt.Println(len(levels), "gray levels:", levels[0], levels[255])
	// Output: 2 gray levels: true true
}
