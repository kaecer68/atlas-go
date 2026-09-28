package marketdata

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// 期貨日行情（first-party）provider。
//
// 兩個來源（皆為期交所 first-party、免費、無需 API key；見
// docs/specs/futures-bars-firstparty-spec.md §1）：
//
//	來源 A  GET  https://openapi.taifex.com.tw/v1/DailyMarketReportFut
//	        JSON；**沒有日期參數**，永遠只回最新一個交易日 ⇒ 每日增量。
//	來源 B  POST https://www.taifex.com.tw/cht/3/futDataDown
//	        CSV（MS950/Big5）；可指定日期區間 ⇒ 歷史回補 + fallback。
//
// 關鍵陷阱（實測，2026-09-28；已寫入規格 §1.3）：
//
//	來源 B 的失敗**不會**反映在 HTTP status —— 查詢區間 >31 天時它仍回
//	HTTP 200，body 卻是 `<script>alert("日期時間錯誤");window.location.replace(...)</script>`。
//	因此判定一律以 **body 表頭哨兵**（首列必須以「交易日期」開頭）為準，
//	**禁止**只看 status code。這與 2026-09-27 #2107 的形狀同型。
const (
	// taifexFuturesBarsOpenAPIPath 是期貨每日交易行情（最新交易日）。
	taifexFuturesBarsOpenAPIPath = "/DailyMarketReportFut"
	// taifexFuturesBarsCSVURL 是官網「期貨每日交易行情下載」表單端點。
	taifexFuturesBarsCSVURL = "https://www.taifex.com.tw/cht/3/futDataDown"
	// futuresBarsMaxRangeDays 是實測單次查詢的區間上限（起訖相差 ≤ 31 天）。
	// 差 32 天即回 HTTP 200 + JS 錯誤頁。
	futuresBarsMaxRangeDays = 31

	futuresBarsSourceOpenAPI = "taifex_openapi"
	futuresBarsSourceCSV     = "taifex_csv"

	// futuresCSVHeaderSentinel 是官網 CSV 的表頭哨兵（見檔案頂端說明）。
	futuresCSVHeaderSentinel = "交易日期"
)

// ErrTAIFEXFuturesBarsSchema 表示回應在傳輸層成功，但**不是**可解析的期貨行情 CSV
// （表頭哨兵失敗、必要欄位缺失、HTML/JS 錯誤頁、欄位值不可解析）。
// 包裝 ErrSchema ⇒ 屬「上游 schema 變了」而非「今天沒資料」，會記 breaker 失敗。
var ErrTAIFEXFuturesBarsSchema = fmt.Errorf("taifex futures bars: schema mismatch: %w", ErrSchema)

// taifexCharsetAliases 把官網回報的 charset 標籤對到解碼器。
//
// 官網實測回 `text/html;charset=MS950`。MS950 是 Big5 的 Microsoft 變體，
// 但 **htmlindex（WHATWG 標籤表）不認得 "MS950" 這個標籤**（實測 err:
// unknown charset），所以必須先正規化，否則每一次官網抓取都會被自己判成
// schema 錯誤。x/text 的 traditionalchinese.Big5 就是 WHATWG 的 big5 編碼
// （涵蓋 MS950 的延伸字集），亦即本 repo 既有 backfill CLI 使用的解碼器。
var taifexCharsetAliases = map[string]encoding.Encoding{
	"ms950": traditionalchinese.Big5,
	"cp950": traditionalchinese.Big5,
	"big5":  traditionalchinese.Big5,
	"big-5": traditionalchinese.Big5,
}

// futuresCSVRequiredColumns 是官網 CSV 的必要欄位（表頭驅動對映；缺一即 ErrSchema）。
var futuresCSVRequiredColumns = []string{
	"交易日期", "契約", "到期月份(週別)",
	"開盤價", "最高價", "最低價", "收盤價",
	"成交量", "結算價", "未沖銷契約數", "交易時段",
}

// FuturesBarsObserver 是 provider 的可選觀測鉤子。
//
// internal/marketdata 目前**沒有**任何 prometheus 指標（見規格 §8），因此本階段
// 不發明新指標；呼叫端（例如 monitoring）若要接線，實作此介面即可，未設定時為 no-op。
type FuturesBarsObserver interface {
	// ObserveFuturesBarsFetch 在每次上游抓取後被呼叫一次。
	// source 為 "taifex_openapi" / "taifex_csv"；rows 為解析出的 bar 數（失敗時 0）。
	ObserveFuturesBarsFetch(source string, err error, rows int)
}

// TAIFEXFuturesBarsProvider 抓取期交所期貨日行情（TX/MTX 等）。
type TAIFEXFuturesBarsProvider struct {
	client         *http.Client
	openAPIBaseURL string
	csvURL         string
	rateLimiter    *rate.Limiter
	retryCfg       retryConfig
	breaker        *providerBreaker
	observer       FuturesBarsObserver
}

// NewTAIFEXFuturesBarsProvider 建立期貨日行情 provider。
func NewTAIFEXFuturesBarsProvider() *TAIFEXFuturesBarsProvider {
	return &TAIFEXFuturesBarsProvider{
		client:         httpclient.NewFactory().NewClient(60 * time.Second), // 全契約單月 CSV 可達 4MB
		openAPIBaseURL: "https://openapi.taifex.com.tw/v1",
		csvURL:         taifexFuturesBarsCSVURL,
		// 官網為 HTML 表單端點，保守限流：每 3 秒 1 次。
		rateLimiter: rate.NewLimiter(rate.Every(3*time.Second), 1),
		retryCfg:    defaultRetryConfig(),
		breaker:     newProviderBreaker("taifex_futures_bars", defaultCircuitBreakerConfig()),
	}
}

// SetHTTPClient 設定自訂 HTTP client（測試用）。
func (p *TAIFEXFuturesBarsProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// SetObserver 設定可選觀測鉤子（nil ⇒ no-op）。
func (p *TAIFEXFuturesBarsProvider) SetObserver(o FuturesBarsObserver) { p.observer = o }

// Name 回傳 provider 名稱。
func (p *TAIFEXFuturesBarsProvider) Name() string { return "taifex_futures_bars" }

func (p *TAIFEXFuturesBarsProvider) observe(source string, err error, rows int) {
	if p.observer == nil {
		return
	}
	p.observer.ObserveFuturesBarsFetch(source, err, rows)
}

func (p *TAIFEXFuturesBarsProvider) breakerRecordSuccess() {
	if p.breaker != nil {
		p.breaker.recordSuccess()
	}
}

func (p *TAIFEXFuturesBarsProvider) breakerRecordFailure() {
	if p.breaker != nil {
		p.breaker.recordFailure()
	}
}

// BreakerInfo 曝露 breaker 狀態（測試與觀測用）。
func (p *TAIFEXFuturesBarsProvider) BreakerInfo() ProviderBreakerInfo {
	if p.breaker == nil {
		return ProviderBreakerInfo{Name: "taifex_futures_bars", State: ProviderCircuitClosed}
	}
	return p.breaker.stateSnapshot()
}

// FetchLatestFuturesBars 以來源 A（OpenAPI）抓取**最新交易日**的日行情。
//
// contracts 為空或 nil ⇒ 回傳全部契約。找不到任何符合的列 ⇒ ErrNoData
// （契約尚未上市／例假，非故障）。
func (p *TAIFEXFuturesBarsProvider) FetchLatestFuturesBars(ctx context.Context, contracts []string) ([]domain.FuturesBar, error) {
	if p.breaker != nil && !p.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: taifex futures bars circuit breaker open", ErrUpstream)
	}
	if err := WaitForLimiter(ctx, p.rateLimiter); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	want := contractsSet(contracts)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.openAPIBaseURL+taifexFuturesBarsOpenAPIPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := fetchWithRetry(ctx, p.client, req, p.retryCfg)
	if err != nil {
		p.fail("openapi", err)
		return nil, fmt.Errorf("taifex futures bars http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readTAIFEXBody(resp)
	if err != nil {
		p.fail("openapi", err)
		return nil, fmt.Errorf("read taifex futures bars body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("%w: taifex futures bars api error: status %d", ErrUpstream, resp.StatusCode)
		p.fail("openapi", err)
		return nil, err
	}

	bars, err := ParseFuturesOpenAPIBars(body, want)
	if err != nil {
		p.fail("openapi", err)
		return nil, err
	}
	if len(bars) == 0 {
		p.observe(futuresBarsSourceOpenAPI, nil, 0)
		p.breakerRecordSuccess()
		return nil, fmt.Errorf("%w: taifex futures bars openapi returned no matching rows", ErrNoData)
	}
	p.observe(futuresBarsSourceOpenAPI, nil, len(bars))
	p.breakerRecordSuccess()
	return bars, nil
}

// FetchFuturesBarsRange 以來源 B（官網 CSV）抓取 [start, end] 的日行情。
//
// 區間自動分段（每段相差 ≤ 31 天）；每段獨立套用表頭哨兵與錯誤分類。
// 單段失敗不會中斷整輪：成功的段照常回傳，失敗段彙整成 error
// （errors.Join）並以 %w 保留 ErrSchema/ErrUpstream/ErrNoData 供 errors.Is 判定。
func (p *TAIFEXFuturesBarsProvider) FetchFuturesBarsRange(ctx context.Context, contracts []string, start, end time.Time) ([]domain.FuturesBar, error) {
	if end.Before(start) {
		return nil, fmt.Errorf("taifex futures bars: end %s before start %s", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}
	want := contractsSet(contracts)
	chunks := ChunkFuturesDateRange(start, end)

	var (
		all          []domain.FuturesBar
		errs         []error
		chunksFailed int
	)
	for _, c := range chunks {
		bars, err := p.fetchFuturesCSVChunk(ctx, want, c[0], c[1])
		if err != nil {
			chunksFailed++
			errs = append(errs, fmt.Errorf("%s..%s: %w", c[0].Format("2006-01-02"), c[1].Format("2006-01-02"), err))
			continue
		}
		all = append(all, bars...)
	}
	if len(errs) > 0 {
		return all, fmt.Errorf("taifex futures bars: %d/%d chunks failed: %w", chunksFailed, len(chunks), errors.Join(errs...))
	}
	return all, nil
}

// FetchFuturesBars 是便利入口：
//   - [start, end] 只涵蓋單日且等於今天（Asia/Taipei）⇒ 先試來源 A，失敗才退回來源 B。
//   - 其餘（歷史區間）⇒ 直接用來源 B。
func (p *TAIFEXFuturesBarsProvider) FetchFuturesBars(ctx context.Context, contracts []string, start, end time.Time) ([]domain.FuturesBar, error) {
	today := time.Now().In(TaiwanLocation())
	sameDay := start.Format("2006-01-02") == end.Format("2006-01-02")
	if sameDay && start.Format("2006-01-02") == today.Format("2006-01-02") {
		bars, err := p.FetchLatestFuturesBars(ctx, contracts)
		if err == nil {
			return bars, nil
		}
		if !errors.Is(err, ErrUpstream) && !errors.Is(err, ErrSchema) {
			return nil, err // ErrNoData：今天還沒出，不要再用 CSV 打一次
		}
	}

	return p.FetchFuturesBarsRange(ctx, contracts, start, end)
}

// fail 統一記錄 breaker 失敗與觀測。
func (p *TAIFEXFuturesBarsProvider) fail(source string, err error) {
	p.observe(source, err, 0)
	p.breakerRecordFailure()
}

func (p *TAIFEXFuturesBarsProvider) fetchFuturesCSVChunk(ctx context.Context, want map[string]bool, start, end time.Time) ([]domain.FuturesBar, error) {
	if p.breaker != nil && !p.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: taifex futures bars circuit breaker open", ErrUpstream)
	}
	if err := WaitForLimiter(ctx, p.rateLimiter); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	form := url.Values{}
	form.Set("down_type", "1") // 1 = 期貨
	form.Set("commodity_id", futuresCSVCommodityID(want))
	form.Set("queryStartDate", start.Format("2006/01/02"))
	form.Set("queryEndDate", end.Format("2006/01/02"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.csvURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/csv,text/html;q=0.9,*/*;q=0.8")

	resp, err := fetchWithRetry(ctx, p.client, req, p.retryCfg)
	if err != nil {
		p.fail(futuresBarsSourceCSV, err)
		return nil, fmt.Errorf("taifex futures csv http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readTAIFEXBody(resp)
	if err != nil {
		p.fail(futuresBarsSourceCSV, err)
		return nil, fmt.Errorf("read taifex futures csv body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("%w: taifex futures csv api error: status %d", ErrUpstream, resp.StatusCode)
		p.fail(futuresBarsSourceCSV, err)
		return nil, err
	}

	decoded, err := decodeTAIFEXCSVBody(body, resp.Header.Get("Content-Type"))
	if err != nil {
		p.fail(futuresBarsSourceCSV, err)
		return nil, err
	}

	bars, err := ParseFuturesCSVBars(bytes.NewReader(decoded), want)
	if err != nil {
		p.fail(futuresBarsSourceCSV, err)
		return nil, err
	}
	// 表頭哨兵通過、但 0 資料列 ⇒ 例假或契約未上市：ErrNoData，不記 breaker 失敗。
	if len(bars) == 0 {
		p.observe(futuresBarsSourceCSV, nil, 0)
		p.breakerRecordSuccess()
		return nil, fmt.Errorf("%w: no futures rows in %s..%s", ErrNoData, start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
	p.observe(futuresBarsSourceCSV, nil, len(bars))
	p.breakerRecordSuccess()
	return bars, nil
}

// futuresCSVCommodityID 回傳官網表單的 commodity_id。
//
// 實測：空字串會回 404（HTML）；"all" 有效但單月約 4.3MB。因此單一契約直接帶代碼，
// 其餘情況用 "all" 再於解析階段過濾。
func futuresCSVCommodityID(want map[string]bool) string {
	if len(want) == 1 {
		for code := range want {
			return code
		}
	}
	return "all"
}

func contractsSet(contracts []string) map[string]bool {
	if len(contracts) == 0 {
		return nil
	}
	out := make(map[string]bool, len(contracts))
	for _, c := range contracts {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" {
			out[c] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ChunkFuturesDateRange 把 [start, end] 切成若干段，每段起訖相差 ≤ 31 天
// （實測上限，見規格 §1.4）。切法以日曆月為單位，確保任何月份都不會超限。
//
// 回傳值為 [][2]time.Time（含頭含尾、遞增、不重疊）。
func ChunkFuturesDateRange(start, end time.Time) [][2]time.Time {
	start = dayFloor(start)
	end = dayFloor(end)
	if end.Before(start) {
		return nil
	}
	var chunks [][2]time.Time
	cur := start
	for !cur.After(end) {
		// 本段上限：cur + 31 天，且不超過月底。
		monthEnd := time.Date(cur.Year(), cur.Month()+1, 1, 0, 0, 0, 0, cur.Location()).AddDate(0, 0, -1)
		limit := cur.AddDate(0, 0, futuresBarsMaxRangeDays)
		if monthEnd.Before(limit) {
			limit = monthEnd
		}
		if limit.After(end) {
			limit = end
		}
		chunks = append(chunks, [2]time.Time{cur, limit})
		cur = limit.AddDate(0, 0, 1)
	}
	return chunks
}

func dayFloor(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// decodeTAIFEXCSVBody 依 Content-Type 的 charset 把官網 CSV body 轉成 UTF-8。
//
// 官網回 `text/html;charset=MS950`（Big5 系）；不轉碼會讓 `契約`/`交易時段` 等
// 中文欄位變亂碼，表頭哨兵與欄位對映都會失敗。
func decodeTAIFEXCSVBody(body []byte, contentType string) ([]byte, error) {
	charset := charsetFromContentType(contentType)
	if isUTF8(charset) {
		return body, nil
	}
	var enc encoding.Encoding
	if alias, ok := taifexCharsetAliases[strings.ToLower(charset)]; ok {
		enc = alias
	} else {
		var err error
		enc, err = htmlindex.Get(charset)
		if err != nil {
			return nil, fmt.Errorf("%w: unknown charset %q in Content-Type %q", ErrTAIFEXFuturesBarsSchema, charset, contentType)
		}
	}
	out, err := io.ReadAll(transform.NewReader(bytes.NewReader(body), enc.NewDecoder()))
	if err != nil {
		return nil, fmt.Errorf("%w: transcode %s csv: %v", ErrTAIFEXFuturesBarsSchema, charset, err)
	}
	return out, nil
}

// ParseFuturesCSVBars 解析官網 `futDataDown` 的 CSV。
//
// 規則（規格 §1.3／§2）：
//  1. **表頭哨兵**：首列必須以「交易日期」開頭；否則回 ErrTAIFEXFuturesBarsSchema
//     （HTML/JS 錯誤頁、轉址頁都走這條，**不得**靜默回空）。
//  2. 必要欄位逐一比對（表頭驅動對映，不硬編索引）。
//  3. 表頭通過但 0 資料列 ⇒ 回空 slice + 不報錯；由呼叫端轉成 ErrNoData。
//  4. 到期月含 "/" 者為價差組合 ⇒ 丟棄；want 非空時只保留 want 內的契約。
//  5. 缺值（"-"/"NULL"/空）⇒ nil；不可解析的非空值 ⇒ ErrTAIFEXFuturesBarsSchema。
func ParseFuturesCSVBars(r io.Reader, want map[string]bool) ([]domain.FuturesBar, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrTAIFEXFuturesBarsSchema)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	text := string(stripBOM(raw))
	if err := verifyFuturesCSVHeader(text); err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: read csv header: %v", ErrTAIFEXFuturesBarsSchema, err)
	}
	idx, err := futuresCSVColumnIndex(header)
	if err != nil {
		return nil, err
	}
	// 必要的**最大欄位索引**：先前只比對「欄數 >= 必要欄位個數」，但表頭驅動對映下
	// 真正的邊界是最後一個必要欄位的位置（例：交易時段在第 18 欄）。欄數不足時
	// 必須回 ErrSchema，不可讓索引越界 panic。
	maxIdx := 0
	for _, col := range futuresCSVRequiredColumns {
		if i := idx[col]; i > maxIdx {
			maxIdx = i
		}
	}

	var bars []domain.FuturesBar
	lineNo := 1
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		lineNo++
		if err != nil {
			return nil, fmt.Errorf("%w: csv line %d: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
		}
		if len(rec) <= maxIdx {
			return nil, fmt.Errorf("%w: csv line %d has %d fields, want > %d (last required column %q)", ErrTAIFEXFuturesBarsSchema, lineNo, len(rec), maxIdx, futuresCSVRequiredColumns[len(futuresCSVRequiredColumns)-1])
		}
		bar, ok, err := futuresBarFromCSVRecord(rec, idx, want, lineNo)
		if err != nil {
			return nil, err
		}
		if ok {
			bars = append(bars, bar)
		}
	}
	return bars, nil
}

// verifyFuturesCSVHeader 是表頭哨兵：body 首列必須以「交易日期」開頭。
func verifyFuturesCSVHeader(text string) error {
	first := text
	if i := strings.IndexAny(first, "\r\n"); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, futuresCSVHeaderSentinel) {
		preview := first
		if len(preview) > 120 {
			preview = preview[:120]
		}
		return fmt.Errorf("%w: taifex futures csv header sentinel missing (first line %q) — upstream returned a non-CSV body (HTML/JS error page) despite HTTP 200", ErrTAIFEXFuturesBarsSchema, preview)
	}
	return nil
}

// futuresCSVColumnIndex 以表頭名稱建立欄位索引；缺任一必要欄位即 ErrSchema。
func futuresCSVColumnIndex(header []string) (map[string]int, error) {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	var missing []string
	for _, want := range futuresCSVRequiredColumns {
		if _, ok := idx[want]; !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: taifex futures csv missing required columns %v (header=%v)", ErrTAIFEXFuturesBarsSchema, missing, header)
	}
	return idx, nil
}

func futuresBarFromCSVRecord(rec []string, idx map[string]int, want map[string]bool, lineNo int) (domain.FuturesBar, bool, error) {
	get := func(col string) string { return strings.TrimSpace(rec[idx[col]]) }

	contract := strings.ToUpper(get("契約"))
	if len(want) > 0 && !want[contract] {
		return domain.FuturesBar{}, false, nil
	}
	month := get("到期月份(週別)")
	// 價差組合（例 202706/202709）不是單一契約的行情列 —— 丟棄。
	if strings.Contains(month, "/") {
		return domain.FuturesBar{}, false, nil
	}

	date, err := parseFuturesDate(get("交易日期"), "2006/01/02")
	if err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}

	bar := domain.FuturesBar{
		Contract:      contract,
		ContractMonth: month,
		TradeDate:     date,
		Session:       futuresSessionFromTAIFEX(get("交易時段")),
		Source:        futuresBarsSourceCSV,
	}
	if bar.Open, err = parseNullableFloat(get("開盤價")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d open: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	if bar.High, err = parseNullableFloat(get("最高價")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d high: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	if bar.Low, err = parseNullableFloat(get("最低價")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d low: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	if bar.Close, err = parseNullableFloat(get("收盤價")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d close: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	if bar.SettlementPrice, err = parseNullableFloat(get("結算價")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d settlement: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	if bar.Volume, err = parseNullableInt(get("成交量")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d volume: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	if bar.OpenInterest, err = parseNullableInt(get("未沖銷契約數")); err != nil {
		return domain.FuturesBar{}, false, fmt.Errorf("%w: csv line %d open interest: %v", ErrTAIFEXFuturesBarsSchema, lineNo, err)
	}
	return bar, true, nil
}

// taifexFuturesBarRaw 是 OpenAPI `/DailyMarketReportFut` 的一列
// （JSON key 原文，含括號）。
type taifexFuturesBarRaw struct {
	Date            string `json:"Date"`
	Contract        string `json:"Contract"`
	ContractMonth   string `json:"ContractMonth(Week)"`
	Open            string `json:"Open"`
	High            string `json:"High"`
	Low             string `json:"Low"`
	Last            string `json:"Last"`
	Volume          string `json:"Volume"`
	SettlementPrice string `json:"SettlementPrice"`
	OpenInterest    string `json:"OpenInterest"`
	TradingSession  string `json:"TradingSession"`
}

// ParseFuturesOpenAPIBars 解析 OpenAPI `/DailyMarketReportFut` 的 JSON。
//
// 欄位比 CSV 少（無價差組合欄），但語意相同：缺值以 "-"/"NULL"/"" 表示 ⇒ nil。
func ParseFuturesOpenAPIBars(body []byte, want map[string]bool) ([]domain.FuturesBar, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("%w: empty openapi body", ErrTAIFEXFuturesBarsSchema)
	}
	var raw []taifexFuturesBarRaw
	if err := DecodeJSON(bytes.NewReader(body), "application/json; charset=utf-8", &raw); err != nil {
		return nil, fmt.Errorf("%w: decode openapi json: %v", ErrTAIFEXFuturesBarsSchema, err)
	}

	bars := make([]domain.FuturesBar, 0, len(raw))
	for i := range raw {
		r := &raw[i]
		contract := strings.ToUpper(strings.TrimSpace(r.Contract))
		if len(want) > 0 && !want[contract] {
			continue
		}
		month := strings.TrimSpace(r.ContractMonth)
		if strings.Contains(month, "/") {
			continue
		}
		date, err := parseFuturesDate(strings.TrimSpace(r.Date), "20060102")
		if err != nil {
			return nil, fmt.Errorf("%w: openapi row %d: %v", ErrTAIFEXFuturesBarsSchema, i, err)
		}
		bar := domain.FuturesBar{
			Contract:      contract,
			ContractMonth: month,
			TradeDate:     date,
			Session:       futuresSessionFromTAIFEX(strings.TrimSpace(r.TradingSession)),
			Source:        futuresBarsSourceOpenAPI,
		}
		var err2 error
		if bar.Open, err2 = parseNullableFloat(r.Open); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d open: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		if bar.High, err2 = parseNullableFloat(r.High); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d high: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		if bar.Low, err2 = parseNullableFloat(r.Low); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d low: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		if bar.Close, err2 = parseNullableFloat(r.Last); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d last: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		if bar.SettlementPrice, err2 = parseNullableFloat(r.SettlementPrice); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d settlement: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		if bar.Volume, err2 = parseNullableInt(r.Volume); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d volume: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		if bar.OpenInterest, err2 = parseNullableInt(r.OpenInterest); err2 != nil {
			return nil, fmt.Errorf("%w: openapi row %d open interest: %v", ErrTAIFEXFuturesBarsSchema, i, err2)
		}
		bars = append(bars, bar)
	}
	return bars, nil
}

// futuresSessionFromTAIFEX 把來源的時段字串正規化。
// 未知值一律 SessionUnknown（不猜測、不靜默歸類為一般）。
func futuresSessionFromTAIFEX(s string) domain.FuturesSession {
	switch strings.TrimSpace(s) {
	case "一般":
		return domain.SessionRegular
	case "盤後":
		return domain.SessionAfterHours
	default:
		return domain.SessionUnknown
	}
}

// futuresMissingValues 是上游表示「沒有值」的字面值。
var futuresMissingValues = map[string]bool{"": true, "-": true, "NULL": true, "null": true, "n/a": true, "N/A": true}

// parseNullableFloat 解析可為缺值的浮點欄位。
// 缺值 ⇒ (nil, nil)；非空但不可解析 ⇒ error（schema 變了，不可吞成 0）。
func parseNullableFloat(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if futuresMissingValues[s] {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("value %q is not a number", s)
	}
	return &v, nil
}

// parseNullableInt 解析可為缺值的整數欄位（成交量／未沖銷契約數）。
func parseNullableInt(s string) (*int64, error) {
	s = strings.TrimSpace(s)
	if futuresMissingValues[s] {
		return nil, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("value %q is not an integer", s)
	}
	return &v, nil
}

// parseFuturesDate 解析 Asia/Taipei 日曆日（時間歸零）。
func parseFuturesDate(s, layout string) (time.Time, error) {
	t, err := time.ParseInLocation(layout, s, TaiwanLocation())
	if err != nil {
		return time.Time{}, fmt.Errorf("date %q does not match %s", s, layout)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, TaiwanLocation()), nil
}
