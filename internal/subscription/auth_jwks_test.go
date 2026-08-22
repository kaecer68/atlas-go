package subscription

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---- RS256 / JWKS test helpers (mirror go-member signing) ----

type rotatingJWKS struct {
	mu  sync.Mutex
	kid string
	key *rsa.PublicKey
}

func (r *rotatingJWKS) set(kid string, pub *rsa.PublicKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kid, r.key = kid, pub
}

func jwkJSON(pub *rsa.PublicKey, kid string) jwkSet {
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	return jwkSet{Keys: []jwkKey{{
		Kty: "RSA",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
		Alg: "RS256",
	}}}
}

// startRotatingJWKSServer serves /.well-known/jwks.json from a mutable key
// so tests can exercise key rotation.
func startRotatingJWKSServer(t *testing.T, initial *rsa.PrivateKey, kid string) (*rotatingJWKS, string) {
	t.Helper()
	r := &rotatingJWKS{}
	r.set(kid, &initial.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/.well-known/jwks.json" {
			http.NotFound(w, req)
			return
		}
		r.mu.Lock()
		set := jwkJSON(r.key, r.kid)
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(srv.Close)
	return r, srv.URL + "/.well-known/jwks.json"
}

// signRS256 produces a go-member style RS256 JWT with the given claims.
func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims memberClaims) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString(
		[]byte(fmt.Sprintf(`{"alg":"RS256","typ":"JWT","kid":%q}`, kid)))
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	toSign := header + "." + payload
	digest := sha256.Sum256([]byte(toSign))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return toSign + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func newJWKSManager(t *testing.T, url string) *JWTManager {
	t.Helper()
	m := NewJWTManager("legacy-secret-not-used-in-jwks-mode", url)
	if !m.JWKSEnabled() {
		t.Fatal("expected JWKS mode")
	}
	return m
}

func mkMemberTier(tier, sub string, expOffset time.Duration) memberClaims {
	return memberClaims{
		Sub:   sub,
		Email: sub + "@member.test",
		Tier:  tier,
		Exp:   time.Now().Add(expOffset).Unix(),
	}
}

// ---- tier mapping ----

func TestJWKSVerifyRegisteredMapsToBasic(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	tok := signRS256(t, priv, "k1", mkMemberTier("registered", "abc-123", time.Hour))
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Tier != string(TierBasic) {
		t.Errorf("registered should map to basic, got %q", claims.Tier)
	}
	if claims.Sub != "abc-123" {
		t.Errorf("sub should be the go-member uuid, got %q", claims.Sub)
	}
}

func TestJWKSVerifyPremiumMapsToPro(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	tok := signRS256(t, priv, "k1", mkMemberTier("premium", "uuid-9", time.Hour))
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Tier != string(TierPro) {
		t.Errorf("premium should map to pro, got %q", claims.Tier)
	}
}

func TestJWKSVerifyPlatinumMapsToPro(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	tok := signRS256(t, priv, "k1", mkMemberTier("platinum", "uuid-9", time.Hour))
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Tier != string(TierPro) {
		t.Errorf("platinum should map to pro, got %q", claims.Tier)
	}
}

func TestJWKSVerifyUnknownTierMapsToFree(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	tok := signRS256(t, priv, "k1", mkMemberTier("superadmin", "uuid-7", time.Hour))
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Tier != string(TierFree) {
		t.Errorf("unknown tier should map to free, got %q", claims.Tier)
	}
}

// ---- rejection cases ----

func TestJWKSVerifyInvalidSignature(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	// Signed by a different key than the one published in the JWKS.
	tok := signRS256(t, other, "k1", mkMemberTier("premium", "u-1", time.Hour))
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expected invalid signature rejection")
	}
}

func TestJWKSVerifyWrongKid(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	tok := signRS256(t, priv, "definitely-not-k1", mkMemberTier("premium", "u-2", time.Hour))
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expected unknown kid rejection")
	}
}

func TestJWKSVerifyExpired(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	tok := signRS256(t, priv, "k1", mkMemberTier("premium", "u-3", -time.Hour))
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expected expired rejection")
	}
}

func TestJWKSRejectsLegacyHS256Token(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)

	hs := NewJWTManager("legacy-secret", "")
	u := &User{ID: 1, Email: "legacy@test.com", Tier: TierRegistered}
	legacyTok, err := hs.Generate(u, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// In JWKS mode the alg is HS256 — not RS256 — so it must be rejected.
	if _, err := m.Verify(legacyTok); err == nil {
		t.Fatal("expected HS256 token to be rejected in JWKS mode")
	}
}

// ---- key rotation / cache refresh ----

func TestJWKSKeyRotationRefresh(t *testing.T) {
	keyA, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyB, _ := rsa.GenerateKey(rand.Reader, 2048)
	r, url := startRotatingJWKSServer(t, keyA, "kA")
	m := newJWKSManager(t, url)
	m.jwksTTL = 50 * time.Millisecond // shrink for test

	// Phase 1: key A is served and verifies.
	tokA := signRS256(t, keyA, "kA", mkMemberTier("registered", "u-a", time.Hour))
	if _, err := m.Verify(tokA); err != nil {
		t.Fatalf("phase1 verify: %v", err)
	}

	// Rotate server-side to key B.
	r.set("kB", &keyB.PublicKey)
	tokB := signRS256(t, keyB, "kB", mkMemberTier("premium", "u-b", time.Hour))

	// Immediate verify (still cached key A) must reject key B token.
	if _, err := m.Verify(tokB); err == nil {
		t.Fatal("expected rejection while stale key A cached")
	}

	// After TTL elapses, the key set is re-fetched and key B verifies.
	time.Sleep(80 * time.Millisecond)
	if _, err := m.Verify(tokB); err != nil {
		t.Fatalf("post-rotation verify: %v", err)
	}
}

// ---- allowGuest fallback through the middleware ----

func TestJWKSGuestFallbackInvalidToken(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)
	mid := NewAuthMiddleware(m, true) // allowGuest=true → demote to guest

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer garbage-token")
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil || claims.Tier != string(TierFree) {
			t.Errorf("expected guest free claims, got %+v", claims)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 guest fallback, got %d", rec.Code)
	}

	// A valid go-member token flips the tier to pro.
	tok := signRS256(t, priv, "k1", mkMemberTier("premium", "u-ok", time.Hour))
	req = httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil || claims.Tier != string(TierPro) {
			t.Errorf("expected pro from valid token, got %+v", claims)
		}
	})).ServeHTTP(rec, req)
}

// ---- GO_MEMBER_JWKS_URL unset → legacy HS256 fallback ----

func TestJWTUnsetURLFallsBackToHS256(t *testing.T) {
	m := NewJWTManager("legacy-secret-32-characters-long", "")
	if m.JWKSEnabled() {
		t.Fatal("expected legacy HS256 mode when URL unset")
	}
	u := &User{ID: 7, Email: "legacy@example.com", Tier: TierRegistered}
	tok, err := m.Generate(u, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("legacy HS256 verify: %v", err)
	}
	if claims.Email != "legacy@example.com" || claims.UserID != 7 {
		t.Errorf("unexpected claims: %+v", claims)
	}
}

// ---- integration: JWKS token through AuthMiddleware → handler tier ----

func TestJWKSHandlerProfileFromClaims(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)
	h := NewHandler(newTestStore(t), m)
	mid := NewAuthMiddleware(m, true)

	tok := signRS256(t, priv, "k1", memberClaims{
		Sub:                 "member-uuid-1",
		Email:               "member@example.com",
		Tier:                "premium",
		MembershipExpiresAt: json.RawMessage(fmt.Sprintf("%d", time.Now().Add(30*24*time.Hour).Unix())),
		Exp:                 time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(h.handleProfile)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["tier"] != string(TierPro) {
		t.Errorf("expected tier pro, got %v", resp["tier"])
	}
	if resp["effective_tier"] != string(TierPro) {
		t.Errorf("expected effective_tier pro, got %v", resp["effective_tier"])
	}
	if resp["guest"] != false {
		t.Errorf("expected guest=false for a valid go-member token, got %v", resp["guest"])
	}
	if resp["trial_end"] == nil {
		t.Error("expected membershipExpiresAt surfaced as trial_end")
	}
}

// ---- allowGuest=false (member mode / GUEST_MODE=false) — strict 401 ----

func TestJWKSAllowGuestFalseRejectsMissingToken(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)
	mid := NewAuthMiddleware(m, false) // allowGuest=false → strict

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run with allowGuest=false and no token")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", rec.Code)
	}
}

func TestJWKSAllowGuestFalseRejectsInvalidToken(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)
	mid := NewAuthMiddleware(m, false) // allowGuest=false → strict

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer garbage-token")
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run with allowGuest=false and invalid token")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", rec.Code)
	}
}

func TestJWKSAllowGuestFalseAcceptsValidToken(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, url := startRotatingJWKSServer(t, priv, "k1")
	m := newJWKSManager(t, url)
	mid := NewAuthMiddleware(m, false) // allowGuest=false, valid token must pass

	tok := signRS256(t, priv, "k1", mkMemberTier("premium", "u-mem", time.Hour))
	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	var gotTier string
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil {
			t.Fatal("expected claims in context")
		}
		gotTier = claims.Tier
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d", rec.Code)
	}
	if gotTier != string(TierPro) {
		t.Errorf("expected premium→pro, got %q", gotTier)
	}
}
