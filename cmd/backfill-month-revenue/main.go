package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

const (
	finmindBaseURL = "https://api.finmindtrade.com/api/v4"
	rateLimit      = 600
	pacingSeconds  = 6
)

var (
	startDate  = flag.String("start", "2024-01-01", "backfill start date (YYYY-MM-DD)")
	endDate    = flag.String("end", "2026-04-30", "backfill end date (YYYY-MM-DD)")
	symbolsArg = flag.String("symbols", "", "comma-separated stock IDs (or use fundamentals.json)")
	dryRun     = flag.Bool("dry-run", false, "print what would be added without writing")
)

type FinMindResponse struct {
	Msg    string `json:"msg"`
	Status int    `json:"status"`
	Data   []struct {
		Date         string  `json:"date"`
		StockID      string  `json:"stock_id"`
		Revenue      float64 `json:"revenue"`
		RevenueMonth int     `json:"revenue_month"`
		RevenueYear  int     `json:"revenue_year"`
	} `json:"data"`
}

func main() {
	flag.Parse()

	stateDir := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "state")
	os.MkdirAll(stateDir, 0755)

	apiKey := os.Getenv("FINMIND_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "FINMIND_API_KEY not set\n")
		os.Exit(1)
	}

	symbols := loadSymbols(*symbolsArg)
	fmt.Printf("Backfill month revenue for %d symbols from %s to %s\n", len(symbols), *startDate, *endDate)

	outputPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "replay", "month_revenue.jsonl")
	existing := loadExistingRecords(outputPath)

	client := &http.Client{Timeout: 30 * time.Second}
	limiter := marketdata.NewRateLimiter(rateLimit)

	var newRecords int
	startTime := time.Now()

	for i, symbol := range symbols {
		fmt.Printf("[%d/%d] Fetching %s...\n", i+1, len(symbols), symbol)

		reqURL := fmt.Sprintf("%s/data?dataset=TaiwanStockMonthRevenue&data_id=%s&start_date=%s&end_date=%s",
			finmindBaseURL, symbol, *startDate, *endDate)

		body, err := marketdata.FetchWithRetry(context.Background(), client, reqURL, apiKey, limiter, 3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] failed: %v\n", symbol, err)
			continue
		}

		var resp FinMindResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] parse error: %v\n", symbol, err)
			continue
		}

		if resp.Status != 200 {
			fmt.Fprintf(os.Stderr, "[%s] API error: %s\n", symbol, resp.Msg)
			continue
		}

		for _, item := range resp.Data {
			key := fmt.Sprintf("%s|%s", item.Date, item.StockID)
			if _, ok := existing[key]; ok {
				continue
			}

			if !*dryRun {
				fmt.Printf("%s|%s|%.0f\n", item.Date, item.StockID, item.Revenue)
			}
			existing[key] = struct{}{}
			newRecords++
		}

		time.Sleep(time.Duration(pacingSeconds) * time.Second)
	}

	elapsed := time.Since(startTime)
	latencyMs := elapsed.Milliseconds()

	monitoring.RecordChannelFetch(stateDir, "backfill_month_revenue", "ok", "", limiter.Remaining(), latencyMs)

	if *dryRun {
		fmt.Printf("\nDry run: would add %d new records for %d symbols\n", newRecords, len(symbols))
	} else {
		fmt.Printf("\nBackfill complete: added %d new records\n", newRecords)
	}

	fmt.Printf("Time elapsed: %v, rate limit remaining: %d\n", elapsed, limiter.Remaining())
}

func loadSymbols(symbolsArg string) []string {
	if symbolsArg != "" {
		parts := strings.Split(symbolsArg, ",")
		return parts
	}

	fundamentalsPath := filepath.Join(os.Getenv("HOME"), "workspace", "atlas", "data", "fundamentals.json")
	data, err := os.ReadFile(fundamentalsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read fundamentals.json: %v\n", err)
		return nil
	}

	var fund map[string]interface{}
	json.Unmarshal(data, &fund)

	var symbols []string
	for id := range fund {
		symbols = append(symbols, id)
	}
	sort.Strings(symbols)
	if len(symbols) > 100 {
		symbols = symbols[:100]
	}
	return symbols
}

func loadExistingRecords(path string) map[string]struct{} {
	existing := make(map[string]struct{})
	f, err := os.Open(path)
	if err != nil {
		return existing
	}
	defer f.Close()

	br := io.LimitReader(f, 100<<20)
	dec := json.NewDecoder(br)
	for dec.More() {
		var rec map[string]interface{}
		if err := dec.Decode(&rec); err != nil {
			continue
		}
		date, _ := rec["date"].(string)
		stockID, _ := rec["stock_id"].(string)
		if date != "" && stockID != "" {
			existing[date+"|"+stockID] = struct{}{}
		}
	}
	return existing
}
