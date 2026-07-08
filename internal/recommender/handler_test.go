package recommender

import (
	"net/http"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

func TestHandleRecommendations(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, err := subscription.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	h := NewHandler(*store, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Tier != string(subscription.TierFree) {
		t.Errorf("expected free tier, got %s", rec.Tier)
	}
}

func TestHandleLoggedInUser(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	h := NewHandler(*store, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Tier != string(subscription.TierPremium) {
		t.Errorf("expected premium (trial), got %s", rec.Tier)
	}
	if rec.Strategies == nil {
		t.Error("premium tier should have strategy recommendations")
	}
}

// T1 RED: P0-2 X-User-Email 偽造 tier 漏洞修復
// DEV_MODE=false (預設) 時, 沒有 JWT + 只帶 X-User-Email 必須被拒絕 (401)
// 不是 free tier fallback, 因為這會讓攻擊者只換 header 就升級 tier
func TestHandleRecommendations_DevModeFallback_Disabled(t *testing.T) {
	os.Unsetenv("ATLAS_DEV_MODE")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	h := NewHandler(*store, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	// 沒帶 Authorization header → 應該 401
	code, _ := h.HandleRecommendations(req)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with DEV_MODE disabled, got %d", code)
	}
}

// T1 GREEN 目標: DEV_MODE=true 時, X-User-Email 仍可用 (向後相容 dev/CI)
func TestHandleRecommendations_DevModeFallback_Enabled(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	h := NewHandler(*store, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 with DEV_MODE=true, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Tier != string(subscription.TierPremium) {
		t.Errorf("expected premium tier in DEV_MODE, got %s", rec.Tier)
	}
}
