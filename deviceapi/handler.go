package deviceapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
)

// Handler holds the application callbacks for each device API endpoint.
// A nil OnDisplay or OnSetup responds with 501 Not Implemented.
// A nil OnLog silently succeeds with 204 No Content.
type Handler struct {
	OnDisplay func(ctx context.Context, device DeviceHeaders) (DisplayResponse, error)
	OnLog     func(ctx context.Context, device DeviceHeaders, entries []LogEntry) error
	OnSetup   func(ctx context.Context, device DeviceHeaders) (SetupResponse, error)
}

// NewMux creates a new *http.ServeMux with the device API routes registered.
func NewMux(h Handler) *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, h)
	return mux
}

// Register adds the device API routes to an existing mux.
func Register(mux *http.ServeMux, h Handler) {
	mux.HandleFunc("GET /api/display", h.serveDisplay)
	mux.HandleFunc("POST /api/log", h.serveLog)
	mux.HandleFunc("GET /api/setup/", h.serveSetup)
}

func (h Handler) serveDisplay(w http.ResponseWriter, r *http.Request) {
	b, _ := httputil.DumpRequest(r, true)
	fmt.Println(string(b))
	device, err := parseDeviceHeaders(r)
	if err != nil {
		http.Error(w, "missing required ID header", http.StatusUnauthorized)
		return
	}
	if h.OnDisplay == nil {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}
	resp, err := h.OnDisplay(r.Context(), device)
	if err != nil {
		slog.Error("deviceapi: OnDisplay error", "err", err, "device_id", device.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h Handler) serveLog(w http.ResponseWriter, r *http.Request) {
	device, err := parseDeviceHeaders(r)
	if err != nil {
		http.Error(w, "missing required ID header", http.StatusUnauthorized)
		return
	}
	var body logRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if h.OnLog == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.OnLog(r.Context(), device, body.Logs); err != nil {
		slog.Error("deviceapi: OnLog error", "err", err, "device_id", device.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) serveSetup(w http.ResponseWriter, r *http.Request) {
	device, err := parseDeviceHeaders(r)
	if err != nil {
		http.Error(w, "missing required ID header", http.StatusUnauthorized)
		return
	}
	if h.OnSetup == nil {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}
	resp, err := h.OnSetup(r.Context(), device)
	if err != nil {
		slog.Error("deviceapi: OnSetup error", "err", err, "device_id", device.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("deviceapi: failed to marshal response", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
