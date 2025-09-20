package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type todo struct {
	ID        int       `json:"id"`
	Task      string    `json:"task"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"createdAt"`
}

type createTodoRequest struct {
	Task string `json:"task"`
}

var (
	todos  = []todo{}
	nextID = 1
	mu     sync.Mutex
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/todos", todosHandler)

	srv := &http.Server{
		Addr:              ":9002",
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("[todo] listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("todo service failed: %v", err)
	}
}

func todosHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListTodos(w, r)
	case http.MethodPost:
		handleCreateTodo(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleListTodos(w http.ResponseWriter, _ *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(todos); err != nil {
		log.Printf("[todo] failed to encode todos: %v", err)
	}
}

func handleCreateTodo(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var payload createTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	if payload.Task == "" {
		http.Error(w, "task is required", http.StatusBadRequest)
		return
	}

	mu.Lock()
	todo := todo{
		ID:        nextID,
		Task:      payload.Task,
		Completed: false,
		CreatedAt: time.Now().UTC(),
	}
	nextID++
	todos = append(todos, todo)
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(todo); err != nil {
		log.Printf("[todo] failed to encode created todo: %v", err)
	}
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
		log.Printf("[todo] %s %s -> %d (%s)", r.Method, r.URL.Path, lrw.status, time.Since(start))
	})
}
