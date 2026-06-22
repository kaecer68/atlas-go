package main

// PR11: HTTP API server lifecycle (start, signal handling, graceful
// shutdown). Extracted from main.go run() to isolate the concerns of
// "what to serve" (route registration, which lives in main.go and
// helper files) from "how to serve and shut down" (this file).
//
// The lifecycle is:
//   1. Wrap mux with auth middleware + CSP header
//   2. Start http.Server in a goroutine
//   3. Wait for OS signal, server error, or external shutdown
//   4. Cancel system context, stop realtime adapter, graceful shutdown

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/realtime"
)

// apiServerLifecycleDeps groups the dependencies needed by
// runAPIServerLifecycle. Passed as a struct so the signature stays
// stable as new concerns (e.g. metrics, tracing) are added.
type apiServerLifecycleDeps struct {
	Mux             *http.ServeMux
	APIAddr         string
	SysCancel       context.CancelFunc
	RealtimeAdapter *realtime.RealTimeAdapter
	Shutdown        <-chan struct{}
	ListenAndServe  func(*http.Server) error
}

// runAPIServerLifecycle wires the auth middleware, starts the HTTP
// server, blocks until shutdown is triggered, then gracefully tears
// down. Returns the server error if the server failed (not on
// graceful shutdown).
func runAPIServerLifecycle(d apiServerLifecycleDeps) error {
	authWrappedMux := shared.AuthMiddleware(d.Mux)
	finalMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		if r.URL.Path == "/metrics" || r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/static/") {
			d.Mux.ServeHTTP(w, r)
			return
		}
		authWrappedMux.ServeHTTP(w, r)
	})
	srv := &http.Server{
		Addr:              d.APIAddr,
		Handler:           finalMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	srvErr := make(chan error, 1)
	go func() {
		if err := d.ListenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- fmt.Errorf("dashboard api server failed: %w", err)
		}
	}()

	sigCh := registerShutdownSignal()
	select {
	case <-sigCh:
		log.Printf("received signal, shutting down api server...")
	case err := <-srvErr:
		d.SysCancel()
		return err
	case <-d.Shutdown:
		log.Printf("shutdown signal received, shutting down api server...")
	}

	d.SysCancel()
	if d.RealtimeAdapter != nil {
		d.RealtimeAdapter.Stop()
		log.Printf("[RealTime] adapter stopped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("api server graceful shutdown failed: %v", err)
	} else {
		log.Printf("api server stopped")
	}
	return nil
}
