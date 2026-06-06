// Command extend-replay-etf fetches ETF daily bar data from TWSE OpenAPI
// (STOCK_DAY endpoint) and outputs it in the replay CSV format expected by
// replay.LoadTWSEOpenDataCSV. TWSE is the primary free data source — no API
// key required. Fubon, Fugle, FinMind are fallbacks per system architecture.
//
// Usage:
//
//	go run ./cmd/extend-replay-etf -symbols 0050,0056,00878 -start 2026-01-01 -end 2026-05-29 -output data/replay/etf_historical.csv
//
// To extend the main replay dataset, merge the output CSV with the existing
// sample CSV (samples/replay/twse_stock_day_all_sample.csv).
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// twseBaseURL is overridden in tests to point at mock servers.
var twseBaseURL = "https://www.twse.com.tw"

func main() {
	symbolsFlag := flag.String("symbols", "", "comma-separated ETF symbols (e.g. 0050,0056,00878)")
	startFlag := flag.String("start", "", "start date YYYY-MM-DD")
	endFlag := flag.String("end", "", "end date YYYY-MM-DD")
	outputFlag := flag.String("output", "data/replay/etf_historical.csv", "output CSV path")
	flag.Parse()

	if *symbolsFlag == "" || *startFlag == "" || *endFlag == "" {
		fmt.Fprintf(os.Stderr, "usage: extend-replay-etf -symbols ETF1,ETF2 -start YYYY-MM-DD -end YYYY-MM-DD [-output path]\n")
		os.Exit(1)
	}

	symbols := strings.Split(*symbolsFlag, ",")
	start, err := time.Parse("2006-01-02", *startFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid start date: %v\n", err)
		os.Exit(1)
	}
	end, err := time.Parse("2006-01-02", *endFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid end date: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	limiter := rate.NewLimiter(rate.Every(3*time.Second), 1) // TWSE rate limit: 1 req per 3s
	client := httpclient.NewFactory().NewClient(15 * time.Second)

	tradingDays := tradingDates(start, end)
	fmt.Printf("Fetching %d ETFs over %d trading days via TWSE OpenAPI...\n", len(symbols), len(tradingDays))

	var allBars []HistoricalBar
	for _, d := range tradingDays {
		for _, sym := range symbols {
			if err := limiter.Wait(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "rate limit: %v\n", err)
				continue
			}

			bar, ok, err := fetchStockDay(ctx, client, sym, d)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fetch %s on %s: %v\n", sym, d.Format("2006-01-02"), err)
				continue
			}
			if !ok {
				continue // no data for this symbol/date (e.g., weekend or not yet listed)
			}
			allBars = append(allBars, bar)
			fmt.Printf("  %s %s: O=%.1f H=%.1f L=%.1f C=%.1f V=%d\n",
				d.Format("2006-01-02"), sym, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume)
		}
	}

	if len(allBars) == 0 {
		fmt.Fprintf(os.Stderr, "no data fetched\n")
		os.Exit(1)
	}

	f, err := os.Create(*outputFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "TradeValue", "Open", "High", "Low", "Close", "Change", "Transaction"})
	for _, bar := range allBars {
		_ = w.Write([]string{
			bar.Date,
			bar.Symbol,
			bar.Name,
			strconv.FormatInt(bar.Volume, 10),
			"0", // TradeValue — not available from STOCK_DAY
			fmt.Sprintf("%.2f", bar.Open),
			fmt.Sprintf("%.2f", bar.High),
			fmt.Sprintf("%.2f", bar.Low),
			fmt.Sprintf("%.2f", bar.Close),
			"0", // Change — not available from STOCK_DAY
			"0", // Transaction — not available
		})
	}
	w.Flush()

	fmt.Printf("Wrote %d bars to %s\n", len(allBars), *outputFlag)
	fmt.Println("To merge: cat", *outputFlag, ">> samples/replay/twse_stock_day_all_sample.csv")
}

// STOCK_DAY response types
type stockDayResponse struct {
	Stat  string     `json:"stat"`
	Data  [][]string `json:"data"`
	Date  string     `json:"date"`
	Title string     `json:"title"`
}

type HistoricalBar struct {
	Date   string
	Symbol string
	Name   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// fetchStockDay fetches a single stock's daily bar from TWSE STOCK_DAY endpoint.
// Returns (bar, found, error). found=false means no trading on that date.
func fetchStockDay(ctx context.Context, client *http.Client, symbol string, date time.Time) (HistoricalBar, bool, error) {
	dateStr := date.Format("20060102")
	url := fmt.Sprintf("%s/exchangeReport/STOCK_DAY?response=json&date=%s&stockNo=%s", twseBaseURL, dateStr, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HistoricalBar{}, false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return HistoricalBar{}, false, fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return HistoricalBar{}, false, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result stockDayResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return HistoricalBar{}, false, fmt.Errorf("decode: %w", err)
	}

	if result.Stat != "OK" || len(result.Data) == 0 {
		return HistoricalBar{}, false, nil
	}

	// STOCK_DAY data format: each row is [Date, Volume, Value, Open, High, Low, Close, Change, Transaction]
	// The response contains data for the whole MONTH. Find the matching date.
	var matched []string
	for _, row := range result.Data {
		if len(row) < 9 {
			continue
		}
		// Date format in STOCK_DAY: "115/05/29" (ROC calendar)
		rocDate := fmt.Sprintf("%d/%02d/%02d", date.Year()-1911, date.Month(), date.Day())
		if strings.TrimSpace(row[0]) == rocDate {
			matched = row
			break
		}
	}

	if matched == nil {
		return HistoricalBar{}, false, nil
	}

	open := parseFloat(matched[3])
	high := parseFloat(matched[4])
	low := parseFloat(matched[5])
	close := parseFloat(matched[6])
	volume := parseInt64(matched[1])
	name := strings.TrimSpace(result.Title)
	// Extract ETF name from title (e.g., "0050 元大台灣50 各日成交資訊")
	if idx := strings.Index(name, " "); idx > 0 {
		if idx2 := strings.Index(name[idx+1:], " "); idx2 > 0 {
			name = name[idx+1 : idx+1+idx2]
		}
	}

	return HistoricalBar{
		Date:   date.Format("2006-01-02"),
		Symbol: symbol,
		Name:   name,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  close,
		Volume: volume,
	}, true, nil
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
