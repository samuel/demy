package builtin

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"strings"
	"sync"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// iconMasterSize is the pixel size of the rasterized weather icon masters.
//
// Icons are vector-sourced but cached as a single high-resolution raster per
// condition and downsampled per device by the image-fit pipeline. The largest
// on-screen physical use is the TRMNL X current-condition icon (28 logical px ×
// Scale 2 = 56 px); 128 gives comfortable headroom so no device ever upscales
// and the vector source supersamples cleanly, which is how retina displays are
// supported without rendering per device scale.
const iconMasterSize = 128

//go:embed icons/*.svg
var iconFS embed.FS

var (
	iconOnce  sync.Once
	iconCache map[string]image.Image
)

// loadWeatherIcons copies the rasterized monochrome weather condition icons into m,
// keyed by condition (e.g. "weather-sunny"). The icons are rasterized once on first
// use and cached for the lifetime of the process.
func loadWeatherIcons(m map[string]image.Image) {
	iconOnce.Do(func() { iconCache = rasterizeWeatherIcons() })
	maps.Copy(m, iconCache)
}

// rasterizeWeatherIcons rasterizes every embedded icons/*.svg to an iconMasterSize
// square image, keyed by the filename stem. An icon that fails to rasterize is
// logged and skipped; the weather template omits any condition whose key is absent.
func rasterizeWeatherIcons() map[string]image.Image {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		slog.Error("reading embedded weather icons", "err", err)
		return map[string]image.Image{}
	}
	icons := make(map[string]image.Image, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".svg") {
			continue
		}
		data, err := fs.ReadFile(iconFS, path.Join("icons", name))
		if err != nil {
			slog.Error("reading embedded weather icon", "icon", name, "err", err)
			continue
		}
		img, err := rasterizeIcon(data)
		if err != nil {
			slog.Error("rasterizing weather icon", "icon", name, "err", err)
			continue
		}
		icons[strings.TrimSuffix(name, ".svg")] = img
	}
	return icons
}

// rasterizeIcon rasterizes a monochrome SVG to an iconMasterSize square image:
// black shapes (currentColor) on an opaque white background. White fills in the
// source (used as cut-outs) are preserved, so the output is true monochrome.
func rasterizeIcon(data []byte) (image.Image, error) {
	// Resolve currentColor to black; the meteocons monochrome icons rely on it.
	icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(data), "black")
	if err != nil {
		return nil, fmt.Errorf("parsing svg: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, iconMasterSize, iconMasterSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	icon.SetTarget(0, 0, float64(iconMasterSize), float64(iconMasterSize))
	scanner := rasterx.NewScannerGV(iconMasterSize, iconMasterSize, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(iconMasterSize, iconMasterSize, scanner), 1.0)

	return img, nil
}
