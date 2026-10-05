package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// ③「需關注」三分類（2026-10-05）的驗收。
//
// 驗收形狀（業主指定）：以 10-02～10-04 的歷史狀態（或模擬輸入）驗
//   ① 週末不再把 SBL / gov 當異常
//   ② 退役通道不出現在「需關注」
//   ③ finmind / tdcc 顯示為「已知上游限制」而不是泛用警告
// 本檔用模擬輸入覆蓋 ① ③ 的分類結果，並以 service 的真實路徑（applyContractVerdicts
// ＋ GetAlerts）覆蓋 ②。

var (
	attnZone   = time.FixedZone("CST", 8*3600)
	attnSunday = time.Date(2026, 10, 4, 22, 28, 0, 0, attnZone)
	// F58 實損讀數：兩個通道最後一次成功都是 2026-10-02（週五）台北 15:20 前後。
	attnLastSuccess = time.Date(2026, 10, 2, 7, 20, 0, 0, time.UTC)
)

func attnRecord(status string) *apigateway.ChannelHealthRecord {
	return &apigateway.ChannelHealthRecord{
		Status:        status,
		LastFetchAt:   attnLastSuccess.Format(time.RFC3339),
		LastSuccessAt: attnLastSuccess.Format(time.RFC3339),
		LastDataAt:    attnLastSuccess.Format(time.RFC3339),
	}
}

// TestClassifyChannelAttention_WeekendCalendarChannels 覆蓋驗收 ①：
// 週末的 twse_sbl / government_broker 不但判定不再是異常（DeriveChannelStatus
// = ok），而且萬一有非 ok 的判定，它也會被歸到「預期等待」而不是「系統錯誤」。
func TestClassifyChannelAttention_WeekendCalendarChannels(t *testing.T) {
	for _, id := range []string{"twse_sbl", "government_broker"} {
		contract := apigateway.ChannelContracts().Contract(id)

		// (a) 狀態層：週末 = ok ⇒ 根本不會進「需關注」清單。
		okRec := attnRecord(apigateway.StatusOK)
		if got := apigateway.DeriveChannelStatus(okRec, contract, attnSunday); got != apigateway.StatusOK {
			t.Errorf("%s 週末狀態 = %q, want ok", id, got)
		}
		if got := ClassifyChannelAttention(contract, okRec, apigateway.StatusOK, "", attnSunday); got != "" {
			t.Errorf("%s 週末（ok）分類 = %q, want 空字串（不是需關注）", id, got)
		}

		// (b) 分類層（第二道防線）：若同一時刻仍有非 ok 判定（例如 warn 的抓取
		// 失敗），成因是「日曆未到」而不是 atlas 壞掉。
		for _, status := range []string{apigateway.StatusWarn, apigateway.StatusStale, apigateway.StatusDegraded} {
			got := ClassifyChannelAttention(contract, attnRecord(status), status, "fetch failed", attnSunday)
			if got != AttentionCategoryExpectedWait {
				t.Errorf("%s（%s）週末分類 = %q, want %q", id, status, got, AttentionCategoryExpectedWait)
			}
		}

		// 負向控制：同一個通道在週一 18:30（發布截止後）仍非 ok 就不再是「預期
		// 等待」——那是真的沒有資料落地。
		mondayEvening := time.Date(2026, 10, 5, 18, 30, 0, 0, attnZone)
		if got := ClassifyChannelAttention(contract, attnRecord(apigateway.StatusStale), apigateway.StatusStale, "", mondayEvening); got != AttentionCategorySystemError {
			t.Errorf("%s 週一 18:30 分類 = %q, want %q", id, got, AttentionCategorySystemError)
		}
	}
}

// TestClassifyChannelAttention_KnownUpstreamLimit 覆蓋驗收 ③：FinMind 配額
// （HTTP 402）造成的 warn 必須歸為「已知上游限制」，而不是泛用警告。
func TestClassifyChannelAttention_KnownUpstreamLimit(t *testing.T) {
	const quota402 = `finmind: daily quota exhausted: {"msg":"Requests reach the upper limit"}`

	for _, id := range []string{"finmind", "tdcc_equity_dispersion"} {
		contract := apigateway.ChannelContracts().Contract(id)
		if contract.KnownUpstreamLimit == "" {
			t.Fatalf("%s 必須宣告 KnownUpstreamLimit（配額／tier 是上游限制）", id)
		}
		got := ClassifyChannelAttention(contract, attnRecord(apigateway.StatusWarn), apigateway.StatusWarn, quota402, attnSunday)
		if got != AttentionCategoryUpstreamLimit {
			t.Errorf("%s 分類 = %q, want %q", id, got, AttentionCategoryUpstreamLimit)
		}
	}

	// 宣告優先於訊息：即使訊息文字被上游改寫，宣告過的通道仍分類正確。
	contract := apigateway.ChannelContracts().Contract("finmind")
	if got := ClassifyChannelAttention(contract, attnRecord(apigateway.StatusWarn), apigateway.StatusWarn, "some new wording nobody has seen yet", attnSunday); got != AttentionCategoryUpstreamLimit {
		t.Errorf("宣告過的通道在訊息改寫後分類 = %q, want %q", got, AttentionCategoryUpstreamLimit)
	}

	// 訊息後備：未宣告的通道遇到已具名的上游限制字樣，也不該被當成 atlas 故障。
	plain := apigateway.ChannelContracts().Contract("twse_margin")
	if got := ClassifyChannelAttention(plain, attnRecord(apigateway.StatusWarn), apigateway.StatusWarn, quota402, attnSunday); got != AttentionCategoryUpstreamLimit {
		t.Errorf("訊息後備分類 = %q, want %q", got, AttentionCategoryUpstreamLimit)
	}
}

// TestClassifyChannelAttention_SystemErrorIsTheDefault 是這個分類器的紀律：
// 沒有證據可以歸因給上游或日曆時，一律算我們的問題（寧可誤指自己，也不要
// 把未知狀況靜默地掛到別人頭上）。
func TestClassifyChannelAttention_SystemErrorIsTheDefault(t *testing.T) {
	plain := apigateway.ChannelContracts().Contract("twse_margin") // 無日曆、未宣告上游限制

	for _, tc := range []struct{ name, status, msg string }{
		{"傳輸錯誤", apigateway.StatusError, "dial tcp: connection refused"},
		{"schema 漂移", apigateway.StatusError, "unexpected JSON schema in payload"},
		{"完全沒有訊息", apigateway.StatusWarn, ""},
	} {
		if got := ClassifyChannelAttention(plain, nil, tc.status, tc.msg, attnSunday); got != AttentionCategorySystemError {
			t.Errorf("%s 分類 = %q, want %q", tc.name, got, AttentionCategorySystemError)
		}
	}

	// 不是需關注的狀態（ok / inactive / unknown）不帶分類。
	for _, status := range []string{apigateway.StatusOK, apigateway.StatusInactive, apigateway.StatusUnknown, ""} {
		if got := ClassifyChannelAttention(plain, nil, status, "", attnSunday); got != "" {
			t.Errorf("status %q 的分類 = %q, want 空字串", status, got)
		}
	}
}

// TestClassifyChannelAttention_RetiredIsNotAttention 覆蓋驗收 ② 的分類層：
// 設計退休的通道是「已決定的狀態」，不進「需關注」。
func TestClassifyChannelAttention_RetiredIsNotAttention(t *testing.T) {
	for _, id := range []string{"twse_oddlot", "twse-oddlot", "twse_etf", "twse-etf"} {
		contract := apigateway.ChannelContracts().Contract(id)
		if !contract.IsRetired() {
			t.Fatalf("%s 的契約必須宣告 Retirement", id)
		}
		got := ClassifyChannelAttention(contract, attnRecord(apigateway.StatusError), apigateway.StatusRetired, "殘留錯誤", attnSunday)
		if got != AttentionCategoryRetired {
			t.Errorf("%s 分類 = %q, want %q", id, got, AttentionCategoryRetired)
		}
	}
}

// TestApplyContractVerdictsAndGetAlerts_RetiredChannel 覆蓋驗收 ② 的真實路徑：
// 後台資料通道列（/api/dashboard/data-channels 的 channels）把退役通道標成
// retired ＋ 資訊級理由，而 alerts（「需關注」清單）不含它。
func TestApplyContractVerdictsAndGetAlerts_RetiredChannel(t *testing.T) {
	dir := t.TempDir()
	store := apigateway.NewChannelHealthStoreWithPool(filepath.Join(dir, "data/state"), nil)
	if err := store.Record("twse_oddlot", apigateway.StatusInactive,
		"BFI84U 上游已由 TWSE 移除（2026-08）⇒ 本 channel 永久退役，不再抓取"); err != nil {
		t.Fatalf("record: %v", err)
	}
	// 未退役的 warn 通道做對照：它必須留在「需關注」。
	if err := store.Record("finmind", apigateway.StatusWarn, "finmind: daily quota exhausted"); err != nil {
		t.Fatalf("record: %v", err)
	}
	// FinMindAPIKey 非空是必要的：finmind 這一列由 getHealthFromStore 決定狀態，
	// 而空 key 會被判定為 inactive（那是另一件事：未設定憑證）。本測要驗的是
	// 「warn 的配額通道留在需關注，且分類為上游限制」。
	svc := &DataChannelService{WorkDir: dir, healthStore: store, FinMindAPIKey: "test-key"}

	retired := svc.applyContractVerdicts(DataChannel{ChannelID: "twse_oddlot", Status: "inactive", StatusText: "未啟用"}, time.Now())
	if retired.Status != apigateway.StatusRetired {
		t.Errorf("退役列 status = %q, want %q", retired.Status, apigateway.StatusRetired)
	}
	if retired.StatusText != "已退役" {
		t.Errorf("退役列 status_text = %q, want 已退役", retired.StatusText)
	}
	if retired.ErrorSeverity != ErrorSeverityInfo {
		t.Errorf("退役列 error_severity = %q, want %q（退休理由是資訊，不是待處理告警）", retired.ErrorSeverity, ErrorSeverityInfo)
	}
	if retired.Category != AttentionCategoryRetired {
		t.Errorf("退役列 category = %q, want %q", retired.Category, AttentionCategoryRetired)
	}
	if retired.LastError == "" || !strings.Contains(retired.LastError, "已退役") {
		t.Errorf("退役列必須帶著理由文字，got %q", retired.LastError)
	}

	alerts, err := svc.GetAlerts(context.Background())
	if err != nil {
		t.Fatalf("GetAlerts: %v", err)
	}
	for _, a := range alerts {
		if a.ChannelID == "twse_oddlot" {
			t.Errorf("退役通道不得出現在「需關注」清單：%+v", a)
		}
	}
	found := false
	for _, a := range alerts {
		if a.ChannelID == "finmind" {
			found = true
			if a.Category != AttentionCategoryUpstreamLimit {
				t.Errorf("finmind alert category = %q, want %q", a.Category, AttentionCategoryUpstreamLimit)
			}
		}
	}
	if !found {
		t.Errorf("warn 的通道必須留在「需關注」清單，got %+v", alerts)
	}
}
