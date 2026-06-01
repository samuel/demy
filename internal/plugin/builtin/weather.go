// Package builtin provides PluginRenderer implementations for the built-in Quoin plugins.
package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"maps"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
	"github.com/samuel/demy"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/plugin"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// WeatherRenderer implements plugin.PluginRenderer for the Home Assistant weather plugin.
type WeatherRenderer struct{}

// Render fetches weather data from Home Assistant and renders it to PNG bytes.
func (r *WeatherRenderer) Render(ctx context.Context, cfg demy.Config, p *model.UserPlugin, _ plugin.ResourceCache) ([]byte, error) {
	var pcfg weatherPluginConfig
	if p.Config != "" {
		if err := json.Unmarshal([]byte(p.Config), &pcfg); err != nil {
			return nil, fmt.Errorf("weather: parsing plugin config: %w", err)
		}
	}
	if pcfg.Entity == "" {
		pcfg.Entity = "weather.home"
	}

	data, err := fetchWeatherFromHA(ctx, pcfg.HAURL, pcfg.HAToken, pcfg.Entity, pcfg.Units)
	if err != nil {
		return nil, err
	}

	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return nil, fmt.Errorf("weather: loading bold font: %w", err)
	}
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}

	loc := time.UTC
	var buf bytes.Buffer
	if err := renderWeather(&buf, data, pcfg.Location, loc, cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type weatherPluginConfig struct {
	HAURL    string `json:"ha_url"`
	HAToken  string `json:"ha_token"`
	Entity   string `json:"entity"`
	Location string `json:"location"`
	Units    string `json:"units"`
}

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
    <frame width="50mm" height="45mm">
      <box padding="12" border-width="1" border-color="#555555" corner-radius="4">
        <vstack gap="2.4mm">
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

type dayForecast struct {
	date time.Time
	high float64
	low  float64
}

type weatherData struct {
	condition    string
	tempC        float64
	apparentTemp float64
	humidity     int
	windSpeed    float64
	windDir      int
	uvIndex      float64
	precip       float64
	tempUnit     string // "°F" or "°C"
	windUnit     string // "mph" or "km/h"
	precipUnit   string // "in" or "mm"
	daily        []dayForecast
	fetchedAt    time.Time
}

func renderWeather(w io.Writer, data *weatherData, location string, loc *time.Location, cfg demy.Config) error {
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
		UpdatedAt: data.fetchedAt.In(loc).Format("15:04"),
		Daily:     daily,
	}

	root, styles, err := xmlscene.ExecuteScene([]byte(weatherTemplate), tmplData, cfg)
	if err != nil {
		return fmt.Errorf("executing weather template: %w", err)
	}

	cfg.Styles = mergeStyles(cfg.Styles, styles)
	c, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	outImg, err := c.Render(root, plugin.EffectiveColorMode(cfg))
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
}

// mergeStyles merges template-defined styles into the base style map.
// Template styles take precedence.
func mergeStyles(base, tmpl map[string]demy.Style) map[string]demy.Style {
	if len(tmpl) == 0 {
		return base
	}
	merged := make(map[string]demy.Style, len(base)+len(tmpl))
	maps.Copy(merged, base)
	maps.Copy(merged, tmpl)
	return merged
}

func fetchWeatherFromHA(ctx context.Context, hassURL, hassToken, entity, units string) (*weatherData, error) {
	client, err := ha.NewClient(hassURL, ha.WithToken(hassToken), ha.WithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("creating HA client: %w", err)
	}

	state, err := client.GetStateForEntity(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("getting HA entity state: %w", err)
	}

	attrs := state.Attributes

	tempUnit := mapString(attrs, "temperature_unit")
	if tempUnit == "" {
		tempUnit = "°C"
	}
	windUnit := mapString(attrs, "wind_speed_unit")
	if windUnit == "" {
		windUnit = "km/h"
	}
	precipUnit := mapString(attrs, "precipitation_unit")
	if precipUnit == "" {
		precipUnit = "mm"
	}

	data := &weatherData{
		condition:    hassConditionDescription(state.State),
		tempC:        mapFloat(attrs, "temperature"),
		apparentTemp: mapFloat(attrs, "apparent_temperature"),
		humidity:     mapInt(attrs, "humidity"),
		windSpeed:    mapFloat(attrs, "wind_speed"),
		windDir:      mapInt(attrs, "wind_bearing"),
		uvIndex:      mapFloat(attrs, "uv_index"),
		precip:       mapFloat(attrs, "precipitation"),
		tempUnit:     tempUnit,
		windUnit:     windUnit,
		precipUnit:   precipUnit,
		fetchedAt:    time.Now(),
	}

	forecasts, err := client.GetWeatherForecasts(ctx, entity, "daily")
	if err != nil {
		slog.Warn("failed to get HA weather forecasts", "err", err)
	} else {
		for _, f := range forecasts.Forecast {
			dtStr := mapString(f, "datetime")
			if dtStr == "" {
				continue
			}
			dt, err := time.Parse(time.RFC3339, dtStr)
			if err != nil {
				dt, err = time.Parse("2006-01-02", dtStr)
				if err != nil {
					continue
				}
			}
			data.daily = append(data.daily, dayForecast{
				date: dt,
				high: mapFloat(f, "temperature"),
				low:  mapFloat(f, "templow"),
			})
		}
	}

	applyUnitConversion(data, units)
	return data, nil
}

func applyUnitConversion(data *weatherData, targetUnits string) {
	if targetUnits == "" {
		return
	}
	wantImperial := targetUnits == "imperial"
	isImperial := data.tempUnit == "°F"
	if wantImperial == isImperial {
		return
	}
	if wantImperial {
		data.tempC = data.tempC*9/5 + 32
		data.apparentTemp = data.apparentTemp*9/5 + 32
		for i := range data.daily {
			data.daily[i].high = data.daily[i].high*9/5 + 32
			data.daily[i].low = data.daily[i].low*9/5 + 32
		}
		data.tempUnit = "°F"

		if data.precipUnit == "mm" {
			data.precip = data.precip / 25.4
			data.precipUnit = "in"
		}
		if data.windUnit == "km/h" {
			data.windSpeed = data.windSpeed * 0.6213711922
			data.windUnit = "mph"
		}
	} else {
		data.tempC = (data.tempC - 32) * 5 / 9
		data.apparentTemp = (data.apparentTemp - 32) * 5 / 9
		for i := range data.daily {
			data.daily[i].high = (data.daily[i].high - 32) * 5 / 9
			data.daily[i].low = (data.daily[i].low - 32) * 5 / 9
		}
		data.tempUnit = "°C"

		if data.precipUnit == "in" {
			data.precip = data.precip * 25.4
			data.precipUnit = "mm"
		}
		if data.windUnit == "mph" {
			data.windSpeed = data.windSpeed * 1.609344
			data.windUnit = "km/h"
		}
	}
}

func hassConditionDescription(s string) string {
	switch s {
	case "sunny":
		return "Sunny"
	case "clear-night":
		return "Clear Night"
	case "partlycloudy":
		return "Partly Cloudy"
	case "cloudy":
		return "Cloudy"
	case "fog":
		return "Foggy"
	case "rainy":
		return "Rainy"
	case "pouring":
		return "Heavy Rain"
	case "snowy":
		return "Snowy"
	case "snowy-rainy":
		return "Sleet"
	case "hail":
		return "Hail"
	case "lightning":
		return "Lightning"
	case "lightning-rainy":
		return "Thunderstorm"
	case "windy", "windy-variant":
		return "Windy"
	case "exceptional":
		return "Exceptional"
	default:
		return s
	}
}

func mapFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func mapInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func mapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// windDirStr converts a wind direction in degrees to a compass abbreviation.
func windDirStr(deg int) string {
	dirs := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	idx := ((deg + 22) % 360) / 45
	if idx < 0 || idx >= len(dirs) {
		return ""
	}
	return dirs[idx]
}
