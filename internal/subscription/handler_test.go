package subscription

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	jwt := NewJWTManager("test-secret", "")
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
	jwt := NewJWTManager("test-secret", "")
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
	mid := NewAuthMiddleware(jwt, false)
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
	jwt := NewJWTManager("", "")
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

func TestLogout(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret-min-32-characters-long", "")
	h := NewHandler(s, jwt)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()
	h.handleLogout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "token=;") && !strings.Contains(cookie, "Max-Age=0") && !strings.Contains(cookie, "Expires=Thu, 01 Jan 1970") {
		t.Fatalf("expected token cookie to be cleared, got %q", cookie)
	}
}

func TestAuthRejectInvalidToken(t *testing.T) {
	jwt := NewJWTManager("test-secret", "")
	mid := NewAuthMiddleware(jwt, false)

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
	jwt := NewJWTManager("test-secret", "")
	mid := NewAuthMiddleware(jwt, false)

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuthGuestModeAllowsAnon(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	h := NewHandler(s, jwt)
	mid := NewAuthMiddleware(jwt, true)

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(h.handleProfile)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["tier"] != string(TierFree) {
		t.Errorf("expected tier=free, got %v", got["tier"])
	}
	if got["email"] != "" {
		t.Errorf("expected empty email for guest, got %v", got["email"])
	}
}

func TestAuthGuestModeDemotesInvalidToken(t *testing.T) {
	jwt := NewJWTManager("test-secret", "")
	mid := NewAuthMiddleware(jwt, true)

	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req.Header.Set("Authorization", "Bearer invalid.token")
	rec := httptest.NewRecorder()
	mid.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r)
		if claims == nil || claims.Tier != string(TierFree) {
			t.Errorf("expected guest claims, got %+v", claims)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (guest fallback), got %d", rec.Code)
	}
}

func TestJWTRoundTrip(t *testing.T) {
	jwt := NewJWTManager("secret123", "")
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

// ---- M4b: go-member thin-proxy login/register ----

// goMemberMock spins up an httptest server that mimics go-member's auth API,
// calling handlerFn per request. Returns the server (URL accessible after).
func goMemberMock(t *testing.T, handlerFn http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handlerFn)
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleLoginProxySuccess(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	got := make(chan string, 1)
	server := goMemberMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			http.NotFound(w, r)
			return
		}
		got <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"mock.rs256.token","refreshToken":"rt","expiresIn":900,"tokenType":"Bearer"}`))
	})
	h := NewHandler(s, jwt).WithGoMember(server.URL)

	body := `{"email":"member@test.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["token"] != "mock.rs256.token" {
		t.Errorf("expected RS256 token echoed, got %v", resp["token"])
	}
	u, ok := resp["user"].(map[string]any)
	if !ok || u["email"] != "member@test.com" {
		t.Errorf("expected user.email, got %v", resp["user"])
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "token=mock.rs256.token") {
		t.Errorf("expected token cookie set, got %q", cookie)
	}
	if path := <-got; path != "/api/v1/auth/login" {
		t.Errorf("proxy hit wrong path: %s", path)
	}
}

func TestHandleLoginProxyErrors(t *testing.T) {
	cases := []struct {
		name       string
		upStatus   int
		upBody     string
		wantStatus int
		wantMsg    string
	}{
		{"unauthorized", 401, `{"error":"invalid credentials"}`, 401, "invalid credentials"},
		{"banned", 403, `{"error":"account banned"}`, 403, "account banned"},
		{"notApproved", 403, `{"message":"account not approved"}`, 403, "account not approved"},
		{"emailNotVerified", 422, `{"error":"email not verified"}`, 422, "email not verified"},
		{"rateLimited", 429, `{"error":"rate limited"}`, 429, "rate limited"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			jwt := NewJWTManager("test-secret", "")
			server := goMemberMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.upStatus)
				_, _ = w.Write([]byte(tc.upBody))
			})
			h := NewHandler(s, jwt).WithGoMember(server.URL)

			body := `{"email":"member@test.com","password":"secret"}`
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
			rec := httptest.NewRecorder()
			h.handleLogin(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}
			var resp map[string]any
			_ = json.NewDecoder(rec.Body).Decode(&resp)
			if resp["error"] != tc.wantMsg {
				t.Errorf("expected error %q, got %v", tc.wantMsg, resp["error"])
			}
			if rec.Header().Get("Set-Cookie") != "" {
				t.Errorf("error path must not set cookie, got %q", rec.Header().Get("Set-Cookie"))
			}
		})
	}
}

func TestHandleLoginProxyLegacyFallback(t *testing.T) {
	// With GoMemberAPIBaseURL empty, login falls back to the local HS256 path.
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	h := NewHandler(s, jwt) // no WithGoMember

	// register a local user first (legacy path)
	reg := `{"email":"legacy@test.com","password":"secret"}`
	rreq := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(reg))
	rrec := httptest.NewRecorder()
	h.handleRegister(rrec, rreq)
	if rrec.Code != http.StatusCreated {
		t.Fatalf("legacy register expected 201, got %d", rrec.Code)
	}

	lreq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(reg))
	lrec := httptest.NewRecorder()
	h.handleLogin(lrec, lreq)
	if lrec.Code != http.StatusOK {
		t.Fatalf("legacy login expected 200, got %d: %s", lrec.Code, lrec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(lrec.Body).Decode(&resp)
	if resp["token"] == nil || resp["token"] == "" {
		t.Errorf("legacy login should mint an HS256 token, got %v", resp["token"])
	}
}

func TestHandleRegisterProxy(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	server := goMemberMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/register" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"uuid-1","email":"new@test.com","message":"註冊成功，請檢查電子郵件完成驗證"}`))
	})
	h := NewHandler(s, jwt).WithGoMember(server.URL)

	body := `{"email":"new@test.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.handleRegister(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "註冊成功，請檢查電子郵件完成驗證" {
		t.Errorf("expected verify message passthrough, got %v", resp["message"])
	}
	if resp["token"] != nil {
		t.Errorf("register proxy must not return a token, got %v", resp["token"])
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("register proxy must not set auth cookie, got %q", rec.Header().Get("Set-Cookie"))
	}
}

func TestHandleRegisterProxyConflict(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	server := goMemberMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"email already exists"}`))
	})
	h := NewHandler(s, jwt).WithGoMember(server.URL)

	body := `{"email":"dup@test.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.handleRegister(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "email already exists" {
		t.Errorf("expected conflict error, got %v", resp["error"])
	}
}
