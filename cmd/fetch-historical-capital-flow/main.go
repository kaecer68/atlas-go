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
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/constants"
)

// CapitalFlowData represents the output format for capital flow data.
type CapitalFlowData struct {
	Date               string  `json:"date"`
	ForeignInvestorNet float64 `json:"foreign_investor_net"`
	DomesticFundNet    float64 `json:"domestic_fund_net"`
	DealerNet          float64 `json:"dealer_net"`
	TotalNet           float64 `json:"total_net"`
}

// twseT86Response represents the TWSE API response structure.
type twseT86Response struct {
	Stat string     `json:"stat"`
	Data [][]string `json:"data"`
}

// Stats tracks fetch statistics.
type Stats struct {
	TotalDates      int
	SuccessFetched  int
	SkippedExisting int
	Failed          int
}

func main() {
	start := flag.String("start", "", "start date in YYYY-MM-DD format (required)")
	end := flag.String("end", "", "end date in YYYY-MM-DD format (default: today)")
	output := flag.String("output", "data/state/capital_flow/", "output directory")
	// CONSTITUTION 第二條: TWSE 三大法人保守 1 req / 5s（TWSECapitalFlowRate）。
	// 預設 0.2/s（5s 間隔）；歷史回填長任務可視需要放寬，但不得超過官方限制。
	rateLimit := flag.Float64("rate-limit", 0.2, "requests per second (CONSTITUTION: TWSE T86 = 0.2/s)")
	dryRun := flag.Bool("dry-run", false, "print what would be fetched without downloading")
	skipExisting := flag.Bool("skip-existing", true, "skip dates that already have files")
	flag.Parse()

	if *start == "" {
		fmt.Fprintf(os.Stderr, "error: -start flag is required\n")
		os.Exit(1)
	}

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid start date format: %v\n", err)
		os.Exit(1)
	}

	endDate := time.Now()
	if *end != "" {
		endDate, err = time.Parse("2006-01-02", *end)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid end date format: %v\n", err)
			os.Exit(1)
		}
	}

	if startDate.After(endDate) {
		fmt.Fprintf(os.Stderr, "error: start date cannot be after end date\n")
		os.Exit(1)
	}

	if startDate.After(time.Now()) {
		fmt.Fprintf(os.Stderr, "error: start date cannot be in the future\n")
		os.Exit(1)
	}

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	limiter := rate.NewLimiter(rate.Limit(*rateLimit), 1)

	stats := fetchHistoricalData(startDate, endDate, *output, limiter, *dryRun, *skipExisting)

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total dates processed: %d\n", stats.TotalDates)
	fmt.Printf("Successfully fetched: %d\n", stats.SuccessFetched)
	fmt.Printf("Skipped (existing): %d\n", stats.SkippedExisting)
	fmt.Printf("Failed: %d\n", stats.Failed)
	if stats.Failed > 0 {
		os.Exit(1)
	}
}

func fetchHistoricalData(start, end time.Time, outputDir string, limiter *rate.Limiter, dryRun, skipExisting bool) Stats {
	stats := Stats{}
	current := start

	for current.Before(end) || current.Equal(end) {
		// Skip weekends
		if current.Weekday() == time.Saturday || current.Weekday() == time.Sunday {
			current = current.AddDate(0, 0, 1)
			continue
		}

		stats.TotalDates++
		dateStr := current.Format("2006-01-02")
		filename := filepath.Join(outputDir, current.Format("20060102")+".json")

		if skipExisting {
			if _, err := os.Stat(filename); err == nil {
				fmt.Printf("[%d] %s - skipped (exists)\n", stats.TotalDates, dateStr)
				stats.SkippedExisting++
				current = current.AddDate(0, 0, 1)
				continue
			}
		}

		if dryRun {
			fmt.Printf("[%d] %s - dry run (would fetch)\n", stats.TotalDates, dateStr)
			current = current.AddDate(0, 0, 1)
			continue
		}

		_ = limiter.Wait(context.Background())

		data, err := fetchCapitalFlowData(dateStr)
		if err != nil {
			fmt.Printf("[%d] %s - error: %v\n", stats.TotalDates, dateStr, err)
			stats.Failed++
			current = current.AddDate(0, 0, 1)
			continue
		}

		if err := saveCapitalFlowData(filename, data); err != nil {
			fmt.Printf("[%d] %s - save error: %v\n", stats.TotalDates, dateStr, err)
			stats.Failed++
			current = current.AddDate(0, 0, 1)
			continue
		}

		fmt.Printf("[%d] %s - fetched successfully\n", stats.TotalDates, dateStr)
		stats.SuccessFetched++

		if stats.TotalDates%10 == 0 {
			fmt.Printf("  Progress: %d fetched, %d skipped, %d failed\n", stats.SuccessFetched, stats.SkippedExisting, stats.Failed)
		}

		current = current.AddDate(0, 0, 1)
	}

	return stats
}

// fetchCapitalFlowData fetches T86 capital flow data from TWSE API for a given date.
// It implements retry logic with exponential backoff (3 attempts) and handles HTTP 429 with 60s wait.
func fetchCapitalFlowData(dateStr string) (*CapitalFlowData, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var lastErr error
	backoff := time.Second

	for attempt := range 3 {
		twseDate := strings.ReplaceAll(dateStr, "-", "")
		url := fmt.Sprintf(constants.TWSEBaseURL+"/rwd/zh/fund/T86?response=json&date=%s&selectType=ALLBUT0999", twseDate)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			lastErr = fmt.Errorf("create request: %w", err)
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			if attempt < 2 {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusTooManyRequests {
			fmt.Printf("Rate limited (429), waiting 60 seconds...\n")
			time.Sleep(60 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("http error: status %d", resp.StatusCode)
			if attempt < 2 {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			if attempt < 2 {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		var apiResp twseT86Response
		if err := json.Unmarshal(body, &apiResp); err != nil {
			lastErr = fmt.Errorf("unmarshal response: %w", err)
			if attempt < 2 {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		if apiResp.Stat != "OK" || len(apiResp.Data) == 0 {
			lastErr = fmt.Errorf("no data: stat=%s", apiResp.Stat)
			if attempt < 2 {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		// Parse capital flow data from TWSE T86 response.
		// Column mapping (0-indexed): 4 = foreign investor volume, 7 = domestic fund volume, 11 = dealer volume.
		// Values are in share counts; convert to hundred-million units by dividing by 1e8.
		var totalForeign, totalDomestic, totalDealer float64
		for _, row := range apiResp.Data {
			if len(row) < 12 {
				continue
			}
			foreign := parseTWDVolume(row[4])
			domestic := parseTWDVolume(row[7])
			dealer := parseTWDVolume(row[11])
			totalForeign += foreign
			totalDomestic += domestic
			totalDealer += dealer
		}

		flow := &CapitalFlowData{
			Date:               dateStr,
			ForeignInvestorNet: totalForeign / 1e8,
			DomesticFundNet:    totalDomestic / 1e8,
			DealerNet:          totalDealer / 1e8,
			TotalNet:           (totalForeign + totalDomestic + totalDealer) / 1e8,
		}
		return flow, nil
	}

	return nil, fmt.Errorf("failed after 3 retries: %w", lastErr)
}

func parseTWDVolume(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func saveCapitalFlowData(path string, data *CapitalFlowData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, jsonData, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Validate file immediately after write to catch corruption.
	if err := validateCapitalFlowFile(path); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return nil
}

func validateCapitalFlowFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var flow CapitalFlowData
	if err := json.Unmarshal(data, &flow); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	if flow.Date == "" {
		return fmt.Errorf("missing date field")
	}

	return nil
}
