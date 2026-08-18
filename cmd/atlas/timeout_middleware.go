package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultRequestTimeout is the wall-clock ceiling for ordinary HTTP API
// requests. It is intentionally below the previous observed 20-45 s hangs so
// that a single stuck upstream cannot cascade to the whole mux.
const defaultRequestTimeout = 8 * time.Second

// longRequestTimeout is applied to routes that are expected to run longer
// than an ordinary API call but still must not hang indefinitely.
const longRequestTimeout = 60 * time.Second

// timeoutRouteOverride maps an exact request path to its allowed duration.
// A duration of zero means the route is exempt from the timeout wrapper
// (used for Server-Sent Events streaming endpoints that are intentionally
// long-lived). Add new routes here explicitly; do not use path patterns.
var timeoutRouteOverrides = map[string]time.Duration{
	// SSE streaming: client connections stay open for events.
	"/api/events/stream": 0,

	// Backtest submission/reporting: may run heavy computations.
	"/api/backtest/run": longRequestTimeout,

	// Daily/periodic reports can be slower than ordinary reads.
	"/api/reports/latest":          longRequestTimeout,
	"/api/reports/archive":         longRequestTimeout,
	"/api/reports/subscribe":       longRequestTimeout,
	"/api/report/latest":           longRequestTimeout,
	"/api/report/list":             longRequestTimeout,
	"/api/dashboard/daily-summary": longRequestTimeout,

	// Cross-market fan-out (28 channels) and US-indices can occasionally
	// exceed 8s on a cache-miss refetch; give them the long budget so a
	// slow channel doesn't 503 (see R3).
	"/api/cross-market/status":  longRequestTimeout,
	"/api/dashboard/us-indices": longRequestTimeout,
}

// timeoutResponseWriter wraps http.ResponseWriter so the timeout middleware
// can detect whether the handler has already started writing a response.
type timeoutResponseWriter struct {
	http.ResponseWriter
	mu          sync.Mutex
	wroteHeader bool
	code        int
}

func (tw *timeoutResponseWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.wroteHeader {
		return
	}
	tw.wroteHeader = true
	tw.code = code
	tw.ResponseWriter.WriteHeader(code)
}

func (tw *timeoutResponseWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	if !tw.wroteHeader {
		tw.wroteHeader = true
		tw.code = http.StatusOK
	}
	tw.mu.Unlock()
	return tw.ResponseWriter.Write(p)
}

func (tw *timeoutResponseWriter) status() (bool, int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	return tw.wroteHeader, tw.code
}

// timeoutConfig lets tests configure the middleware without changing the
// production default.
type timeoutConfig struct {
	DefaultTimeout time.Duration
	Overrides      map[string]time.Duration
}

// withTimeout wraps h with a request-scoped context timeout. The timeout
// context replaces r.Context(), so cancellation propagates to handlers and
// downstream I/O. If the deadline fires before the handler finishes and the
// handler has not yet written a response, the wrapper writes a 503 JSON body.
//
// Whitelisted routes (duration == 0) are passed through untouched so SSE
// streams and similar long-lived connections are not interrupted.
func withTimeout(h http.Handler, cfg timeoutConfig) http.Handler {
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = defaultRequestTimeout
	}
	if cfg.Overrides == nil {
		cfg.Overrides = timeoutRouteOverrides
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dur := cfg.timeoutFor(r.URL.Path)
		if dur == 0 {
			h.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), dur)
		defer cancel()
		r = r.WithContext(ctx)

		tw := &timeoutResponseWriter{ResponseWriter: w}
		done := make(chan struct{})

		go func() {
			defer close(done)
			h.ServeHTTP(tw, r)
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			alreadyWrote, _ := tw.status()
			if alreadyWrote {
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "upstream timeout",
				"degraded": true,
			})
		}
	})
}

func (cfg timeoutConfig) timeoutFor(path string) time.Duration {
	if d, ok := cfg.Overrides[path]; ok {
		return d
	}
	// Task execution SSE uses a dynamic task ID: /api/tasks/{id}/events
	if strings.HasPrefix(path, "/api/tasks/") && strings.HasSuffix(path, "/events") {
		return 0
	}
	return cfg.DefaultTimeout
}
