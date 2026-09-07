package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/channelsecrets"
)

// newTestChannelKeys builds a Handlers with a real manager backed by a
// temp-dir SQLite store under a random master key.
func newTestChannelKeys(t *testing.T) *Handlers {
	t.Helper()
	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	t.Setenv(channelsecrets.MasterKeyEnv, base64.StdEncoding.EncodeToString(master))
	store, err := channelsecrets.NewStore(context.Background(), "sqlite", nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := channelsecrets.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandlers(t.TempDir(), t.TempDir())
	h.ChannelKeys = mgr
	return h
}

func TestHandleChannelKeysList_UnsetAndEncryption(t *testing.T) {
	h := newTestChannelKeys(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channel-keys", nil)
	status, data := h.HandleChannelKeysList(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	b, _ := json.Marshal(data)
	var out struct {
		Keys              []channelsecrets.KeyStatus `json:"keys"`
		EncryptionEnabled bool                       `json:"encryption_enabled"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Keys) != 2 {
		t.Fatalf("keys = %d, want 2 (finmind, fugle)", len(out.Keys))
	}
	if !out.EncryptionEnabled {
		t.Error("encryption_enabled must be true with master key set")
	}
	for _, k := range out.Keys {
		if k.Source != "unset" || k.MaskedKey != "" {
			t.Errorf("%s = %+v, want unset/empty", k.Provider, k)
		}
	}
}

func TestHandleChannelKeyUpdate_HappyPath(t *testing.T) {
	h := newTestChannelKeys(t)
	body := `{"api_key": "brand-new-finmind-token", "updated_by": "ops@member"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channel-keys/finmind", strings.NewReader(body))
	req.SetPathValue("provider", "finmind")
	status, data := h.HandleChannelKeyUpdate(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, data=%v", status, data)
	}
	b, _ := json.Marshal(data)
	if !strings.Contains(string(b), "****oken") {
		t.Errorf("response must contain masked key only: %s", b)
	}
	if strings.Contains(string(b), "brand-new-finmind-token") {
		t.Fatal("response leaks plaintext key")
	}

	// List reflects the persisted row.
	req = httptest.NewRequest(http.MethodGet, "/api/admin/channel-keys", nil)
	_, list := h.HandleChannelKeysList(req)
	lb, _ := json.Marshal(list)
	if !strings.Contains(string(lb), "****oken") || !strings.Contains(string(lb), "ops@member") {
		t.Errorf("list must show masked db row: %s", lb)
	}
}

func TestHandleChannelKeyUpdate_Errors(t *testing.T) {
	h := newTestChannelKeys(t)

	// Unknown provider.
	req := httptest.NewRequest(http.MethodPut, "/api/admin/channel-keys/tej", strings.NewReader(`{"api_key":"1234567890"}`))
	req.SetPathValue("provider", "tej")
	status, _ := h.HandleChannelKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Errorf("tej status = %d, want 400", status)
	}
	// Too short.
	req = httptest.NewRequest(http.MethodPut, "/api/admin/channel-keys/finmind", strings.NewReader(`{"api_key":"short"}`))
	req.SetPathValue("provider", "finmind")
	status, _ = h.HandleChannelKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Errorf("short status = %d, want 400", status)
	}
	// Missing body field.
	req = httptest.NewRequest(http.MethodPut, "/api/admin/channel-keys/finmind", strings.NewReader(`{}`))
	req.SetPathValue("provider", "finmind")
	status, _ = h.HandleChannelKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Errorf("missing api_key status = %d, want 400", status)
	}
}

func TestHandleChannelKeys_NoManager(t *testing.T) {
	h := NewHandlers(t.TempDir(), t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channel-keys", nil)
	if status, _ := h.HandleChannelKeysList(req); status != http.StatusServiceUnavailable {
		t.Errorf("list status = %d, want 503", status)
	}
	req = httptest.NewRequest(http.MethodPut, "/api/admin/channel-keys/finmind", strings.NewReader(`{"api_key":"1234567890"}`))
	if status, _ := h.HandleChannelKeyUpdate(req); status != http.StatusServiceUnavailable {
		t.Errorf("update status = %d, want 503", status)
	}
}

// TestChannelKeysRoutes_AdminGated (issue #1776): the routes must be
// registered admin-protected — without ATLAS_ADMIN_KEY (non-prod) they pass
// through; with a key set, a wrong/missing key gets 401.
func TestChannelKeysRoutes_AdminGated(t *testing.T) {
	h := newTestChannelKeys(t)
	t.Setenv("ATLAS_ADMIN_KEY", "secret-admin-key")
	t.Setenv("ATLAS_API_KEY", "service-api-key")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Missing key → 401.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channel-keys", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET without key = %d, want 401", rec.Code)
	}

	// Correct key → 200.
	req = httptest.NewRequest(http.MethodGet, "/api/admin/channel-keys", nil)
	req.Header.Set("X-Admin-Key", "secret-admin-key")
	req.Header.Set("Authorization", "Bearer service-api-key")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET with key = %d, want 200", rec.Code)
	}

	// PUT with key → 200 and persisted.
	req = httptest.NewRequest(http.MethodPut, "/api/admin/channel-keys/fugle", strings.NewReader(`{"api_key":"routed-fugle-key-777"}`))
	req.Header.Set("X-Admin-Key", "secret-admin-key")
	req.Header.Set("Authorization", "Bearer service-api-key")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("PUT with key = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Method guard: POST → 405.
	req = httptest.NewRequest(http.MethodPost, "/api/admin/channel-keys/fugle", strings.NewReader(`{}`))
	req.Header.Set("X-Admin-Key", "secret-admin-key")
	req.Header.Set("Authorization", "Bearer service-api-key")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", rec.Code)
	}
}
