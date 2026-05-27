package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/samuel/demy"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func TestRenderWeather(t *testing.T) {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		t.Fatal(err)
	}
	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}

	b := &bytes.Buffer{}
	now := time.Unix(1622537600, 0)
	if err := renderWeather(b, &weatherData{
		condition:    "Partly Cloudy",
		tempC:        20.1,
		apparentTemp: 21.2,
		humidity:     35,
		windSpeed:    10.3,
		windDir:      220,
		windUnit:     "mph",
		tempUnit:     "F",
		uvIndex:      2.3,
		precipUnit:   "inch",
		daily: []dayForecast{
			{date: now.Add(-7 * time.Hour * 24), high: 21.3, low: 11.2},
			{date: now.Add(-6 * time.Hour * 24), high: 28.3, low: 28.2},
			{date: now.Add(-5 * time.Hour * 24), high: 24.3, low: 38.2},
			{date: now.Add(-4 * time.Hour * 24), high: 12.3, low: 98.2},
			{date: now.Add(-3 * time.Hour * 24), high: 42.3, low: 11.2},
			{date: now.Add(-2 * time.Hour * 24), high: 32.3, low: 10.2},
			{date: now.Add(-time.Hour * 24), high: 22.3, low: 48.2},
		},
		fetchedAt: time.Unix(1622537600, 0),
	}, "London", time.Local, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("weather.png", b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}
