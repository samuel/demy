package main

import (
	"fmt"
	"io"
	"os"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

func renderWeather(w io.Writer, data *weatherData, location string) error {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return fmt.Errorf("loading bold font: %w", err)
	}

	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	c, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	sTitle := demy.Style{Font: "sans", Size: 18, Weight: demy.FontBold}
	sLocation := demy.Style{Font: "sans", Size: 12, Color: demy.DarkGray}
	sTempBig := demy.Style{Font: "sans", Size: 64, Weight: demy.FontBold, Align: demy.TextAlignCenter}
	sTempUnit := demy.Style{Font: "sans", Size: 20, Weight: demy.FontBold}
	sCondition := demy.Style{Font: "sans", Size: 13, Align: demy.TextAlignCenter}
	sSub := demy.Style{Font: "sans", Size: 11, Color: demy.DarkGray, Align: demy.TextAlignCenter}
	sDetailLabel := demy.Style{Font: "sans", Size: 11, Color: demy.DarkGray}
	sDetailValue := demy.Style{Font: "sans", Size: 11, Weight: demy.FontBold, Align: demy.TextAlignRight}
	sForecastDay := demy.Style{Font: "sans", Size: 10, Weight: demy.FontBold, Align: demy.TextAlignCenter}
	sForecastHigh := demy.Style{Font: "sans", Size: 12, Weight: demy.FontBold, Align: demy.TextAlignCenter}
	sForecastLow := demy.Style{Font: "sans", Size: 10, Color: demy.DarkGray, Align: demy.TextAlignCenter}

	tempStr := fmt.Sprintf("%.0f", data.tempC)
	condition := wmoDescription(data.code)
	feelsLike := fmt.Sprintf("Feels like %.0f%s", data.apparentTemp, data.tempUnit)

	windStr := fmt.Sprintf("%.0f %s %s", data.windSpeed, data.windUnit, windDirStr(data.windDir))
	humidStr := fmt.Sprintf("%d%%", data.humidity)
	uvStr := fmt.Sprintf("%.0f", data.uvIndex)
	precipStr := fmt.Sprintf("%.2f %s", data.precip, data.precipUnit)

	// Header: title + optional location.
	headerOpts := []demy.Option{
		demy.Text("WEATHER", sTitle),
		demy.Spacer(),
	}
	if location != "" {
		headerOpts = append(headerOpts, demy.Text(location, sLocation))
	}

	// Left column: big temp + condition + optional today high/low.
	leftOpts := []demy.Option{
		demy.AlignCenter, demy.Gap(6),
		demy.HStack(demy.AlignEnd, demy.Gap(4),
			demy.Text(tempStr, sTempBig),
			demy.VStack(
				demy.Text(data.tempUnit, sTempUnit),
				demy.Spacer(),
			),
		),
		demy.Text(condition, sCondition),
		demy.Text(feelsLike, sSub),
	}
	if len(data.daily) > 0 {
		hlStr := fmt.Sprintf("H: %.0f%s   L: %.0f%s",
			data.daily[0].high, data.tempUnit,
			data.daily[0].low, data.tempUnit)
		leftOpts = append(leftOpts, demy.Text(hlStr, sSub))
	}

	// Forecast columns: day label / high / low with spacers between.
	forecastCols := make([]demy.Node, 0, len(data.daily)*2)
	for i, day := range data.daily {
		forecastCols = append(forecastCols, demy.VStack(
			demy.AlignCenter, demy.Gap(3),
			demy.Text(day.date.Format("Mon"), sForecastDay),
			demy.Text(fmt.Sprintf("%.0f°", day.high), sForecastHigh),
			demy.Text(fmt.Sprintf("%.0f°", day.low), sForecastLow),
		))
		if i < len(data.daily)-1 {
			forecastCols = append(forecastCols, demy.Spacer())
		}
	}

	root := demy.VStack(demy.Padding(16), demy.Gap(0),
		demy.HStack(headerOpts...),
		demy.Box(demy.Padding(6)),
		demy.Divider(),
		demy.Box(demy.Padding(6)),
		// Body: current conditions left, detail sidebar right.
		demy.HStack(demy.AlignCenter, demy.Gap(16),
			demy.VStack(leftOpts...),
			demy.Spacer(),
			// Frame ensures minimum width so label+value spacers can expand.
			demy.Frame(240, 220,
				demy.Box(demy.Border(1, demy.DarkGray), demy.Padding(12), demy.CornerRadius(4),
					demy.VStack(demy.Gap(12),
						detailRow("Humidity", humidStr, sDetailLabel, sDetailValue),
						detailRow("Wind", windStr, sDetailLabel, sDetailValue),
						detailRow("UV Index", uvStr, sDetailLabel, sDetailValue),
						detailRow("Precip", precipStr, sDetailLabel, sDetailValue),
						demy.Box(demy.Padding(2)),
						demy.Text("Updated "+data.fetchedAt.Format("3:04 PM"), demy.Style{
							Font: "sans", Size: 9, Color: demy.LightGray, Align: demy.TextAlignRight,
						}),
					),
				),
			),
		),
		demy.Box(demy.Padding(6)),
		demy.Divider(),
		demy.Box(demy.Padding(6)),
		demy.HStack(demy.Children(forecastCols...)),
	)

	b, err := xmlscene.Marshal(root)
	_ = os.WriteFile("scene.xml", b, 0644)

	return c.EncodePNG(root, w, demy.TwoBit)
}
