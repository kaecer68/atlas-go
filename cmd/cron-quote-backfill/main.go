package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

var (
	startDate   = flag.String("start", "", "backfill start date (YYYY-MM-DD); default = today - defaultLookbackDays")
	endDate     = flag.String("end", "", "backfill end date (YYYY-MM-DD); default = today")
	symbols     = flag.String("symbols", "", "comma-separated stock IDs; empty = all from fundamentals.json")
	workDir     = flag.String("workdir", ".", "atlas repo root")
	dryRun      = flag.Bool("dry-run", false, "print plan without writing")
	concurrency = flag.Int("concurrency", 1, "symbol workers (1-60; FinMind burst=60 is the upper bound; rate limit is the real bottleneck)")
)

const defaultLookbackDays = 30

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	end := time.Now()
	if *endDate != "" {
		t, err := time.Parse("2006-01-02", *endDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -end: %v\n", err)
			os.Exit(1)
		}
		end = t
	}
	start := end.AddDate(0, 0, -defaultLookbackDays)
	if *startDate != "" {
		t, err := time.Parse("2006-01-02", *startDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -start: %v\n", err)
			os.Exit(1)
		}
		start = t
	}

	cfg := config.Load()
	finmindKey := cfg.FinMindAPIKey
	if finmindKey == "" {
		fmt.Fprintln(os.Stderr, "FINMIND_API_KEY not set; required for quote backfill")
		os.Exit(1)
	}

	syms := splitSymbols(*symbols)
	if len(syms) == 0 {
		syms = loadSymbolsFromFundamentals(filepath.Join(*workDir, "data", "fundamentals.json"))
	}
	if len(syms) == 0 {
		fmt.Fprintln(os.Stderr, "no symbols resolved; pass -symbols or seed data/fundamentals.json")
		os.Exit(1)
	}

	client := marketdata.GetSharedFinMindClient(finmindKey, *workDir)
	store := ledger.NewJSONLQuoteStore(filepath.Join(*workDir, "data", "state", "quotes"))

	conc := min(max(*concurrency, 1), 60)

	days := int(end.Sub(start).Hours()/24) + 1
	calls := len(syms) * days
	estHours := float64(calls) * 6.0 / 3600.0
	fmt.Fprintf(os.Stderr, "Estimated runtime: %d symbols × %d days = %d FinMind calls × ~6s/call = ~%.1f hours (FinMind free tier 600/hr; -concurrency=%d helps only within burst window)\n",
		len(syms), days, calls, estHours, conc)

	fmt.Printf("cron-quote-backfill: %d symbols, %s → %s (dry=%v, concurrency=%d)\n",
		len(syms), start.Format("2006-01-02"), end.Format("2006-01-02"), *dryRun, conc)

	jobs := make(chan string, len(syms))
	for _, sym := range syms {
		jobs <- sym
	}
	close(jobs)

	var wg sync.WaitGroup
	var total atomic.Int64
	for i := 0; i < conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sym := range jobs {
				n, err := backfillSymbol(ctx, client, store, sym, start, end, *dryRun)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %s: %v\n", sym, err)
					continue
				}
				total.Add(int64(n))
			}
		}()
	}
	wg.Wait()
	fmt.Printf("wrote %d bars across %d symbols\n", total.Load(), len(syms))
}

func backfillSymbol(ctx context.Context, c *marketdata.FinMindClient, s ledger.QuoteStore, sym string, start, end time.Time, dry bool) (int, error) {
	var bars []domain.DailyBar
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		// Skip weekends and Taiwan public holidays: FinMind returns the last
		// trading day's quote for a closed-market date, which backfills a
		// "copy of the previous trading day" row and poisons ForwardReturn
		// duplicate detection downstream (2026-08-23 replay bug). No fetch,
		// no write. Same guard as cmd/daily-replay-sync runGapBackfill.
		if !marketdata.IsTaiwanTradingDay(d) {
			continue
		}
		q, err := c.GetStockPrice(ctx, sym, d.Format("2006-01-02"))
		if err != nil {
			return len(bars), fmt.Errorf("%s @ %s: %w", sym, d.Format("2006-01-02"), err)
		}
		if q.Symbol == "" {
			continue
		}
		bars = append(bars, domain.DailyBar{
			Symbol: sym, Date: d, Close: q.Last, Volume: q.Volume,
		})
	}
	if dry || len(bars) == 0 {
		return len(bars), nil
	}
	if err := s.RecordQuotes(bars); err != nil {
		return 0, err
	}
	return len(bars), nil
}

func splitSymbols(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, s := range splitComma(raw) {
		if s = trimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitComma(s string) []string {
	out := []string{}
	start := 0
	for i, r := range s {
		if r == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func loadSymbolsFromFundamentals(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := jsonUnmarshal(data, &m); err != nil {
		return nil
	}
	var out []string
	for k := range m {
		k = trimSuffixTW(k)
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

// jsonUnmarshal wraps encoding/json Unmarshal to keep the file compact.
func jsonUnmarshal(data []byte, v any) error { return jsonImpl(data, v) }

func trimSuffixTW(s string) string {
	if len(s) > 3 && s[len(s)-3:] == ".TW" {
		return s[:len(s)-3]
	}
	return s
}
