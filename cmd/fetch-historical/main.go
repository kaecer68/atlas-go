package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata/twse"
)

const (
	twseAPIBaseURL = "https://www.twse.com.tw"
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

type TWSEQuote struct {
	Code         string `json:"Code"`
	Name         string `json:"Name"`
	TradeVolume  string `json:"TradeVolume"`
	TradeValue   string `json:"TradeValue"`
	OpeningPrice string `json:"OpeningPrice"`
	HighestPrice string `json:"HighestPrice"`
	LowestPrice  string `json:"LowestPrice"`
	ClosingPrice string `json:"ClosingPrice"`
	Change       string `json:"Change"`
	Transaction  string `json:"Transaction"`
}

// MIINDEXResponse wraps the TWSE MI_INDEX endpoint response format.
// Unlike STOCK_DAY_ALL (flat array, no date support), MI_INDEX accepts
// the date parameter and returns historical data for any trading day.
type MIINDEXResponse struct {
	Stat   string     `json:"stat"`
	Date   string     `json:"date"`
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
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

// FetchDay fetches all stock quotes for a specific trading date
// using TWSE MI_INDEX endpoint (which accepts historical date parameter).
func (f *Fetcher) FetchDay(ctx context.Context, date time.Time) ([]TWSEQuote, error) {
	if err := f.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

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

	var wrapper MIINDEXResponse
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if wrapper.Stat != "OK" || len(wrapper.Data) == 0 {
		return nil, nil
	}

	// MI_INDEX returns fields as 2D string arrays: [Code, Name, Volume, Value, Open, High, Low, Close, Change, TxCount]
	quotes := make([]TWSEQuote, 0, len(wrapper.Data))
	for _, row := range wrapper.Data {
		if len(row) < 10 {
			continue
		}
		quotes = append(quotes, TWSEQuote{
			Code:         row[0],
			Name:         row[1],
			TradeVolume:  row[2],
			TradeValue:   row[3],
			OpeningPrice: row[4],
			HighestPrice: row[5],
			LowestPrice:  row[6],
			ClosingPrice: row[7],
			Change:       row[8],
			Transaction:  row[9],
		})
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

	for i, d := range dates {
		fmt.Printf("Fetched %s (%d/%d)", d.Format("2006-01-02"), i+1, total)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		quotes, err := fetcher.FetchDay(ctx, d)
		cancel()

		if err != nil {
			fmt.Printf(" [SKIP - error: %v]\n", err)
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
				time.Sleep(5 * time.Second)
				continue
			}
		}
		fmt.Printf(" [%d records]\n", len(bars))

		time.Sleep(5 * time.Second)
	}

	fmt.Printf("\nDone. Data saved to %s\n", *output)
}
