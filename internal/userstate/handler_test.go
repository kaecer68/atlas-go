package userstate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// makeJWT signs a minimal test token for claims.UserID=userID. Real
// production uses subscription.JWTManager; here we use the same primitive
// to keep the test self-contained.
func makeJWT(t *testing.T, jwtMgr *subscription.JWTManager, userID int64) string {
	t.Helper()
	tok, err := jwtMgr.Generate(&subscription.User{ID: userID, Email: "u@example.com", Tier: subscription.TierRegistered, CreatedAt: time.Now()}, time.Hour)
	if err != nil {
		t.Fatalf("generate jwt: %v", err)
	}
	return tok
}

func authHeader(token string) string { return "Bearer " + token }

func newTestHandler(t *testing.T) (*Handler, *subscription.JWTManager, *subscription.AuthMiddleware) {
	t.Helper()
	store := NewJSONLStore(t.TempDir())
	h := NewHandler(store)
	jwtMgr := subscription.NewJWTManager("test-secret-do-not-use-in-prod", "")
	mid := subscription.NewAuthMiddleware(jwtMgr, false) // strict for tests
	return h, jwtMgr, mid
}

func TestHandler_ListRequiresAuth(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil) // nil middleware → not registered
	// Without middleware, the route is not served at all → 404.
	req := httptest.NewRequest("GET", "/api/user/signals", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET without middleware: code = %d, want 404 (route not registered)", rec.Code)
	}
}

func TestHandler_RegisterNilMiddlewareSkipsAllRoutes(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil) // should be a no-op
	for _, path := range []string{
		"/api/user/signals",
		"/api/user/signals/foreign-3day-inflow/ack",
		"/api/user/signals/foreign-3day-inflow/dismiss",
	} {
		req := httptest.NewRequest("PUT", path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: code = %d, want 404 (route not registered)", path, rec.Code)
		}
	}
}

func TestHandler_ListReturnsEmptyForNewUser(t *testing.T) {
	h, jwtMgr, mid := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, mid)
	token := makeJWT(t, jwtMgr, 42)

	req := httptest.NewRequest("GET", "/api/user/signals", nil)
	req.Header.Set("Authorization", authHeader(token))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Count  int               `json:"count"`
		States []UserSignalState `json:"signals"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 0 || len(resp.States) != 0 {
		t.Errorf("expected empty list, got count=%d states=%d", resp.Count, len(resp.States))
	}
}

func TestHandler_AckThenListReturnsRecord(t *testing.T) {
	h, jwtMgr, mid := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, mid)
	token := makeJWT(t, jwtMgr, 7)

	// 1. Ack
	ackReq := httptest.NewRequest("PUT", "/api/user/signals/foreign-3day-inflow/ack", nil)
	ackReq.Header.Set("Authorization", authHeader(token))
	ackRec := httptest.NewRecorder()
	mux.ServeHTTP(ackRec, ackReq)
	if ackRec.Code != http.StatusOK {
		t.Fatalf("ack: code = %d, body=%s", ackRec.Code, ackRec.Body.String())
	}
	var ackState UserSignalState
	if err := json.NewDecoder(ackRec.Body).Decode(&ackState); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ackState.UserID != 7 || ackState.SignalKey != "foreign-3day-inflow" {
		t.Errorf("ack state = %+v", ackState)
	}
	if ackState.Dismissed {
		t.Error("Dismissed = true after ack, want false")
	}
	if ackState.AcknowledgedAt == nil {
		t.Error("AcknowledgedAt = nil after ack — the 'read' badge never gets set")
	}

	// 2. List returns the one record.
	listReq := httptest.NewRequest("GET", "/api/user/signals", nil)
	listReq.Header.Set("Authorization", authHeader(token))
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	var resp struct {
		Count  int               `json:"count"`
		States []UserSignalState `json:"signals"`
	}
	_ = json.NewDecoder(listRec.Body).Decode(&resp)
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
}

func TestHandler_DismissSetsDismissedTrue(t *testing.T) {
	h, jwtMgr, mid := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, mid)
	token := makeJWT(t, jwtMgr, 9)

	req := httptest.NewRequest("PUT", "/api/user/signals/sig-x/dismiss", nil)
	req.Header.Set("Authorization", authHeader(token))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dismiss: code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var s UserSignalState
	_ = json.NewDecoder(rec.Body).Decode(&s)
	if !s.Dismissed {
		t.Errorf("Dismissed = false after /dismiss, want true (got %+v)", s)
	}
	if s.AcknowledgedAt == nil {
		t.Error("AcknowledgedAt = nil after dismiss — dismissing should also acknowledge")
	}
}

func TestHandler_DeleteResetsRecord(t *testing.T) {
	h, jwtMgr, mid := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, mid)
	token := makeJWT(t, jwtMgr, 11)

	// 1. Ack
	ackReq := httptest.NewRequest("PUT", "/api/user/signals/sig/ack", nil)
	ackReq.Header.Set("Authorization", authHeader(token))
	mux.ServeHTTP(httptest.NewRecorder(), ackReq)

	// 2. Delete (resets to AcknowledgedAt=nil, Dismissed=false)
	delReq := httptest.NewRequest("DELETE", "/api/user/signals/sig", nil)
	delReq.Header.Set("Authorization", authHeader(token))
	delRec := httptest.NewRecorder()
	mux.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete: code = %d", delRec.Code)
	}

	// 3. Read back — store should have a record (the delete wrote a zero
	// state) with AcknowledgedAt=nil and Dismissed=false.
	store := NewJSONLStore(mustTempDir(t))
	_ = store // just to make linter happy
	// Re-list to verify state
	listReq := httptest.NewRequest("GET", "/api/user/signals", nil)
	listReq.Header.Set("Authorization", authHeader(token))
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	var resp struct {
		States []UserSignalState `json:"signals"`
	}
	_ = json.NewDecoder(listRec.Body).Decode(&resp)
	if len(resp.States) != 1 {
		t.Fatalf("expected 1 record after delete, got %d", len(resp.States))
	}
	if resp.States[0].AcknowledgedAt != nil {
		t.Error("AcknowledgedAt = non-nil after delete, want nil")
	}
	if resp.States[0].Dismissed {
		t.Error("Dismissed = true after delete, want false")
	}
}

func TestHandler_RequiresValidJWT(t *testing.T) {
	h, _, mid := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, mid)

	// No auth header → 401
	req := httptest.NewRequest("GET", "/api/user/signals", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no auth: code = %d, want 401", rec.Code)
	}

	// Bad token → 401
	req2 := httptest.NewRequest("GET", "/api/user/signals", nil)
	req2.Header.Set("Authorization", "Bearer not-a-real-jwt")
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("bad token: code = %d, want 401", rec2.Code)
	}
}

func TestHandler_PerUserIsolation(t *testing.T) {
	h, jwtMgr, mid := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, mid)
	token7 := makeJWT(t, jwtMgr, 7)
	token8 := makeJWT(t, jwtMgr, 8)

	// User 7 acks one signal.
	req := httptest.NewRequest("PUT", "/api/user/signals/sig-a/ack", nil)
	req.Header.Set("Authorization", authHeader(token7))
	mux.ServeHTTP(httptest.NewRecorder(), req)

	// User 7 list: 1 record.
	list7 := httptest.NewRequest("GET", "/api/user/signals", nil)
	list7.Header.Set("Authorization", authHeader(token7))
	rec7 := httptest.NewRecorder()
	mux.ServeHTTP(rec7, list7)
	var r7 struct {
		States []UserSignalState `json:"signals"`
	}
	_ = json.NewDecoder(rec7.Body).Decode(&r7)
	if len(r7.States) != 1 {
		t.Errorf("user 7: expected 1, got %d", len(r7.States))
	}

	// User 8 list: 0 records (isolation).
	list8 := httptest.NewRequest("GET", "/api/user/signals", nil)
	list8.Header.Set("Authorization", authHeader(token8))
	rec8 := httptest.NewRecorder()
	mux.ServeHTTP(rec8, list8)
	var r8 struct {
		States []UserSignalState `json:"signals"`
	}
	_ = json.NewDecoder(rec8.Body).Decode(&r8)
	if len(r8.States) != 0 {
		t.Errorf("user 8: expected 0, got %d (isolation broken!)", len(r8.States))
	}
}

func mustTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}
