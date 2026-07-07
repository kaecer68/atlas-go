package subscription

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const claimsContextKey contextKey = "subscription_claims"

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if cookie, err := r.Cookie("token"); err == nil {
		return cookie.Value
	}
	return ""
}

func contextWithClaims(r *http.Request, claims *TokenClaims) context.Context {
	return context.WithValue(r.Context(), claimsContextKey, claims)
}

// TokenClaims carries the JWT payload.
type TokenClaims struct {
	UserID int64  `json:"sub"`
	Email  string `json:"email"`
	Tier   string `json:"tier"`
	Exp    int64  `json:"exp"`
}

// JWTManager handles token creation and verification.
type JWTManager struct {
	secret []byte
}

// NewJWTManager creates a JWT manager with the given secret.
// If secret is empty, generates a random key (not persistent across restarts).
func NewJWTManager(secret string) *JWTManager {
	if secret == "" {
		key := make([]byte, 32)
		rand.Read(key)
		secret = hex.EncodeToString(key)
	}
	return &JWTManager{secret: []byte(secret)}
}

// Generate creates a signed JWT token valid for the given duration.
func (m *JWTManager) Generate(user *User, duration time.Duration) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := TokenClaims{
		UserID: user.ID,
		Email:  user.Email,
		Tier:   string(user.EffectiveTier()),
		Exp:    time.Now().Add(duration).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	toSign := header + "." + payload
	sig := m.sign(toSign)
	return toSign + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify validates and parses a JWT token.
func (m *JWTManager) Verify(token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}
	toSign := parts[0] + "." + parts[1]
	expectedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	actualSig := m.sign(toSign)
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("invalid signature")
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}
	var claims TokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims json")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}
	return &claims, nil
}

func (m *JWTManager) sign(data string) []byte {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// AuthMiddleware validates the bearer token from Authorization header
// or "token" cookie, injecting claims into the request context.
type AuthMiddleware struct {
	jwt *JWTManager
}

// NewAuthMiddleware creates an auth middleware.
func NewAuthMiddleware(jwt *JWTManager) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwt}
}

// GetClaims extracts token claims from the request context.
func GetClaims(r *http.Request) *TokenClaims {
	claims, _ := r.Context().Value(claimsContextKey).(*TokenClaims)
	return claims
}

// Wrap returns an http.Handler that validates the JWT.
func (am *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		claims, err := am.jwt.Verify(token)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusUnauthorized)
			return
		}
		ctx := contextWithClaims(r, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
