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
	// Upstream（go-member）回傳的 access token 必須是 atlas 能驗證的 token；
	// 這裡用同一個 JWTManager 簽一個 HS256 token 模擬。
	upstreamUser := &User{ID: 99, Email: "member@test.com", Tier: TierRegistered}
	upstreamToken, err := jwt.Generate(upstreamUser, 15*time.Minute)
	if err != nil {
		t.Fatalf("generate upstream token: %v", err)
	}
	got := make(chan string, 1)
	server := goMemberMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			http.NotFound(w, r)
			return
		}
		got <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"` + upstreamToken + `","refreshToken":"rt","expiresIn":900,"tokenType":"Bearer"}`))
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
	// 2026-08-24：回應 body 應為重新簽發的 atlas session token（≠ upstream token）
	sessionToken, _ := resp["token"].(string)
	if sessionToken == "" || sessionToken == upstreamToken {
		t.Errorf("expected re-minted session token (≠ upstream), got %v", resp["token"])
	}
	claims, err := jwt.Verify(sessionToken)
	if err != nil {
		t.Fatalf("session token should verify: %v", err)
	}
	if claims.Email != "member@test.com" {
		t.Errorf("session claims email: %s", claims.Email)
	}
	u, ok := resp["user"].(map[string]any)
	if !ok || u["email"] != "member@test.com" {
		t.Errorf("expected user.email, got %v", resp["user"])
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "token="+sessionToken) {
		t.Errorf("expected session token cookie set, got %q", cookie)
	}
	if path := <-got; path != "/api/v1/auth/login" {
		t.Errorf("proxy hit wrong path: %s", path)
	}
}

func TestHandleLoginProxyInvalidUpstreamToken(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	server := goMemberMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"not.a.valid.jwt","refreshToken":"rt","expiresIn":900,"tokenType":"Bearer"}`))
	})
	h := NewHandler(s, jwt).WithGoMember(server.URL)

	body := `{"email":"bad@test.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.handleLogin(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for invalid upstream token, got %d: %s", rec.Code, rec.Body.String())
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

func TestSSOEndpoint(t *testing.T) {
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	h := NewHandler(s, jwt)

	// 有效 token → 重新簽發長效 session token + 設 cookie + JSON redirect
	user := &User{ID: 1, Email: "sso@test.com", Tier: "registered"}
	token, err := jwt.Generate(user, time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/sso?token="+token+"&redirect=/client/home", nil)
	rec := httptest.NewRecorder()
	h.handleSSO(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["ok"] != "true" || resp["redirect"] != "/client/home" {
		t.Errorf("unexpected resp: %v", resp)
	}
	// 2026-08-24（登入記憶）：cookie 應是重新簽發的 atlas session token
	//（≠ 原 upstream token），可被 Verify 驗證，MaxAge = 7 天。
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "token" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected token cookie to be set")
	}
	if !sessionCookie.HttpOnly {
		t.Error("expected HttpOnly cookie")
	}
	if sessionCookie.Value == token {
		t.Error("cookie should hold re-minted session token, not the raw upstream token")
	}
	claims, err := jwt.Verify(sessionCookie.Value)
	if err != nil {
		t.Fatalf("session cookie token should verify: %v", err)
	}
	if claims.Email != "sso@test.com" || claims.Tier != string(TierRegistered) {
		t.Errorf("session claims: email=%s tier=%s", claims.Email, claims.Tier)
	}
	if sessionCookie.MaxAge < int((6 * 24 * time.Hour).Seconds()) {
		t.Errorf("session cookie MaxAge should be ~7 days, got %d", sessionCookie.MaxAge)
	}

	// 缺 token → 400
	req2 := httptest.NewRequest(http.MethodGet, "/api/auth/sso?redirect=/client/home", nil)
	rec2 := httptest.NewRecorder()
	h.handleSSO(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec2.Code)
	}

	// 無效 token → 401
	req3 := httptest.NewRequest(http.MethodGet, "/api/auth/sso?token=bad.token.here", nil)
	rec3 := httptest.NewRecorder()
	h.handleSSO(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec3.Code)
	}

	// open redirect 防護：外部 URL → 預設 /client/home
	req4 := httptest.NewRequest(http.MethodGet, "/api/auth/sso?token="+token+"&redirect=https%3A%2F%2Fevil.example.com", nil)
	rec4 := httptest.NewRecorder()
	h.handleSSO(rec4, req4)
	var resp4 map[string]string
	_ = json.NewDecoder(rec4.Body).Decode(&resp4)
	if resp4["redirect"] != "/client/home" {
		t.Errorf("open redirect not blocked: %v", resp4["redirect"])
	}
}

// TestGenerateSessionRoundTrip: atlas session token（string sub）可驗證回原 claims。
func TestGenerateSessionRoundTrip(t *testing.T) {
	jwt := NewJWTManager("session-secret", "")
	claims := &TokenClaims{Sub: "uuid-1234", Email: "m@test.com", Tier: "pro", MembershipExpiresAt: 1777777777}
	tok, err := jwt.GenerateSession(claims, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateSession: %v", err)
	}
	got, err := jwt.Verify(tok)
	if err != nil {
		t.Fatalf("Verify session token: %v", err)
	}
	if got.Sub != "uuid-1234" || got.Email != "m@test.com" || got.Tier != "pro" || got.MembershipExpiresAt != 1777777777 {
		t.Errorf("session claims mismatch: %+v", got)
	}
	if got.Exp < time.Now().Add(6*24*time.Hour).Unix() {
		t.Errorf("session exp should be ~7 days out, got %d", got.Exp)
	}
}
