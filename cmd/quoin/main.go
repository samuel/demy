package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/deviceapi"
	"golang.org/x/image/font/gofont/goregular"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	imageFile := flag.String("image", "", "path to image file to use as display content")
	fontFile := flag.String("font", "", "path to TTF font file for alt text")
	baseURL := flag.String("base-url", "http://localhost:8080", "base URL of this server, used in image_url responses")
	refreshRate := flag.Int("refresh-rate", 300, "display refresh rate in seconds")

	weatherLat := flag.Float64("weather-lat", 0, "latitude for weather display (enables weather mode)")
	weatherLon := flag.Float64("weather-lon", 0, "longitude for weather display")
	weatherLocation := flag.String("weather-location", "", "location name for weather display (e.g. 'San Francisco, CA')")
	weatherUnits := flag.String("weather-units", "imperial", "units for weather display: imperial or metric")
	weatherTTL := flag.Int("weather-ttl", 900, "weather cache TTL in seconds")
	flag.Parse()

	imageName := "weather-1.png"
	screenPath := "/screen/weather.png" // + imageName
	screenURL := *baseURL + screenPath

	weatherMode := *weatherLat != 0 || *weatherLon != 0

	var displayPNG []byte

	if weatherMode {
		wc := weatherCfg{
			lat:      *weatherLat,
			lon:      *weatherLon,
			location: *weatherLocation,
			units:    *weatherUnits,
			ttl:      time.Duration(*weatherTTL) * time.Second,
		}
		cache := &weatherCache{cfg: wc}
		if err := cache.refresh(context.Background()); err != nil {
			log.Fatalf("initial weather fetch failed: %v", err)
		}

		mux := http.NewServeMux()
		mux.HandleFunc("GET "+screenPath, func(w http.ResponseWriter, r *http.Request) {
			png, err := cache.png(r.Context())
			if err != nil {
				slog.Error("weather render failed", "err", err)
				http.Error(w, "render error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		})
		deviceapi.Register(mux, deviceapi.Handler{
			OnDisplay: func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.DisplayResponse, error) {
				logDevice("display request", device)
				return deviceapi.DisplayResponse{
					Filename:    imageName,
					ImageURL:    screenURL,
					RefreshRate: *refreshRate,
				}, nil
			},
			OnLog: func(ctx context.Context, device deviceapi.DeviceHeaders, entries []deviceapi.LogEntry) error {
				logDevice("device log batch", device)
				for _, e := range entries {
					slog.Info("device log entry",
						"device_id", device.ID,
						"log_id", e.ID,
						"message", e.Message,
						"wifi_status", e.WiFiStatus,
						"wake_reason", e.WakeReason,
						"firmware_version", e.FirmwareVersion,
						"battery_voltage", e.BatteryVoltage,
						"wifi_signal", e.WiFiSignal,
						"free_heap_size", e.FreeHeapSize,
						"special_function", e.SpecialFunction,
					)
				}
				return nil
			},
			OnSetup: func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.SetupResponse, error) {
				logDevice("setup request", device)
				return deviceapi.SetupResponse{
					FriendlyID: device.ID,
					ImageURL:   screenURL,
					Message:    "Welcome to demyd",
				}, nil
			},
		})
		srv := &http.Server{
			Addr:           *addr,
			Handler:        requestLogger(mux),
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
		}
		slog.Info("demyd listening", "addr", *addr, "lat", *weatherLat, "lon", *weatherLon)
		log.Fatal(srv.ListenAndServe())
		return
	}

	if *imageFile != "" {
		var buf bytes.Buffer
		if err := renderDisplay(&buf, *imageFile, *fontFile); err != nil {
			log.Fatalf("failed to render display: %v", err)
		}
		displayPNG = buf.Bytes()
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /screen/"+imageName, func(w http.ResponseWriter, r *http.Request) {
		if displayPNG == nil {
			http.Error(w, "no image configured; start demyd with -image <path>", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(displayPNG)
	})

	deviceapi.Register(mux, deviceapi.Handler{
		OnDisplay: func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.DisplayResponse, error) {
			logDevice("display request", device)
			return deviceapi.DisplayResponse{
				Filename:    imageName,
				ImageURL:    screenURL,
				RefreshRate: *refreshRate,
			}, nil
		},
		OnLog: func(ctx context.Context, device deviceapi.DeviceHeaders, entries []deviceapi.LogEntry) error {
			logDevice("device log batch", device)
			for _, e := range entries {
				slog.Info("device log entry",
					"device_id", device.ID,
					"log_id", e.ID,
					"message", e.Message,
					"wifi_status", e.WiFiStatus,
					"wake_reason", e.WakeReason,
					"firmware_version", e.FirmwareVersion,
					"battery_voltage", e.BatteryVoltage,
					"wifi_signal", e.WiFiSignal,
					"free_heap_size", e.FreeHeapSize,
					"special_function", e.SpecialFunction,
				)
			}
			return nil
		},
		OnSetup: func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.SetupResponse, error) {
			logDevice("setup request", device)
			return deviceapi.SetupResponse{
				FriendlyID: device.ID,
				ImageURL:   screenURL,
				Message:    "Welcome to demyd",
			}, nil
		},
	})

	srv := &http.Server{
		Addr:           *addr,
		Handler:        requestLogger(mux),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	slog.Info("demyd listening", "addr", *addr)
	log.Fatal(srv.ListenAndServe())
}

// --- Weather types and cache ---

type weatherCfg struct {
	lat      float64
	lon      float64
	location string
	units    string // "imperial" or "metric"
	ttl      time.Duration
}

type dayForecast struct {
	date time.Time
	high float64
	low  float64
	code int
}

type weatherData struct {
	tempC        float64
	apparentTemp float64
	humidity     int
	code         int
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

type weatherCache struct {
	cfg weatherCfg
	mu  sync.Mutex
	// protected by mu:
	data       *weatherData
	renderedAt time.Time
	pngBytes   []byte
}

func (wc *weatherCache) refresh(ctx context.Context) error {
	data, err := fetchWeather(ctx, wc.cfg)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := renderWeather(&buf, data, wc.cfg.location); err != nil {
		return err
	}
	wc.mu.Lock()
	wc.data = data
	wc.renderedAt = time.Now()
	wc.pngBytes = buf.Bytes()
	wc.mu.Unlock()
	return nil
}

func (wc *weatherCache) png(ctx context.Context) ([]byte, error) {
	wc.mu.Lock()
	stale := time.Since(wc.renderedAt) > wc.cfg.ttl
	wc.mu.Unlock()
	if stale {
		if err := wc.refresh(ctx); err != nil {
			slog.Warn("weather refresh failed, serving stale data", "err", err)
		}
	}
	wc.mu.Lock()
	b := wc.pngBytes
	wc.mu.Unlock()
	return b, nil
}

// --- Open-Meteo API client ---

type openMeteoResp struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		ApparentTemp  float64 `json:"apparent_temperature"`
		Humidity      int     `json:"relative_humidity_2m"`
		WeatherCode   int     `json:"weather_code"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WindDirection int     `json:"wind_direction_10m"`
		UVIndex       float64 `json:"uv_index"`
		Precipitation float64 `json:"precipitation"`
	} `json:"current"`
	Daily struct {
		Time        []string  `json:"time"`
		TempMax     []float64 `json:"temperature_2m_max"`
		TempMin     []float64 `json:"temperature_2m_min"`
		WeatherCode []int     `json:"weather_code"`
	} `json:"daily"`
}

func fetchWeather(ctx context.Context, cfg weatherCfg) (*weatherData, error) {
	tempUnit := "fahrenheit"
	windUnit := "mph"
	precipUnit := "inch"
	if cfg.units == "metric" {
		tempUnit = "celsius"
		windUnit = "kmh"
		precipUnit = "mm"
	}

	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast"+
			"?latitude=%f&longitude=%f"+
			"&current=temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m,wind_direction_10m,uv_index,precipitation"+
			"&daily=temperature_2m_max,temperature_2m_min,weather_code"+
			"&forecast_days=7"+
			"&temperature_unit=%s&wind_speed_unit=%s&precipitation_unit=%s"+
			"&timezone=auto",
		cfg.lat, cfg.lon, tempUnit, windUnit, precipUnit,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating weather request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching weather: %w", err)
	}
	//nolint:errcheck
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned %s", resp.Status)
	}

	var raw openMeteoResp
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding weather response: %w", err)
	}

	tUnit := "°F"
	wUnit := "mph"
	pUnit := "in"
	if cfg.units == "metric" {
		tUnit = "°C"
		wUnit = "km/h"
		pUnit = "mm"
	}

	data := &weatherData{
		tempC:        raw.Current.Temperature,
		apparentTemp: raw.Current.ApparentTemp,
		humidity:     raw.Current.Humidity,
		code:         raw.Current.WeatherCode,
		windSpeed:    raw.Current.WindSpeed,
		windDir:      raw.Current.WindDirection,
		uvIndex:      raw.Current.UVIndex,
		precip:       raw.Current.Precipitation,
		tempUnit:     tUnit,
		windUnit:     wUnit,
		precipUnit:   pUnit,
		fetchedAt:    time.Now(),
	}

	for i, t := range raw.Daily.Time {
		d, err := time.Parse("2006-01-02", t)
		if err != nil {
			continue
		}
		f := dayForecast{date: d}
		if i < len(raw.Daily.TempMax) {
			f.high = raw.Daily.TempMax[i]
		}
		if i < len(raw.Daily.TempMin) {
			f.low = raw.Daily.TempMin[i]
		}
		if i < len(raw.Daily.WeatherCode) {
			f.code = raw.Daily.WeatherCode[i]
		}
		data.daily = append(data.daily, f)
	}

	return data, nil
}

// --- Weather rendering ---

func detailRow(label, value string, labelStyle, valueStyle demy.Style) *demy.HStackNode {
	return demy.HStack(
		demy.Text(label, labelStyle),
		demy.Spacer(),
		demy.Text(value, valueStyle),
	)
}

// wmoDescription maps WMO weather interpretation codes to short descriptions.
func wmoDescription(code int) string {
	switch {
	case code == 0:
		return "Clear Sky"
	case code == 1:
		return "Mainly Clear"
	case code == 2:
		return "Partly Cloudy"
	case code == 3:
		return "Overcast"
	case code == 45 || code == 48:
		return "Foggy"
	case code >= 51 && code <= 55:
		return "Drizzle"
	case code >= 56 && code <= 57:
		return "Freezing Drizzle"
	case code >= 61 && code <= 65:
		return "Rain"
	case code >= 66 && code <= 67:
		return "Freezing Rain"
	case code >= 71 && code <= 75:
		return "Snow"
	case code == 77:
		return "Snow Grains"
	case code >= 80 && code <= 82:
		return "Rain Showers"
	case code >= 85 && code <= 86:
		return "Snow Showers"
	case code == 95:
		return "Thunderstorm"
	case code >= 96 && code <= 99:
		return "Thunderstorm w/ Hail"
	default:
		return "Unknown"
	}
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

// --- Static image display ---

func renderDisplay(w io.Writer, imageFile, fontFile string) error {
	f, err := os.Open(imageFile)
	if err != nil {
		return fmt.Errorf("opening image: %w", err)
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("decoding image: %w", err)
	}

	var font *demy.FontFamily
	if fontFile != "" {
		font, err = demy.LoadFontFile("sans", fontFile)
		if err != nil {
			return fmt.Errorf("loading font: %w", err)
		}
	} else {
		font = demy.MustLoadFontBytes("sans", goregular.TTF)
	}

	cfg := demy.TRMNLOriginal
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	c, err := demy.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	root := demy.ZStack(
		demy.Image(img, demy.ImageFit),
		demy.VStack(
			demy.Spacer(),
			demy.Box(demy.Background(demy.White), demy.Padding(8),
				demy.Text("This is where the alt text would go", demy.Style{
					Font:  "sans",
					Size:  11,
					Color: demy.Black,
					Align: demy.TextAlignCenter,
				}),
			),
		),
	)

	return c.EncodePNG(root, w, demy.TwoBit)
}

// --- Device logging and HTTP middleware ---

func logDevice(msg string, d deviceapi.DeviceHeaders) {
	args := []any{
		"device_id", d.ID,
		"fw_version", d.FWVersion,
		"model", d.Model,
		"host", d.Host,
		"user_agent", d.UserAgent,
		"update_source", d.UpdateSource,
	}
	if d.AccessToken != "" {
		args = append(args, "access_token", d.AccessToken)
	}
	if d.PercentCharged != nil {
		args = append(args, "percent_charged", *d.PercentCharged)
	}
	if d.BatteryVoltage != nil {
		args = append(args, "battery_voltage", *d.BatteryVoltage)
	}
	if d.RSSI != nil {
		args = append(args, "rssi", *d.RSSI)
	}
	if d.Width != nil && d.Height != nil {
		args = append(args, "width", *d.Width, "height", *d.Height)
	}
	if d.RefreshRate != nil {
		args = append(args, "refresh_rate", *d.RefreshRate)
	}
	slog.Info(msg, args...)
}

// responseRecorder captures the HTTP status code written by a handler.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
		)
	})
}
