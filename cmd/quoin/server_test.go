package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/deviceapi"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// fakeWeatherPlugin renders a weather screen with hardcoded test data, no HA needed.
type fakeWeatherPlugin struct{}

func (p *fakeWeatherPlugin) Render(_ context.Context, cfg demy.Config) ([]byte, error) {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return nil, err
	}
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := renderWeather(&buf, &weatherData{
		condition:    "Sunny",
		tempC:        22,
		apparentTemp: 21,
		humidity:     50,
		windSpeed:    15,
		windDir:      270,
		windUnit:     "km/h",
		uvIndex:      5,
		precip:       0,
		tempUnit:     "°C",
		precipUnit:   "mm",
		daily: []dayForecast{
			{date: now, high: 25, low: 15},
			{date: now.AddDate(0, 0, 1), high: 22, low: 14},
			{date: now.AddDate(0, 0, 2), high: 20, low: 12},
		},
		fetchedAt: now,
	}, "Test City", time.UTC, cfg)
	return buf.Bytes(), err
}

// fakeCalendarPlugin renders a calendar screen with hardcoded test events, no HA needed.
type fakeCalendarPlugin struct{}

func (p *fakeCalendarPlugin) Render(_ context.Context, cfg demy.Config) ([]byte, error) {
	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	if err := font.AddStyle(gobold.TTF, demy.FontBold); err != nil {
		return nil, err
	}
	cfg.Fonts = map[string]*demy.FontFamily{"sans": font}
	now := time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)
	events := []calendarEvent{
		{title: "Team Standup", start: now.Add(1 * time.Hour), end: now.Add(2 * time.Hour)},
		{title: "All Hands", start: now.AddDate(0, 0, 1).Add(10 * time.Hour), end: now.AddDate(0, 0, 1).Add(11 * time.Hour)},
		{title: "Conference", start: now.AddDate(0, 0, 2), end: now.AddDate(0, 0, 3), allDay: true},
	}
	var buf bytes.Buffer
	err := renderCalendar(&buf, events, now, cfg)
	return buf.Bytes(), err
}

// newTestServer builds a Server with fake weather+calendar plugins and starts an httptest.Server.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv := &Server{
		store:   newMemStore(),
		devices: make(map[string]*deviceEntry),
		images:  make(map[string]imageEntry),
	}
	srv.devices["AA:BB:CC:DD:EE:FF"] = &deviceEntry{
		cfg: DeviceConfig{
			ID:          "AA:BB:CC:DD:EE:FF",
			Name:        "test-device",
			RefreshRate: 300,
		},
		loc: time.UTC,
		playlist: []screenEntry{
			{name: "weather", cache: newScreenCache(&fakeWeatherPlugin{}, 15*time.Minute)},
			{name: "calendar", cache: newScreenCache(&fakeCalendarPlugin{}, 15*time.Minute)},
		},
	}
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return srv, ts
}

// deviceReq builds an HTTP request with TRMNL device headers.
func deviceReq(t *testing.T, method, url, deviceID string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(deviceapi.HeaderID, deviceID)
	req.Header.Set(deviceapi.HeaderWidth, "800")
	req.Header.Set(deviceapi.HeaderHeight, "480")
	req.Header.Set(deviceapi.HeaderModel, "TRMNL")
	req.Header.Set(deviceapi.HeaderFWVersion, "1.2.3")
	return req
}

// fetchPNG downloads url, asserts it is a valid PNG, and returns the decoded image bounds.
func fetchPNG(t *testing.T, client *http.Client, url string) (w, h int) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	img, err := png.Decode(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: not a valid PNG: %v", url, err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

func TestServerTRMNLFlow(t *testing.T) {
	_, ts := newTestServer(t)
	client := ts.Client()
	const deviceID = "AA:BB:CC:DD:EE:FF"

	// --- Setup ---
	req := deviceReq(t, "GET", ts.URL+"/api/setup/", deviceID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: status %d", resp.StatusCode)
	}
	var setup deviceapi.SetupResponse
	if err := json.NewDecoder(resp.Body).Decode(&setup); err != nil {
		t.Fatalf("setup: decoding response: %v", err)
	}
	if setup.FriendlyID != "test-device" {
		t.Errorf("setup FriendlyID = %q, want %q", setup.FriendlyID, "test-device")
	}
	if setup.Message == "" {
		t.Error("setup Message is empty")
	}

	// --- Display polling: three rounds to verify playlist rotation ---
	// Round 0 → weather, Round 1 → calendar, Round 2 → weather again (wraps).
	type round struct {
		screen string
	}
	rounds := []round{
		{"weather"},
		{"calendar"},
		{"weather"},
	}

	var prevImageURL string
	for i, r := range rounds {
		req := deviceReq(t, "GET", ts.URL+"/api/display", deviceID)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("round %d display: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("round %d display: status %d", i, resp.StatusCode)
		}
		var dr deviceapi.DisplayResponse
		if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
			resp.Body.Close()
			t.Fatalf("round %d display: decoding response: %v", i, err)
		}
		resp.Body.Close()

		if dr.ImageURL == "" {
			t.Fatalf("round %d: empty image_url", i)
		}
		if dr.RefreshRate != 300 {
			t.Errorf("round %d: refresh_rate = %d, want 300", i, dr.RefreshRate)
		}
		if !strings.Contains(dr.Filename, r.screen) {
			t.Errorf("round %d: filename %q does not contain screen name %q", i, dr.Filename, r.screen)
		}

		// Successive screens must produce different image URLs.
		if i == 1 && dr.ImageURL == prevImageURL {
			t.Errorf("round 1: image_url unchanged from round 0 (playlist did not advance)")
		}
		prevImageURL = dr.ImageURL

		// Download and validate the PNG.
		w, h := fetchPNG(t, client, dr.ImageURL)
		if w != 800 || h != 480 {
			t.Errorf("round %d (%s): image size = %dx%d, want 800x480", i, r.screen, w, h)
		}
	}

	// --- Unknown device returns 500 ---
	req = deviceReq(t, "GET", ts.URL+"/api/display", "FF:FF:FF:FF:FF:FF")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("unknown device: got status %d, want 500", resp.StatusCode)
	}
}
