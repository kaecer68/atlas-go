package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// wrapAdminAuth returns an http.HandlerFunc that enforces an API key
// guard around h. If ATLAS_API_KEY is unset, the guard is a no-op
// (intended for local development). Otherwise, requests must supply
// the key via X-API-Key header or Authorization: Bearer <key>.
func wrapAdminAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := os.Getenv("ATLAS_API_KEY")
		if apiKey != "" {
			provided := r.Header.Get("X-API-Key")
			if provided == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					provided = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if provided != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				//nolint:errcheck
				fmt.Fprintf(w, `{"error":"unauthorized"}`+"\n")
				return
			}
		}
		h(w, r)
	}
}

// RegisterAdminRoutes installs the low-risk admin endpoints on mux.
// The complex /admin/trigger-simulation route stays inline in run()
// until Wave 3 PR8 (api bootstrap extraction).
func RegisterAdminRoutes(mux *http.ServeMux, cfg config.Config) {
	mux.HandleFunc("/admin/reload-config", wrapAdminAuth(handleAdminReloadConfig))
	mux.HandleFunc("/api/admin/calibrate-thresholds", wrapAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		handleAdminCalibrateThresholds(w, r, cfg)
	}))
}

// handleAdminReloadConfig reloads parameters.json from disk and returns
// the new version string. POST only.
func handleAdminReloadConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := config.ReloadParametersConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to reload config: %v", err), http.StatusInternalServerError)
		return
	}
	cfg := config.GetParametersConfig()
	w.Header().Set("Content-Type", "application/json")
	//nolint:errcheck
	fmt.Fprintf(w, `{"status":"ok","version":"%s"}`+"\n", cfg.Version)
}

// handleAdminCalibrateThresholds re-runs industry.RecalibrateThresholds
// against the latest month_revenue.jsonl and writes the result back to
// parameters.json. POST only.
func handleAdminCalibrateThresholds(w http.ResponseWriter, r *http.Request, cfg config.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	revenuePath := filepath.Join(cfg.WorkDir, "data", "replay", "month_revenue.jsonl")
	configPath := filepath.Join(cfg.WorkDir, "configs", "parameters.json")
	if err := industry.RecalibrateThresholds(revenuePath, configPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		//nolint:errcheck
		fmt.Fprintf(w, `{"error":"%s"}`+"\n", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	//nolint:errcheck
	fmt.Fprintf(w, `{"status":"ok","message":"thresholds recalibrated"}`+"\n")
}
