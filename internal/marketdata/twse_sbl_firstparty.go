package marketdata

// twse_sbl_firstparty.go — twse_sbl 的第一方（TWSE/TPEx 官方）來源。
//
// 背景（2026-09-24 生產實證）：twse_sbl 走 FinMind dataset
// TaiwanDailyShortSaleBalances，2026-09-23 FinMind 日配額（finmindDailyLimit
// = 14400）被其他 FinMind 消費者用完 → 通道 warn
// 「finmind: daily quota exhausted (used=14400, remaining=0）」。
//
// 但該 dataset 本身只是交易所官方日報的鏡像：
//   - 上市：TWSE「融券借券賣出餘額」（信用額度總量管制餘額表）TWT93U
//   - 上櫃：TPEx 同一張表 margin/sbl
//
// 2026-09-24 逐列比對（生產 data/state/sbl/*.json vs 官方端點）：
//   - 2026-09-23：上市 1301/1301、上櫃 931/931，三個欄位全部逐值相同
//   - 2026-03-02：2157/2157 完全相同
// 因此 twse_sbl 改吃第一方來源：零 FinMind 配額、零新增付費依賴，且與
// channel contract 宣告的 SourcePriority=["TWSE","TPEx"] 一致（舊版宣告
// TWSE 但實際走 FinMind，屬於契約與實作不符）。
//
// 表格欄位（兩家交易所相同，14 欄）：
//
//	0 代號  1 名稱  2 前日餘額  3 賣出  4 買進  5 現券  6 今日餘額
//	7 次一營業日限額                                  ← 融資半部
//	8 前日餘額  9 當日賣出  10 當日還券  11 當日調整  12 當日餘額
//	13 次一營業日可限額  14 備註                       ← 借券半部
//
// 本檔只讀「借券半部」：當日賣出 → SBLShortVolume、當日還券 →
// SBLReturnVolume、當日餘額 → SBLShortBalance。索引以表頭名稱解析（不用硬
// 編號），「當日餘額」取最後一次出現（前半是融資「今日餘額」，名稱不同，
// 但仍以最後一次出現為準以防上游改名）。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// twseSBLReportPath 是 TWSE「融券借券賣出餘額」（上市，全市場，一日一 call）。
	twseSBLReportPath = "/exchangeReport/TWT93U"
	// tpexSBLReportPath 是 TPEx 同一張表（上櫃）。
	tpexSBLReportPath = "/www/zh-tw/margin/sbl"
	// tpexSBLBaseURL 是櫃買中心官網。constants 沒有 TPEx 常數（twse_sbl 是
	// 第一個 TPEx 消費端），因此常數留在本檔。
	tpexSBLBaseURL = "https://www.tpex.org.tw"

	// sblMaxBodyBytes 上限：官方回應約 150KB（上市）／95KB（上櫃），留 8MB
	// 餘裕避免上游異常回應把記憶體吃光。
	sblMaxBodyBytes = 8 << 20

	// sblProbeDays 是「探測日往回找」的最大天數（週末/假日/尚未發布）。
	sblProbeDays = 5
)

// 借券半部的表頭名稱（兩家交易所一致）。
const (
	sblColSell    = "當日賣出"
	sblColReturn  = "當日還券"
	sblColBalance = "當日餘額"
)

// twseSBLResponse 是 TWSE exchangeReport 系列的共同信封。
type twseSBLResponse struct {
	Stat   string     `json:"stat"`
	Date   string     `json:"date"`
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
}

// tpexSBLResponse 是 TPEx www JSON 信封（stat + tables[]）。
type tpexSBLResponse struct {
	Stat   string `json:"stat"`
	Date   string `json:"date"`
	Tables []struct {
		Title      string     `json:"title"`
		Date       string     `json:"date"`
		TotalCount int        `json:"totalCount"`
		Fields     []string   `json:"fields"`
		Data       [][]string `json:"data"`
	} `json:"tables"`
}

// ─── 共用 token bucket（TPEx） ───────────────────────────────────────────────
//
// 與 TWSE 相同的理由（twse_openapi.go P1-13）：多個 provider 各自建 limiter
// 會集體超過官方政策。TPEx 走同一個政策值（marketdata.twse_api_rate_limit /
// twse_api_rate_burst，預設 3 req / 5s），但獨立桶：TPEx 與 TWSE 是不同主機。
var (
	tpexSharedLimiterMu sync.Mutex
	tpexSharedLimiter   *rate.Limiter
)

func getTPExSharedLimiter() *rate.Limiter {
	tpexSharedLimiterMu.Lock()
	defer tpexSharedLimiterMu.Unlock()
	if tpexSharedLimiter == nil {
		params := config.GetParametersConfig()
		tpexSharedLimiter = rate.NewLimiter(
			rate.Limit(params.Marketdata.TWSEAPIRateLimit.Value),
			params.Marketdata.TWSEAPIRateBurst.Value,
		)
	}
	return tpexSharedLimiter
}

// SetTPExSharedLimiterForTest replaces the shared TPEx bucket (tests only).
// Returns the previous limiter for restoring via defer/Cleanup.
func SetTPExSharedLimiterForTest(l *rate.Limiter) *rate.Limiter {
	tpexSharedLimiterMu.Lock()
	defer tpexSharedLimiterMu.Unlock()
	old := tpexSharedLimiter
	tpexSharedLimiter = l
	return old
}

// sblColumns 解析「借券半部」的欄位索引。
//
// 這兩張表混了兩組語意不同的欄位（融券 index 2-7 vs 借券 index 8-13），且欄位
// 名稱會重複：TWSE 用「今日餘額」/「當日餘額」區分兩組，TPEx 兩組都叫
// 「當日餘額」。因此：
//   - 「當日餘額」取最後一次出現（= 借券半部）；
//   - 「當日賣出」/「當日還券」只在借券半部出現（唯一）；
//   - 最後再以位置順序（當日賣出 < 當日還券 < 當日餘額）做一致性檢查，避免
//     上游改版時把融券欄位當成借券欄位而靜默寫錯資料。
//
// ok=false 代表表頭不符預期 —— 呼叫端必須回報錯誤，不可產生空/錯資料。
func sblColumns(fields []string) (sell, ret, bal int, ok bool) {
	sell, ret, bal = -1, -1, -1
	for i, f := range fields {
		switch strings.TrimSpace(f) {
		case sblColSell:
			if sell < 0 {
				sell = i
			}
		case sblColReturn:
			if ret < 0 {
				ret = i
			}
		case sblColBalance:
			bal = i // 取最後一次出現（借券半部）
		}
	}
	if sell < 0 || ret < 0 || bal < 0 {
		return -1, -1, -1, false
	}
	if !(sell < ret && ret < bal) {
		return -1, -1, -1, false
	}
	return sell, ret, bal, true
}

// isSBLStockID 只接受英數字代號（TWSE/TPEx 證券代號），濾掉「合計」等中文
// 彙總列與空列。
func isSBLStockID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		default:
			return false
		}
	}
	return true
}

// sblAmount 解析千分位數字。空字串、"-"、"X"、"---" 視為 0（上游對無資料欄位
// 用這些符號；與 FinMind 路徑 floatField 的 0 語意一致）。
func sblAmount(s string) int64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	switch s {
	case "", "-", "--", "---", "X", "x":
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseSBLRows 把官方表格列轉成 SBLStats。ok=false 表示表頭不符預期。
func parseSBLRows(fields []string, rows [][]string, date string) ([]SBLStats, bool) {
	sell, ret, bal, ok := sblColumns(fields)
	if !ok {
		return nil, false
	}
	out := make([]SBLStats, 0, len(rows))
	for _, row := range rows {
		if len(row) <= bal || len(row) <= sell || len(row) <= ret {
			continue
		}
		sym := strings.TrimSpace(row[0])
		if !isSBLStockID(sym) {
			continue
		}
		out = append(out, SBLStats{
			Date:            date,
			Symbol:          sym,
			SBLShortBalance: sblAmount(row[bal]),
			SBLShortVolume:  sblAmount(row[sell]),
			SBLReturnVolume: sblAmount(row[ret]),
		})
	}
	return out, true
}

// sblReportDate 正規化官方回傳的報告日期為 "2006-01-02"。官方格式為
// "20260923"（TWSE/TPEx 頂層 date）；fallback 為請求日（"20060102" 或
// "2006/01/02"）。
func sblReportDate(raw, fallback string) string {
	cand := strings.TrimSpace(raw)
	if cand == "" {
		cand = strings.TrimSpace(fallback)
	}
	cand = strings.ReplaceAll(cand, "/", "")
	cand = strings.ReplaceAll(cand, "-", "")
	if len(cand) == 8 {
		if d, err := time.Parse("20060102", cand); err == nil {
			return d.Format("2006-01-02")
		}
	}
	return ""
}

// dedupeSBLStats 依代號去重合併（上市優先：TWSE 先加入）。同一代號不應同時
// 出現在上市與上櫃表，若出現以上市為準。
func dedupeSBLStats(stats []SBLStats) []SBLStats {
	seen := make(map[string]struct{}, len(stats))
	out := make([]SBLStats, 0, len(stats))
	for _, s := range stats {
		if _, ok := seen[s.Symbol]; ok {
			continue
		}
		seen[s.Symbol] = struct{}{}
		out = append(out, s)
	}
	return out
}

// sortSBLStats 依代號排序，讓檔案內容穩定（舊 FinMind 路徑由 map 產生，順序
// 隨機 → 每次重跑檔案都不同，難以 diff/驗證）。
func sortSBLStats(stats []SBLStats) []SBLStats {
	sort.Slice(stats, func(i, j int) bool { return stats[i].Symbol < stats[j].Symbol })
	return stats
}

// sblHTTPGet 取回官方 JSON body（含 limiter 與大小上限）。
func (p *TWSESBLProvider) sblHTTPGet(ctx context.Context, limiter *rate.Limiter, rawURL string) ([]byte, string, error) {
	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return nil, "", fmt.Errorf("rate limit wait: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, sblMaxBodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// fetchSBLTWSE 取得上市當日全市場 SBL。回傳 (nil, nil) 代表上游明確無資料
// （假日/週末/尚未發布），不是錯誤。
func (p *TWSESBLProvider) fetchSBLTWSE(ctx context.Context, dateStr string) ([]SBLStats, error) {
	rawURL := fmt.Sprintf("%s%s?response=json&date=%s", p.baseURL, twseSBLReportPath, dateStr)
	body, ct, err := p.sblHTTPGet(ctx, p.limiter, rawURL)
	if err != nil {
		return nil, fmt.Errorf("twse_sbl: TWSE TWT93U %s: %w", dateStr, err)
	}
	var r twseSBLResponse
	if err := DecodeJSON(bytes.NewReader(body), ct, &r); err != nil {
		return nil, fmt.Errorf("twse_sbl: TWSE TWT93U %s decode: %w", dateStr, err)
	}
	if len(r.Data) == 0 {
		// stat OK + 空資料（假日/未發布）或 stat 為「查詢日期大於今日」等
		// 上游提示訊息 —— 兩者都代表該日無資料。
		return nil, nil
	}
	stats, ok := parseSBLRows(r.Fields, r.Data, sblReportDate(r.Date, dateStr))
	if !ok {
		return nil, fmt.Errorf("twse_sbl: TWSE TWT93U %s unexpected table layout: fields=%v", dateStr, r.Fields)
	}
	return stats, nil
}

// fetchSBLTPEx 取得上櫃當日全市場 SBL。(nil, nil) = 該日無資料。
func (p *TWSESBLProvider) fetchSBLTPEx(ctx context.Context, dateStr string) ([]SBLStats, error) {
	rawURL := fmt.Sprintf("%s%s?response=json&date=%s", p.tpexURL, tpexSBLReportPath, dateStr)
	body, ct, err := p.sblHTTPGet(ctx, p.tpexLimiter(), rawURL)
	if err != nil {
		return nil, fmt.Errorf("twse_sbl: TPEx margin/sbl %s: %w", dateStr, err)
	}
	var r tpexSBLResponse
	if err := DecodeJSON(bytes.NewReader(body), ct, &r); err != nil {
		return nil, fmt.Errorf("twse_sbl: TPEx margin/sbl %s decode: %w", dateStr, err)
	}
	if len(r.Tables) == 0 || len(r.Tables[0].Data) == 0 {
		return nil, nil
	}
	t := r.Tables[0]
	stats, ok := parseSBLRows(t.Fields, t.Data, sblReportDate(r.Date, dateStr))
	if !ok {
		return nil, fmt.Errorf("twse_sbl: TPEx margin/sbl %s unexpected table layout: fields=%v", dateStr, t.Fields)
	}
	return stats, nil
}

// fetchSBLDayFirstParty 抓「一個報告日」的官方資料：上市 + 上櫃合併。
//
// 錯誤語意：
//   - 兩家都失敗 → 回傳 joined error（真正的上游故障）。
//   - 只有一家失敗 → 回傳另一家的資料 + warn log（部分覆蓋勝過整日缺資料）。
//   - 兩家都無資料（假日/未發布）→ (nil, nil, nil)，由呼叫端決定是否回探。
func (p *TWSESBLProvider) fetchSBLDayFirstParty(ctx context.Context, day time.Time) ([]SBLStats, []string, error) {
	twseDay := day.Format("20060102")
	tpexDay := day.Format("2006/01/02")

	var (
		stats []SBLStats
		srcs  []string
		errs  []error
	)
	if s, err := p.fetchSBLTWSE(ctx, twseDay); err != nil {
		errs = append(errs, err)
	} else if len(s) > 0 {
		stats = append(stats, s...)
		srcs = append(srcs, "TWSE:TWT93U")
	}
	if s, err := p.fetchSBLTPEx(ctx, tpexDay); err != nil {
		errs = append(errs, err)
	} else if len(s) > 0 {
		stats = append(stats, s...)
		srcs = append(srcs, "TPEx:margin/sbl")
	}
	if len(errs) == 2 {
		return nil, nil, errors.Join(errs...)
	}
	if len(errs) == 1 {
		// 單邊失敗：仍回傳另一邊的資料，但留下 warn 供運維看見覆蓋不完整。
		logging.Warn("twse_sbl_provider", "first_party_partial", "day", twseDay, logging.Err(errs[0]))
	}
	if len(stats) == 0 {
		return nil, nil, nil
	}
	return sortSBLStats(dedupeSBLStats(stats)), srcs, nil
}
