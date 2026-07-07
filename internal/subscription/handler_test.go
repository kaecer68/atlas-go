package subscription

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir, _ := os.MkdirTemp("", "subtest")
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRegisterLogin(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret")
	h := NewHandler(s, jwt)

	// Register
	body := `{"email":"test@example.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handleRegister(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var regResp map[string]any
	json.NewDecoder(rec.Body).Decode(&regResp)
	if regResp["token"] == nil {
		t.Error("expected token in response")
	}

	// Login
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProfileAndSubscription(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret")
	h := NewHandler(s, jwt)

	// Register first
	body := `{"email":"test@example.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handleRegister(rec, req)

	var regResp map[string]any
	json.NewDecoder(rec.Body).Decode(&regResp)
	token := regResp["token"].(string)

	// Profile
	req = httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	mid := NewAuthMiddleware(jwt)
	mid.Wrap(http.HandlerFunc(h.handleProfile)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var profResp map[string]any
	json.NewDecoder(rec.Body).Decode(&profResp)

	effTier, _ := profResp["effective_tier"].(string)
	if effTier != string(TierPremium) {
		t.Errorf("expected premium trial, got %s", effTier)
	}
}

func TestTierRegistration(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("")
	h := NewHandler(s, jwt)

	body := `{"email":"tier@test.com","password":"pass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handleRegister(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	email := "tier@test.com"
	u, err := s.GetByEmail(email)
	if err != nil || u == nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	if u.Tier != TierRegistered {
		t.Errorf("expected registered, got %s", u.Tier)
	}
	if u.EffectiveTier() != TierPremium {
		t.Errorf("expected premium trial effective tier, got %s", u.EffectiveTier())
	}
}

func TestAuthRejectInvalidToken(t *testing.T) {
	jwt := NewJWTManager("test-secret")
	mid := NewAuthMiddleware(jwt)

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthRejectNoToken(t *testing.T) {
	jwt := NewJWTManager("test-secret")
	mid := NewAuthMiddleware(jwt)

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTRoundTrip(t *testing.T) {
	jwt := NewJWTManager("secret123")
	u := &User{ID: 1, Email: "jwt@test.com", Tier: TierRegistered}
	token, err := jwt.Generate(u, 1*time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := jwt.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Email != "jwt@test.com" {
		t.Errorf("email: %s", claims.Email)
	}
	if claims.UserID != 1 {
		t.Errorf("userID: %d", claims.UserID)
	}
}
