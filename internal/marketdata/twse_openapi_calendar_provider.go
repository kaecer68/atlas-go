package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// TWSEOpenAPICalendarProvider fetches Taiwan market calendar events from the
// TWSE OpenAPI v1 endpoints (openapi.twse.com.tw), the working successors to
// the deprecated rwd/zh/{exRight,meeting} endpoints (deprecated 2026-06:
// both return 302 → /page-not-found.html for every year).
//
// Endpoints used (verified 2026-08):
//   - GET /v1/exchangeReport/TWT48U_ALL — 上市股票除權除息預告表
//     (ex-dividend / ex-right pre-announcement table; Date field is ROC YYYYMMDD)
//   - GET /v1/opendata/t187ap41_L       — 上市公司召開股東常(臨時)會日期
//     地點及採用電子投票情形等資料彙總表 (shareholder meeting dates)
//
// Known limitation: OpenAPI v1 serves the current snapshot only. FetchEvents
// for a year other than the current year returns an empty slice with a warn
// log. Historical years must come from curated/static sources (see
// cmd/backfill-event-calendar and MSCIRebalanceCalendarProvider).
//
// Maturity: evolving
type TWSEOpenAPICalendarProvider struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
	now         func() time.Time
}

// NewTWSEOpenAPICalendarProvider creates a provider backed by TWSE OpenAPI v1.
func NewTWSEOpenAPICalendarProvider() *TWSEOpenAPICalendarProvider {
	params := config.GetParametersConfig()
	return &TWSEOpenAPICalendarProvider{
		httpClient:  httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second),
		baseURL:     "https://openapi.twse.com.tw/v1",
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
		now:         time.Now,
	}
}

// SetHTTPClient sets a custom HTTP client (for testing).
func (p *TWSEOpenAPICalendarProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSEOpenAPICalendarProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.rateLimiter = l
	}
}

// SetNow overrides the clock (tests only).
func (p *TWSEOpenAPICalendarProvider) SetNow(now func() time.Time) {
	if now != nil {
		p.now = now
	}
}

// Name returns the provider name.
func (p *TWSEOpenAPICalendarProvider) Name() string {
	return "twse_openapi"
}

// FetchEvents fetches calendar events for the given year from TWSE OpenAPI.
// Only the current year is served by OpenAPI v1 snapshots; other years are
// skipped with a warn (documented limitation — historical backfill uses
// curated/static sources instead).
func (p *TWSEOpenAPICalendarProvider) FetchEvents(ctx context.Context, year int) ([]CalendarProviderData, error) {
	if year != p.now().Year() {
		logging.Warn(
			"twse_openapi_calendar", "year_not_served",
			logging.FInt("requested_year", year),
			logging.FInt("current_year", p.now().Year()),
			logging.FStr("hint", "TWSE OpenAPI v1 serves the current snapshot only; use cmd/backfill-event-calendar static/curated sources for historical years"),
		)
		return nil, nil
	}

	var allEvents []CalendarProviderData
	var errs []string

	dividendEvents, dividendErr := p.fetchExDividend(ctx, year)
	if dividendErr != nil {
		errs = append(errs, fmt.Sprintf("ex_dividend: %v", dividendErr))
	}
	allEvents = append(allEvents, dividendEvents...)

	meetingEvents, meetingErr := p.fetchShareholderMeetings(ctx, year)
	if meetingErr != nil {
		errs = append(errs, fmt.Sprintf("shareholder_meetings: %v", meetingErr))
	}
	allEvents = append(allEvents, meetingEvents...)

	if len(allEvents) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("twse_openapi_calendar: all fetches failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		logging.Warn(
			"twse_openapi_calendar", "partial_failure",
			logging.FStr("errors", strings.Join(errs, "; ")),
		)
	}
	return allEvents, nil
}

// ---------------------------------------------------------------------------
// TWSE OpenAPI v1 response types
// ---------------------------------------------------------------------------

// twseOpenAPIExRightRow is one row of TWT48U_ALL (除權除息預告表).
// Dates are ROC-format YYYYMMDD (e.g. "1150907" = 2026-09-07).
type twseOpenAPIExRightRow struct {
	Date               string `json:"Date"`
	Code               string `json:"Code"`
	Name               string `json:"Name"`
	Exdividend         string `json:"Exdividend"` // 息 / 權 / 權息
	StockDividendRatio string `json:"StockDividendRatio"`
	CashDividend       string `json:"CashDividend"`
}

// twseOpenAPIMeetingRow is one row of t187ap41_L (股東會日期彙總表).
// 開會日期 is ROC-format YYYYMMDD (e.g. "1150522" = 2026-05-22).
type twseOpenAPIMeetingRow struct {
	CompanyCode string `json:"公司代號"`
	CompanyName string `json:"公司名稱"`
	MeetingDate string `json:"開會日期"`
	MeetingType string `json:"股東常(臨時)會"` // 常會 / 臨時會
}

// ---------------------------------------------------------------------------
// Ex-dividend / ex-right (TWT48U_ALL)
// ---------------------------------------------------------------------------

func (p *TWSEOpenAPICalendarProvider) fetchExDividend(ctx context.Context, year int) ([]CalendarProviderData, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
	}

	url := p.baseURL + "/exchangeReport/TWT48U_ALL"
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
	if isHTMLContentType(resp.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("TWT48U_ALL returned HTML (endpoint changed?)")
	}

	var rows []twseOpenAPIExRightRow
	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &rows); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var events []CalendarProviderData
	for _, row := range rows {
		isoDate := normalizeROCDate(row.Date)
		if isoDate == "" {
			continue
		}
		if !strings.HasPrefix(isoDate, fmt.Sprintf("%d-", year)) {
			continue
		}
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)
		if code == "" || name == "" {
			continue
		}

		weight := 0.3
		kind := strings.TrimSpace(row.Exdividend)
		desc := fmt.Sprintf("%s(%s) 除權息 %s", name, code, isoDate)
		if strings.Contains(kind, "息") && !strings.Contains(kind, "權") {
			weight = 0.4
			desc = fmt.Sprintf("%s(%s) 現金除息 %s", name, code, isoDate)
		} else if strings.Contains(kind, "權") {
			weight = 0.35
			desc = fmt.Sprintf("%s(%s) 除權 %s", name, code, isoDate)
		}

		events = append(events, CalendarProviderData{
			Date:        isoDate,
			EventType:   "ex_dividend",
			Name:        fmt.Sprintf("%s 除權息", name),
			Symbol:      code,
			Direction:   "mixed",
			Weight:      weight,
			Description: desc,
			Source:      "twse_openapi",
		})
	}
	return events, nil
}

// ---------------------------------------------------------------------------
// Shareholder meetings (t187ap41_L)
// ---------------------------------------------------------------------------

func (p *TWSEOpenAPICalendarProvider) fetchShareholderMeetings(ctx context.Context, year int) ([]CalendarProviderData, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
	}

	url := p.baseURL + "/opendata/t187ap41_L"
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
	if isHTMLContentType(resp.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("t187ap41_L returned HTML (endpoint changed?)")
	}

	var rows []twseOpenAPIMeetingRow
	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &rows); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var events []CalendarProviderData
	for _, row := range rows {
		isoDate := normalizeROCDate(row.MeetingDate)
		if isoDate == "" {
			continue
		}
		if !strings.HasPrefix(isoDate, fmt.Sprintf("%d-", year)) {
			continue
		}
		code := strings.TrimSpace(row.CompanyCode)
		name := strings.TrimSpace(row.CompanyName)
		if code == "" || name == "" {
			continue
		}

		events = append(events, CalendarProviderData{
			Date:        isoDate,
			EventType:   "shareholder_meeting",
			Name:        fmt.Sprintf("%s 股東會", name),
			Symbol:      code,
			Direction:   "bullish",
			Weight:      0.25,
			Description: fmt.Sprintf("%s(%s) 股東%s %s", name, code, strings.TrimSpace(row.MeetingType), isoDate),
			Source:      "twse_openapi",
		})
	}
	return events, nil
}

// normalizeROCDate converts a ROC (民國) 7-digit compact date "YYYMMDD" to ISO
// "YYYY-MM-DD". TWSE OpenAPI tables (TWT48U_ALL, t187ap41_L, …) use this
// format, e.g. "1150907" → "2026-09-07". Returns "" for malformed input.
func normalizeROCDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) != 7 {
		return ""
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return ""
		}
	}
	rocYear := parseTSWESafe(raw[0:3])
	month := parseTSWESafe(raw[3:5])
	day := parseTSWESafe(raw[5:7])
	if rocYear == 0 || month < 1 || month > 12 || day < 1 || day > 31 {
		return ""
	}
	gregYear := rocYear + 1911
	return fmt.Sprintf("%04d-%02d-%02d", gregYear, month, day)
}
