package marketdata

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// TWSECalendarProvider fetches Taiwan market calendar events from TWSE OpenAPI.
// Covers ex-dividend dates (exRight) and shareholder meetings (meeting).
//
// Maturity: evolving
type TWSECalendarProvider struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSECalendarProvider creates a new TWSE calendar event provider.
func NewTWSECalendarProvider() *TWSECalendarProvider {
	params := config.GetParametersConfig()
	return &TWSECalendarProvider{
		httpClient:  httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
	}
}

// SetHTTPClient sets a custom HTTP client (for testing).
func (p *TWSECalendarProvider) SetHTTPClient(client *http.Client) {
	p.httpClient = client
}

// Name returns the provider name.
func (p *TWSECalendarProvider) Name() string {
	return "twse_calendar"
}
// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSECalendarProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.rateLimiter = l
	}
}


// FetchEvents fetches calendar events for the given year from TWSE.
// Returns ex-dividend dates and shareholder meetings.
func (p *TWSECalendarProvider) FetchEvents(ctx context.Context, year int) ([]CalendarProviderData, error) {
	var allEvents []CalendarProviderData
	var errs []string

	// Fetch ex-dividend/ex-right dates for each month
	dividendEvents, dividendErrs := p.fetchExDividendYear(ctx, year)
	if dividendErrs != nil {
		errs = append(errs, fmt.Sprintf("ex_dividend: %v", dividendErrs))
	}
	allEvents = append(allEvents, dividendEvents...)

	// Fetch shareholder meetings for each month
	meetingEvents, meetingErrs := p.fetchShareholderMeetingsYear(ctx, year)
	if meetingErrs != nil {
		errs = append(errs, fmt.Sprintf("shareholder_meetings: %v", meetingErrs))
	}
	allEvents = append(allEvents, meetingEvents...)

	if len(allEvents) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("twse_calendar: all fetches failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		logging.Warn(
			"twse_calendar", "partial_failure",
			logging.FStr("errors", strings.Join(errs, "; ")),
		)
	}
	return allEvents, nil
}

// fetchExDividendYear fetches ex-dividend data for all months in the given year.
func (p *TWSECalendarProvider) fetchExDividendYear(ctx context.Context, year int) ([]CalendarProviderData, error) {
	var all []CalendarProviderData
	var errs []error
	for month := 1; month <= 12; month++ {
		dateStr := fmt.Sprintf("%d%02d01", year, month)
		events, err := p.fetchExDividendMonth(ctx, dateStr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, events...)
	}
	if len(errs) == 12 {
		return all, fmt.Errorf("all 12 months failed: %w", errs[0])
	}
	return all, nil
}

// fetchShareholderMeetingsYear fetches shareholder meeting data for all months.
func (p *TWSECalendarProvider) fetchShareholderMeetingsYear(ctx context.Context, year int) ([]CalendarProviderData, error) {
	var all []CalendarProviderData
	var errs []error
	for month := 1; month <= 12; month++ {
		dateStr := fmt.Sprintf("%d%02d01", year, month)
		events, err := p.fetchMeetingMonth(ctx, dateStr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, events...)
	}
	if len(errs) == 12 {
		return all, fmt.Errorf("all 12 months failed: %w", errs[0])
	}
	return all, nil
}

// ---------------------------------------------------------------------------
// TWSE API response types
// ---------------------------------------------------------------------------

type twseCalendarResponse struct {
	Stat   string     `json:"stat"`
	Date   string     `json:"date"`
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Total  int        `json:"total"`
}

// ---------------------------------------------------------------------------
// Ex-dividend / Ex-right
// ---------------------------------------------------------------------------

// fetchExDividendMonth fetches ex-dividend dates for a given year-month.
// TWSE endpoint: /rwd/zh/exRight?date=YYYYMMDD&response=json
//
// Fields typically: ["股票代號", "股票名稱", "除權息日期", "種類",
//
//	"除權息前收盤價", "除權息參考價", "權值", "息值", "權值+息值",
//	"漲停價", "跌停價", "開始交易日期", "現金股利發放日"]
func (p *TWSECalendarProvider) fetchExDividendMonth(ctx context.Context, dateStr string) ([]CalendarProviderData, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
	}

	url := fmt.Sprintf("%s/rwd/zh/exRight?date=%s&response=json", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	// TWSE calendar API deprecation (2026-06): exRight endpoint 回 HTML body
	// (302 → /page-not-found.html)。偵測 HTML 並優雅降級回空 events。
	if isHTMLContentType(resp.Header.Get("Content-Type")) {
		logging.Warn(
			"twse_calendar", "endpoint_html_response_deprecated",
			logging.FStr("endpoint", "exRight"),
			logging.FStr("date", dateStr),
		)
		return nil, nil
	}

	var apiResp twseCalendarResponse
	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Stat != "OK" {
		// Empty months return stat "OK" but with no/few data rows — this is normal.
		if apiResp.Stat == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("TWSE API returned stat=%s", apiResp.Stat)
	}

	// Map field names to column indices
	colIdx := make(map[string]int)
	for i, f := range apiResp.Fields {
		colIdx[f] = i
	}

	// Find the critical columns: stock code (股票代號), stock name (股票名稱),
	// ex-date (除權息日期), dividend amount (息值), stock dividend (權值)
	codeCol := colIdx["股票代號"]
	nameCol := colIdx["股票名稱"]
	dateCol := colIdx["除權息日期"]

	var events []CalendarProviderData
	for _, row := range apiResp.Data {
		if len(row) <= maxInt(codeCol, nameCol, dateCol) {
			continue
		}
		code := rowAt(row, codeCol, "")
		name := rowAt(row, nameCol, "")
		exDate := rowAt(row, dateCol, "")
		if code == "" || exDate == "" {
			continue
		}

		// Convert TWSE date format (YYYYMMDD or "民國年/月/日") to ISO
		isoDate := normalizeTWDate(exDate)
		if isoDate == "" {
			continue
		}

		// Determine weight based on dividend type
		kindCol := colIdx["種類"]
		kind := rowAt(row, kindCol, "")
		weight := 0.3
		desc := fmt.Sprintf("%s(%s) 除權息日 %s", name, code, isoDate)
		if strings.Contains(kind, "除息") || strings.Contains(kind, "現金") {
			weight = 0.4
			desc = fmt.Sprintf("%s(%s) 現金除息 %s", name, code, isoDate)
		} else if strings.Contains(kind, "除權") {
			weight = 0.35
			desc = fmt.Sprintf("%s(%s) 股票除權 %s", name, code, isoDate)
		}

		events = append(events, CalendarProviderData{
			Date:        isoDate,
			EventType:   "ex_dividend",
			Name:        fmt.Sprintf("%s 除權息", name),
			Symbol:      code,
			Direction:   "mixed",
			Weight:      weight,
			Description: desc,
			Source:      "twse",
		})
	}
	return events, nil
}

// ---------------------------------------------------------------------------
// Shareholder meetings
// ---------------------------------------------------------------------------

// fetchMeetingMonth fetches shareholder meeting dates for a given year-month.
// TWSE endpoint: /rwd/zh/meeting?date=YYYYMMDD&response=json
//
// Fields typically: ["公司代號", "公司名稱", "股東會日期", "最後過戶日",
//
//	"停止過戶起始日期", "停止過戶截止日期", "紀念品代號", "紀念品名稱",
//	"開會地點", "備註", "是否發放紀念品"]
func (p *TWSECalendarProvider) fetchMeetingMonth(ctx context.Context, dateStr string) ([]CalendarProviderData, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
	}

	url := fmt.Sprintf("%s/rwd/zh/meeting?date=%s&response=json", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	// TWSE calendar API deprecation (2026-06): meeting endpoint 同樣 deprecated。
	if isHTMLContentType(resp.Header.Get("Content-Type")) {
		logging.Warn(
			"twse_calendar", "endpoint_html_response_deprecated",
			logging.FStr("endpoint", "meeting"),
			logging.FStr("date", dateStr),
		)
		return nil, nil
	}

	var apiResp twseCalendarResponse
	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Stat != "OK" && apiResp.Stat != "" {
		return nil, fmt.Errorf("TWSE API returned stat=%s", apiResp.Stat)
	}

	// Map field names to column indices
	colIdx := make(map[string]int)
	for i, f := range apiResp.Fields {
		colIdx[f] = i
	}

	codeCol := colIdx["公司代號"]
	nameCol := colIdx["公司名稱"]
	dateCol := colIdx["股東會日期"]

	var events []CalendarProviderData
	for _, row := range apiResp.Data {
		if len(row) <= maxInt(codeCol, nameCol, dateCol) {
			continue
		}
		code := rowAt(row, codeCol, "")
		name := rowAt(row, nameCol, "")
		meetingDate := rowAt(row, dateCol, "")
		if code == "" || meetingDate == "" {
			continue
		}

		// Convert TWSE date format to ISO
		isoDate := normalizeTWDate(meetingDate)
		if isoDate == "" {
			continue
		}

		events = append(events, CalendarProviderData{
			Date:        isoDate,
			EventType:   "shareholder_meeting",
			Name:        fmt.Sprintf("%s 股東會", name),
			Symbol:      code,
			Direction:   "bullish",
			Weight:      0.25,
			Description: fmt.Sprintf("%s(%s) 股東會 %s", name, code, isoDate),
			Source:      "twse",
		})
	}
	return events, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isHTMLContentType reports whether the Content-Type header indicates an
// HTML response (vs. JSON). Used to detect TWSE calendar API deprecation:
// as of 2026-06, exRight and meeting endpoints return HTML (302 redirect
// to /page-not-found.html) instead of JSON. Callers should treat HTML
// responses as graceful empty results rather than propagating JSON
// decode errors downstream.
//
// Case-insensitive match per RFC 7231 §3.1.1.1 (media type is case-insensitive).
func isHTMLContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.EqualFold(mediaType, "text/html")
}

// normalizeTWDate converts TWSE date formats to ISO 8601 (2006-01-02).
// TWSE uses multiple formats:
//   - "20260513" (YYYYMMDD) → "2026-05-13"
//   - "115/05/13" (民國年/月/日) → "2026-05-13"
func normalizeTWDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// YYYYMMDD
	if len(raw) == 8 {
		return raw[0:4] + "-" + raw[4:6] + "-" + raw[6:8]
	}

	// 民國年/月/日 (e.g., "115/05/13")
	if strings.Count(raw, "/") == 2 {
		parts := strings.Split(raw, "/")
		if len(parts) != 3 {
			return ""
		}
		rocYear := parseTSWESafe(parts[0])
		month := parseTSWESafe(parts[1])
		day := parseTSWESafe(parts[2])
		if rocYear == 0 {
			return ""
		}
		gregYear := rocYear + 1911
		return fmt.Sprintf("%04d-%02d-%02d", gregYear, month, day)
	}

	return ""
}

// parseTSWESafe parses a string to int, returning 0 on failure.
func parseTSWESafe(s string) int {
	s = strings.TrimSpace(s)
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// maxInt returns the maximum of the given ints.
func maxInt(vals ...int) int {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
