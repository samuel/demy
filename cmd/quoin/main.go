package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	ha "github.com/mkelcik/go-ha-client/v2"
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

	hassURL := flag.String("hass-url", "", "Home Assistant base URL (enables weather and calendar modes)")
	hassToken := flag.String("hass-token", "", "Home Assistant long-lived access token")
	hassEntity := flag.String("hass-entity", "weather.home", "Home Assistant weather entity ID")
	weatherLocation := flag.String("weather-location", "", "location name for weather display (e.g. 'San Francisco, CA')")
	weatherUnits := flag.String("weather-units", "", "override temperature units: imperial or metric (default: use HA native)")
	weatherTTL := flag.Int("weather-ttl", 900, "weather cache TTL in seconds")
	hassCalendars := flag.String("hass-calendars", "", "comma-separated calendar entity IDs (e.g. 'calendar.family,calendar.work')")
	calendarTTL := flag.Int("calendar-ttl", 900, "calendar cache TTL in seconds")
	flag.Parse()

	if *hassToken == "" {
		*hassToken = os.Getenv("HASS_TOKEN")
	}

	weatherMode := *hassURL != "" && *hassToken != "" && *hassEntity != ""
	calendarMode := *hassURL != "" && *hassToken != "" && *hassCalendars != ""

	if weatherMode || calendarMode {
		mux := http.NewServeMux()

		var primaryScreenURL string
		var primaryRefresh func(context.Context) error
		var primaryRenderedAt func() time.Time

		if weatherMode {
			screenPath := "/screen/weather.png"
			screenURL := *baseURL + screenPath
			primaryScreenURL = screenURL
			wc := &weatherCache{cfg: weatherCfg{
				hassURL:    *hassURL,
				hassToken:  *hassToken,
				hassEntity: *hassEntity,
				location:   *weatherLocation,
				units:      *weatherUnits,
				ttl:        time.Duration(*weatherTTL) * time.Second,
			}}
			if err := wc.refresh(context.Background()); err != nil {
				log.Fatalf("initial weather fetch failed: %v", err)
			}
			mux.HandleFunc("GET "+screenPath, func(w http.ResponseWriter, r *http.Request) {
				p, err := wc.png(r.Context())
				if err != nil {
					slog.Error("weather render failed", "err", err)
					http.Error(w, "render error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(p)
			})
			primaryRefresh = wc.conditionalRefresh
			primaryRenderedAt = func() time.Time {
				wc.mu.Lock()
				defer wc.mu.Unlock()
				return wc.renderedAt
			}
		}

		if calendarMode {
			calPath := "/screen/calendar.png"
			calURL := *baseURL + calPath
			var calendars []string
			for id := range strings.SplitSeq(*hassCalendars, ",") {
				if id = strings.TrimSpace(id); id != "" {
					calendars = append(calendars, id)
				}
			}
			cc := &calendarCache{cfg: calendarCfg{
				hassURL:   *hassURL,
				hassToken: *hassToken,
				calendars: calendars,
				ttl:       time.Duration(*calendarTTL) * time.Second,
			}}
			if err := cc.refresh(context.Background()); err != nil {
				log.Fatalf("initial calendar fetch failed: %v", err)
			}
			mux.HandleFunc("GET "+calPath, func(w http.ResponseWriter, r *http.Request) {
				p, err := cc.png(r.Context())
				if err != nil {
					slog.Error("calendar render failed", "err", err)
					http.Error(w, "render error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(p)
			})
			if !weatherMode {
				primaryScreenURL = calURL
				primaryRefresh = cc.conditionalRefresh
				primaryRenderedAt = func() time.Time {
					cc.mu.Lock()
					defer cc.mu.Unlock()
					return cc.renderedAt
				}
			}
		}

		deviceapi.Register(mux, deviceapi.Handler{
			OnDisplay: func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.DisplayResponse, error) {
				logDevice("display request", device)
				if err := primaryRefresh(ctx); err != nil {
					slog.Error("refresh failed", "err", err)
				}
				return deviceapi.DisplayResponse{
					Filename:    fmt.Sprintf("screen-%s.png", primaryRenderedAt().Format(time.RFC3339)),
					ImageURL:    primaryScreenURL,
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
					ImageURL:   primaryScreenURL,
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
		slog.Info("demyd listening", "addr", *addr, "weather_mode", weatherMode, "calendar_mode", calendarMode)
		log.Fatal(srv.ListenAndServe())
		return
	}

	// Static image mode.
	imageName := "weather.png"
	screenURL := *baseURL + "/screen/" + imageName

	var displayPNG []byte
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
	hassURL    string
	hassToken  string
	hassEntity string
	location   string
	units      string // "imperial", "metric", or "" (use HA native)
	ttl        time.Duration
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

type weatherCache struct {
	cfg weatherCfg
	mu  sync.Mutex
	// protected by mu:
	data       *weatherData
	renderedAt time.Time
	pngBytes   []byte
}

func (wc *weatherCache) refresh(ctx context.Context) error {
	data, err := fetchWeatherFromHA(ctx, wc.cfg)
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

func (wc *weatherCache) conditionalRefresh(ctx context.Context) error {
	wc.mu.Lock()
	stale := time.Since(wc.renderedAt) > wc.cfg.ttl
	wc.mu.Unlock()
	if !stale {
		return nil
	}
	return wc.refresh(ctx)
}

func (wc *weatherCache) png(ctx context.Context) ([]byte, error) {
	if err := wc.conditionalRefresh(ctx); err != nil {
		slog.Warn("weather refresh failed, serving stale data", "err", err)
	}
	wc.mu.Lock()
	b := wc.pngBytes
	wc.mu.Unlock()
	return b, nil
}

// --- Home Assistant weather client ---

func fetchWeatherFromHA(ctx context.Context, cfg weatherCfg) (*weatherData, error) {
	client, err := ha.NewClient(cfg.hassURL, ha.WithToken(cfg.hassToken), ha.WithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("creating HA client: %w", err)
	}

	state, err := client.GetStateForEntity(ctx, cfg.hassEntity)
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

	forecasts, err := client.GetWeatherForecasts(ctx, cfg.hassEntity, "daily")
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

	applyUnitConversion(data, cfg.units)
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
	} else {
		data.tempC = (data.tempC - 32) * 5 / 9
		data.apparentTemp = (data.apparentTemp - 32) * 5 / 9
		for i := range data.daily {
			data.daily[i].high = (data.daily[i].high - 32) * 5 / 9
			data.daily[i].low = (data.daily[i].low - 32) * 5 / 9
		}
		data.tempUnit = "°C"
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

// --- Calendar types and cache ---

type calendarCfg struct {
	hassURL   string
	hassToken string
	calendars []string
	ttl       time.Duration
}

type calendarCache struct {
	cfg calendarCfg
	mu  sync.Mutex
	// protected by mu:
	events     []calendarEvent
	renderedAt time.Time
	pngBytes   []byte
}

func (cc *calendarCache) refresh(ctx context.Context) error {
	events, err := fetchCalendarFromHA(ctx, cc.cfg)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := renderCalendar(&buf, events, time.Now()); err != nil {
		return err
	}
	cc.mu.Lock()
	cc.events = events
	cc.renderedAt = time.Now()
	cc.pngBytes = buf.Bytes()
	cc.mu.Unlock()
	return nil
}

func (cc *calendarCache) conditionalRefresh(ctx context.Context) error {
	cc.mu.Lock()
	stale := time.Since(cc.renderedAt) > cc.cfg.ttl
	cc.mu.Unlock()
	if !stale {
		return nil
	}
	return cc.refresh(ctx)
}

func (cc *calendarCache) png(ctx context.Context) ([]byte, error) {
	if err := cc.conditionalRefresh(ctx); err != nil {
		slog.Warn("calendar refresh failed, serving stale data", "err", err)
	}
	cc.mu.Lock()
	b := cc.pngBytes
	cc.mu.Unlock()
	return b, nil
}

// --- Home Assistant calendar client ---

func fetchCalendarFromHA(ctx context.Context, cfg calendarCfg) ([]calendarEvent, error) {
	client, err := ha.NewClient(cfg.hassURL, ha.WithToken(cfg.hassToken), ha.WithTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("creating HA client: %w", err)
	}

	now := time.Now()
	end := now.AddDate(0, 0, 7)

	var events []calendarEvent
	for _, calID := range cfg.calendars {
		haEvents, err := client.GetCalendarEvents(ctx, calID, now, end)
		if err != nil {
			slog.Warn("failed to get calendar events", "calendar", calID, "err", err)
			continue
		}
		for _, e := range haEvents {
			ce, ok := parseCalendarEvent(e)
			if !ok {
				continue
			}
			events = append(events, ce)
		}
	}

	return events, nil
}

func parseCalendarEvent(e ha.CalendarEvent) (calendarEvent, bool) {
	var start, end time.Time
	var allDay bool

	if e.Start.DateTime != "" {
		t, err := time.Parse(time.RFC3339, e.Start.DateTime)
		if err != nil {
			return calendarEvent{}, false
		}
		start = t
	} else if e.Start.Date != "" {
		t, err := time.Parse("2006-01-02", e.Start.Date)
		if err != nil {
			return calendarEvent{}, false
		}
		start = t
		allDay = true
	} else {
		return calendarEvent{}, false
	}

	if e.End.DateTime != "" {
		t, err := time.Parse(time.RFC3339, e.End.DateTime)
		if err == nil {
			end = t
		}
	} else if e.End.Date != "" {
		t, err := time.Parse("2006-01-02", e.End.Date)
		if err == nil {
			end = t
		}
	}

	return calendarEvent{
		title:  e.Summary,
		start:  start,
		end:    end,
		allDay: allDay,
	}, true
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

	outImg, err := c.Render(root, demy.TwoBit)
	if err != nil {
		return fmt.Errorf("rendering canvas: %w", err)
	}
	return png.Encode(w, outImg)
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
