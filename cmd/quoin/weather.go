package main

import (
	"fmt"
	"image/png"
	"io"
	"os"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const weatherTempalte = `<scene>
<styles>
  <style name="sub" font="sans" size="11" color="#555555" align="center" />
  <style name="detail-label" font="sans" size="11" color="#555555" />
  <style name="detail-value" font="sans" size="11" weight="bold" 
align="right" />
  <style name="forecast-day" font="sans" size="10" weight="bold" 
align="center" />
  <style name="forecast-high" font="sans" size="12" weight="bold" 
align="center" />
  <style name="forecast-low" font="sans" size="10" color="#555555" align="center" />
</styles>
<vstack padding="16">
	<hstack>
		<text font="sans" size="18" weight="bold">WEATHER</text>
		<spacer weight="1"></spacer>
		<text font="sans" size="12" color="#555555">London</text>
	</hstack>
	<box padding="6"></box>
	<divider></divider>
	<box padding="6"></box>
	<hstack gap="16" align="center">
		<vstack gap="6" align="center">
			<hstack gap="4" align="end">
				<text font="sans" size="64" weight="bold" align="center">20</text>
				<vstack>
					<text font="sans" size="20" weight="bold">F</text>
					<spacer weight="1"></spacer>
				</vstack>
			</hstack>
			<text font="sans" size="13" align="center">Clear Sky</text>
			<text class="sub">Feels like 21F</text>
			<text class="sub">H: 21F   L: 11F</text>
		</vstack>
		<spacer weight="1"></spacer>
		<frame width="240" height="220">
			<box padding="12" border-width="1" border-color="#555555" corner-radius="4">
				<vstack gap="12">
					<hstack>
						<text class="detail-label">Humidity</text>
						<spacer weight="1"></spacer>
						<text class="detail-value">35%</text>
					</hstack>
					<hstack>
						<text class="detail-label">Wind</text>
						<spacer weight="1"></spacer>
						<text class="detail-value">10 mph SW</text>
					</hstack>
					<hstack>
						<text class="detail-label">UV Index</text>
						<spacer weight="1"></spacer>
						<text class="detail-value">2</text>
					</hstack>
					<hstack>
						<text class="detail-label">Precip</text>
						<spacer weight="1"></spacer>
						<text class="detail-value">0.00 inch</text>
					</hstack>
					<box padding="2"></box>
					<text font="sans" size="9" color="#aaaaaa" align="right">Updated 1:53 AM</text>
				</vstack>
			</box>
		</frame>
	</hstack>
	<box padding="6"></box>
	<divider></divider>
	<box padding="6"></box>
	<hstack>
		<vstack gap="3" align="center">
			<text class="forecast-day">Tue</text>
			<text class="forecast-high">21°</text>
			<text class="forecast-low">11°</text>
		</vstack>
		<spacer weight="1"></spacer>
		<vstack gap="3" align="center">
			<text font="sans" size="10" weight="bold" align="center">Wed</text>
			<text font="sans" size="12" weight="bold" align="center">28°</text>
			<text font="sans" size="10" color="#555555" align="center">28°</text>
		</vstack>
		<spacer weight="1"></spacer>
		<vstack gap="3" align="center">
			<text font="sans" size="10" weight="bold" align="center">Thu</text>
			<text font="sans" size="12" weight="bold" align="center">24°</text>
			<text font="sans" size="10" color="#555555" align="center">38°</text>
		</vstack>
		<spacer weight="1"></spacer>
		<vstack gap="3" align="center">
			<text font="sans" size="10" weight="bold" align="center">Fri</text>
			<text font="sans" size="12" weight="bold" align="center">12°</text>
			<text font="sans" size="10" color="#555555" align="center">98°</text>
		</vstack>
		<spacer weight="1"></spacer>
		<vstack gap="3" align="center">
			<text font="sans" size="10" weight="bold" align="center">Sat</text>
			<text font="sans" size="12" weight="bold" align="center">42°</text>
			<text font="sans" size="10" color="#555555" align="center">11°</text>
		</vstack>
		<spacer weight="1"></spacer>
		<vstack gap="3" align="center">
			<text font="sans" size="10" weight="bold" align="center">Sun</text>
			<text font="sans" size="12" weight="bold" align="center">32°</text>
			<text font="sans" size="10" color="#555555" align="center">10°</text>
		</vstack>
		<spacer weight="1"></spacer>
		<vstack gap="3" align="center">
			<text font="sans" size="10" weight="bold" align="center">Mon</text>
			<text font="sans" size="12" weight="bold" align="center">22°</text>
			<text font="sans" size="10" color="#555555" align="center">48°</text>
		</vstack>
	</hstack>
</vstack>
</scene>`

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

	outImg, err := c.Render(root, demy.TwoBit)
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
}
