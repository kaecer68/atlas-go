package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
)

// Handler is the new handler signature for monitoring API handlers.
// It receives only the HTTP request and returns a status code with structured data.
// Unlike the standard http.Handler, it does NOT write to the ResponseWriter directly —
// WriteJSON is called centrally by Adapt(), eliminating 73 duplicate call sites.
type Handler func(r *http.Request) (status int, data any)

// authFreeExactPaths and authFreePrefixPaths are kept for callers
// that wire AuthMiddleware directly (e.g. Adapt). The canonical
// public-path bypass now lives in cmd/atlas/main.go isPublicPath,
// which is the single source of truth for the top-level HTTP mux.
// These lists mirror that decision so AuthMiddleware stays
// self-contained for direct callers.
var authFreeExactPaths = map[string]bool{
	"/health":                       true,
	"/metrics":                      true,
	"/admin":                        true,
	"/client":                       true,
	"/api/llm/health":               true,
	"/api/health/aggregate":         true, // Stage 6 PR#1: 4-tier health aggregation for frontend banner
	"/api/v1/alerts":                true, // Alertmanager webhook inbound
	"/api/alerts":                   true,
	"/api/stock":                    true,
	"/api/recommendations":          true,
	"/api/tasks":                    true,
	"/api/field-contract":           true,
	"/api/control/audit-log":        true,
	"/api/control/active-overrides": true,
	"/api/experiment/history":       true,
	"/api/experiment/diff":          true,
	"/api/strategies":               true,
}

// Per internal/monitoring/AGENTS.md, these prefix lists must mirror
// cmd/atlas/main.go isPublicPath. Write operations under /api/control/
// and /api/experiment/ (approve-recommendation, reject-recommendation,
// pause/resume-agent, sector-ban, promote, revert, judge) intentionally
// remain outside this whitelist and continue to require an API key.
var authFreePrefixPaths = []string{
	"/admin/",
	"/client/",
	"/api/dashboard/",
	"/api/taiwan/",
	"/api/narrative/",
	"/api/macro/",
	"/api/market/",
	"/api/alerts/",
	"/api/synergy/",
	"/api/cross-market/",
	"/api/detector/",
	"/api/capital-flow/",
	"/api/events/",
	"/api/industry/",
	"/api/stock/",
	"/api/janus/",
	"/api/strategies/",
	"/api/risk/",
	"/api/regime/",
	"/api/geopolitical/",
	"/api/scheduler/",
	"/api/tasks/",
	"/api/traces/",
	"/api/llm/",
	"/api/llm_annotator/",
	"/api/prism/",
	"/api/recommendations/",
	"/api/reports/",
	"/api/strategy-ranker/",
	"/api/parameters/",
	"/api/backtest/",
	"/api/janus/",
	"/api/dashboard/sessions/",
}

func isAuthFreePath(p string) bool {
	if authFreeExactPaths[p] {
		return true
	}
	for _, prefix := range authFreePrefixPaths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// sha256Hex returns the hex-encoded SHA-256 hash of s, or empty string if s is empty.
// Used to compare API keys via hash rather than plaintext, preventing plaintext key
// exposure in process memory if an env-var dump occurs.
func sha256Hex(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// AuthStatus returns the current authentication posture for system health reporting:
// "production" (key set + prod env), "authenticated" (key set + non-prod),
// or "dev_no_auth" (key not set — all /api/* endpoints are open).
func AuthStatus() string {
	apiKey := os.Getenv("ATLAS_API_KEY")
	isProduction := strings.ToLower(os.Getenv("ATLAS_ENV")) == "production"
	if isProduction && apiKey != "" {
		return "production"
	}
	if apiKey != "" {
		return "authenticated"
	}
	return "dev_no_auth"
}

// AuthMiddleware checks API key authentication.
// In production (ATLAS_ENV=production), ATLAS_API_KEY is mandatory.
// It accepts either Authorization: Bearer <key> or X-API-Key: <key>.
//
// Probing paths (/health, /metrics) are always passed through
// unconditionally — see authFreePaths. This makes the middleware
// self-contained: callers don't need to remember to route around it.
func AuthMiddleware(next http.Handler) http.Handler {
	apiKey := os.Getenv("ATLAS_API_KEY")
	isProduction := strings.ToLower(os.Getenv("ATLAS_ENV")) == "production"
	if isProduction && apiKey == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAuthFreePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			WriteJSONErrorEx(w, http.StatusServiceUnavailable, "503", "server misconfigured: ATLAS_API_KEY required in production")
		})
	}
	if apiKey == "" {
		log.Println("[WARNING] ATLAS_API_KEY not set — all /api/* endpoints are unauthenticated (dev mode)")
		return next
	}
	expectedHash := sha256Hex(apiKey)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAuthFreePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get("X-API-Key")
		if provided == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if sha256Hex(provided) != expectedHash {
			WriteJSONErrorEx(w, http.StatusUnauthorized, "401", "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Adapt wraps a Handler into a standard http.Handler.
// This is the single place in the monitoring API layer where WriteJSON is called.
// All new handlers should use this pattern.
func Adapt(h Handler) http.Handler {
	return AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Protect against large request body attacks — 1MB limit for JSON APIs
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
		status, data := h(r)
		WriteJSON(w, status, data)
	}))
}

// Get is a convenience adapter for GET-only endpoints.
// It adds method enforcement before delegating to the Handler.
func Get(h Handler) http.Handler {
	return Adapt(func(r *http.Request) (int, any) {
		if r.Method != http.MethodGet {
			return http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}
		}
		return h(r)
	})
}

// Post is a convenience adapter for POST-only endpoints.
// It adds method enforcement before delegating to the Handler.
func Post(h Handler) http.Handler {
	return Adapt(func(r *http.Request) (int, any) {
		if r.Method != http.MethodPost {
			return http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}
		}
		return h(r)
	})
}

// RawHandler is for handlers that need direct access to http.ResponseWriter
// (raw bytes, streaming, non-JSON content types like HTML/markdown).
// When a RawHandler returns (0, nil), it signals that the response was
// already written and AdaptRaw should not write anything further.
type RawHandler func(w http.ResponseWriter, r *http.Request) (status int, data any)

// AdaptRaw wraps a RawHandler into a standard http.Handler.
// If the handler already wrote the response (returned 0, nil), AdaptRaw
// returns immediately. Otherwise it writes JSON like Adapt.
func AdaptRaw(h RawHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
		status, data := h(w, r)
		if status == 0 && data == nil {
			return
		}
		WriteJSON(w, status, data)
	})
}

// GetRaw is a convenience adapter for GET-only RawHandler endpoints.
func GetRaw(h RawHandler) http.Handler {
	return AdaptRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		if r.Method != http.MethodGet {
			return http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}
		}
		return h(w, r)
	})
}

// RequireAdmin wraps a Handler to require ATLAS_ADMIN_KEY for destructive operations.
// In non-production environments without an admin key set, it passes through.
func RequireAdmin(h Handler) Handler {
	return func(r *http.Request) (int, any) {
		adminKey := os.Getenv("ATLAS_ADMIN_KEY")
		if adminKey == "" {
			return h(r)
		}
		expectedHash := sha256Hex(adminKey)
		provided := r.Header.Get("X-Admin-Key")
		if provided == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Admin ") {
				provided = strings.TrimPrefix(auth, "Admin ")
			}
		}
		if sha256Hex(provided) != expectedHash {
			return http.StatusUnauthorized, map[string]string{"error": "admin access required"}
		}
		return h(r)
	}
}

// AdminPost is a convenience adapter for POST-only admin-protected endpoints.
func AdminPost(h Handler) http.Handler {
	return Post(RequireAdmin(h))
}
