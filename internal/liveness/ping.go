package liveness

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// PingRequest is the payload of POST /api/internal/task-liveness, sent by
// cron containers when a job finishes.
type PingRequest struct {
	TaskName   string `json:"task_name"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// PingEnvToken is the env var (also read through config.GetSecret, i.e.
// macOS Keychain fallback) holding the shared secret for the internal
// liveness ping endpoint.
//
//nolint:gosec // G101 false positive: the value is an env-var NAME, not a credential
const PingEnvToken = "ATLAS_LIVENESS_TOKEN"

// HandlePing is the HTTP handler for the internal liveness ping. Security
// model (deliberately minimal, see PR body):
//   - The route is NOT in isPublicPath: it bypasses the API-key
//     AuthMiddleware only via an explicit mux-level allowlist in
//     cmd/atlas/main.go and is otherwise unreachable.
//   - It requires the shared secret in the X-Liveness-Token header.
//   - Fail-closed: when ATLAS_LIVENESS_TOKEN is not configured the endpoint
//     returns 503 so a misconfigured deployment is loud, not silently open.
//   - It is a plain upsert: exit_code 0 -> success, non-zero -> failure with
//     "exit code N" as the last error. Write failure never affects the cron
//     job itself (the caller already finished).
func HandlePing(store *Store, w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if store == nil {
		writePingError(w, http.StatusServiceUnavailable, "liveness store not configured")
		return
	}
	expected := config.GetSecret(PingEnvToken)
	if expected == "" {
		writePingError(w, http.StatusServiceUnavailable,
			"ATLAS_LIVENESS_TOKEN not configured; set it in the atlas env to enable internal liveness pings")
		return
	}
	got := r.Header.Get("X-Liveness-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		writePingError(w, http.StatusUnauthorized, "invalid or missing X-Liveness-Token")
		return
	}

	var req PingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePingError(w, http.StatusBadRequest, "decode ping body: "+err.Error())
		return
	}
	if req.TaskName == "" {
		writePingError(w, http.StatusBadRequest, "task_name is required")
		return
	}

	var runErr error
	if req.ExitCode != 0 {
		msg := req.Error
		if msg == "" {
			msg = "exit code " + strconv.Itoa(req.ExitCode)
		}
		runErr = &PingError{ExitCode: req.ExitCode, Msg: msg}
	}
	dur := time.Duration(req.DurationMs) * time.Millisecond
	if err := store.Record(ctx, RecordInput{TaskName: req.TaskName, Err: runErr, Duration: dur}); err != nil {
		logging.Warn("liveness", "ping_record_failed", "task", req.TaskName, "err", err.Error())
		writePingError(w, http.StatusInternalServerError, "record failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"task_name": req.TaskName,
	})
}

// PingError marks a non-zero cron exit as a task failure.
type PingError struct {
	ExitCode int
	Msg      string
}

func (e *PingError) Error() string { return e.Msg }

func writePingError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "error": msg})
}
