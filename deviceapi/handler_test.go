package deviceapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newRequest(method, path string, headers map[string]string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, path, body)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func deviceHeaders() map[string]string {
	return map[string]string{"ID": "AA:BB:CC:DD:EE:FF"}
}

// --- parseDeviceHeaders ---

func TestParseDeviceHeaders_MissingID(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	_, err := parseDeviceHeaders(r)
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestParseDeviceHeaders_AllFields(t *testing.T) {
	r := newRequest("GET", "/", map[string]string{
		"ID":              "AA:BB:CC:DD:EE:FF",
		"ACCESS_TOKEN":    "tok",
		"BATTERY_VOLTAGE": "3.7",
		"FW_VERSION":      "1.2.3",
		"HEIGHT":          "480",
		"MODEL":           "esp32",
		"PERCENT_CHARGED": "80",
		"REFRESH_RATE":    "130",
		"RSSI":            "-54",
		"SENSORS":         "temp",
		"UPDATE_SOURCE":   "timer",
		"USER_AGENT":      "TRMNL",
		"WIDTH":           "800",
	}, nil)
	r.Host = "192.168.1.10"

	d, err := parseDeviceHeaders(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.ID != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("ID = %q", d.ID)
	}
	if d.AccessToken != "tok" {
		t.Errorf("AccessToken = %q", d.AccessToken)
	}
	if d.BatteryVoltage == nil || *d.BatteryVoltage != 3.7 {
		t.Errorf("BatteryVoltage = %v", d.BatteryVoltage)
	}
	if d.FWVersion != "1.2.3" {
		t.Errorf("FWVersion = %q", d.FWVersion)
	}
	if d.Height == nil || *d.Height != 480 {
		t.Errorf("Height = %v", d.Height)
	}
	if d.Host != "192.168.1.10" {
		t.Errorf("Host = %q", d.Host)
	}
	if d.Model != "esp32" {
		t.Errorf("Model = %q", d.Model)
	}
	if d.PercentCharged == nil || *d.PercentCharged != 80 {
		t.Errorf("PercentCharged = %v", d.PercentCharged)
	}
	if d.RefreshRate == nil || *d.RefreshRate != 130 {
		t.Errorf("RefreshRate = %v", d.RefreshRate)
	}
	if d.RSSI == nil || *d.RSSI != -54 {
		t.Errorf("RSSI = %v", d.RSSI)
	}
	if d.Width == nil || *d.Width != 800 {
		t.Errorf("Width = %v", d.Width)
	}
}

func TestParseDeviceHeaders_AbsentNumericFields(t *testing.T) {
	r := newRequest("GET", "/", deviceHeaders(), nil)
	d, err := parseDeviceHeaders(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.BatteryVoltage != nil || d.Height != nil || d.Width != nil {
		t.Error("expected nil pointer fields for absent headers")
	}
}

func TestParseDeviceHeaders_InvalidNumericFields(t *testing.T) {
	r := newRequest("GET", "/", map[string]string{
		"ID":     "AA:BB:CC:DD:EE:FF",
		"HEIGHT": "notanumber",
		"RSSI":   "bad",
	}, nil)
	d, err := parseDeviceHeaders(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tolerant parsing: invalid values become nil, not an error.
	if d.Height != nil {
		t.Errorf("expected nil Height for invalid value, got %v", d.Height)
	}
	if d.RSSI != nil {
		t.Errorf("expected nil RSSI for invalid value, got %v", d.RSSI)
	}
}

// --- GET /api/display ---

func TestServeDisplay_MissingID(t *testing.T) {
	h := Handler{OnDisplay: func(_ context.Context, _ DeviceHeaders) (DisplayResponse, error) {
		return DisplayResponse{}, nil
	}}
	w := httptest.NewRecorder()
	h.serveDisplay(w, httptest.NewRequest("GET", "/api/display", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestServeDisplay_NilHandler(t *testing.T) {
	h := Handler{}
	w := httptest.NewRecorder()
	h.serveDisplay(w, newRequest("GET", "/api/display", deviceHeaders(), nil))
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w.Code)
	}
}

func TestServeDisplay_HandlerError(t *testing.T) {
	h := Handler{OnDisplay: func(_ context.Context, _ DeviceHeaders) (DisplayResponse, error) {
		return DisplayResponse{}, errors.New("db down")
	}}
	w := httptest.NewRecorder()
	h.serveDisplay(w, newRequest("GET", "/api/display", deviceHeaders(), nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestServeDisplay_Success(t *testing.T) {
	want := DisplayResponse{
		Filename:    "demo.bmp",
		ImageURL:    "https://example.com/demo.bmp",
		RefreshRate: 130,
	}
	h := Handler{OnDisplay: func(_ context.Context, _ DeviceHeaders) (DisplayResponse, error) {
		return want, nil
	}}
	w := httptest.NewRecorder()
	h.serveDisplay(w, newRequest("GET", "/api/display", deviceHeaders(), nil))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"refresh_rate":130`) {
		t.Errorf("body missing refresh_rate: %s", body)
	}
}

// --- POST /api/log ---

func TestServeLog_MissingID(t *testing.T) {
	h := Handler{OnLog: func(_ context.Context, _ DeviceHeaders, _ []LogEntry) error { return nil }}
	w := httptest.NewRecorder()
	h.serveLog(w, httptest.NewRequest("POST", "/api/log", strings.NewReader(`{"logs":[]}`)))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestServeLog_MalformedJSON(t *testing.T) {
	h := Handler{OnLog: func(_ context.Context, _ DeviceHeaders, _ []LogEntry) error { return nil }}
	w := httptest.NewRecorder()
	h.serveLog(w, newRequest("POST", "/api/log", deviceHeaders(), strings.NewReader(`not json`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestServeLog_NilHandler(t *testing.T) {
	h := Handler{}
	w := httptest.NewRecorder()
	h.serveLog(w, newRequest("POST", "/api/log", deviceHeaders(), strings.NewReader(`{"logs":[]}`)))
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestServeLog_HandlerError(t *testing.T) {
	h := Handler{OnLog: func(_ context.Context, _ DeviceHeaders, _ []LogEntry) error {
		return errors.New("write failed")
	}}
	w := httptest.NewRecorder()
	h.serveLog(w, newRequest("POST", "/api/log", deviceHeaders(), strings.NewReader(`{"logs":[]}`)))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestServeLog_Success(t *testing.T) {
	var got []LogEntry
	h := Handler{OnLog: func(_ context.Context, _ DeviceHeaders, entries []LogEntry) error {
		got = entries
		return nil
	}}
	body := `{"logs":[{"id":1,"message":"hello","battery_voltage":3.7}]}`
	w := httptest.NewRecorder()
	h.serveLog(w, newRequest("POST", "/api/log", deviceHeaders(), strings.NewReader(body)))
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if len(got) != 1 || got[0].Message != "hello" || got[0].BatteryVoltage != 3.7 {
		t.Errorf("unexpected entries: %+v", got)
	}
}

// --- GET /api/setup/ ---

func TestServeSetup_MissingID(t *testing.T) {
	h := Handler{OnSetup: func(_ context.Context, _ DeviceHeaders) (SetupResponse, error) {
		return SetupResponse{}, nil
	}}
	w := httptest.NewRecorder()
	h.serveSetup(w, httptest.NewRequest("GET", "/api/setup/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestServeSetup_NilHandler(t *testing.T) {
	h := Handler{}
	w := httptest.NewRecorder()
	h.serveSetup(w, newRequest("GET", "/api/setup/", deviceHeaders(), nil))
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", w.Code)
	}
}

func TestServeSetup_HandlerError(t *testing.T) {
	h := Handler{OnSetup: func(_ context.Context, _ DeviceHeaders) (SetupResponse, error) {
		return SetupResponse{}, errors.New("not found")
	}}
	w := httptest.NewRecorder()
	h.serveSetup(w, newRequest("GET", "/api/setup/", deviceHeaders(), nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestServeSetup_Success(t *testing.T) {
	want := SetupResponse{
		APIKey:     "secret",
		FriendlyID: "ABC123",
		ImageURL:   "https://example.com/setup.bmp",
		Message:    "Welcome",
	}
	h := Handler{OnSetup: func(_ context.Context, _ DeviceHeaders) (SetupResponse, error) {
		return want, nil
	}}
	w := httptest.NewRecorder()
	h.serveSetup(w, newRequest("GET", "/api/setup/", deviceHeaders(), nil))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"friendly_id":"ABC123"`) {
		t.Errorf("body missing friendly_id: %s", body)
	}
}
