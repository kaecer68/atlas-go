package shared

import (
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// JWTError is the standard 401 body returned by RequireUserJWT.
const JWTError = `{"error":"unauthorized"}`

// RequireUserJWT returns a middleware that validates a user JWT carried either
// in the HttpOnly cookie set by /api/auth/login (preferred, used by the
// browser via credentials: 'include') or the Authorization: Bearer header
// (used by atlas-mcp and other non-browser callers).
//
// Use this middleware for USER-SPECIFIC stock endpoints (e.g. saved
// searches, watchlists, personalized rankings). Do NOT use it for the
// public quote / fundamentals / chips / technical endpoints — those are
// intentionally public (per PR #1050) since they contain no user data.
//
// Typical wiring (cmd/atlas/main.go):
//
//	stockDeps.JWT = subscription.GetSharedJWTManager()
//	stocktools.RegisterUserRoutes(mux, stockDeps) // separate from RegisterRoutes
//
// The middleware is intentionally not the default for /api/stock/* —
// existing public endpoints must keep working. New user-specific stock
// endpoints should opt in.
func RequireUserJWT(jm *subscription.JWTManager, next http.Handler) http.Handler {
	if jm == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(JWTError))
			return
		}
		claims, err := jm.Verify(token)
		if err != nil || claims == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(JWTError))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	if c, err := r.Cookie("token"); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return after
	}
	return ""
}
