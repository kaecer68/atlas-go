package shared

import "net/http"

// Handler is the new handler signature for monitoring API handlers.
// It receives only the HTTP request and returns a status code with structured data.
// Unlike the standard http.Handler, it does NOT write to the ResponseWriter directly —
// WriteJSON is called centrally by Adapt(), eliminating 73 duplicate call sites.
type Handler func(r *http.Request) (status int, data any)

// Adapt wraps a Handler into a standard http.Handler.
// This is the single place in the monitoring API layer where WriteJSON is called.
// All new handlers should use this pattern.
func Adapt(h Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, data := h(r)
		WriteJSON(w, status, data)
	})
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
