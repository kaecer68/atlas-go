package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/marketdata/twse"
)

const (
	twseAPIBaseURL = constants.TWSEBaseURL
)

type HistoricalBar struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Name   string  `json:"name,omitempty"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

type Fetcher struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

func NewFetcher() *Fetcher {
	params := config.GetParametersConfig()
	rateLimit := rate.Limit(0.2)
	burst := 1
	if params != nil {
		rateLimit = rate.Limit(params.Marketdata.TWSEAPIRateLimit.Value)
		if params.Marketdata.TWSEAPIRateBurst.Value > 0 {
			burst = params.Marketdata.TWSEAPIRateBurst.Value
		}
	}
	return &Fetcher{
		client:      &http.Client{Timeout: 30 * time.Second},
		baseURL:     twseAPIBaseURL,
		rateLimiter: rate.NewLimiter(rateLimit, burst),
	}
}

// FetchDay fetches every listed stock's quotes for one trading date from the
// TWSE MI_INDEX endpoint, which accepts a historical date parameter.
//
// 2026-09-26: this function used to decode a FLAT `fields`/`data` envelope.
// MI_INDEX actually answers with `{stat, date, tables:[…]}`, and only the
// 每日收盤行情 section holds per-symbol rows — so `len(wrapper.Data) == 0` was
// true for every date, the tool wrote nothing, printed `[0 records]` and exited
// 0. A silent success is the worst possible failure mode here: it is
// indistinguishable from "the exchange had no data". The envelope is now owned
// by marketdata.ParseMIIndexDailyQuotes, shared with cmd/daily-replay-sync, and
// the two outcomes are separated:
//
//   - closed market / unpublished date → ErrTWSEEmptyData (expected, no rows)
//   - stat=OK but no daily table, or an unparseable envelope → error
//     (a schema change must never look like an empty day)
//
// The payload's own title date must match the requested date (provenance
// guard, same as cmd/daily-replay-sync): writing rows stamped with the
// requested date while the prices belong to another session is how the replay
// dataset acquires phantom dates.
func (f *Fetcher) FetchDay(ctx context.Context, date time.Time) ([]marketdata.TWSEQuote, error) {
	if err := f.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	wantDate := date.Format("2006-01-02")
	dateStr := formatYYYYMMDD(date)
	url := fmt.Sprintf("%s/exchangeReport/MI_INDEX?type=ALLBUT0999&date=%s&response=json", f.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	dataDate, quotes, err := marketdata.ParseMIIndexDailyQuotes(body)
	if err != nil {
		return nil, err
	}
	if dataDate != wantDate {
		return nil, fmt.Errorf("MI_INDEX answered for %s while %s was requested; refusing to use a mis-dated payload", dataDate, wantDate)
	}
	return quotes, nil
}

func tradingDates(start, end time.Time) []time.Time {
	return twse.TradingDates(start, end)
}

func formatYYYYMMDD(t time.Time) string {
	return t.Format("20060102")
}

func loadExistingKeys(path string) (map[string]bool, error) {
	m := map[string]bool{}
	if path == "" {
		return m, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("open existing file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var bar HistoricalBar
		if err := json.Unmarshal(sc.Bytes(), &bar); err != nil {
			continue
		}
		m[bar.Date+"+"+bar.Symbol] = true
	}
	return m, sc.Err()
}

func appendJSONL(path string, bars []HistoricalBar) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	for _, bar := range bars {
		data, err := json.Marshal(bar)
		if err != nil {
			return fmt.Errorf("marshal bar: %w", err)
		}
		if _, err := fmt.Fprintln(f, string(data)); err != nil {
			return fmt.Errorf("write line: %w", err)
		}
	}
	return nil
}

func main() {
	startStr := flag.String("start", "2020-01-01", "Start date (YYYY-MM-DD)")
	endStr := flag.String("end", "", "End date (YYYY-MM-DD), defaults to today")
	output := flag.String("output", "data/replay/historical.jsonl", "Output JSONL path")
	mergeWith := flag.String("merge-with", "", "Existing JSONL to merge/deduplicate")

	flag.Parse()

	start, err := twse.ParseDate(*startStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid start date: %v\n", err)
		os.Exit(1)
	}

	end := time.Now()
	if *endStr != "" {
		end, err = twse.ParseDate(*endStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid end date: %v\n", err)
			os.Exit(1)
		}
	}

	if start.After(end) {
		fmt.Fprintf(os.Stderr, "start date must be before end date\n")
		os.Exit(1)
	}

	fmt.Printf("Fetching TWSE historical data from %s to %s\n", start.Format("2006-01-02"), end.Format("2006-01-02"))
	fmt.Printf("Output: %s\n", *output)

	existing := map[string]bool{}
	if *mergeWith != "" {
		existing, err = loadExistingKeys(*mergeWith)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read existing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Loaded %d existing records for deduplication\n", len(existing))
	}

	fetcher := NewFetcher()
	dates := tradingDates(start, end)
	total := len(dates)

	// 2026-09-26: the run summary used to be a single "Done." line, so a whole
	// range that fetched nothing (the flat-envelope parse bug) exited 0 and
	// looked like a successful run against a quiet market. Failures are now
	// counted and the exit code carries them.
	var totalRows, emptyDates, failedDates int

	for i, d := range dates {
		fmt.Printf("Fetched %s (%d/%d)", d.Format("2006-01-02"), i+1, total)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		quotes, err := fetcher.FetchDay(ctx, d)
		cancel()

		if errors.Is(err, marketdata.ErrTWSEEmptyData) {
			// Closed market (public holiday — tradingDates already drops
			// weekends) or a date the exchange has not published. Nothing to
			// write, and not a failure.
			fmt.Printf(" [no data (non-trading day or not published)]\n")
			emptyDates++
			time.Sleep(5 * time.Second)
			continue
		}
		if err != nil {
			fmt.Printf(" [SKIP - error: %v]\n", err)
			failedDates++
			time.Sleep(5 * time.Second)
			continue
		}

		var bars []HistoricalBar
		for _, q := range quotes {
			close := twse.ParseFloat(q.ClosingPrice)
			if close == 0 {
				continue
			}
			key := d.Format("2006-01-02") + "+" + q.Code
			if existing[key] {
				continue
			}
			existing[key] = true
			bars = append(bars, HistoricalBar{
				Date:   d.Format("2006-01-02"),
				Symbol: q.Code + ".TW",
				Name:   q.Name,
				Open:   twse.ParseFloat(q.OpeningPrice),
				High:   twse.ParseFloat(q.HighestPrice),
				Low:    twse.ParseFloat(q.LowestPrice),
				Close:  close,
				Volume: twse.ParseInt64(q.TradeVolume),
			})
		}

		if len(bars) > 0 {
			if err := appendJSONL(*output, bars); err != nil {
				fmt.Printf(" [SKIP - write error: %v]\n", err)
				failedDates++
				time.Sleep(5 * time.Second)
				continue
			}
		}
		totalRows += len(bars)
		switch {
		case len(bars) > 0:
			fmt.Printf(" [%d records]\n", len(bars))
		case len(quotes) > 0:
			// Rows came back but none was usable: every close was 0/"--"
			// (suspended) or all of them are already in -merge-with. A re-run
			// is idempotent so this is not a failure, but it must never print a
			// bare "0 records" — that is the wording that let a 0-row bug pass
			// for weeks.
			fmt.Printf(" [0 new records — %d rows fetched, all already present or without a usable close]\n", len(quotes))
		default:
			fmt.Printf(" [0 rows returned]\n")
		}

		time.Sleep(5 * time.Second)
	}

	fmt.Printf("\nDone. %d rows written to %s (%d dates with no data, %d failed)\n",
		totalRows, *output, emptyDates, failedDates)
	if failedDates > 0 {
		fmt.Fprintf(os.Stderr, "\nERROR: %d of %d dates failed — see the [SKIP - error] lines above. A changed upstream shape must never look like an empty market.\n",
			failedDates, total)
		os.Exit(1)
	}
}
