package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"log"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/samuel/demy"
	"github.com/samuel/demy/scene/xmlscene"
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed static
var staticFiles embed.FS

//go:embed fonts/spleen-8x16.bdf
var spleenBDF []byte

//go:embed fonts/tom-thumb.bdf
var tomThumbBDF []byte

var baseCfg demy.Config

type lesson struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	GoEquiv  string `json:"go_equiv"`
	SceneXML string `json:"scene_xml"`
}

// maxBodyBytes bounds the size of a posted scene.
const maxBodyBytes = 1 << 20

// renderTimeout bounds the time spent rendering a posted scene.
const renderTimeout = 10 * time.Second

func main() {
	addr := flag.String("addr", "127.0.0.1:8081", "listen address")
	flag.Parse()

	font := demy.MustLoadFontBytes("sans", goregular.TTF)
	baseCfg = demy.TRMNLOriginal
	baseCfg.Fonts = map[string]*demy.FontFamily{
		"sans":     font,
		"spleen":   demy.MustLoadBitmapFontBDF("spleen", spleenBDF),
		"tomthumb": demy.MustLoadBitmapFontBDF("tomthumb", tomThumbBDF),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", indexHandler)
	mux.HandleFunc("GET /lessons", lessonsHandler)
	mux.HandleFunc("POST /render", renderHandler)

	// Reject cross-origin POSTs so other web pages can't drive /render.
	handler := http.NewCrossOriginProtection().Handler(mux)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           requestLogger(handler),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	slog.Info("demy-tutorial listening", "addr", *addr, "url", "http://"+*addr)
	log.Fatal(srv.ListenAndServe())
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func lessonsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, tutorialLessons)
}

// palettes are the output palettes the UI offers, by mode name. Color
// palettes show the accent role in a panel color.
var palettes = map[string]struct {
	colors []color.Color
	accent color.Color
}{
	"bw":       {colors: demy.PaletteBW},
	"gray4":    {colors: demy.PaletteGray4},
	"gray16":   {colors: demy.PaletteGray16},
	"bwr":      {colors: demy.PaletteBWR, accent: demy.Red},
	"bwy":      {colors: demy.PaletteBWY, accent: demy.Yellow},
	"bwry":     {colors: demy.PaletteBWRY, accent: demy.Red},
	"spectra6": {colors: demy.PaletteSpectra6, accent: demy.Red},
	"acep7":    {colors: demy.PaletteACeP7, accent: demy.Red},
}

func renderHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		status := http.StatusBadRequest
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, fmt.Errorf("reading body: %w", err), status)
		return
	}

	node, sceneStyles, err := xmlscene.UnmarshalScene(body, baseCfg)
	if err != nil {
		writeJSONError(w, fmt.Errorf("parsing scene: %w", err), http.StatusBadRequest)
		return
	}

	cfg := baseCfg
	colorMode := demy.Grayscale
	if mode := r.URL.Query().Get("mode"); mode != "" && mode != "grayscale" {
		p, ok := palettes[mode]
		if !ok {
			writeJSONError(w, fmt.Errorf("unknown mode %q", mode), http.StatusBadRequest)
			return
		}
		colorMode = demy.Paletted
		cfg.Palette = p.colors
		if p.accent != nil {
			cfg.Roles.Accent = p.accent
		}
	}
	if len(sceneStyles) > 0 {
		cfg.Styles = make(map[string]demy.Style, len(sceneStyles))
		maps.Copy(cfg.Styles, sceneStyles)
	}
	canvas, err := demy.New(cfg)
	if err != nil {
		writeJSONError(w, fmt.Errorf("creating canvas: %w", err), http.StatusInternalServerError)
		return
	}

	ditherAlgo := demy.NoDithering
	switch r.URL.Query().Get("dither") {
	case "floyd":
		ditherAlgo = demy.FloydSteinberg
	case "atkinson":
		ditherAlgo = demy.Atkinson
	}

	ctx, cancel := context.WithTimeout(r.Context(), renderTimeout)
	defer cancel()
	var buf bytes.Buffer
	img, err := canvas.Render(ctx, node, colorMode, ditherAlgo)
	if err != nil {
		writeJSONError(w, fmt.Errorf("rendering: %w", err), http.StatusInternalServerError)
		return
	}
	if err := png.Encode(&buf, img); err != nil {
		writeJSONError(w, fmt.Errorf("rendering: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(buf.Bytes())
}

func writeJSONError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, map[string]string{"error": err.Error()})
}

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
		)
	})
}
