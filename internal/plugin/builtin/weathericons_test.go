package builtin

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samuel/demy"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// allConditionSlugs is the set of Home Assistant condition slugs that conditionIconKey
// maps to a non-empty icon key; every resulting key must rasterize.
var allConditionSlugs = []string{
	"sunny", "clear-night", "partlycloudy", "cloudy", "fog",
	"rainy", "snowy", "hail", "lightning", "windy", "exceptional",
}

func TestLoadWeatherIcons(t *testing.T) {
	icons := map[string]image.Image{}
	loadWeatherIcons(icons)

	dumpDir := filepath.Join(os.TempDir(), "demy-weathericons")
	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Logf("icon PNGs written to %s", dumpDir)

	for _, slug := range allConditionSlugs {
		key := conditionIconKey(slug)
		if key == "" {
			t.Errorf("condition %q has no icon key", slug)
			continue
		}
		img, ok := icons[key]
		if !ok || img == nil {
			t.Errorf("missing rasterized icon for key %q (condition %q)", key, slug)
			continue
		}
		b := img.Bounds()
		if b.Dx() != iconMasterSize || b.Dy() != iconMasterSize {
			t.Errorf("icon %q: bounds = %dx%d, want %dx%d", key, b.Dx(), b.Dy(), iconMasterSize, iconMasterSize)
		}
		if dark := darkPixelCount(img); dark < 50 {
			t.Errorf("icon %q: only %d dark pixels — likely rasterized empty", key, dark)
		}
		writePNG(t, filepath.Join(dumpDir, key+".png"), img)
	}
}

// TestRenderWeatherScene is a smoke test that renders the full weather scene for both
// device profiles and writes the PNGs for visual inspection of icon crispness.
func TestRenderWeatherScene(t *testing.T) {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		t.Fatal(err)
	}

	data := &weatherData{
		conditionSlug: "partlycloudy",
		condition:     "Partly Cloudy",
		tempC:         72,
		apparentTemp:  70,
		humidity:      54,
		windSpeed:     8,
		windDir:       225,
		uvIndex:       5,
		precip:        0.02,
		tempUnit:      "°F",
		windUnit:      "mph",
		precipUnit:    "in",
		fetchedAt:     time.Date(2026, 6, 2, 14, 30, 0, 0, time.UTC),
		daily: []dayForecast{
			{date: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC), high: 75, low: 58, condition: "partlycloudy"},
			{date: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), high: 80, low: 61, condition: "sunny"},
			{date: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC), high: 68, low: 55, condition: "rainy"},
			{date: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC), high: 64, low: 50, condition: "lightning"},
			{date: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), high: 70, low: 53, condition: "windy"},
		},
	}

	dumpDir := filepath.Join(os.TempDir(), "demy-weathericons")
	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for name, cfg := range map[string]demy.Config{
		"trmnl-original": demy.TRMNLOriginal,
		"trmnl-x":        demy.TRMNLX,
	} {
		cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
		var buf bytes.Buffer
		if err := renderWeather(&buf, data, "San Francisco", time.UTC, cfg); err != nil {
			t.Fatalf("renderWeather(%s): %v", name, err)
		}
		out := filepath.Join(dumpDir, "scene-"+name+".png")
		if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rendered %s scene to %s", name, out)
	}
}

// darkPixelCount returns the number of pixels whose luminance is below 50% — used to
// confirm an icon actually drew its (black) shapes rather than rasterizing empty.
func darkPixelCount(img image.Image) int {
	b := img.Bounds()
	var n int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if (r+g+bl)/3 < 0x8000 {
				n++
			}
		}
	}
	return n
}

func writePNG(t *testing.T, name string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
