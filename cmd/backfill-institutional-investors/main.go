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

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

const (
	rateLimit     = 600
	pacingSeconds = 6
)

var (
	symbolsArg = flag.String("symbols", "", "comma-separated stock IDs (or use fundamentals.json)")
	date       = flag.String("date", "", "specific date (YYYY-MM-DD), default: yesterday")
	dryRun     = flag.Bool("dry-run", false, "print what would be added without writing")
	workDir    = flag.String("workdir", ".", "atlas repo root (data/ + configs/ + state/ live here)")
)

type InstitutionalInvestorRow struct {
	Date    string  `json:"date"`
	StockID string  `json:"stock_id"`
	Name    string  `json:"name"`
	Buy     float64 `json:"buy"`
	Sell    float64 `json:"sell"`
	Net     float64 `json:"net"`
}

func main() {
	flag.Parse()

	stateDir := filepath.Join(*workDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create state dir: %v\n", err)
		os.Exit(1)
	}

	apiKey := os.Getenv("FINMIND_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "FINMIND_API_KEY not set\n")
		os.Exit(1)
	}

	symbols := loadSymbols(*symbolsArg)

	targetDate := *date
	if targetDate == "" {
		yesterday := time.Now().AddDate(0, 0, -1)
		targetDate = yesterday.Format("2006-01-02")
	}

	fmt.Printf("Backfill institutional investors for %d symbols on %s\n", len(symbols), targetDate)

	outputPath := filepath.Join(*workDir, "data", "replay", "institutional_investors.jsonl")
	existing := loadExistingRecords(outputPath)

	client := &http.Client{Timeout: 30 * time.Second}
	limiter := marketdata.NewRateLimiter(rateLimit)

	var newRecords int
	startTime := time.Now()

	for i, symbol := range symbols {
		fmt.Printf("[%d/%d] Fetching %s...\n", i+1, len(symbols), symbol)

		reqURL := fmt.Sprintf("%s/data?dataset=TaiwanStockInstitutionalInvestorsBuySell&data_id=%s&start_date=%s&end_date=%s",
			constants.FinMindBaseURL, symbol, targetDate, targetDate)

		body, err := marketdata.FetchWithRetry(context.Background(), client, reqURL, apiKey, limiter, 3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] failed: %v\n", symbol, err)
			continue
		}

		var resp struct {
			Msg    string `json:"msg"`
			Status int    `json:"status"`
			Data   []struct {
				Date    string  `json:"date"`
				StockID string  `json:"stock_id"`
				Name    string  `json:"name"`
				Buy     float64 `json:"buy"`
				Sell    float64 `json:"sell"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] parse error: %v\n", symbol, err)
			continue
		}

		if resp.Status != 200 {
			fmt.Fprintf(os.Stderr, "[%s] API error: %s\n", symbol, resp.Msg)
			continue
		}

		for _, item := range resp.Data {
			key := fmt.Sprintf("%s|%s|%s", item.Date, item.StockID, item.Name)
			if _, ok := existing[key]; ok {
				continue
			}

			if !*dryRun {
				fmt.Printf("%s|%s|%s|buy=%.0f|sell=%.0f|net=%.0f\n",
					item.Date, item.StockID, item.Name, item.Buy, item.Sell, item.Buy-item.Sell)
			}
			existing[key] = struct{}{}
			newRecords++
		}

		time.Sleep(time.Duration(pacingSeconds) * time.Second)
	}

	elapsed := time.Since(startTime)
	monitoring.RecordChannelFetch(stateDir, "backfill_institutional_investors", "ok", "", limiter.Remaining(), elapsed.Milliseconds())

	if *dryRun {
		fmt.Printf("\nDry run: would add %d new records\n", newRecords)
	} else {
		fmt.Printf("\nBackfill complete: added %d new records\n", newRecords)
	}
}

func loadSymbols(symbolsArg string) []string {
	if symbolsArg != "" {
		return strings.Split(symbolsArg, ",")
	}

	fundamentalsPath := filepath.Join(*workDir, "data", "fundamentals.json")
	data, err := os.ReadFile(fundamentalsPath)
	if err != nil {
		return nil
	}

	var fund map[string]any
	if err := json.Unmarshal(data, &fund); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse fundamentals: %v\n", err)
	}

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
	defer func() { _ = f.Close() }()

	br := io.LimitReader(f, 100<<20)
	dec := json.NewDecoder(br)
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			continue
		}
		date, _ := rec["date"].(string)
		stockID, _ := rec["stock_id"].(string)
		name, _ := rec["name"].(string)
		if date != "" && stockID != "" {
			existing[date+"|"+stockID+"|"+name] = struct{}{}
		}
	}
	return existing
}
