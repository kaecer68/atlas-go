// Command backfill-margin-history backfills TWSE margin balance history into
// data/state/margin/<YYYYMMDD>_margin.json for a date range.
//
// It drives the same narrative.MarginHistoryBackfiller used by the daily
// margin_history_backfill task, with an explicit start date, bounded retries
// (≤3) and a configurable throttle (≥1s/req). This extends the golden-test
// margin window (MPk/MC5) back to 2024-07-01.
//
// Usage:
//
//	backfill-margin-history -workdir . -start 2024-07-01 -end 2026-08-21
//	backfill-margin-history -workdir . -start 2024-07-01 -days 5   # smoke run
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func main() {
	var (
		workDir  = flag.String("workdir", ".", "atlas work directory (repo root)")
		startStr = flag.String("start", "2024-07-01", "backfill start date YYYY-MM-DD (inclusive)")
		endStr   = flag.String("end", "", "backfill end date YYYY-MM-DD (inclusive; default: today)")
		days     = flag.Int("days", 0, "limit to first N calendar days from start (smoke validation)")
		retries  = flag.Int("retries", 3, "max fetch retries per date (capped at 3)")
		throttle = flag.Duration("throttle", 5*time.Second, "minimum interval between TWSE requests (>= 1s)")
	)
	flag.Parse()

	if *retries < 0 {
		log.Fatalf("retries must be >= 0, got %d", *retries)
	}
	if *throttle < time.Second {
		log.Fatalf("throttle must be >= 1s (got %v)", *throttle)
	}

	start, err := time.ParseInLocation("2006-01-02", *startStr, time.Local)
	if err != nil {
		log.Fatalf("parse start %q: %v", *startStr, err)
	}
	end := time.Now()
	if *endStr != "" {
		end, err = time.ParseInLocation("2006-01-02", *endStr, time.Local)
		if err != nil {
			log.Fatalf("parse end %q: %v", *endStr, err)
		}
	}
	if *days > 0 {
		end = start.AddDate(0, 0, *days-1)
	}

	bf := narrative.NewMarginHistoryBackfiller(*workDir)
	bf.StartDate = start
	bf.EndDate = end
	bf.MaxRetries = *retries
	// TWSE HiNetCDN issues a __chtcdn challenge cookie after bursts of
	// cookie-less requests (HTTP 428/403). A jar keeps the cookie across
	// requests so a long backfill is not blocked.
	if jar, err := cookiejar.New(nil); err == nil {
		client := httpclient.NewFactory().NewClient(30 * time.Second)
		client.Jar = jar
		bf.Provider.SetHTTPClient(client)
	}
	// One-shot backfill may use a tighter throttle than the production 5s
	// default; the contract requires >= 1s/req.
	bf.Provider.SetRateLimiter(rate.NewLimiter(rate.Every(*throttle), 1))

	log.Printf("margin-history-backfill: window %s..%s workdir=%s retries=%d throttle=%v",
		start.Format("2006-01-02"), end.Format("2006-01-02"), *workDir, *retries, *throttle)

	if err := bf.Backfill(context.Background()); err != nil {
		log.Fatalf("margin-history-backfill: %v", err)
	}

	summary(marginDir(*workDir))
}

func marginDir(workDir string) string {
	return filepath.Join(workDir, "data", "state", "margin")
}

// summary prints the on-disk margin file count plus first/last date.
func summary(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("margin-history-backfill: read %s: %v", dir, err)
		return
	}
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "_margin.json") {
			continue
		}
		d := strings.TrimSuffix(name, "_margin.json")
		if len(d) == 8 {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	if len(dates) == 0 {
		fmt.Printf("margin files: 0\n")
		return
	}
	fmt.Printf("margin files: %d (first=%s last=%s)\n", len(dates), dates[0], dates[len(dates)-1])
}
