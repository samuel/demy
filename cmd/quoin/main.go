package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/google/uuid"
	"github.com/samuel/demy/deviceapi"
	"github.com/samuel/demy/internal/model"
	"github.com/samuel/demy/internal/mqttbridge"
	"github.com/samuel/demy/internal/plugin"
	"github.com/samuel/demy/internal/plugin/builtin"

	"github.com/samuel/demy/internal/plugin/dynamic"
	"github.com/samuel/demy/internal/service"
	"github.com/samuel/demy/internal/sqlstore"
	"github.com/samuel/demy/internal/web"
)

func main() {
	cfgPath := flag.String("config", "quoin.yaml", "path to YAML config file")
	makeAdmin := flag.String("make-admin", "", "email address to grant admin access")
	verbose := flag.Bool("v", false, "enable debug logging")
	flag.Parse()

	cfg, err := service.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if p := cfg.TRMNLDeviceAPIPrefix; p != "" && p[0] != '/' {
		log.Fatalf("trmnl_device_api_prefix must start with /: %q", p)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		slog.Info("shutting down")
		cancel()
	}()

	store, err := sqlstore.Open(ctx, cfg.Database.Path)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer store.Close()

	if *makeAdmin != "" {
		u, err := store.GetUserByEmail(ctx, *makeAdmin)
		if err != nil {
			log.Fatalf("make-admin: user %q not found", *makeAdmin)
		}
		u.IsAdmin = true
		if err := store.UpdateUser(ctx, u); err != nil {
			log.Fatalf("make-admin: %v", err)
		}
		slog.Info("granted admin", "email", *makeAdmin)
	}

	rpDisplayName := cfg.WebAuthn.RPDisplayName
	if rpDisplayName == "" {
		rpDisplayName = "Quoin"
	}
	svc, err := service.New(store, cfg.RPID(), rpDisplayName)
	if err != nil {
		log.Fatalf("service init: %v", err)
	}

	if err := svc.SeedDefaultLibraryPlugins(ctx); err != nil {
		log.Fatalf("seeding library plugins: %v", err)
	}

	// Build the render engine with all renderers registered.
	engine := plugin.NewRenderEngine(store)
	engine.RegisterBuiltin("weather", &builtin.WeatherRenderer{})
	engine.RegisterBuiltin("xkcd", &builtin.XKCDRenderer{})
	engine.RegisterBuiltin("calendar", &builtin.CalendarRenderer{})
	engine.RegisterBuiltin("grayscale", &builtin.GrayscaleRenderer{})
	engine.RegisterType(string(model.PluginTypeJSONTemplate), &dynamic.JSONTemplateRenderer{})
	engine.RegisterType(string(model.PluginTypeStarlark), &dynamic.StarlarkRenderer{})

	rc := plugin.NewStoreResourceCache(store)

	// In-memory image store for serving signed image URLs to devices.
	imgStore := newImageStore()

	renderFn := func(ctx context.Context, p *model.UserPlugin, deviceID string) ([]byte, error) {
		d, err := store.GetDeviceByID(ctx, deviceID)
		if err != nil {
			return nil, fmt.Errorf("loading device: %w", err)
		}
		var cc *plugin.CanvasConfig
		if d.CanvasConfig != "" {
			cc = new(plugin.CanvasConfig)
			if err := json.Unmarshal([]byte(d.CanvasConfig), cc); err != nil {
				return nil, fmt.Errorf("parsing canvas config: %w", err)
			}
		}
		demyCfg, err := plugin.BuildDemyConfig(cc, deviceapi.DeviceHeaders{})
		if err != nil {
			return nil, fmt.Errorf("building demy config: %w", err)
		}
		loc := time.UTC
		if d.Timezone != "" {
			if l, err := time.LoadLocation(d.Timezone); err == nil {
				loc = l
			}
		}
		pngBytes, _, err := engine.Render(ctx, p, demyCfg, rc, loc)
		return pngBytes, err
	}

	webSrv, err := web.New(svc, cfg.BaseURL, renderFn)
	if err != nil {
		log.Fatalf("web server init: %v", err)
	}

	// Optional MQTT bridge for Home Assistant integration.
	var bridge *mqttbridge.Bridge
	if cfg.MQTT.BrokerURL != "" {
		b, err := mqttbridge.New(
			cfg.MQTT.BrokerURL,
			cfg.MQTT.Username,
			cfg.MQTT.Password,
			cfg.MQTT.BaseTopic,
			cfg.MQTT.HAPrefix,
			store,
			svc,
		)
		if err != nil {
			log.Fatalf("mqtt bridge init: %v", err)
		}
		if err := b.Start(ctx); err != nil {
			log.Fatalf("mqtt bridge start: %v", err)
		}
		bridge = b
		webSrv.Notifier = bridge
		defer bridge.Close()
	}

	mux := http.NewServeMux()

	// Device API handlers.
	devHandler := deviceapi.Handler{
		OnSetup:   makeSetupHandler(svc),
		OnDisplay: makeDisplayHandler(ctx, svc, store, engine, rc, imgStore, cfg.BaseURL, bridge),
		OnLog:     makeLogHandler(store),
	}
	deviceapi.Register(mux, cfg.TRMNLDeviceAPIPrefix, devHandler)

	// Image serving endpoint.
	mux.HandleFunc("GET /image/{deviceID}/{filename}", makeImageHandler(store, imgStore))

	// Web UI routes.
	webSrv.Register(mux)

	httpSrv := &http.Server{
		Handler:        requestLogger(mux),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Background jobs.
	go runEvery(ctx, 5*time.Minute, "resource cache eviction", func(ctx context.Context) {
		if err := store.EvictExpiredResources(ctx); err != nil {
			slog.Warn("resource cache eviction failed", "err", err)
		}
	})
	go runEvery(ctx, time.Hour, "session cleanup", func(ctx context.Context) {
		if err := store.DeleteExpiredSessions(ctx); err != nil {
			slog.Warn("session cleanup failed", "err", err)
		}
	})
	go runEvery(ctx, 10*time.Minute, "image memory cleanup", func(ctx context.Context) {
		imgStore.evict()
	})

	// Start server according to TLS mode.
	switch cfg.TLS.Mode {
	case "auto":
		if len(cfg.TLS.ACME.Domains) == 0 {
			log.Fatal("tls.mode=auto requires tls.acme.domains")
		}
		if cfg.TLS.ACME.Challenge == "dns" {
			// DNS-01 challenge: no port 80 required.
			mgr := newDNSCertManager(
				cfg.TLS.ACME.Domains,
				cfg.TLS.ACME.Email,
				cfg.TLS.ACME.CacheDir,
				cfg.TLS.ACME.Route53,
				cfg.TLS.ACME.Staging,
			)
			if err := mgr.Start(ctx); err != nil {
				log.Fatalf("ACME DNS-01: %v", err)
			}
			httpSrv.Addr = ":443"
			httpSrv.TLSConfig = &tls.Config{GetCertificate: mgr.GetCertificate}
			slog.Info("quoin starting (auto TLS / DNS-01)", "domains", cfg.TLS.ACME.Domains)
			go func() {
				if err := httpSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Fatalf("HTTPS server: %v", err)
				}
			}()
		} else {
			// HTTP-01 challenge (default): needs port 80 reachable from internet.
			m := &autocert.Manager{
				Cache:      autocert.DirCache(cfg.TLS.ACME.CacheDir),
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(cfg.TLS.ACME.Domains...),
				Email:      cfg.TLS.ACME.Email,
			}
			go func() {
				if err := http.ListenAndServe(":80", m.HTTPHandler(nil)); err != nil {
					slog.Warn("ACME HTTP-01 listener error", "err", err)
				}
			}()
			httpSrv.Addr = ":443"
			httpSrv.TLSConfig = m.TLSConfig()
			slog.Info("quoin starting (auto TLS / HTTP-01)", "domains", cfg.TLS.ACME.Domains)
			go func() {
				if err := httpSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Fatalf("HTTPS server: %v", err)
				}
			}()
		}
	case "manual":
		httpSrv.Addr = cfg.Listen
		slog.Info("quoin starting (manual TLS)", "addr", cfg.Listen)
		go func() {
			if err := httpSrv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTPS server: %v", err)
			}
		}()
	default: // "off"
		httpSrv.Addr = cfg.Listen
		slog.Info("quoin starting", "addr", cfg.Listen)
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTP server: %v", err)
			}
		}()
	}

	<-ctx.Done()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = httpSrv.Shutdown(shutCtx)
}

// runEvery runs fn on a ticker, stopping when ctx is cancelled.
func runEvery(ctx context.Context, interval time.Duration, name string, fn func(context.Context)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
			slog.Debug("background job ran", "job", name)
		}
	}
}

// imageEntry is a signed, in-memory PNG blob.
type imageEntry struct {
	data      []byte
	expiresAt time.Time
}

// imageStore is a thread-safe in-memory image store keyed by "deviceID/filename".
type imageStore struct {
	mu     sync.RWMutex
	images map[string]imageEntry
}

func newImageStore() *imageStore {
	return &imageStore{images: make(map[string]imageEntry)}
}

func (s *imageStore) put(deviceID, filename string, data []byte) {
	key := deviceID + "/" + filename
	s.mu.Lock()
	s.images[key] = imageEntry{data: data, expiresAt: time.Now().Add(30 * time.Minute)}
	s.mu.Unlock()
}

func (s *imageStore) get(deviceID, filename string) ([]byte, bool) {
	key := deviceID + "/" + filename
	s.mu.RLock()
	e, ok := s.images[key]
	s.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func (s *imageStore) evict() {
	now := time.Now()
	s.mu.Lock()
	for k, v := range s.images {
		if now.After(v.expiresAt) {
			delete(s.images, k)
		}
	}
	s.mu.Unlock()
}

// signImageURL builds and signs an image URL using the device's API key.
func signImageURL(baseURL, deviceID, filename, apiKey string) string {
	exp := strconv.FormatInt(time.Now().Add(35*time.Minute).Unix(), 10)
	msg := deviceID + "\n" + filename + "\n" + exp
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s/image/%s/%s?sig=%s&exp=%s", baseURL, deviceID, filename, sig, exp)
}

// verifyImageURL checks sig and exp query params against the device's API key.
func verifyImageURL(deviceID, filename, apiKey, sig, exp string) bool {
	expI, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > expI {
		return false
	}
	msg := deviceID + "\n" + filename + "\n" + exp
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(msg))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func makeSetupHandler(svc *service.Service) func(context.Context, deviceapi.DeviceHeaders) (deviceapi.SetupResponse, error) {
	return func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.SetupResponse, error) {
		d, wasJustIssued, err := svc.SetupDevice(ctx, device.ID, device.AccessToken)
		if err != nil {
			if errors.Is(err, service.ErrDeviceNotFound) {
				return deviceapi.SetupResponse{}, fmt.Errorf("device not registered")
			}
			if errors.Is(err, service.ErrAPIKeyAlreadyIssued) {
				return deviceapi.SetupResponse{}, fmt.Errorf("API key already issued")
			}
			return deviceapi.SetupResponse{}, err
		}
		var apiKey string
		var status int
		if wasJustIssued {
			apiKey = d.APIKey
			status = 200
		}
		return deviceapi.SetupResponse{
			APIKey:     apiKey,
			FriendlyID: d.Name,
			Status:     status,
			Message:    "Welcome to Quoin",
		}, nil
	}
}

func makeDisplayHandler(
	_ context.Context,
	svc *service.Service,
	store model.Store,
	engine *plugin.RenderEngine,
	rc plugin.ResourceCache,
	imgStore *imageStore,
	baseURL string,
	bridge *mqttbridge.Bridge,
) func(context.Context, deviceapi.DeviceHeaders) (deviceapi.DisplayResponse, error) {
	return func(ctx context.Context, device deviceapi.DeviceHeaders) (deviceapi.DisplayResponse, error) {
		d, err := svc.AuthenticateDevice(ctx, device.ID, device.AccessToken)
		if err != nil {
			return deviceapi.DisplayResponse{}, fmt.Errorf("authentication: %w", err)
		}

		items, err := svc.ListPlaylistItems(ctx, d.ID)
		if err != nil {
			return deviceapi.DisplayResponse{}, fmt.Errorf("loading playlist: %w", err)
		}
		if len(items) == 0 {
			return deviceapi.DisplayResponse{}, errors.New("empty playlist")
		}

		var cc *plugin.CanvasConfig
		if d.CanvasConfig != "" {
			cc = new(plugin.CanvasConfig)
			if err := json.Unmarshal([]byte(d.CanvasConfig), cc); err != nil {
				slog.Warn("invalid canvas config JSON, using defaults", "device_id", d.ID, "err", err)
				cc = nil
			}
		}
		demyCfg, err := plugin.BuildDemyConfig(cc, device)
		if err != nil {
			return deviceapi.DisplayResponse{}, fmt.Errorf("building canvas config: %w", err)
		}

		loc := time.UTC
		if d.Timezone != "" {
			if l, err := time.LoadLocation(d.Timezone); err == nil {
				loc = l
			}
		}

		if headersJSON, merr := json.Marshal(device); merr == nil {
			d.LastHeaders = string(headersJSON)
		}

		pos, _, err := svc.AdvanceDisplay(ctx, d, len(items))
		if err != nil {
			return deviceapi.DisplayResponse{}, fmt.Errorf("advance display: %w", err)
		}
		if pos >= len(items) {
			pos = 0
		}

		item := items[pos]
		p, err := store.GetUserPluginByID(ctx, item.UserPluginID)
		if err != nil {
			return deviceapi.DisplayResponse{}, fmt.Errorf("loading plugin: %w", err)
		}

		pngBytes, renderedAt, err := engine.Render(ctx, p, demyCfg, rc, loc)
		if err != nil {
			return deviceapi.DisplayResponse{}, fmt.Errorf("rendering plugin: %w", err)
		}

		if bridge != nil {
			bridge.PublishDeviceState(ctx, d, device, p.Name)
		}

		// Build the filename slug from the library plugin slug if available.
		slug := "plugin"
		if p.LibraryPluginID != "" {
			if lp, lerr := store.GetLibraryPluginByID(ctx, p.LibraryPluginID); lerr == nil {
				slug = lp.Slug
			}
		}
		filename := plugin.RenderedFilename(slug, renderedAt)

		imgStore.put(d.ID, filename, pngBytes)

		imageURL := signImageURL(baseURL, d.ID, filename, d.APIKey)

		refreshRate := d.RefreshRate
		if refreshRate == 0 {
			refreshRate = 300
		}

		res := deviceapi.DisplayResponse{
			Filename:    filename,
			ImageURL:    imageURL,
			RefreshRate: refreshRate,
		}
		// if device.Model == "x" && device.FWVersion != "1.8.5" {
		// 	res.FirmwareURL = "https://trmnl-fw.s3.us-east-2.amazonaws.com/trmnl_x/FW1.8.5.bin"
		// 	res.FirmwareVersion = "1.8.5"
		// 	res.UpdateFirmware = true
		// }
		// if device.Model == "og" && device.FWVersion != "1.8.5" {
		// 	res.FirmwareURL = "https://trmnl-fw.s3.us-east-2.amazonaws.com/trmnl_og/FW1.8.5.bin"
		// 	res.FirmwareVersion = "1.8.5"
		// 	res.UpdateFirmware = true
		// }
		return res, nil
	}
}

func logEntryID(deviceID string, entryID int) string {
	if entryID != 0 {
		return fmt.Sprintf("%s:%d", deviceID, entryID)
	}
	return uuid.New().String()
}

func makeLogHandler(store model.Store) func(context.Context, deviceapi.DeviceHeaders, []deviceapi.LogEntry) error {
	return func(ctx context.Context, device deviceapi.DeviceHeaders, entries []deviceapi.LogEntry) error {
		for _, e := range entries {
			entry := &model.DeviceLogEntry{
				ID:              logEntryID(device.ID, e.ID),
				DeviceID:        device.ID,
				LoggedAt:        time.Unix(e.CreatedAt, 0),
				Message:         e.Message,
				WiFiStatus:      e.WiFiStatus,
				WakeReason:      e.WakeReason,
				FirmwareVersion: e.FirmwareVersion,
				BatteryVoltage:  e.BatteryVoltage,
				WiFiSignal:      e.WiFiSignal,
				FreeHeapSize:    e.FreeHeapSize,
			}
			slog.Info("device log", "id", e.ID, "device_id", device.ID, "entry", e.Message)
			if err := store.InsertDeviceLog(ctx, entry); err != nil {
				slog.Warn("failed to store device log entry", "err", err, "device_id", device.ID)
			}
		}
		return nil
	}
}

func makeImageHandler(store model.Store, imgStore *imageStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.PathValue("deviceID")
		filename := r.PathValue("filename")
		sig := r.URL.Query().Get("sig")
		exp := r.URL.Query().Get("exp")

		if sig == "" || exp == "" {
			http.Error(w, "missing sig or exp", http.StatusBadRequest)
			return
		}

		expI, err := strconv.ParseInt(exp, 10, 64)
		if err != nil || time.Now().Unix() > expI {
			http.Error(w, "URL expired", http.StatusForbidden)
			return
		}

		d, err := store.GetDeviceByID(r.Context(), deviceID)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if !verifyImageURL(deviceID, filename, d.APIKey, sig, exp) {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}

		data, ok := imgStore.get(deviceID, filename)
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}
}

// responseRecorder captures the HTTP status for logging.
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
