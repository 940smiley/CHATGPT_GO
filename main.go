package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"chatgpt_go/internal/gateway"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	defaultConfigDir := filepath.Join(".", "mcp_servers")
	configDir := envOrDefault("CHATGPT_GATEWAY_CONFIG", defaultConfigDir)
	addr := resolveListenAddr()

	flag.StringVar(&configDir, "config", configDir, "Directory containing MCP server definitions")
	flag.StringVar(&addr, "addr", addr, "Address for the gateway server (host:port or :port)")
	flag.Parse()

	gw, err := gateway.New(configDir)
	if err != nil {
		log.Fatalf("failed to initialise gateway: %v", err)
	}

	if err := gw.LoadExisting(); err != nil {
		log.Fatalf("failed to load service definitions: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := gw.Watch(ctx); err != nil {
		log.Fatalf("failed to start config watcher: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.json", gw.OpenAPIHandler)
	mux.HandleFunc("/", gw.ProxyHandler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		services := gw.ServicesSnapshot()
		if len(services) == 0 {
			log.Printf("[gateway] no services detected yet. Drop MCP YAML files into %s", gw.ConfigDir())
		} else {
			for _, svc := range services {
				log.Printf("[gateway] service ready: %s -> %s (%d endpoints)", svc.Name, svc.Address, len(svc.Endpoints))
			}
		}
		log.Printf("[gateway] listening on %s (config dir: %s)", addr, gw.ConfigDir())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("[gateway] received signal %s, shutting down", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("[gateway] shutdown cancelled: %v", err)
		} else {
			log.Printf("[gateway] graceful shutdown failed: %v", err)
		}
	} else {
		log.Printf("[gateway] shutdown complete")
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func resolveListenAddr() string {
	if addr := os.Getenv("CHATGPT_GATEWAY_ADDR"); addr != "" {
		return addr
	}
	port := envOrDefault("CHATGPT_GATEWAY_PORT", "8080")
	if !strings.Contains(port, ":") {
		return fmt.Sprintf(":%s", port)
	}
	return port
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lrw *loggingResponseWriter) WriteHeader(statusCode int) {
	lrw.status = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)
		log.Printf("[http] %s %s -> %d (%s)", r.Method, r.URL.Path, lrw.status, duration)
	})
}
