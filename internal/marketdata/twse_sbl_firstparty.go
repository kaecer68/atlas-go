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
//
// ── 供給面（借券餘額）＝ TWSE TWT72U（2026-10-16 新增，FU-20260929-02） ──
//
// 上面兩張表都沒有「借券餘額」（= 券源／供給面）欄位，只有「借券賣出餘額」
// （需求面）。因此 SBLBorrowBalance 在 2026-10-15 前的全部 34 萬筆日檔中恆為
// 0（placeholder，沒有任何路徑填過它）。官方有現成的免費序列：
//
//	TWSE exchangeReport/TWT72U「證交所借券系統與證商/證金營業處所借券餘額合計表」
//	9 欄：0 證券代號 1 證券名稱 2 前日借券餘額(1)股 3 本日異動股借券(2)
//	      4 本日異動股還券(3) 5 本日借券餘額股(4)=(1)+(2)-(3)
//	      6 本日收盤價(5)單位：元 7 借券餘額市值單位：元(6)=(4)*(5) 8 市場別
//
// 該表同時涵蓋集中市場與櫃檯買賣中心（合計表），所以一天只要一個 call，且與
// TWT93U 同主機、免 key、零 FinMind 配額。實測（2026-10-16）：
//   - 2026-08-03..2026-10-16 共 52/52 個交易日，TWT93U 與 TWT72U **同日有無資料
//     完全一致** ⇒ 只有 TWT72U 缺日代表「日報尚未齊備」，呼叫端視為該日無資料。
//   - 2330 2026-09-16：借券餘額 59,693,000 ≥ 借券賣出餘額 14,684,514（供給 ≥ 需求）。
//   - 該表會列出餘額為 0 的證券，但仍有證券只出現在 TWT93U/TPEx 而不在 TWT72U
//     （2026-09-16 集中市場 1302 檔 vs 合計表 2070 檔，165 檔缺席且其借券賣出餘額
//     全為 0）⇒「表上缺席」不可當成 0 寫入，只能留 nil（不可用）。詳見
//     applyBorrowBalance。

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
	// twseSBLBorrowReportPath 是 TWSE「證交所借券系統與證商/證金營業處所借券餘額
	// 合計表」（上市 + 上櫃，全市場，一日一 call）——sbl_borrow_balance（借券餘額，
	// 供給面）的唯一來源。TWT93U / TPEx margin/sbl 只有「借券賣出」半部，沒有這個欄位
	// （2026-10-16 實測兩張表的 15 欄皆無「借券餘額」），所以 FU-20260929-02 的
	// 全 0 不是「來源回 0」而是「從來沒有抓過這個序列」。
	twseSBLBorrowReportPath = "/exchangeReport/TWT72U"
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
	// sblColBorrowPrefix 是 TWT72U 的借券餘額欄名前綴（實際表頭為
	// 「本日借券餘額股(4)=(1)+(2)-(3)」，公式後綴屬上游文案，故用前綴比對）。
	sblColBorrowPrefix = "本日借券餘額"
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

// sblBorrowColumn 解析 TWT72U「本日借券餘額」的欄位索引。
//
// ok=false 代表表頭不符預期（0 個命中，或同一表頭出現兩次）——呼叫端必須回報
// 錯誤，不可靜默產生空/錯資料。
func sblBorrowColumn(fields []string) (int, bool) {
	idx := -1
	for i, f := range fields {
		if !strings.HasPrefix(strings.TrimSpace(f), sblColBorrowPrefix) {
			continue
		}
		if idx >= 0 {
			return -1, false // 欄名重複 ⇒ 契約不符（無法判斷哪一欄才是借券餘額）
		}
		idx = i
	}
	return idx, idx >= 0
}

// parseSBLBorrowRows 把 TWT72U 的列轉成 symbol → 借券餘額（股）。
//
// 只收英數代號（濾掉「合計」等彙總列，該列的市場別為「整體市場」）。
// ok=false 表示表頭不符預期。
func parseSBLBorrowRows(fields []string, rows [][]string) (map[string]int64, bool) {
	bal, ok := sblBorrowColumn(fields)
	if !ok {
		return nil, false
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		if len(row) <= bal {
			continue
		}
		sym := strings.TrimSpace(row[0])
		if !isSBLStockID(sym) {
			continue
		}
		out[sym] = sblAmount(row[bal])
	}
	return out, true
}

// applyBorrowBalance 把 TWT72U 的借券餘額併進既有列。
//
// 只有「TWT72U 表上確實有這個證券」的列才填值；表上沒有的證券保持 nil
// （= 上游未提供，不可用），**不可填 0**：0 在本契約裡代表「上游明確回報
// 餘額為 0」（TWT72U 會列出餘額 0 的證券，2026-09-16 例：8183 精星 0 股），
// 而「未列出」只代表該證券不在這張表上（2026-09-16：TWT93U 1302 檔中有 165 檔
// 不在 TWT72U，那些檔的借券賣出餘額全為 0）。把缺席當成 0 會把「不可得」寫成
// 「已知為 0」，正是 FU-20260929-02 的原始病徵。
func applyBorrowBalance(stats []SBLStats, borrow map[string]int64) {
	for i := range stats {
		v, ok := borrow[stats[i].Symbol]
		if !ok {
			continue
		}
		val := v
		stats[i].SBLBorrowBalance = &val
	}
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

// fetchSBLBorrowTWSE 取得「借券餘額」（TWT72U，上市 + 上櫃合計表）。
//
// found=false 代表上游對該日明確無資料（假日／尚未發布），不是錯誤。
func (p *TWSESBLProvider) fetchSBLBorrowTWSE(ctx context.Context, dateStr string) (borrow map[string]int64, found bool, err error) {
	rawURL := fmt.Sprintf("%s%s?response=json&date=%s", p.baseURL, twseSBLBorrowReportPath, dateStr)
	body, ct, err := p.sblHTTPGet(ctx, p.limiter, rawURL)
	if err != nil {
		return nil, false, fmt.Errorf("twse_sbl: TWSE TWT72U %s: %w", dateStr, err)
	}
	var r twseSBLResponse
	if err := DecodeJSON(bytes.NewReader(body), ct, &r); err != nil {
		return nil, false, fmt.Errorf("twse_sbl: TWSE TWT72U %s decode: %w", dateStr, err)
	}
	if len(r.Data) == 0 {
		// stat 為「很抱歉，沒有符合條件的資料!」或空資料（假日/未發布）。
		return nil, false, nil
	}
	borrow, ok := parseSBLBorrowRows(r.Fields, r.Data)
	if !ok {
		return nil, false, fmt.Errorf("twse_sbl: TWSE TWT72U %s unexpected table layout: fields=%v", dateStr, r.Fields)
	}
	return borrow, true, nil
}

// fetchSBLDayFirstParty 抓「一個報告日」的官方資料：上市 + 上櫃合併，再補上借券餘額。
//
// 錯誤語意：
//   - 兩家都失敗 → 回傳 joined error（真正的上游故障）。
//   - 只有一家失敗 → 回傳另一家的資料 + warn log（部分覆蓋勝過整日缺資料）。
//   - 兩家都無資料（假日/未發布）→ (nil, nil, nil)，由呼叫端決定是否回探。
//   - 供給面（TWT72U 借券餘額）表頭不符預期或 HTTP 失敗 → 回傳錯誤（不可靜默把
//     供給面留成 0）；該日報尚未發布（TWT93U 有資料但 TWT72U 沒有）→ (nil,nil,nil)，
//     由呼叫端回探／下輪重試，以免落地一個供給面全 null 且日後不會被覆寫的日檔。
func (p *TWSESBLProvider) fetchSBLDayFirstParty(ctx context.Context, day time.Time) ([]SBLStats, []string, error) {
	twseDay := day.Format("20060102")
	tpexDay := day.Format("2006/01/02")

	var (
		stats  []SBLStats
		srcs   []string
		errs   []error
		twseOK bool
	)
	if s, err := p.fetchSBLTWSE(ctx, twseDay); err != nil {
		errs = append(errs, err)
	} else if len(s) > 0 {
		stats = append(stats, s...)
		srcs = append(srcs, "TWSE:TWT93U")
		twseOK = true
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
	merged := sortSBLStats(dedupeSBLStats(stats))
	if !twseOK {
		// 上市主機失敗（TWT93U）⇒ 同主機的 TWT72U 必然也不可得；此時維持既有的
		// 「部分覆蓋」語意（回傳上櫃資料），供給面留 nil（不可用）並留下 warn。
		logging.Warn("twse_sbl_provider", "first_party_borrow_unavailable", "day", twseDay,
			"reason", "TWSE TWT93U failed, so the same-host TWT72U borrow balance is unavailable")
		return merged, srcs, nil
	}
	borrow, found, err := p.fetchSBLBorrowTWSE(ctx, twseDay)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		// TWT93U 與 TWT72U 同日發布（2026-08-03..10-16 共 52/52 交易日一致）⇒
		// TWT93U 有、TWT72U 沒有 = 今天的日報還沒齊備，整日視為無資料。
		return nil, nil, nil
	}
	applyBorrowBalance(merged, borrow)
	srcs = append(srcs, "TWSE:TWT72U")
	return merged, srcs, nil
}
