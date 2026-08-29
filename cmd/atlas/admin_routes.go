package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/adminapi/deployment"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// sha256HexKey returns the hex-encoded SHA-256 of s, or empty if s is empty.
// Local helper mirroring internal/monitoring/api/shared.sha256Hex for the
// atlas main binary's wrapAdminAuth guard.
func sha256HexKey(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// wrapAdminAuth returns an http.HandlerFunc that enforces an API key
// guard around h. In production (ATLAS_ENV=production), ATLAS_API_KEY is
// mandatory — the guard returns 503 if the key is unset, matching the
// AuthMiddleware production fail-closed policy. In non-production
// environments without a key set, the guard is a no-op.
func wrapAdminAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := os.Getenv("ATLAS_API_KEY")
		isProduction := strings.ToLower(os.Getenv("ATLAS_ENV")) == "production"
		if isProduction && apiKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			//nolint:errcheck
			fmt.Fprintf(w, `{"error":"server misconfigured: ATLAS_API_KEY required in production"}`+"\n")
			return
		}
		if apiKey != "" {
			expectedHash := sha256HexKey(apiKey)
			provided := r.Header.Get("X-API-Key")
			if provided == "" {
				auth := r.Header.Get("Authorization")
				if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
					provided = after
				}
			}
			if sha256HexKey(provided) != expectedHash {
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
func RegisterAdminRoutes(mux *http.ServeMux, cfg config.Config, pm *fubonproxy.ProcessManager) {
	mux.HandleFunc("/admin/reload-config", wrapAdminAuth(handleAdminReloadConfig))
	mux.HandleFunc("/api/admin/calibrate-thresholds", wrapAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		handleAdminCalibrateThresholds(w, r, cfg)
	}))

	// Phase 3.5 M1: deployment dashboard — read-only snapshot of fubon-proxy status.
	dh := deployment.NewHandlers(pm)
	mux.HandleFunc("/api/admin/live/deployment/dashboard", wrapAdminAuth(dh.HandleDeploymentDashboard))
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
