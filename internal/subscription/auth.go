package subscription

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type contextKey string

const claimsContextKey contextKey = "subscription_claims"

// defaultJWKSTTL is how long a fetched JWKS result is trusted before it is
// re-fetched, enabling go-member key rotation without a restart.
const defaultJWKSTTL = 10 * time.Minute

// ExtractToken retrieves the JWT from the Authorization header (Bearer)
// or the "token" cookie. Returns empty string if no token found.
func ExtractToken(r *http.Request) string {
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

// TokenClaims carries the JWT payload after verification.
//
// UserID is the legacy numeric atlas user id carried by self-signed HS256
// tokens (pre go-member). Sub holds the go-member member uuid (a string) for
// RS256 tokens verified against the go-member JWKS — atlas never re-mints a
// numeric id for go-member members (C-02 contract: store the sub string only).
type TokenClaims struct {
	UserID              int64  `json:"sub"` // legacy atlas numeric id (HS256 self-signed)
	Sub                 string // go-member member uuid, set only by JWKS RS256 verify
	Email               string `json:"email"`
	Tier                string `json:"tier"`
	MembershipExpiresAt int64  `json:"membershipExpiresAt"`
	Exp                 int64  `json:"exp"`
}

// memberClaims is the payload shape issued by go-member (RS256). It is
// parsed separately from TokenClaims because go-member's sub is a string
// uuid rather than an int, and it carries a membershipExpiresAt claim.
type memberClaims struct {
	Sub                 string `json:"sub"`
	Email               string `json:"email"`
	Tier                string `json:"tier"`
	MembershipExpiresAt int64  `json:"membershipExpiresAt"`
	Exp                 int64  `json:"exp"`
}

// JWK (RSA public key) as served by go-member /.well-known/jwks.json.
type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

type jwkSet struct {
	Keys []jwkKey `json:"keys"`
}

// JWTManager verifies member tokens.
//
// Two operational modes (C-02 atlas-jwt-trust):
//   - go-member JWKS mode: GO_MEMBER_JWKS_URL is set. Verify() validates the
//     RS256 signature against the go-member JWKS public key and maps the
//     member tier claim to an atlas tier (registered→basic, premium→pro).
//   - legacy HS256 mode: GO_MEMBER_JWKS_URL is empty (backward compat). Verify()
//     validates self-signed HMAC-SHA256 tokens exactly as before the migration.
//
// The JWKS result is cached with a TTL (defaultJWKSTTL) and lazily refreshed,
// so go-member key rotation takes effect without a restart.
type JWTManager struct {
	secret []byte // legacy HS256 secret (used only in HS256 mode)

	jwksURL    string
	jwksTTL    time.Duration
	httpClient *http.Client

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey // kid -> public key (JWKS mode)
	fetchedAt time.Time
}

// NewJWTManager creates a JWT manager. secret is the legacy HS256 secret used
// in backward-compat mode; jwksURL is the go-member /.well-known/jwks.json
// endpoint (from GO_MEMBER_JWKS_URL). When jwksURL is empty the manager stays
// in legacy HS256 mode. An empty secret generates a random key (not persistent
// across restarts).
func NewJWTManager(secret, jwksURL string) *JWTManager {
	if secret == "" {
		key := make([]byte, 32)
		rand.Read(key)
		secret = hex.EncodeToString(key)
	}
	return &JWTManager{
		secret:     []byte(secret),
		jwksURL:    strings.TrimSpace(jwksURL),
		jwksTTL:    defaultJWKSTTL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Generate creates a signed HS256 JWT token valid for the given duration.
//
// NOTE: as of C-02, atlas no longer mints member tokens in go-member JWKS
// mode — login is delegated to go-member, which signs RS256. Generate stays
// the active path of the legacy HS256 self-signed mode (register/login
// fallback while GO_MEMBER_JWKS_URL is unset) and for tests.
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

// Verify validates and parses a JWT token according to the active mode.
func (m *JWTManager) Verify(token string) (*TokenClaims, error) {
	if m.jwksURL != "" {
		return m.verifyRS256(token)
	}
	return m.verifyHS256(token)
}

// verifyHS256 is the pre-migration HMAC-SHA256 verification path.
func (m *JWTManager) verifyHS256(token string) (*TokenClaims, error) {
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

// verifyRS256 validates a go-member token against the JWKS public key and
// maps the member tier claim to an atlas tier.
func (m *JWTManager) verifyRS256(token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header encoding")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("invalid header json")
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported alg %q", header.Alg)
	}

	keys, err := m.keyset()
	if err != nil {
		return nil, err
	}
	key, ok := keys[header.Kid]
	if !ok {
		return nil, fmt.Errorf("unknown kid %q", header.Kid)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], sig); err != nil {
		return nil, fmt.Errorf("invalid signature")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}
	var mc memberClaims
	if err := json.Unmarshal(payloadJSON, &mc); err != nil {
		return nil, fmt.Errorf("invalid claims json")
	}
	if time.Now().Unix() > mc.Exp {
		return nil, fmt.Errorf("token expired")
	}
	return &TokenClaims{
		Sub:                 mc.Sub,
		Email:               mc.Email,
		Tier:                string(mapMemberTier(mc.Tier)),
		MembershipExpiresAt: mc.MembershipExpiresAt,
		Exp:                 mc.Exp,
	}, nil
}

// keyset returns the verified-JWKS key map, refreshing from the remote
// endpoint when the cached result is missing or past its TTL.
func (m *JWTManager) keyset() (map[string]*rsa.PublicKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys != nil && time.Since(m.fetchedAt) < m.jwksTTL {
		return m.keys, nil
	}
	keys, err := m.fetchJWKS()
	if err != nil {
		// Refresh failure keeps serving the last-known keys (rotation
		// resilience). Only fail when we have nothing cached at all.
		if m.keys != nil {
			return m.keys, nil
		}
		return nil, err
	}
	m.keys = keys
	m.fetchedAt = time.Now()
	return m.keys, nil
}

// fetchJWKS downloads and parses the go-member JWKS document.
func (m *JWTManager) fetchJWKS() (map[string]*rsa.PublicKey, error) {
	client := m.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(m.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks fetch status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("jwks read: %w", err)
	}
	var set jwkSet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("jwks parse: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, k := range set.Keys {
		if k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := keyFromJWK(k)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("jwks contains no usable keys")
	}
	return keys, nil
}

// keyFromJWK builds an rsa.PublicKey from base64url n/e JWK fields.
func keyFromJWK(k jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	if len(eBytes) == 0 {
		return nil, fmt.Errorf("empty exponent")
	}
	e := new(big.Int).SetBytes(eBytes).Int64()
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e)}, nil
}

// mapMemberTier converts a go-member tier claim to the atlas access tier:
// registered→basic, premium→pro, anything else→free (guest fallback).
func mapMemberTier(claim string) Tier {
	switch claim {
	case "registered", "basic":
		return TierBasic
	case "premium", "pro", "platinum":
		return TierPro
	default:
		return TierFree
	}
}

func (m *JWTManager) sign(data string) []byte {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// guestClaims returns synthetic TierFree claims used when allowGuest
// mode is on and the request has no/invalid token. The fallback keeps
// demos alive without minting a user row or surfacing email leakage.
func guestClaims() *TokenClaims {
	return &TokenClaims{UserID: 0, Email: "", Tier: string(TierFree)}
}

// JWKSEnabled reports whether the manager is in go-member JWKS (RS256) mode.
func (m *JWTManager) JWKSEnabled() bool { return m.jwksURL != "" }

// AuthMiddleware validates the bearer token from Authorization header
// or "token" cookie, injecting claims into the request context.
//
// With allowGuest=true, missing OR invalid tokens downgrade to
// TierFree instead of 401 — useful for pre-commercialisation demos.
// With allowGuest=false, behavior is strict.
type AuthMiddleware struct {
	jwt        *JWTManager
	allowGuest bool
}

func NewAuthMiddleware(jwt *JWTManager, allowGuest bool) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwt, allowGuest: allowGuest}
}

// GetClaims extracts token claims from the request context.
func GetClaims(r *http.Request) *TokenClaims {
	claims, _ := r.Context().Value(claimsContextKey).(*TokenClaims)
	return claims
}

// Wrap returns an http.Handler that validates the JWT.
func (am *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)
		if token == "" {
			if am.allowGuest {
				ctx := contextWithClaims(r, guestClaims())
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		claims, err := am.jwt.Verify(token)
		if err != nil {
			if am.allowGuest {
				// Invalid token (expired / rotated secret) also falls
				// through to guest to avoid kicking demo users out
				// every server restart.
				ctx := contextWithClaims(r, guestClaims())
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusUnauthorized)
			return
		}
		ctx := contextWithClaims(r, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
