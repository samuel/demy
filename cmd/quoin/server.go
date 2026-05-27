package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/samuel/demy/deviceapi"
)

type contextKey int

const hostKey contextKey = 0

// Server wires together config, plugin caches, playlist state, and HTTP handlers.
type Server struct {
	store    Store
	devices  map[string]*deviceEntry // keyed by MAC address (DeviceHeaders.ID)
	imagesMu sync.RWMutex
	images   map[string]imageEntry // key: "deviceID/filename"
}

type deviceEntry struct {
	cfg      DeviceConfig
	loc      *time.Location
	playlist []screenEntry
}

type screenEntry struct {
	name  string
	cache *screenCache
}

type imageEntry struct {
	data      []byte
	expiresAt time.Time
}

// NewServer builds a Server from the parsed config.
func NewServer(cfg *Config, store Store) (*Server, error) {
	srv := &Server{
		store:   store,
		devices: make(map[string]*deviceEntry),
		images:  make(map[string]imageEntry),
	}

	for _, acc := range cfg.Accounts {
		loc := time.Local
		if acc.Timezone != "" {
			var err error
			loc, err = time.LoadLocation(acc.Timezone)
			if err != nil {
				return nil, fmt.Errorf("account %q: invalid timezone: %w", acc.Name, err)
			}
		}

		// Build screen map for this account.
		screensByName := make(map[string]*screenCache, len(acc.Screens))
		for _, sc := range acc.Screens {
			plugin, err := newPlugin(acc.HomeAssistant, loc, sc)
			if err != nil {
				return nil, fmt.Errorf("account %q screen %q: %w", acc.Name, sc.Name, err)
			}
			ttl := sc.Refresh.D
			if ttl == 0 {
				ttl = 15 * time.Minute
			}
			screensByName[sc.Name] = newScreenCache(plugin, ttl)
		}

		for _, dev := range acc.Devices {
			if _, exists := srv.devices[dev.ID]; exists {
				return nil, fmt.Errorf("account %q: device %q already registered", acc.Name, dev.ID)
			}
			playlist := make([]screenEntry, 0, len(dev.Playlist))
			for _, name := range dev.Playlist {
				cache, ok := screensByName[name]
				if !ok {
					return nil, fmt.Errorf("account %q device %q: screen %q not found", acc.Name, dev.ID, name)
				}
				playlist = append(playlist, screenEntry{name: name, cache: cache})
			}
			srv.devices[dev.ID] = &deviceEntry{
				cfg:      dev,
				loc:      loc,
				playlist: playlist,
			}
		}
	}
	return srv, nil
}

// newPlugin instantiates the correct Plugin implementation for a screen.
func newPlugin(ha HAConfig, loc *time.Location, sc ScreenConfig) (Plugin, error) {
	switch sc.Plugin {
	case "weather":
		return newWeatherPlugin(ha, loc, sc.Config)
	case "calendar":
		return newCalendarPlugin(ha, loc, sc.Config)
	case "xkcd":
		return newXKCDPlugin(ha, loc, sc.Config)
	case "grayscale":
		return newGrayscalePlugin(ha, loc, sc.Config)
	default:
		return nil, fmt.Errorf("unknown plugin %q", sc.Plugin)
	}
}

// Register wires the server's HTTP handlers into mux.
func (srv *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /image/{deviceID}/{filename}", srv.serveImage)

	handler := deviceapi.Handler{
		OnDisplay: srv.onDisplay,
		OnLog:     srv.onLog,
		OnSetup:   srv.onSetup,
	}

	// Wrap deviceapi registration with middleware that injects r.Host into ctx.
	inner := deviceapi.NewMux(handler)
	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), hostKey, r.Host)
		inner.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func hostFromCtx(ctx context.Context) string {
	h, _ := ctx.Value(hostKey).(string)
	return h
}

func (srv *Server) onDisplay(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.DisplayResponse, error) {
	logDevice("display request", device)

	entry, ok := srv.devices[device.ID]
	if !ok {
		slog.Warn("display request from unconfigured device", "device_id", device.ID)
		return deviceapi.DisplayResponse{}, fmt.Errorf("device %q is not configured", device.ID)
	}

	pos, _ := srv.store.Position(device.ID)
	if pos >= len(entry.playlist) {
		pos = 0
	}

	current := entry.playlist[pos]
	demyCfg, err := buildDemyConfig(entry.cfg.Canvas, device)
	if err != nil {
		return deviceapi.DisplayResponse{}, fmt.Errorf("building canvas config: %w", err)
	}

	pngBytes, renderedAt, err := current.cache.PNG(ctx, demyCfg)
	if err != nil {
		return deviceapi.DisplayResponse{}, fmt.Errorf("rendering screen %q: %w", current.name, err)
	}

	filename := renderedFilename(current.name, renderedAt)
	host := hostFromCtx(ctx)
	imageURL := fmt.Sprintf("http://%s/image/%s/%s", host, device.ID, filename)

	srv.storeImage(device.ID, filename, pngBytes)

	// Advance only after a successful render.
	srv.store.Advance(device.ID, len(entry.playlist))

	refreshRate := entry.cfg.RefreshRate
	if refreshRate == 0 {
		refreshRate = 300
	}

	res := deviceapi.DisplayResponse{
		Filename:    filename,
		ImageURL:    imageURL,
		RefreshRate: refreshRate,
	}
	// if device.Model == "x" && device.FWVersion == "1.8.2" {
	// 	res.UpdateFirmware = true
	// 	res.FirmwareURL = "https://trmnl-fw.s3.us-east-2.amazonaws.com/trmnl_x/FW1.8.3.bin"
	// }
	return res, nil
}

func (srv *Server) onLog(ctx context.Context, device deviceapi.DeviceHeaders, entries []deviceapi.LogEntry) error {
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
}

func (srv *Server) onSetup(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.SetupResponse, error) {
	logDevice("setup request", device)
	name := device.ID
	if entry, ok := srv.devices[device.ID]; ok && entry.cfg.Name != "" {
		name = entry.cfg.Name
	}
	return deviceapi.SetupResponse{
		FriendlyID: name,
		Message:    "Welcome to quoin",
	}, nil
}

func (srv *Server) storeImage(deviceID, filename string, data []byte) {
	key := deviceID + "/" + filename
	now := time.Now()
	srv.imagesMu.Lock()
	srv.images[key] = imageEntry{data: data, expiresAt: now.Add(30 * time.Minute)}
	// Opportunistically evict expired entries.
	for k, v := range srv.images {
		if now.After(v.expiresAt) {
			delete(srv.images, k)
		}
	}
	srv.imagesMu.Unlock()
}

func (srv *Server) serveImage(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceID")
	filename := r.PathValue("filename")
	key := deviceID + "/" + filename

	slog.Debug("serving image", "device_id", deviceID, "filename", filename)

	srv.imagesMu.RLock()
	entry, ok := srv.images[key]
	srv.imagesMu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(entry.data)
}

// logDevice logs common device header fields.
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
