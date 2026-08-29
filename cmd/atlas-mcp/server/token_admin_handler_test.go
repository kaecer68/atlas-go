package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminHandler_Register_200(t *testing.T) {
	store := newMapTokenStore()
	handler := NewTokenAdminHandler(store, "admin-secret")

	body := `{"tenant_id":"t1","agent_id":"a1","scopes":["read-only"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/mcp/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", "admin-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["token"]; !ok {
		t.Fatal("response missing 'token' field")
	}
	if _, ok := resp["token_id"]; !ok {
		t.Fatal("response missing 'token_id' field")
	}
}

func TestAdminHandler_Register_401_NoToken(t *testing.T) {
	store := newMapTokenStore()
	handler := NewTokenAdminHandler(store, "admin-secret")

	body := `{"tenant_id":"t1","agent_id":"a1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/mcp/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-Admin-Token header.
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAdminHandler_Revoke_200(t *testing.T) {
	store := newMapTokenStore()
	handler := NewTokenAdminHandler(store, "admin-secret")

	// First register a token.
	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	info, _ := store.Lookup(context.Background(), raw)
	tokenID := info.TokenID.String()

	// Revoke via DELETE /api/admin/mcp/tokens/{id}.
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/mcp/tokens/"+tokenID, nil)
	req.Header.Set("X-Admin-Token", "admin-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify token is revoked.
	_, err = store.Lookup(context.Background(), raw)
	if err == nil {
		t.Fatal("expected ErrRevoked after DELETE, got nil")
	}
}

func TestAdminHandler_Rotate_200(t *testing.T) {
	store := newMapTokenStore()
	handler := NewTokenAdminHandler(store, "admin-secret")

	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	info, _ := store.Lookup(context.Background(), raw)
	tokenID := info.TokenID.String()

	// Rotate via POST /api/admin/mcp/tokens/{id}/rotate.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/mcp/tokens/"+tokenID+"/rotate", nil)
	req.Header.Set("X-Admin-Token", "admin-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	newToken, ok := resp["token"].(string)
	if !ok || newToken == "" {
		t.Fatal("response missing 'token' field")
	}

	// Old token must fail.
	_, err = store.Lookup(context.Background(), raw)
	if err == nil {
		t.Fatal("old token should be invalid after rotate")
	}

	// New token must work.
	_, err = store.Lookup(context.Background(), newToken)
	if err != nil {
		t.Fatalf("new token after rotate should work: %v", err)
	}
}

func TestAdminHandler_List_Redacted(t *testing.T) {
	store := newMapTokenStore()
	handler := NewTokenAdminHandler(store, "admin-secret")

	_, raw, err := store.Register(context.Background(), TokenRegistration{
		TenantID: "t1",
		AgentID:  "a1",
		Scopes:   []string{"read-only"},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/mcp/tokens", nil)
	req.Header.Set("X-Admin-Token", "admin-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var tokens []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&tokens); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(tokens) == 0 {
		t.Fatal("expected at least 1 token in list")
	}

	// Verify no raw token leaked and token_hash is redacted to sha256:prefix.
	for _, tok := range tokens {
		bodyBytes, _ := json.Marshal(tok)
		bodyStr := string(bodyBytes)
		if raw != "" && strings.Contains(bodyStr, raw) {
			t.Fatal("list response leaked raw token")
		}
		hash, ok := tok["token_hash"].(string)
		if !ok || !strings.HasPrefix(hash, "sha256:") {
			t.Fatalf("list response token_hash not redacted with sha256: prefix, got %q", hash)
		}
		if len(hash) <= len("sha256:")+6 {
			t.Fatalf("redacted hash too short: %q", hash)
		}
		// Should have tenant_id, agent_id, scopes.
		if tid, ok := tok["tenant_id"].(string); !ok || tid == "" {
			t.Fatal("list response missing tenant_id")
		}
		if aid, ok := tok["agent_id"].(string); !ok || aid == "" {
			t.Fatal("list response missing agent_id")
		}
	}
}

// TestAdminHandler_EmptyAdminTokenRejects ensures the management API rejects
// all requests when ATLAS_MCP_ADMIN_TOKEN is unset (no default / dev mode).
func TestAdminHandler_EmptyAdminTokenRejects(t *testing.T) {
	store := newMapTokenStore()
	handler := NewTokenAdminHandler(store, "")

	body := `{"tenant_id":"t1","agent_id":"a1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/mcp/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when admin token empty, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_NilStore_Returns503(t *testing.T) {
	handler := NewTokenAdminHandler(nil, "admin-secret")

	body := `{"tenant_id":"t1","agent_id":"a1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/mcp/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", "admin-secret")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil store, got %d: %s", w.Code, w.Body.String())
	}
}
