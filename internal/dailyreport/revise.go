package dailyreport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// sha256HexKey returns the hex-encoded SHA-256 of s, or empty if s is empty.
// Local helper mirroring cmd/atlas/admin_routes.go::sha256HexKey (which
// itself mirrors internal/monitoring/api/shared.sha256Hex) so the dailyreport
// package enforces the same ATLAS_API_KEY guard without importing main.
func sha256HexKey(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// wrapAdminAuth enforces the ATLAS_API_KEY admin guard around h, following
// cmd/atlas/admin_routes.go::wrapAdminAuth exactly:
//   - production + key unset → 503 (fail-closed, mirror of AuthMiddleware)
//   - key set → X-API-Key or Authorization: Bearer <key> must match
//   - non-production without a key → pass-through (dev mode)
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
				if strings.HasPrefix(auth, "Bearer ") {
					provided = strings.TrimPrefix(auth, "Bearer ")
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

// errReportNotFound marks a revise/approve request for a date with no
// persisted report.
var errReportNotFound = errors.New("report not found")

// HandleRevise applies a manual correction to a persisted report.
// POST /api/reports/{date}/revise — admin auth required.
//
// Body: {"note": "...", "by": "operator", "fields": [{"path": "...", "value": ...}]}.
// Whitelisted paths cover StrategySection / PeriodSection / RiskSection only.
// On success the corrected report (with full revision history embedded) is
// persisted back to the same YYYY-MM-DD.json file.
func (h *Handler) HandleRevise(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	if date == "" {
		http.Error(w, "date path segment required", http.StatusBadRequest)
		return
	}
	if h.gen == nil {
		http.Error(w, "report generator unavailable", http.StatusServiceUnavailable)
		return
	}
	var req ReviseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON body: %v", err), http.StatusBadRequest)
		return
	}
	rep, err := h.gen.Revise(date, req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errReportNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rep); err != nil {
		log.Printf("[DailyReport] encode revise response: %v", err)
	}
}

// HandleApprove marks a persisted report as approved without revision.
// POST /api/reports/{date}/approve — admin auth required.
func (h *Handler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	if date == "" {
		http.Error(w, "date path segment required", http.StatusBadRequest)
		return
	}
	if h.gen == nil {
		http.Error(w, "report generator unavailable", http.StatusServiceUnavailable)
		return
	}
	rep, err := h.gen.Approve(date)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errReportNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rep); err != nil {
		log.Printf("[DailyReport] encode approve response: %v", err)
	}
}

// HandleTrackedClaims lists tracked claims, optionally filtered by
// ?report_date=YYYY-MM-DD and ?status=tracking|verified|expired.
// GET /api/reports/tracked-claims — public read, mirrors latest/archive.
func (h *Handler) HandleTrackedClaims(w http.ResponseWriter, r *http.Request) {
	if h.tracker == nil {
		http.Error(w, "claim tracker unavailable", http.StatusServiceUnavailable)
		return
	}
	claims := h.tracker.ListClaims()
	reportDate := r.URL.Query().Get("report_date")
	status := r.URL.Query().Get("status")
	if reportDate != "" || status != "" {
		filtered := make([]TrackedClaim, 0, len(claims))
		for _, c := range claims {
			if reportDate != "" && c.ReportDate != reportDate {
				continue
			}
			if status != "" && string(c.Status) != status {
				continue
			}
			filtered = append(filtered, c)
		}
		claims = filtered
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"claims": claims, "count": len(claims)}); err != nil {
		log.Printf("[DailyReport] encode tracked claims: %v", err)
	}
}
