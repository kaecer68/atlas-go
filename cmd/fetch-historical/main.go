package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"golang.org/x/time/rate"
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

type Fetcher struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

func NewFetcher() *Fetcher {
	params := config.GetParametersConfig()
	return &Fetcher{
		client:      &http.Client{Timeout: 30 * time.Second},
		baseURL:     twseAPIBaseURL,
		rateLimiter: rate.NewLimiter(rate.Limit(params.Marketdata.TWSEAPIRateLimit.Value), 1),
	}
}

func (f *Fetcher) FetchDay(ctx context.Context) ([]TWSEQuote, error) {
	if err := f.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/STOCK_DAY_ALL", f.baseURL)
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

	var quotes []TWSEQuote
	if err := json.NewDecoder(resp.Body).Decode(&quotes); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return quotes, nil
}

func parseFloat(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || s == "--" || s == "-" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseInt64(s string) int64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || s == "--" || s == "-" {
		return 0
	}
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func tradingDates(start, end time.Time) []time.Time {
	var dates []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			dates = append(dates, d)
		}
	}
	return dates
}

func formatYYYYMMDD(t time.Time) string {
	return t.Format("20060102")
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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

	start, err := parseDate(*startStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid start date: %v\n", err)
		os.Exit(1)
	}

	end := time.Now()
	if *endStr != "" {
		end, err = parseDate(*endStr)
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
		quotes, err := fetcher.FetchDay(ctx)
		cancel()

		if err != nil {
			fmt.Printf(" [SKIP - error: %v]\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		var bars []HistoricalBar
		for _, q := range quotes {
			close := parseFloat(q.ClosingPrice)
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
				Open:   parseFloat(q.OpeningPrice),
				High:   parseFloat(q.HighestPrice),
				Low:    parseFloat(q.LowestPrice),
				Close:  close,
				Volume: parseInt64(q.TradeVolume),
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
