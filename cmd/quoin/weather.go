package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// xmlEsc escapes s for safe embedding in XML text content.
func xmlEsc(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

const weatherTemplate = `<scene>
<styles>
  <style name="sub" font="sans" size="11" color="#555555" align="center" />
  <style name="detail-label" font="sans" size="11" color="#555555" />
  <style name="detail-value" font="sans" size="11" weight="bold" align="right" />
  <style name="forecast-day" font="sans" size="10" weight="bold" align="center" />
  <style name="forecast-high" font="sans" size="12" weight="bold" align="center" />
  <style name="forecast-low" font="sans" size="10" color="#555555" align="center" />
</styles>
<vstack padding="16">
  <hstack>
    <text font="sans" size="18" weight="bold">WEATHER</text>
    <spacer weight="1"></spacer>
    {{- if .Location}}
    <text font="sans" size="12" color="#555555">{{.Location}}</text>
    {{- end}}
  </hstack>
  <box padding="6"></box>
  <divider></divider>
  <box padding="6"></box>
  <hstack gap="16" align="center">
    <vstack gap="6" align="center">
      <hstack gap="4" align="end">
        <text font="sans" size="64" weight="bold" align="center">{{.Temp}}</text>
        <vstack>
          <text font="sans" size="20" weight="bold">{{.TempUnit}}</text>
          <spacer weight="1"></spacer>
        </vstack>
      </hstack>
      <text font="sans" size="13" align="center">{{.Condition}}</text>
      <text class="sub">{{.FeelsLike}}</text>
      {{- if .HL}}
      <text class="sub">{{.HL}}</text>
      {{- end}}
    </vstack>
    <spacer weight="1"></spacer>
    <frame width="240" height="220">
      <box padding="12" border-width="1" border-color="#555555" corner-radius="4">
        <vstack gap="12">
          <hstack>
            <text class="detail-label">Humidity</text>
            <spacer weight="1"></spacer>
            <text class="detail-value">{{.Humidity}}</text>
          </hstack>
          <hstack>
            <text class="detail-label">Wind</text>
            <spacer weight="1"></spacer>
            <text class="detail-value">{{.Wind}}</text>
          </hstack>
          <hstack>
            <text class="detail-label">UV Index</text>
            <spacer weight="1"></spacer>
            <text class="detail-value">{{.UVIndex}}</text>
          </hstack>
          <hstack>
            <text class="detail-label">Precip</text>
            <spacer weight="1"></spacer>
            <text class="detail-value">{{.Precip}}</text>
          </hstack>
          <box padding="2"></box>
          <text font="sans" size="9" color="#aaaaaa" align="right">Updated {{.UpdatedAt}}</text>
        </vstack>
      </box>
    </frame>
  </hstack>
  <box padding="6"></box>
  <divider></divider>
  <box padding="6"></box>
  <hstack>
    {{- range $i, $day := .Daily}}
    {{- if $i}}<spacer weight="1"></spacer>{{end}}
    <vstack gap="3" align="center">
      <text class="forecast-day">{{$day.Day}}</text>
      <text class="forecast-high">{{$day.High}}</text>
      <text class="forecast-low">{{$day.Low}}</text>
    </vstack>
    {{- end}}
  </hstack>
</vstack>
</scene>`

type weatherTemplateData struct {
	Location  string
	Temp      string
	TempUnit  string
	Condition string
	FeelsLike string
	HL        string
	Humidity  string
	Wind      string
	UVIndex   string
	Precip    string
	UpdatedAt string
	Daily     []dailyTemplateData
}

type dailyTemplateData struct {
	Day  string
	High string
	Low  string
}

func renderWeather(w io.Writer, data *weatherData, location string) error {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return fmt.Errorf("loading bold font: %w", err)
	}

	var hl string
	if len(data.daily) > 0 {
		tUnit := xmlEsc(data.tempUnit)
		hl = fmt.Sprintf("H: %.0f%s   L: %.0f%s",
			data.daily[0].high, tUnit,
			data.daily[0].low, tUnit)
	}

	daily := make([]dailyTemplateData, len(data.daily))
	for i, day := range data.daily {
		daily[i] = dailyTemplateData{
			Day:  day.date.Format("Mon"),
			High: fmt.Sprintf("%.0f°", day.high),
			Low:  fmt.Sprintf("%.0f°", day.low),
		}
	}

	tmplData := weatherTemplateData{
		Location:  xmlEsc(location),
		Temp:      fmt.Sprintf("%.0f", data.tempC),
		TempUnit:  xmlEsc(data.tempUnit),
		Condition: xmlEsc(data.condition),
		FeelsLike: fmt.Sprintf("Feels like %.0f%s", data.apparentTemp, xmlEsc(data.tempUnit)),
		HL:        hl,
		Humidity:  fmt.Sprintf("%d%%", data.humidity),
		Wind:      fmt.Sprintf("%.0f %s %s", data.windSpeed, xmlEsc(data.windUnit), windDirStr(data.windDir)),
		UVIndex:   fmt.Sprintf("%.0f", data.uvIndex),
		Precip:    fmt.Sprintf("%.2f %s", data.precip, xmlEsc(data.precipUnit)),
		UpdatedAt: data.fetchedAt.Local().Format("15:04"),
		Daily:     daily,
	}

	root, styles, err := xmlscene.ExecuteScene([]byte(weatherTemplate), tmplData)
	if err != nil {
		return fmt.Errorf("executing weather template: %w", err)
	}

	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	cfg.Styles = styles
	c, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	// b, err := xmlscene.Marshal(root)
	// if err == nil {
	// 	_ = os.WriteFile("scene.xml", b, 0644)
	// }

	outImg, err := c.Render(root, demy.TwoBit)
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
}
