// Package metrics exposes a dependency-free Prometheus text endpoint for service basics.
package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

type Registry struct {
	mu       sync.Mutex
	requests map[string]uint64
}

func New() *Registry { return &Registry{requests: make(map[string]uint64)} }

func (r *Registry) Observe(method, route string, status int) {
	r.mu.Lock()
	r.requests[method+"\x00"+route+"\x00"+strconv.Itoa(status)]++
	r.mu.Unlock()
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintln(w, "# HELP kiticket_http_requests_total Total HTTP responses.")
	_, _ = fmt.Fprintln(w, "# TYPE kiticket_http_requests_total counter")
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range r.requests {
		parts := splitKey(key)
		_, _ = fmt.Fprintf(w, "kiticket_http_requests_total{method=%q,route=%q,status=%q} %d\n", parts[0], parts[1], parts[2], value)
	}
}

func splitKey(key string) [3]string {
	var result [3]string
	index := 0
	start := 0
	for i := range key {
		if key[i] == 0 && index < 2 {
			result[index] = key[start:i]
			index++
			start = i + 1
		}
	}
	result[index] = key[start:]
	return result
}
