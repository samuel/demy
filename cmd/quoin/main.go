package main

import (
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	cfgPath := flag.String("config", "quoin.yaml", "path to YAML config file")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	flag.Parse()

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		log.Fatalf("invalid log level %q: %v", *logLevel, err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	store := newMemStore()

	srv, err := NewServer(cfg, store)
	if err != nil {
		log.Fatalf("server init: %v", err)
	}

	mux := http.NewServeMux()
	srv.Register(mux)

	httpSrv := &http.Server{
		Addr:           *addr,
		Handler:        requestLogger(mux),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	slog.Info("quoin listening", "addr", *addr)
	log.Fatal(httpSrv.ListenAndServe())
}
