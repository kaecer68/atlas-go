package shared

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response with the given status code and payload.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteJSONError writes a JSON error response with the given status code and message.
// Response shape: {"error": "<message>"}
// For machine-readable error codes, use WriteJSONErrorEx.
func WriteJSONError(w http.ResponseWriter, status int, message string) {
	WriteJSONErrorEx(w, status, "", message)
}

// WriteJSONErrorEx writes a JSON error response with a human-readable message
// and an optional machine-readable code.
// Response shape: {"error": "<message>", "code": "<code>"}
// Pass empty string for code to omit the field (backward-compatible with
// WriteJSONError). Codes are stable identifiers suitable for client-side
// branching; messages are human-readable and may change.
func WriteJSONErrorEx(w http.ResponseWriter, status int, code, message string) {
	payload := map[string]string{"error": message}
	if code != "" {
		payload["code"] = code
	}
	WriteJSON(w, status, payload)
}
