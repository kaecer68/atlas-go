// internal/monitoring/api/system/health_aggregate_test.go
//
// Stage 6 PR#1: /api/health/aggregate 的單元測試。
//
// 覆蓋：
//   - 4 個 tier check 函式各自的正常 + 邊界路徑
//   - runTier 的 panic 恢復（tier 不能讓整體 endpoint 崩潰）
//   - summarizeOverall 的 liveness gateway 邏輯
//   - aggregate endpoint 的整體 happy-path shape
//
// 雙 whitelist 同步（cmd/atlas/main.go isPublicPath + shared/handler.go
// authFreeExactPaths）跨 package 測不到，靠 PR reviewer 守護。

package system

import (
	"github.com/kaecer68/atlas-go/internal/apigateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- summarizeOverall ----

func TestSummarizeOverall_LivenessDownPropagates(t *testing.T) {
	tiers := map[string]tierReport{
		"liveness":       {OK: false, Reason: "port not healthy"},
		"channel_health": {OK: true},
		"llm_ready":      {OK: true},
		"auth_posture":   {OK: true},
	}
	got := summarizeOverall(tiers)
	if got.OK {
		t.Fatalf("overall.OK = true, want false (liveness down)")
	}
	if !strings.Contains(got.Reason, "liveness") {
		t.Errorf("overall.Reason = %q, want substring 'liveness'", got.Reason)
	}
}

func TestSummarizeOverall_NonLivenessFailureStillOK(t *testing.T) {
	tiers := map[string]tierReport{
		"liveness":       {OK: true},
		"channel_health": {OK: false, Reason: "stale"},
		"llm_ready":      {OK: false, Reason: "no key"},
		"auth_posture":   {OK: false},
	}
	got := summarizeOverall(tiers)
	if !got.OK {
		t.Fatalf("overall.OK = false, want true (liveness still up)")
	}
}

func TestSummarizeOverall_EmptyTiersDefaultsOK(t *testing.T) {
	got := summarizeOverall(map[string]tierReport{})
	if !got.OK {
		t.Fatalf("overall.OK = false, want true (empty map → no failure path)")
	}
}

// ---- runTier 強健性 ----
//
// 沒這條保證整體 endpoint 會被任一 tier 連帶拖崩，這違反 health_aggregate.go
// lines 12-14「任何 Tier 失敗不會讓整體 endpoint 500」設計備註。

func TestRunTier_PanicRecovers(t *testing.T) {
	h := &HealthHandlers{}
	got := h.runTier(func() (bool, string, any) {
		panic("simulated downstream bug")
	})
	if got.OK {
		t.Fatalf("runTier after panic: OK = true, want false")
	}
	if got.LatencyMS < 0 {
		t.Errorf("LatencyMS = %d, want >= 0", got.LatencyMS)
	}
}

func TestRunTier_HealthyReturnsTrueWithLatency(t *testing.T) {
	h := &HealthHandlers{}
	got := h.runTier(func() (bool, string, any) {
		return true, "", map[string]string{"a": "b"}
	})
	if !got.OK {
		t.Fatalf("OK = false, want true")
	}
	if got.Reason != "" {
		t.Errorf("Reason = %q, want empty", got.Reason)
	}
	if got.LatencyMS < 0 {
		t.Errorf("LatencyMS = %d, want >= 0", got.LatencyMS)
	}
}

// ---- Tier 1: liveness ----
//
// 注意：portprobe.Probe 是實際的 OS port 探測；在 test 環境會拿到「未啟動 →
// free/unknown」任一狀態。我們只斷言「不 panic + 回傳合法 struct」，不斷言 ok
// 的值（那依賴測試環境的 port 狀態）。

func TestCheckLiveness_NeverPanics(t *testing.T) {
	h := &HealthHandlers{APIAddr: "127.0.0.1:0", FubonAddr: "127.0.0.1:0"}
	ok, reason, details := h.checkLiveness()
	_, _, _ = ok, reason, details // 不斷言具體值（依環境）
	if details == nil {
		t.Errorf("details = nil, want non-nil portProbeDetail")
	}
}

// ---- Tier 2: channel_health (uses ChannelHealthStore) ----
//
// checkChannelHealth 透過 ChannelHealthStore.All() 取得所有通道狀態，
// 按 status 欄位分類計數（ok/warn/error/stale），error > 0 時回報 unhealthy。

func TestCheckChannelHealth_NoStoreWired(t *testing.T) {
	h := &HealthHandlers{} // ChannelHealth nil
	ok, reason, details := h.checkChannelHealth()
	if !ok {
		t.Errorf("ok = false, want true (nil store = not wired, not unhealthy)")
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
	detail, ok2 := details.(channelHealthDetail)
	if !ok2 {
		t.Fatalf("details type = %T, want channelHealthDetail", details)
	}
	if detail.Total != 0 {
		t.Errorf("detail.Total = %d, want 0", detail.Total)
	}
}

func TestCheckChannelHealth_EmptyStore(t *testing.T) {
	store := apigateway.NewChannelHealthStore(t.TempDir())
	h := &HealthHandlers{ChannelHealth: store}
	ok, reason, details := h.checkChannelHealth()
	if !ok {
		t.Errorf("ok = false, reason = %q; want true (空 store = 無錯誤)", reason)
	}
	detail := details.(channelHealthDetail)
	if detail.Total != 0 {
		t.Errorf("detail.Total = %d, want 0", detail.Total)
	}
}

func TestCheckChannelHealth_WithRecords(t *testing.T) {
	tmpDir := t.TempDir()
	store := apigateway.NewChannelHealthStore(tmpDir)
	// 寫入三筆紀錄：ok, warn, error 各一
	for _, ch := range []struct {
		id     string
		status string
	}{
		{"fugle", "ok"},
		{"twse", "warn"},
		{"yahoo", "error"},
	} {
		if err := store.Record(ch.id, ch.status, ""); err != nil {
			t.Fatalf("Record(%s, %s): %v", ch.id, ch.status, err)
		}
	}
	h := &HealthHandlers{ChannelHealth: store}
	ok, _, details := h.checkChannelHealth()
	if ok {
		t.Errorf("ok = true, want false (有一筆 error)")
	}
	detail := details.(channelHealthDetail)
	if detail.Total != 3 {
		t.Errorf("detail.Total = %d, want 3", detail.Total)
	}
	if detail.OK != 1 {
		t.Errorf("detail.OK = %d, want 1", detail.OK)
	}
	if detail.Warn != 1 {
		t.Errorf("detail.Warn = %d, want 1", detail.Warn)
	}
	if detail.Error != 1 {
		t.Errorf("detail.Error = %d, want 1", detail.Error)
	}
}

// ---- Tier 3: llm_ready ----

func TestCheckLLMReady_MissingKeyFails(t *testing.T) {
	h := &HealthHandlers{}
	t.Setenv("LLM_DEEPSEEK_API_KEY", "")
	ok, reason, _ := h.checkLLMReady()
	if ok {
		t.Fatalf("ok = true, want false (key 為空)")
	}
	if !strings.Contains(reason, "LLM_DEEPSEEK_API_KEY") {
		t.Errorf("reason = %q, want substring 'LLM_DEEPSEEK_API_KEY'", reason)
	}
}

func TestCheckLLMReady_PresentKeyPasses(t *testing.T) {
	h := &HealthHandlers{}
	t.Setenv("LLM_DEEPSEEK_API_KEY", "sk-dev-fake-key-for-test")
	ok, reason, _ := h.checkLLMReady()
	if !ok {
		t.Errorf("ok = false, reason = %q; want true (key 有值)", reason)
	}
}

// ---- Tier 4: auth_posture ----

func TestCheckAuthPosture_StatusPaths(t *testing.T) {
	h := &HealthHandlers{}
	cases := []struct {
		name     string
		env      string
		key      string
		wantStat string
	}{
		{"dev_no_auth", "", "", "dev_no_auth"},
		{"dev_with_key_is_authenticated", "staging", "k", "authenticated"},
		{"production_full", "production", "k", "production"},
		{"prod_no_key_dev_fallback", "production", "", "dev_no_auth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATLAS_ENV", tc.env)
			t.Setenv("ATLAS_API_KEY", tc.key)
			ok, _, details := h.checkAuthPosture()
			if !ok {
				t.Errorf("ok = false, want true (status 必為 3 種合法值之一)")
			}
			d, _ := details.(authPostureDetail)
			if d.Status != tc.wantStat {
				t.Errorf("status = %q, want %q", d.Status, tc.wantStat)
			}
		})
	}
}

// ---- Aggregate endpoint happy-path ----

func TestHandleHealthAggregate_ReturnsAllFourTiers(t *testing.T) {
	h := &HealthHandlers{APIAddr: "127.0.0.1:0", FubonAddr: "127.0.0.1:0"}
	t.Chdir(t.TempDir()) // channel_health.json 沒有 → channel_health tier 必 fail，但其他 OK
	t.Setenv("LLM_DEEPSEEK_API_KEY", "sk-test")
	req := httptest.NewRequest(http.MethodGet, "/api/health/aggregate", nil)

	status, data := h.handleHealthAggregate(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := data.(aggregateResponse)
	if !ok {
		t.Fatalf("data type = %T, want aggregateResponse", data)
	}
	for _, tierName := range []string{"liveness", "channel_health", "llm_ready", "auth_posture"} {
		if _, present := resp.Tiers[tierName]; !present {
			t.Errorf("tier %q missing from response", tierName)
		}
	}
	for tierName, tier := range resp.Tiers {
		if tier.LatencyMS < 0 {
			t.Errorf("%s.LatencyMS = %d, want >= 0", tierName, tier.LatencyMS)
		}
	}
	_ = resp.Overall.OK // 不斷言：test 環境 port 0 → liveness 必 fail，overall 會 false，端點仍回 200，符合設計。
}
