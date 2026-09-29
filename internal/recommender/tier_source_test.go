package recommender

// #2128：存取層級的來源與語彙收斂（C-02）。
//
// 三條要釘住的事實：
//  1. 有 token ⇒ tier 取自**已驗證的 claims**（本地 users 列不是來源）
//  2. 兩代語彙同一個存取等級要給同一份內容（basic/registered、pro/premium）
//  3. 本地列只做稽核：class 不一致 ⇒ logging.Warn，但**以 claims 為準**

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/subscription"
)

// ---------------------------------------------------------------------------
// 測試用 logger 擷取
// ---------------------------------------------------------------------------

type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

// logCapture 是 slog.Handler 的最小實作：只把訊息與屬性記下來供斷言。
type logCapture struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{level: r.Level, msg: r.Message, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.String()
		return true
	})
	c.mu.Lock()
	c.records = append(c.records, rec)
	c.mu.Unlock()
	return nil
}

func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) find(msg string) (capturedRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.records {
		if r.msg == msg {
			return r, true
		}
	}
	return capturedRecord{}, false
}

// captureLogs 把全域 logger 換成擷取器（logging.SetLogger 是既有 seam），
// 測試結束還原。
func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	prev := logging.Default()
	logging.SetLogger(slog.New(c))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return c
}

// ---------------------------------------------------------------------------
// 測試用 token / handler
// ---------------------------------------------------------------------------

// itTokenWithTier 造一個合法 HS256 token，其 claims.Tier 就是給定的 tier
// （legacy 語彙 registered/premium 與 access 語彙 basic/pro 都能這樣造，
// 因為 Generate 直接把 EffectiveTier() 寫進 claims）。
func itTokenWithTier(t *testing.T, email string, tier subscription.Tier) string {
	t.Helper()
	jwtMgr := subscription.NewJWTManager("tier-source-test-secret", "")
	token, err := jwtMgr.Generate(&subscription.User{ID: 7, Email: email, Tier: tier}, time.Hour)
	if err != nil {
		t.Fatalf("Generate token: %v", err)
	}
	return token
}

// itHandlerWithToken 造 handler（生產形狀：devMode=false）＋帶 token 的請求。
func itHandlerWithToken(t *testing.T, token string) (*Handler, *http.Request) {
	t.Helper()
	store, err := subscription.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("subscription.NewStore: %v", err)
	}
	h := NewHandler(*store, subscription.NewJWTManager("tier-source-test-secret", ""))
	req, err := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return h, req
}

// itCall 呼叫 handler 並把 panic 轉成失敗。
func itCall(t *testing.T, h *Handler, r *http.Request) TierRecommendation {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("HandleRecommendations panicked: %v", rec)
		}
	}()
	code, data := h.HandleRecommendations(r)
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200", code)
	}
	rec, ok := data.(TierRecommendation)
	if !ok {
		t.Fatalf("data type = %T, want TierRecommendation", data)
	}
	return rec
}

// ---------------------------------------------------------------------------
// ① / ③：來源與語彙
// ---------------------------------------------------------------------------

// TestClaims_ProGetsPremiumContent：本地無列 ＋ claims=pro ⇒ tier=pro 且**有** strategies。
// 這是「只改來源不改語彙」會出貨的中間狀態（tier=pro 但 strategies 空）的回歸釘子。
func TestClaims_ProGetsPremiumContent(t *testing.T) {
	logs := captureLogs(t)
	h, req := itHandlerWithToken(t, itTokenWithTier(t, "pro-member@example.test", subscription.TierPro))

	rec := itCall(t, h, req)

	if rec.Tier != string(subscription.TierPro) {
		t.Errorf("tier = %q, want %q (claims are the C-02 authority)", rec.Tier, subscription.TierPro)
	}
	if rec.Strategies == nil || rec.Strategies.Active == "" {
		t.Fatalf("strategies = %+v, want premium content (tier=pro must not be an empty shell)", rec.Strategies)
	}
	if _, ok := logs.find("tier_claim_local_drift"); ok {
		t.Error("no local row is the normal go-member case; it must not emit a drift warning")
	}
}

// TestClaims_BasicGetsRegisteredContent：claims=basic ⇒ all_weather（與 legacy registered 同內容）。
func TestClaims_BasicGetsRegisteredContent(t *testing.T) {
	h, req := itHandlerWithToken(t, itTokenWithTier(t, "basic-member@example.test", subscription.TierBasic))

	rec := itCall(t, h, req)

	if rec.Tier != string(subscription.TierBasic) {
		t.Errorf("tier = %q, want %q", rec.Tier, subscription.TierBasic)
	}
	if rec.Strategies == nil || rec.Strategies.Active != "all_weather" {
		t.Fatalf("strategies = %+v, want all_weather for the basic access tier", rec.Strategies)
	}
}

// TestClaims_LegacyVocabularyStillWorks 是「legacy 不得回歸」的硬約束：
// legacy 自簽 token（registered／premium）行為必須與 #2128 之前一致。
func TestClaims_LegacyVocabularyStillWorks(t *testing.T) {
	t.Run("premium", func(t *testing.T) {
		h, req := itHandlerWithToken(t, itTokenWithTier(t, "legacy-premium@example.test", subscription.TierPremium))
		rec := itCall(t, h, req)
		if rec.Tier != string(subscription.TierPremium) {
			t.Errorf("tier = %q, want premium", rec.Tier)
		}
		if rec.Strategies == nil || rec.Strategies.Active == "" {
			t.Fatalf("strategies = %+v, want premium content for legacy premium", rec.Strategies)
		}
	})
	t.Run("registered", func(t *testing.T) {
		h, req := itHandlerWithToken(t, itTokenWithTier(t, "legacy-registered@example.test", subscription.TierRegistered))
		rec := itCall(t, h, req)
		if rec.Tier != string(subscription.TierRegistered) {
			t.Errorf("tier = %q, want registered", rec.Tier)
		}
		if rec.Strategies == nil || rec.Strategies.Active != "all_weather" {
			t.Fatalf("strategies = %+v, want all_weather for legacy registered", rec.Strategies)
		}
	})
}

// TestClaims_FreeStaysFree：claims=free ⇒ 無 strategies（免費層）。
func TestClaims_FreeStaysFree(t *testing.T) {
	h, req := itHandlerWithToken(t, itTokenWithTier(t, "free-member@example.test", subscription.TierFree))
	rec := itCall(t, h, req)
	if rec.Tier != string(subscription.TierFree) {
		t.Errorf("tier = %q, want free", rec.Tier)
	}
	if rec.Strategies != nil {
		t.Errorf("strategies = %+v, want nil for the free tier", rec.Strategies)
	}
}

// TestAnonymous_NoTokenStaysFree：沒有 token ⇒ free（allowGuest；行為不變）。
func TestAnonymous_NoTokenStaysFree(t *testing.T) {
	store, err := subscription.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	h := NewHandler(*store, subscription.NewJWTManager("tier-source-test-secret", ""))
	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)

	rec := itCall(t, h, req)

	if rec.Tier != string(subscription.TierFree) || rec.Strategies != nil {
		t.Fatalf("anonymous = (tier=%q, strategies=%v), want free with no strategies", rec.Tier, rec.Strategies)
	}
}

// ---------------------------------------------------------------------------
// ②：本地列只做稽核，claims 為準
// ---------------------------------------------------------------------------

// itHandlerWithLocalRow 造一個「本地已有該列」的 handler：
// subscription.Register 會設 7 天試用 ⇒ EffectiveTier() = premium（class 2）。
func itHandlerWithLocalRow(t *testing.T, email string, tokenTier subscription.Tier) (*Handler, *http.Request) {
	t.Helper()
	store, err := subscription.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Register(email, "hash"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	h := NewHandler(*store, subscription.NewJWTManager("tier-source-test-secret", ""))
	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("Authorization", "Bearer "+itTokenWithTier(t, email, tokenTier))
	return h, req
}

// TestLocalRowDrift_ClaimsWinAndWarn：本地列（試用中 ⇒ premium class）與 claims=free 不同 class
// ⇒ 以 claims 為準（tier=free），並記一筆稽核警告。
func TestLocalRowDrift_ClaimsWinAndWarn(t *testing.T) {
	const email = "stale-row@example.test"
	logs := captureLogs(t)
	h, req := itHandlerWithLocalRow(t, email, subscription.TierFree)

	rec := itCall(t, h, req)

	if rec.Tier != string(subscription.TierFree) {
		t.Errorf("tier = %q, want free (claims win over the local row)", rec.Tier)
	}
	if rec.Strategies != nil {
		t.Errorf("strategies = %+v, want nil (claims tier is free)", rec.Strategies)
	}
	warn, ok := logs.find("tier_claim_local_drift")
	if !ok {
		t.Fatal("want a tier_claim_local_drift warning when the local row disagrees with the claims")
	}
	if warn.level != slog.LevelWarn {
		t.Errorf("level = %v, want Warn", warn.level)
	}
	if got := warn.attrs["claims_tier"]; got != "free" {
		t.Errorf("claims_tier = %q, want free", got)
	}
	if got := warn.attrs["local_tier"]; got == "" {
		t.Error("local_tier must be recorded so the stale row can be found and fixed")
	}
}

// TestLocalRowAgreesAcrossVocabulary_NoWarn：本地 premium（class 2）與 claims pro（class 2）
// 是**同一等級的兩代語彙** ⇒ 不記警告，且以 claims 的 pro 為準。
func TestLocalRowAgreesAcrossVocabulary_NoWarn(t *testing.T) {
	logs := captureLogs(t)
	h, req := itHandlerWithLocalRow(t, "agreeing-row@example.test", subscription.TierPro)

	rec := itCall(t, h, req)

	if rec.Tier != string(subscription.TierPro) {
		t.Errorf("tier = %q, want pro", rec.Tier)
	}
	if rec.Strategies == nil || rec.Strategies.Active == "" {
		t.Fatalf("strategies = %+v, want premium content", rec.Strategies)
	}
	if _, ok := logs.find("tier_claim_local_drift"); ok {
		t.Error("premium (legacy) and pro (access) are the same class; no drift warning expected")
	}
}

// ---------------------------------------------------------------------------
// ⑤ 純函式表
// ---------------------------------------------------------------------------

func TestAccessClassAndTierDrift(t *testing.T) {
	classCases := []struct {
		tier subscription.Tier
		want int
	}{
		{subscription.TierFree, 0},
		{subscription.TierBasic, 1},
		{subscription.TierRegistered, 1},
		{subscription.TierPro, 2},
		{subscription.TierPremium, 2},
		{subscription.Tier("platinum"), 0}, // 未知一律最低，不得因此取得更多內容
		{subscription.Tier(""), 0},
	}
	for _, c := range classCases {
		if got := accessClass(c.tier); got != c.want {
			t.Errorf("accessClass(%q) = %d, want %d", c.tier, got, c.want)
		}
	}

	driftCases := []struct {
		claims, local subscription.Tier
		want          bool
	}{
		{subscription.TierFree, subscription.TierFree, false},
		{subscription.TierBasic, subscription.TierRegistered, false},
		{subscription.TierPro, subscription.TierPremium, false},
		{subscription.TierPro, subscription.TierRegistered, true},
		{subscription.TierFree, subscription.TierPremium, true},
		{subscription.TierBasic, subscription.TierFree, true},
	}
	for _, c := range driftCases {
		got, detail := tierDrift(c.claims, c.local)
		if got != c.want {
			t.Errorf("tierDrift(claims=%q, local=%q) = %v, want %v", c.claims, c.local, got, c.want)
		}
		if got && detail == "" {
			t.Errorf("tierDrift(claims=%q, local=%q) must explain the mismatch", c.claims, c.local)
		}
		if !got && detail != "" {
			t.Errorf("tierDrift(claims=%q, local=%q) = no drift but detail %q", c.claims, c.local, detail)
		}
	}
}
