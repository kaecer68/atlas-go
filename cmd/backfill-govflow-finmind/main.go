// Command backfill-govflow-finmind backfills daily government-bank flow
// readings for 2021-06-30..2024-12-31 from FinMind's Sponsor-tier
// TaiwanStockGovernmentBankBuySell dataset into data/state/government_flow/.
//
// #1740 D06 R5: HiStock's broker8 page history stops at ~2024-06, so the
// 2021-2024 history must come from FinMind. The dataset does NOT accept
// data_id — one request returns the whole market for a date range — so this
// CLI walks one trading day at a time (weekdays only; holidays return empty
// data and are counted as skipped) and the provider aggregates the
// tw50Symbols universe by bank_name.
//
// Sponsor tier: 6000 req/hr. The provider's own token bucket paces at 600ms
// per request (burst 100) and the finmind_sponsor DailyQuotaTracker gates the
// 144,000/day ceiling; fetchWithRetry + the provider breaker are inherited
// from FinMindClient.fetchDataset. Output files are byte-compatible with the
// HiStock writer: YYYYMMDD.json (GovernmentFlowReading) + YYYYMMDD_brokers.json.
//
// Token: Sponsor token via $FINMIND_TOKEN (falls back to FINMIND_API_KEY).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	startStr := flag.String("start", "20210630", "start date YYYYMMDD (dataset starts 2021-06-30)")
	endStr := flag.String("end", "20241231", "end date YYYYMMDD (inclusive)")
	dir := flag.String("dir", "data/state/government_flow", "output directory")
	stateDir := flag.String("state-dir", "data/state", "state directory for the daily-quota tracker")
	force := flag.Bool("force", false, "refetch even if the reading file exists")
	flag.Parse()

	start, err := time.Parse("20060102", *startStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -start %q: %v\n", *startStr, err)
		os.Exit(2)
	}
	end, err := time.Parse("20060102", *endStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -end %q: %v\n", *endStr, err)
		os.Exit(2)
	}
	if end.Before(start) {
		fmt.Fprintln(os.Stderr, "-end is before -start")
		os.Exit(2)
	}

	token := os.Getenv("FINMIND_TOKEN")
	if token == "" {
		token = os.Getenv("FINMIND_API_KEY")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "FINMIND_TOKEN (Sponsor) not set; required for the GovernmentBankBuySell dataset")
		os.Exit(2)
	}

	p := marketdata.NewFinMindGovernmentBankProvider(token, *stateDir)

	var okDays, emptyDays, failDays, skipDays int
	ctx := context.Background()
	startWall := time.Now()
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		dateStr := d.Format("20060102")
		if !*force {
			if _, err := os.Stat(*dir + "/" + dateStr + ".json"); err == nil {
				skipDays++
				continue
			}
		}
		reading, err := p.BackfillDay(ctx, d, *dir)
		if err != nil {
			failDays++
			logging.Error("backfill-govflow-finmind", "day_failed", "date", dateStr, logging.Err(err))
		} else if reading == nil {
			emptyDays++
			fmt.Printf("%s no-data (holiday or not published)\n", dateStr)
		} else {
			okDays++
			fmt.Printf("%s ok total_net=%d source=%s\n", dateStr, reading.TotalNet, reading.Source)
		}
	}
	fmt.Printf("\ndone: ok=%d empty=%d failed=%d skipped(existing)=%d elapsed=%s\n",
		okDays, emptyDays, failDays, skipDays, time.Since(startWall).Round(time.Second))
	if failDays > 0 {
		os.Exit(1)
	}
}
