package main

import (
	"bytes"
	"context"
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
	"time"

	"github.com/samuel/pate"
	"github.com/samuel/pate/deviceapi"
	"golang.org/x/image/font/gofont/goregular"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	imageFile := flag.String("image", "", "path to image file to use as display content")
	fontFile := flag.String("font", "", "path to TTF font file for alt text")
	baseURL := flag.String("base-url", "http://localhost:8080", "base URL of this server, used in image_url responses")
	refreshRate := flag.Int("refresh-rate", 300, "display refresh rate in seconds")
	flag.Parse()

	imageName := "display4.png"
	imageURL := *baseURL + "/screen/" + imageName

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
			http.Error(w, "no image configured; start pated with -image <path>", http.StatusNotFound)
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
				ImageURL:    imageURL,
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
				ImageURL:   imageURL,
				Message:    "Welcome to pated",
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
	slog.Info("pated listening", "addr", *addr)
	log.Fatal(srv.ListenAndServe())
}

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

	var font *pate.FontFamily
	if fontFile != "" {
		font, err = pate.LoadFontFile("sans", fontFile)
		if err != nil {
			return fmt.Errorf("loading font: %w", err)
		}
	} else {
		font = pate.MustLoadFontBytes("sand", goregular.TTF)
	}

	cfg := pate.TRMNLOriginal
	cfg.Fonts = map[string]*pate.FontFamily{"sans": font}
	c, err := pate.New(cfg)
	if err != nil {
		return fmt.Errorf("creating canvas: %w", err)
	}

	// Image fills the entire canvas.
	c.Image(img, pate.ImageFit)

	// Text overlay at the bottom, centered, with a white background bar.
	c.VStack(func() {
		c.Spacer()
		c.Box(pate.Background(pate.White), pate.Padding(8), func() {
			c.Text("This is where the alt text would go", pate.Style{
				Font:  "sans",
				Size:  11,
				Color: pate.Black,
				Align: pate.TextAlignCenter,
			})
		})
	})

	return c.EncodePNG(w, pate.TwoBit)
}

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
