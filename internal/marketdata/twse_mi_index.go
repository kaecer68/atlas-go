package marketdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ─── MI_INDEX (www.twse.com.tw/exchangeReport/MI_INDEX) ─────────────────────
//
// STOCK_DAY_ALL (GetQuotes) answers with "today's" snapshot and accepts NO
// date parameter: on a closed market it simply replays the previous trading
// day's rows. Any caller that stamps the payload with time.Now() therefore
// writes phantom rows for weekends and holidays (2026-09-26 incident: the
// replay CSV held 1150924 rows labelled 2026-09-26, and the CSV's date column
// IS the replay trading calendar, so the phantom dates poisoned
// NextTradingSession, ForwardReturn's duplicate-row guard and the twse_replay
// freshness reading).
//
// MI_INDEX is date-addressed and honest. Measured 2026-09-26 against the live
// endpoint:
//
//	date=20260924 (Thu, trading) → stat="OK", 1,380 rows in the 每日收盤行情
//	    section, a superset of all 44 replay symbols (0 missing)
//	date=20260925 (Fri, 中秋節)  → {"stat":"很抱歉，沒有符合條件的資料!"}
//	date=20260926 (Sat)         → {"stat":"很抱歉，沒有符合條件的資料!"}
//
// So ONE request covers the whole replay universe AND states which trading day
// the payload describes. The date must then come from the payload, never from
// the clock.
//
// Envelope note: MI_INDEX wraps its payload in `tables`, unlike the legacy
// STOCK_DAY_ALL `fields`/`data` shape. Decoding it as fields/data yields zero
// rows with NO error — the reason cmd/fetch-historical silently fetched 0 rows
// every day while exiting 0.

// twseMIIndexResponse is the MI_INDEX JSON envelope. `stat` is Chinese prose on a
// closed market ("很抱歉，沒有符合條件的資料!"), not an error code, so callers
// must read a non-"OK" stat as "nothing traded that day", never as a fetch
// failure.
type twseMIIndexResponse struct {
	Stat   string        `json:"stat"`
	Date   string        `json:"date"`
	Tables []twseMITable `json:"tables"`
}

// twseMITable is one section of the report (價格指數, 大盤統計資訊,
// 每日收盤行情, …). Only 每日收盤行情 carries per-symbol OHLCV.
type twseMITable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
}

// Column names of the 每日收盤行情 section (measured 2026-09-24).
const (
	miIndexFieldCode       = "證券代號"
	miIndexFieldName       = "證券名稱"
	miIndexFieldVolume     = "成交股數"
	miIndexFieldTradeValue = "成交金額"
	miIndexFieldOpen       = "開盤價"
	miIndexFieldHigh       = "最高價"
	miIndexFieldLow        = "最低價"
	miIndexFieldClose      = "收盤價"
	miIndexFieldChange     = "漲跌價差"
	miIndexFieldTxCount    = "成交筆數"
)

// DailyQuotesTable returns the 每日收盤行情 section: the only one holding
// per-symbol OHLCV. Selection is by FIELD NAMES, not by the localised title or
// a positional index, so a title rewording or a new section upstream cannot
// silently hand back indices instead of stocks (contrast
// MarketVolumeProvider, which still indexes Tables[6] positionally).
func (r twseMIIndexResponse) DailyQuotesTable() (twseMITable, bool) {
	for _, t := range r.Tables {
		if t.hasFields(miIndexFieldCode, miIndexFieldClose) {
			return t, true
		}
	}
	return twseMITable{}, false
}

// hasFields reports whether the table's header carries every wanted column.
func (t twseMITable) hasFields(want ...string) bool {
	for _, w := range want {
		found := false
		for _, f := range t.Fields {
			if strings.TrimSpace(f) == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// miIndexTitleDateRE matches the ROC date TWSE prefixes onto a table title,
// e.g. "115年09月24日 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))".
var miIndexTitleDateRE = regexp.MustCompile(`(\d{2,3})年(\d{1,2})月(\d{1,2})日`)

// TitleDate returns YYYY-MM-DD for the trading date the table itself claims to
// describe. This is the payload's OWN provenance: unlike the envelope's `date`
// (which an always-latest upstream would simply echo from our request), the
// title is generated server-side from the data it is returning.
func (t twseMITable) TitleDate() (string, bool) {
	m := miIndexTitleDateRE.FindStringSubmatch(t.Title)
	if m == nil {
		return "", false
	}
	year, err1 := strconv.Atoi(m[1])
	month, err2 := strconv.Atoi(m[2])
	day, err3 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return "", false
	}
	d := time.Date(year+1911, time.Month(month), day, 12, 0, 0, 0, time.UTC)
	if int(d.Month()) != month || d.Day() != day {
		// e.g. 月=13 or 日=32 — reject instead of normalising into another day.
		return "", false
	}
	return d.Format("2006-01-02"), true
}

// QuoteRows maps the table onto TWSEQuote using ITS OWN header, so columns
// added or reordered upstream cannot shift the mapping (the fixed-index
// parsers elsewhere in this package would silently mis-map). Rows without a
// 證券代號 are skipped.
func (t twseMITable) QuoteRows() []TWSEQuote {
	idx := make(map[string]int, len(t.Fields))
	for i, f := range t.Fields {
		idx[strings.TrimSpace(f)] = i
	}
	cell := func(row []string, field string) string {
		i, ok := idx[field]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	out := make([]TWSEQuote, 0, len(t.Data))
	for _, row := range t.Data {
		code := cell(row, miIndexFieldCode)
		if code == "" {
			continue
		}
		out = append(out, TWSEQuote{
			Code:         code,
			Name:         cell(row, miIndexFieldName),
			TradeVolume:  cell(row, miIndexFieldVolume),
			TradeValue:   cell(row, miIndexFieldTradeValue),
			OpeningPrice: cell(row, miIndexFieldOpen),
			HighestPrice: cell(row, miIndexFieldHigh),
			LowestPrice:  cell(row, miIndexFieldLow),
			ClosingPrice: cell(row, miIndexFieldClose),
			Change:       cell(row, miIndexFieldChange),
			Transaction:  cell(row, miIndexFieldTxCount),
		})
	}
	return out
}

// ErrMIDataDateUnknown reports that a payload arrived without any usable
// provenance stamp (no title date and no envelope date).
var ErrMIDataDateUnknown = errors.New("twse MI_INDEX: payload carries no data date")

// DatedQuotes is one trading day's whole-market quotes together with the
// trading date the payload itself describes.
type DatedQuotes struct {
	// DataDate is YYYY-MM-DD taken from the response (the 每日收盤行情 title
	// date when present, otherwise the envelope's `date`). It is deliberately
	// NOT the requested date: callers MUST compare the two and refuse to store
	// a payload whose provenance differs from what they asked for. That check
	// is what keeps a "latest-only" upstream from ever resurrecting the
	// phantom-row defect.
	DataDate string
	Quotes   []domain.Quote
}

// GetQuotesForDate fetches the whole-market 每日收盤行情 for one date with a
// SINGLE MI_INDEX request (one token-bucket slot instead of one per symbol —
// runBackfill's per-symbol path costs 44 slots per day).
//
// The returned DatedQuotes.DataDate is the date the upstream answered for. It
// is the CALLER's job to reject a mismatch; this method reports provenance, it
// does not judge it. A closed market (weekend, holiday, or a date the exchange
// has not published) is returned as ErrTWSEEmptyData and does not trip the
// breaker, matching the P1-7 no-data semantics used by GetQuotes.
func (c *TWSEClient) GetQuotesForDate(ctx context.Context, date string) (DatedQuotes, error) {
	if _, err := time.Parse("20060102", date); err != nil {
		return DatedQuotes{}, fmt.Errorf("twse MI_INDEX: invalid date %q: %w", date, err)
	}

	if c.breaker != nil && !c.breaker.shouldTry() {
		return DatedQuotes{}, fmt.Errorf("%w: twse circuit breaker open", ErrUpstream)
	}
	waitCtx, cancel := context.WithTimeout(ctx, twseRateLimitWaitAllowance)
	defer cancel()
	if err := c.rateLimiter.Wait(waitCtx); err != nil {
		return DatedQuotes{}, fmt.Errorf("rate limit wait (%s timeout): %w", twseRateLimitWaitAllowance, err)
	}

	endpoint := fmt.Sprintf("%s/exchangeReport/MI_INDEX?type=ALLBUT0999&date=%s&response=json", c.baseURL, date)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return DatedQuotes{}, fmt.Errorf("create request: %w", err)
	}
	// TWSE's WAF answers 403 to short User-Agents (observed 2026-08-21 on the
	// MI_INDEX table used by MarketVolumeProvider). Send the same full browser
	// UA here instead of relying on Go's default.
	req.Header.Set("User-Agent", twseBrowserUserAgent)

	resp, err := fetchWithRetry(ctx, c.httpClient, req, c.retryPolicy())
	if err != nil {
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr == "" {
			bodyStr = "(empty body)"
		}
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, bodyStr)
	}

	// Buffer the body so DecodeJSON sees a fresh reader (charset transcode).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("read body: %w", err)
	}

	var mi twseMIIndexResponse
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &mi); err != nil {
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("decode response: %w", err)
	}

	// A closed market is reported as prose in `stat`, not as an HTTP error.
	// Empty data is expected (not a failure), so it must not trip the breaker.
	if !strings.EqualFold(mi.Stat, "OK") {
		c.breakerRecordSuccess()
		return DatedQuotes{}, fmt.Errorf("%w: MI_INDEX stat=%q for %s (closed market, or that date is not published)", ErrTWSEEmptyData, mi.Stat, date)
	}

	table, ok := mi.DailyQuotesTable()
	if !ok || len(table.Data) == 0 {
		// stat=OK with no per-symbol table is a SCHEMA change: a closed market
		// never reaches this branch (it fails the stat check above). Trip the
		// breaker so the whole host stops hammering an endpoint whose shape we
		// no longer understand.
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("twse MI_INDEX: stat=OK but %s returned no rows for %s (schema change?)", "每日收盤行情", date)
	}

	dataDate := ""
	if d, ok := table.TitleDate(); ok {
		dataDate = d
	} else if len(mi.Date) == 8 {
		if d, perr := time.Parse("20060102", mi.Date); perr == nil {
			dataDate = d.Format("2006-01-02")
		}
	}
	if dataDate == "" {
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("%w (date=%s, title=%q)", ErrMIDataDateUnknown, date, table.Title)
	}

	// The fetch is date-addressed, so the only meaningful timestamp is the
	// trading date; the time-of-day is pinned to the 13:30 TWSE close.
	dataDay, _ := time.Parse("2006-01-02", dataDate)
	asOf := time.Date(dataDay.Year(), dataDay.Month(), dataDay.Day(), 13, 30, 0, 0, twseExchangeLocation())

	rows := table.QuoteRows()
	quotes := make([]domain.Quote, 0, len(rows))
	parseFailures := 0
	for _, r := range rows {
		quote, err := c.convertToQuote(r)
		if err != nil {
			parseFailures++
			continue
		}
		quote.AsOf = asOf
		quotes = append(quotes, quote)
	}
	if len(quotes) == 0 {
		c.breakerRecordFailure()
		return DatedQuotes{}, fmt.Errorf("twse MI_INDEX: all %d rows of %s failed to parse (schema change?)", parseFailures, table.Title)
	}

	c.breakerRecordSuccess()
	return DatedQuotes{DataDate: dataDate, Quotes: quotes}, nil
}

// twseExchangeLocation returns the exchange's timezone (Asia/Taipei), falling
// back to UTC when the tz database is absent — e.g. a distroless image without
// tzdata. The fallback shifts only the time-of-day of a date-addressed quote,
// never the date, because the date is taken from the payload.
func twseExchangeLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Taipei"); err == nil {
		return loc
	}
	return time.UTC
}

// ParseMIIndexDailyQuotes decodes a raw MI_INDEX body and returns the
// 每日收盤行情 rows plus their provenance date. It exists so non-TWSEClient
// callers (cmd/fetch-historical has its own Fetcher) share exactly one
// understanding of the `tables` envelope instead of re-deriving it — the
// duplicated flat `fields`/`data` guess is what made that tool fetch 0 rows
// per day while exiting 0.
//
// A non-"OK" stat returns ErrTWSEEmptyData (nothing traded that day).
func ParseMIIndexDailyQuotes(body []byte) (dataDate string, rows []TWSEQuote, err error) {
	var mi twseMIIndexResponse
	if err := DecodeJSON(bytes.NewReader(body), "", &mi); err != nil {
		return "", nil, fmt.Errorf("decode MI_INDEX response: %w", err)
	}
	if !strings.EqualFold(mi.Stat, "OK") {
		return "", nil, fmt.Errorf("%w: MI_INDEX stat=%q (closed market, or that date is not published)", ErrTWSEEmptyData, mi.Stat)
	}
	table, ok := mi.DailyQuotesTable()
	if !ok || len(table.Data) == 0 {
		return "", nil, fmt.Errorf("MI_INDEX: stat=OK but %s returned no rows (schema change?)", "每日收盤行情")
	}
	if d, ok := table.TitleDate(); ok {
		dataDate = d
	} else if len(mi.Date) == 8 {
		if d, perr := time.Parse("20060102", mi.Date); perr == nil {
			dataDate = d.Format("2006-01-02")
		}
	}
	if dataDate == "" {
		return "", nil, fmt.Errorf("%w (title=%q)", ErrMIDataDateUnknown, table.Title)
	}
	return dataDate, table.QuoteRows(), nil
}
