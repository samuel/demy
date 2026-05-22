package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/samuel/pate"
	"github.com/samuel/pate/scene"
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed static
var staticFiles embed.FS

var renderCanvas *pate.Canvas

type lesson struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Desc      string `json:"desc"`
	GoEquiv   string `json:"go_equiv"`
	SceneJSON string `json:"scene_json"`
}

func main() {
	font := pate.MustLoadFontBytes("sans", goregular.TTF)
	cfg := pate.TRMNLOriginal
	cfg.Fonts = map[string]*pate.FontFamily{"sans": font}
	var err error
	renderCanvas, err = pate.New(cfg)
	if err != nil {
		log.Fatalf("creating canvas: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", indexHandler)
	mux.HandleFunc("GET /lessons", lessonsHandler)
	mux.HandleFunc("POST /render", renderHandler)

	const addr = ":8081"
	srv := &http.Server{
		Addr:           addr,
		Handler:        requestLogger(mux),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	slog.Info("pate-tutorial listening", "addr", addr, "url", "http://localhost"+addr)
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
	_ = json.NewEncoder(w).Encode(tutorialLessons)
}

func renderHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSONError(w, fmt.Errorf("reading body: %w", err), http.StatusBadRequest)
		return
	}

	node, err := scene.Unmarshal(body)
	if err != nil {
		writeJSONError(w, fmt.Errorf("parsing scene: %w", err), http.StatusBadRequest)
		return
	}

	colorMode := pate.Grayscale
	switch r.URL.Query().Get("mode") {
	case "2bit":
		colorMode = pate.TwoBit
	case "1bit":
		colorMode = pate.OneBit
	}

	ditherAlgo := pate.NoDithering
	switch r.URL.Query().Get("dither") {
	case "floyd":
		ditherAlgo = pate.FloydSteinberg
	case "atkinson":
		ditherAlgo = pate.Atkinson
	}

	var buf bytes.Buffer
	if err := renderCanvas.EncodePNG(node, &buf, colorMode, ditherAlgo); err != nil {
		writeJSONError(w, fmt.Errorf("rendering: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(buf.Bytes())
}

func writeJSONError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
