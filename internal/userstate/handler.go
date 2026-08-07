package userstate

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// Handler exposes the per-user signal-state store over HTTP. Routes are
// registered under /api/user/signals/* and require a valid JWT (the
// AuthMiddleware injects claims into the request context; we extract the
// userID via subscription.GetClaims).
//
// Endpoints:
//
//	GET    /api/user/signals                       — list current user's states
//	PUT    /api/user/signals/{signalKey}/ack       — mark acknowledged
//	PUT    /api/user/signals/{signalKey}/dismiss   — mark dismissed
//	DELETE /api/user/signals/{signalKey}           — remove (reset to "new")
type Handler struct {
	store SignalStateStore
}

// NewHandler binds the HTTP layer to a SignalStateStore.
func NewHandler(store SignalStateStore) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes wires the 4 endpoints under /api/user/signals. The
// authMiddleware must be the production AuthMiddleware (subscription's
// NewAuthMiddleware with allowGuest=false in production). With
// allowGuest=true, the guest claim has UserID=0 and routes would be
// effectively read-only (no per-user records to load) — callers should
// ensure the middleware is configured strict for user state.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authMiddleware *subscription.AuthMiddleware) {
	if authMiddleware == nil {
		// Defensive: refuse to register un-authenticated routes. The product
		// positioning §9 promise ("追蹤 → 紀律") is per-user and exposing
		// it without a user identity is meaningless.
		return
	}
	mux.Handle("GET /api/user/signals", authMiddleware.Wrap(http.HandlerFunc(h.handleList)))
	mux.Handle("PUT /api/user/signals/{signalKey}/ack", authMiddleware.Wrap(http.HandlerFunc(h.handleAck)))
	mux.Handle("PUT /api/user/signals/{signalKey}/dismiss", authMiddleware.Wrap(http.HandlerFunc(h.handleDismiss)))
	mux.Handle("DELETE /api/user/signals/{signalKey}", authMiddleware.Wrap(http.HandlerFunc(h.handleDelete)))
}

// handleList returns all signal states for the authenticated user, ordered
// by UpdatedAt DESC (newest first — matches the store's sort).
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	claims := subscription.GetClaims(r)
	if claims == nil || claims.UserID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	records, err := h.store.LoadByUser(claims.UserID)
	if err != nil {
		http.Error(w, errJSON(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"signals": records,
		"count":   len(records),
	})
}

// handleAck marks the (user, signal) as acknowledged — sets AcknowledgedAt
// to now (preserving an earlier acknowledgement timestamp) and refreshes
// UpdatedAt. The store replaces records verbatim, so the full state must be
// constructed here (never passed as a zero-value state).
func (h *Handler) handleAck(w http.ResponseWriter, r *http.Request) {
	signalKey, ok := requireSignalKey(w, r)
	if !ok {
		return
	}
	claims := subscription.GetClaims(r)
	if claims == nil || claims.UserID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	now := time.Now().UTC()
	state := UserSignalState{
		UserID:    claims.UserID,
		SignalKey: signalKey,
	}
	// Preserve the first acknowledgement timestamp across repeated acks
	// (idempotent "read" — re-reading does not move the ack time).
	if existing, err := h.store.LoadByUserAndSignal(claims.UserID, signalKey); err == nil && existing.AcknowledgedAt != nil {
		state.AcknowledgedAt = existing.AcknowledgedAt
	} else {
		state.AcknowledgedAt = &now
	}
	state.UpdatedAt = now
	if err := h.store.Upsert(state); err != nil {
		http.Error(w, errJSON(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

// handleDismiss marks the (user, signal) as dismissed — the user does not
// want to see this signal at all. Dismissing also acknowledges (the user
// has clearly seen the signal).
func (h *Handler) handleDismiss(w http.ResponseWriter, r *http.Request) {
	signalKey, ok := requireSignalKey(w, r)
	if !ok {
		return
	}
	claims := subscription.GetClaims(r)
	if claims == nil || claims.UserID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	now := time.Now().UTC()
	state := UserSignalState{
		UserID:         claims.UserID,
		SignalKey:      signalKey,
		AcknowledgedAt: &now,
		Dismissed:      true,
		UpdatedAt:      now,
	}
	if err := h.store.Upsert(state); err != nil {
		http.Error(w, errJSON(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

// handleDelete removes the (user, signal) record — the dashboard then
// re-renders the signal as "new" on the next page load.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	signalKey, ok := requireSignalKey(w, r)
	if !ok {
		return
	}
	claims := subscription.GetClaims(r)
	if claims == nil || claims.UserID == 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	// Upsert with a zero-value (AcknowledgedAt=nil, Dismissed=false) — the
	// store's Upsert replaces the existing record, effectively "resetting"
	// the user's state for this signal. We do not delete the file row
	// directly to keep the JSONL append-only invariant.
	state := UserSignalState{
		UserID:    claims.UserID,
		SignalKey: signalKey,
	}
	if err := h.store.Upsert(state); err != nil {
		http.Error(w, errJSON(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, state)
}

// requireSignalKey extracts {signalKey} from the path and rejects empty
// keys with 400.
func requireSignalKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := strings.TrimSpace(r.PathValue("signalKey"))
	if key == "" {
		http.Error(w, `{"error":"signalKey required"}`, http.StatusBadRequest)
		return "", false
	}
	return key, true
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func errJSON(err error) string {
	// Minimal JSON-encoded error string. The frontend expects the
	// "error" field; full message returned for diagnostics.
	return `{"error":"` + err.Error() + `"}`
}
