package main

import (
	"fmt"
	"io"

	"github.com/samuel/pate"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func renderWeather(w io.Writer, data *weatherData, location string) error {
	font := pate.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, pate.FontBold); err != nil {
		return fmt.Errorf("loading bold font: %w", err)
	}

	cfg := pate.TRMNLOriginal
	cfg.Fonts = map[string]*pate.FontFamily{"sans": font}
	c, err := pate.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	sTitle := pate.Style{Font: "sans", Size: 18, Weight: pate.FontBold}
	sLocation := pate.Style{Font: "sans", Size: 12, Color: pate.DarkGray}
	sTempBig := pate.Style{Font: "sans", Size: 64, Weight: pate.FontBold, Align: pate.TextAlignCenter}
	sTempUnit := pate.Style{Font: "sans", Size: 20, Weight: pate.FontBold}
	sCondition := pate.Style{Font: "sans", Size: 13, Align: pate.TextAlignCenter}
	sSub := pate.Style{Font: "sans", Size: 11, Color: pate.DarkGray, Align: pate.TextAlignCenter}
	sDetailLabel := pate.Style{Font: "sans", Size: 11, Color: pate.DarkGray}
	sDetailValue := pate.Style{Font: "sans", Size: 11, Weight: pate.FontBold, Align: pate.TextAlignRight}
	sForecastDay := pate.Style{Font: "sans", Size: 10, Weight: pate.FontBold, Align: pate.TextAlignCenter}
	sForecastHigh := pate.Style{Font: "sans", Size: 12, Weight: pate.FontBold, Align: pate.TextAlignCenter}
	sForecastLow := pate.Style{Font: "sans", Size: 10, Color: pate.DarkGray, Align: pate.TextAlignCenter}

	tempStr := fmt.Sprintf("%.0f", data.tempC)
	condition := wmoDescription(data.code)
	feelsLike := fmt.Sprintf("Feels like %.0f%s", data.apparentTemp, data.tempUnit)

	windStr := fmt.Sprintf("%.0f %s %s", data.windSpeed, data.windUnit, windDirStr(data.windDir))
	humidStr := fmt.Sprintf("%d%%", data.humidity)
	uvStr := fmt.Sprintf("%.0f", data.uvIndex)
	precipStr := fmt.Sprintf("%.2f %s", data.precip, data.precipUnit)

	// Header: title + optional location.
	headerOpts := []pate.Option{
		pate.Text("WEATHER", sTitle),
		pate.Spacer(),
	}
	if location != "" {
		headerOpts = append(headerOpts, pate.Text(location, sLocation))
	}

	// Left column: big temp + condition + optional today high/low.
	leftOpts := []pate.Option{
		pate.AlignCenter, pate.Gap(6),
		pate.HStack(pate.AlignEnd, pate.Gap(4),
			pate.Text(tempStr, sTempBig),
			pate.VStack(
				pate.Text(data.tempUnit, sTempUnit),
				pate.Spacer(),
			),
		),
		pate.Text(condition, sCondition),
		pate.Text(feelsLike, sSub),
	}
	if len(data.daily) > 0 {
		hlStr := fmt.Sprintf("H: %.0f%s   L: %.0f%s",
			data.daily[0].high, data.tempUnit,
			data.daily[0].low, data.tempUnit)
		leftOpts = append(leftOpts, pate.Text(hlStr, sSub))
	}

	// Forecast columns: day label / high / low with spacers between.
	forecastCols := make([]pate.Node, 0, len(data.daily)*2)
	for i, day := range data.daily {
		forecastCols = append(forecastCols, pate.VStack(
			pate.AlignCenter, pate.Gap(3),
			pate.Text(day.date.Format("Mon"), sForecastDay),
			pate.Text(fmt.Sprintf("%.0f°", day.high), sForecastHigh),
			pate.Text(fmt.Sprintf("%.0f°", day.low), sForecastLow),
		))
		if i < len(data.daily)-1 {
			forecastCols = append(forecastCols, pate.Spacer())
		}
	}

	root := pate.VStack(pate.Padding(16), pate.Gap(0),
		pate.HStack(headerOpts...),
		pate.Box(pate.Padding(6)),
		pate.Divider(),
		pate.Box(pate.Padding(6)),
		// Body: current conditions left, detail sidebar right.
		pate.HStack(pate.AlignCenter, pate.Gap(16),
			pate.VStack(leftOpts...),
			pate.Spacer(),
			// Frame ensures minimum width so label+value spacers can expand.
			pate.Frame(240, 220,
				pate.Box(pate.Border(1, pate.DarkGray), pate.Padding(12), pate.CornerRadius(4),
					pate.VStack(pate.Gap(12),
						detailRow("Humidity", humidStr, sDetailLabel, sDetailValue),
						detailRow("Wind", windStr, sDetailLabel, sDetailValue),
						detailRow("UV Index", uvStr, sDetailLabel, sDetailValue),
						detailRow("Precip", precipStr, sDetailLabel, sDetailValue),
						pate.Box(pate.Padding(2)),
						pate.Text("Updated "+data.fetchedAt.Format("3:04 PM"), pate.Style{
							Font: "sans", Size: 9, Color: pate.LightGray, Align: pate.TextAlignRight,
						}),
					),
				),
			),
		),
		pate.Box(pate.Padding(6)),
		pate.Divider(),
		pate.Box(pate.Padding(6)),
		pate.HStack(pate.Children(forecastCols...)),
	)

	return c.EncodePNG(root, w, pate.TwoBit)
}
