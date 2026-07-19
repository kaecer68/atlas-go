// BK-12: TAIFEX 外資期貨未平倉 90 天歷史回填。
//
// TAIFEX OpenAPI 只回傳最新交易日資料，無法回溯。本命令使用 FinMind
// TaiwanFuturesInstitutionalTraders dataset 抓取歷史逐日外資/投信/自營
// 臺股期貨未平倉淨口數，寫入 data/replay/taifex_oi_history.jsonl。
//
// 用法：
//
//	backfill-taifex-oi -days 90
//	backfill-taifex-oi -days 90 -dry-run
//
// 必須設定環境變數 FINMIND_API_KEY。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
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
	days    = flag.Int("days", 90, "number of calendar days to backfill")
	dryRun  = flag.Bool("dry-run", false, "print what would be added without writing")
	workDir = flag.String("workdir", ".", "atlas repo root")
)

// OIRecord is one row in the output JSONL.
type OIRecord struct {
	Date            string `json:"date"`
	ContractCode    string `json:"contract_code"`
	ForeignOINet    int64  `json:"foreign_oi_net"`
	TrustOINet      int64  `json:"trust_oi_net"`
	DealerOINet     int64  `json:"dealer_oi_net"`
	ForeignTradeNet int64  `json:"foreign_trade_net"`
	TrustTradeNet   int64  `json:"trust_trade_net"`
	DealerTradeNet  int64  `json:"dealer_trade_net"`
}

func main() {
	flag.Parse()

	apiKey := os.Getenv("FINMIND_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "FINMIND_API_KEY not set\n")
		os.Exit(1)
	}

	stateDir := filepath.Join(*workDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create state dir: %v\n", err)
		os.Exit(1)
	}

	outputPath := filepath.Join(*workDir, "data", "replay", "taifex_oi_history.jsonl")
	existing := loadExistingOIRecords(outputPath)

	endDate := time.Now().AddDate(0, 0, -1) // yesterday (today's data not yet available)
	startDate := endDate.AddDate(0, 0, -(*days))

	fmt.Printf("Backfill TAIFEX OI from %s to %s\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	client := &http.Client{Timeout: 30 * time.Second}
	limiter := marketdata.NewRateLimiter(rateLimit)

	var newRecords int
	startTime := time.Now()

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		// Skip weekends (FinMind may return empty; skip for efficiency)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}

		// Check if already have this date
		if _, ok := existing[dateStr]; ok {
			continue
		}

		rec, err := fetchOIDate(client, limiter, apiKey, dateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] fetch failed: %v\n", dateStr, err)
			continue
		}
		if rec == nil {
			continue // no data for this date (holiday, etc.)
		}

		fmt.Printf("[%s] foreign_oi=%d trust_oi=%d dealer_oi=%d\n",
			dateStr, rec.ForeignOINet, rec.TrustOINet, rec.DealerOINet)

		if !*dryRun {
			if err := appendOIRecord(outputPath, rec); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] write failed: %v\n", dateStr, err)
				continue
			}
		}
		existing[dateStr] = struct{}{}
		newRecords++

		time.Sleep(time.Duration(pacingSeconds) * time.Second)
	}

	elapsed := time.Since(startTime)
	monitoring.RecordChannelFetch(stateDir, "backfill_taifex_oi", "ok", "", limiter.Remaining(), elapsed.Milliseconds())
	fmt.Printf("\nBackfill complete: added %d new records (%v elapsed)\n", newRecords, elapsed.Round(time.Second))
}

func fetchOIDate(client *http.Client, limiter *marketdata.RateLimiter, apiKey, date string) (*OIRecord, error) {
	url := fmt.Sprintf("%s/data?dataset=TaiwanFuturesInstitutionalTraders&data_id=TX&start_date=%s&end_date=%s",
		constants.FinMindBaseURL, date, date)

	body, err := marketdata.FetchWithRetry(context.Background(), client, url, apiKey, limiter, 3)
	if err != nil {
		return nil, fmt.Errorf("finmind fetch: %w", err)
	}

	var resp struct {
		Msg    string `json:"msg"`
		Status int    `json:"status"`
		Data   []struct {
			Date            string  `json:"date"`
			ContractCode    string  `json:"futures_id"`
			ForeignOINet    float64 `json:"foreign_institutional_investors_oi_net"`
			TrustOINet      float64 `json:"investment_trust_oi_net"`
			DealerOINet     float64 `json:"dealer_oi_net"`
			ForeignTradeNet float64 `json:"foreign_institutional_investors_trade_net"`
			TrustTradeNet   float64 `json:"investment_trust_trade_net"`
			DealerTradeNet  float64 `json:"dealer_trade_net"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("finmind API error: %s", resp.Msg)
	}

	for _, d := range resp.Data {
		if d.ContractCode != "TX" {
			continue
		}
		return &OIRecord{
			Date:            d.Date,
			ContractCode:    d.ContractCode,
			ForeignOINet:    int64(math.Round(d.ForeignOINet)),
			TrustOINet:      int64(math.Round(d.TrustOINet)),
			DealerOINet:     int64(math.Round(d.DealerOINet)),
			ForeignTradeNet: int64(math.Round(d.ForeignTradeNet)),
			TrustTradeNet:   int64(math.Round(d.TrustTradeNet)),
			DealerTradeNet:  int64(math.Round(d.DealerTradeNet)),
		}, nil
	}
	return nil, nil // no data = not a trading day
}

func loadExistingOIRecords(path string) map[string]struct{} {
	existing := make(map[string]struct{})
	f, err := os.Open(path)
	if err != nil {
		return existing
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec OIRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.Date != "" {
			existing[rec.Date] = struct{}{}
		}
	}
	return existing
}

func appendOIRecord(path string, rec *OIRecord) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = f.Write(b)
	return err
}
