package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var ErrNoMatchingRoute = errors.New("no matching route found")

type Gateway struct {
	mu            sync.RWMutex
	services      map[string]*Service
	fileToService map[string]string
	routes        map[string][]*route
	client        *http.Client
	configDir     string
}

func New(configDir string) (*Gateway, error) {
	absDir, err := filepath.Abs(configDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("unable to create config directory %s: %w", absDir, err)
	}
	g := &Gateway{
		services:      make(map[string]*Service),
		fileToService: make(map[string]string),
		routes:        make(map[string][]*route),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		configDir: absDir,
	}
	return g, nil
}

func (g *Gateway) ConfigDir() string {
	return g.configDir
}

func (g *Gateway) LoadExisting() error {
	entries, err := os.ReadDir(g.configDir)
	if err != nil {
		return fmt.Errorf("unable to read config directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(g.configDir, entry.Name())
		if !isYAMLFile(path) {
			continue
		}
		g.loadService(path)
	}
	return nil
}

func (g *Gateway) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(g.configDir); err != nil {
		watcher.Close()
		return fmt.Errorf("unable to watch directory %s: %w", g.configDir, err)
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event := <-watcher.Events:
				g.handleWatcherEvent(event)
			case err := <-watcher.Errors:
				if err != nil {
					log.Printf("[gateway] watcher error: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (g *Gateway) handleWatcherEvent(event fsnotify.Event) {
	if event.Name == "" {
		return
	}
	if !isYAMLFile(event.Name) {
		if event.Op&fsnotify.Rename != 0 {
			go g.refreshDirectory()
		}
		return
	}
	if event.Op&fsnotify.Rename != 0 {
		g.removeService(event.Name)
		go g.refreshDirectory()
		return
	}
	if event.Op&fsnotify.Remove != 0 {
		g.removeService(event.Name)
		return
	}
	if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
		go func(path string) {
			time.Sleep(200 * time.Millisecond)
			g.loadService(path)
		}(event.Name)
	}
}

func (g *Gateway) loadService(path string) {
	svc, err := LoadService(path)
	if err != nil {
		log.Printf("[gateway] failed to load service from %s: %v", filepath.Base(path), err)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if oldName, ok := g.fileToService[path]; ok && oldName != svc.Name {
		delete(g.services, oldName)
	}
	g.services[svc.Name] = svc
	g.fileToService[path] = svc.Name
	g.rebuildRoutesLocked()

	log.Printf("[gateway] loaded service %q from %s", svc.Name, filepath.Base(path))
}

func (g *Gateway) removeService(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	name, ok := g.fileToService[path]
	if !ok {
		return
	}
	delete(g.fileToService, path)
	delete(g.services, name)
	g.rebuildRoutesLocked()
	log.Printf("[gateway] removed service %q (source %s)", name, filepath.Base(path))
}

func (g *Gateway) rebuildRoutesLocked() {
	routes := make(map[string][]*route)
	for _, svc := range g.services {
		for _, ep := range svc.Endpoints {
			rt, err := newRoute(svc, ep)
			if err != nil {
				log.Printf("[gateway] skipping endpoint %s %s: %v", ep.Method, ep.Path, err)
				continue
			}
			method := strings.ToUpper(ep.Method)
			routes[method] = append(routes[method], rt)
		}
	}
	g.routes = routes
}

func (g *Gateway) refreshDirectory() {
	time.Sleep(300 * time.Millisecond)
	entries, err := os.ReadDir(g.configDir)
	if err != nil {
		log.Printf("[gateway] failed to refresh directory: %v", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(g.configDir, entry.Name())
		if !isYAMLFile(path) {
			continue
		}
		g.loadService(path)
	}
}

func (g *Gateway) matchRoute(method, requestPath string) (*route, string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	method = strings.ToUpper(method)
	candidates := g.routes[method]
	if len(candidates) == 0 && method == http.MethodHead {
		candidates = g.routes[http.MethodGet]
	}
	for _, rt := range candidates {
		if targetPath, ok := rt.matchPath(requestPath); ok {
			return rt, targetPath
		}
	}
	return nil, ""
}

func (g *Gateway) ProxyRequest(w http.ResponseWriter, r *http.Request) error {
	rt, targetPath := g.matchRoute(r.Method, r.URL.Path)
	if rt == nil {
		return ErrNoMatchingRoute
	}

	baseURL, err := url.Parse(rt.service.Address)
	if err != nil {
		return fmt.Errorf("invalid service address for %s: %w", rt.service.Name, err)
	}

	requestURL := &url.URL{Path: targetPath, RawQuery: r.URL.RawQuery}
	fullURL := baseURL.ResolveReference(requestURL)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, fullURL.String(), r.Body)
	if err != nil {
		return err
	}
	copyHeaders(req.Header, r.Header)
	req.Header.Set("X-Forwarded-Host", r.Host)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		req.Header.Set("X-Forwarded-Proto", proto)
	} else if r.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	log.Printf("[gateway] proxy %s %s -> %s", r.Method, r.URL.Path, fullURL.String())
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead && resp.Body != nil {
		if _, err := io.Copy(w, resp.Body); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gateway) OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	scheme := "http"
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	} else if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	payload, err := g.BuildOpenAPISpec(baseURL)
	if err != nil {
		log.Printf("[gateway] failed to build OpenAPI spec: %v", err)
		http.Error(w, "failed to build OpenAPI spec", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

func (g *Gateway) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := g.ProxyRequest(w, r); err != nil {
		if errors.Is(err, ErrNoMatchingRoute) {
			http.Error(w, "no matching endpoint", http.StatusNotFound)
			return
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("[gateway] proxy error: %v", err)
		http.Error(w, "proxy error", http.StatusBadGateway)
	}
}

func (g *Gateway) ServicesSnapshot() []*Service {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*Service, 0, len(g.services))
	for _, svc := range g.services {
		out = append(out, svc)
	}
	return out
}

func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func copyHeaders(dst, src http.Header) {
	for k := range dst {
		dst.Del(k)
	}
	for k, vals := range src {
		if isHopHeader(k) {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for _, header := range hopHeaders {
		dst.Del(header)
	}
	for k, vals := range src {
		if isHopHeader(k) {
			continue
		}
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func isHopHeader(name string) bool {
	name = http.CanonicalHeaderKey(name)
	for _, h := range hopHeaders {
		if http.CanonicalHeaderKey(h) == name {
			return true
		}
	}
	return false
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
}
