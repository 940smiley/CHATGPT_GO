package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type forecast struct {
	City        string `json:"city"`
	Temperature string `json:"temperature"`
	Condition   string `json:"condition"`
	UpdatedAt   string `json:"updatedAt"`
}

func main() {
	rand.Seed(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/weather/", weatherHandler)

	srv := &http.Server{
		Addr:              ":9001",
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("[weather] listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("weather service failed: %v", err)
	}
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	citySlug := strings.TrimPrefix(r.URL.Path, "/weather/")
	citySlug = strings.Trim(citySlug, "/")
	if citySlug == "" {
		http.Error(w, "city is required", http.StatusBadRequest)
		return
	}

	conditions := []string{"Sunny", "Partly Cloudy", "Overcast", "Rain", "Thunderstorms", "Snow"}
	condition := conditions[rand.Intn(len(conditions))]
	temperature := fmt.Sprintf("%d°C", rand.Intn(15)+10)

	forecast := forecast{
		City:        prettifyCity(citySlug),
		Temperature: temperature,
		Condition:   condition,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(forecast); err != nil {
		log.Printf("[weather] failed to encode response: %v", err)
	}
}

func prettifyCity(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		parts[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lrw *loggingResponseWriter) WriteHeader(statusCode int) {
	lrw.status = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(lrw, r)
		log.Printf("[weather] %s %s -> %d (%s)", r.Method, r.URL.Path, lrw.status, time.Since(start))
	})
}
