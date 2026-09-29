package recommender

// #2125：`subscription.Store.GetByEmail` 查無資料時回 `(nil, nil)`，而
// HandleRecommendations 只檢查 `err == nil` 就呼叫 `user.EffectiveTier()`
// （指標接收者，直接讀 `u.Tier`）⇒ nil pointer panic，請求以 500／連線中斷收場。
//
// 本檔兩個站點各一條：
//  1. dev-mode fallback（`X-User-Email` 指向未註冊 email）
//  2. JWT 路徑（合法 HS256 token，但 email 在本地 users 表沒有列 —— go-member
//     模式下 `internal/subscription/handler.go` 明言「JWKS 會員沒有本地列」）
//
// 修好之後這兩條即為釘子：不得 panic，且一律以 TierFree 回應（不再有
// EffectiveTier 的可達路徑）。

import (
	"net/http"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// itNewHandler 造一個「本地 users 表是空的」handler（store 指向空的 temp workdir）。
func itNewHandler(t *testing.T, devMode bool, jwtMgr *subscription.JWTManager) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := subscription.NewStore(dir)
	if err != nil {
		t.Fatalf("subscription.NewStore: %v", err)
	}
	h := NewHandler(*store, jwtMgr).WithDevMode(devMode)
	return h, dir
}

// itCallHandler 呼叫 handler，把 panic 轉成測試失敗（否則 test binary 直接掛）。
func itCallHandler(t *testing.T, h *Handler, r *http.Request) (code int, data any) {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("HandleRecommendations panicked: %v (#2125)", rec)
		}
	}()
	return h.HandleRecommendations(r)
}

func TestHandleRecommendations_UnknownDevEmailDoesNotPanic(t *testing.T) {
	h, _ := itNewHandler(t, true, nil)

	req, err := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// 未註冊的 email：dev-mode fallback 會去 GetByEmail，查無資料 ⇒ (nil, nil)。
	req.Header.Set("X-User-Email", "never-registered@example.test")

	code, data := itCallHandler(t, h, req)
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (unknown email must not crash the handler)", code)
	}
	rec, ok := data.(TierRecommendation)
	if !ok {
		t.Fatalf("data type = %T, want TierRecommendation", data)
	}
	if rec.Tier != string(subscription.TierFree) {
		t.Errorf("tier = %q, want %q (no local row ⇒ anonymous/free, not a guessed tier)", rec.Tier, subscription.TierFree)
	}
}

func TestHandleRecommendations_UnknownJWTEmailDoesNotPanic(t *testing.T) {
	jwtMgr := subscription.NewJWTManager("nil-user-test-secret", "")
	token, err := jwtMgr.Generate(&subscription.User{
		ID:    4242,
		Email: "ghost-member@example.test", // 本地 users 表沒有這一列
		Tier:  subscription.TierPremium,
	}, time.Hour)
	if err != nil {
		t.Fatalf("Generate token: %v", err)
	}

	h, _ := itNewHandler(t, false, jwtMgr)

	req, err := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	code, data := itCallHandler(t, h, req)
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (verified token + missing local row must not crash)", code)
	}
	rec, ok := data.(TierRecommendation)
	if !ok {
		t.Fatalf("data type = %T, want TierRecommendation", data)
	}
	if rec.Tier != string(subscription.TierFree) {
		t.Errorf("tier = %q, want %q (no local row ⇒ free tier)", rec.Tier, subscription.TierFree)
	}
}
