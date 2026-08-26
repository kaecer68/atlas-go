// Command backfill-govflow-histock backfills daily government-bank flow
// readings from the HiStock broker8 page into data/state/government_flow/.
//
// Root-cause context (2026-08-26): the legacy TWSE bsr scraper never produced
// non-zero data, so the government force's 60-day rolling z-score has no
// history. HiStock supports ?d=YYYY/MM/DD back to ~2024-06; this CLI walks a
// date range and writes the standard YYYYMMDD.json + YYYYMMDD_brokers.json
// files via GovernmentBrokerAggregator.AggregateDate (same code path as the
// production channel), skipping weekends and already-present files.
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
	startStr := flag.String("start", "", "start date YYYYMMDD (required)")
	endStr := flag.String("end", time.Now().Format("20060102"), "end date YYYYMMDD (inclusive)")
	dir := flag.String("dir", "data/state/government_flow", "output directory")
	force := flag.Bool("force", false, "refetch even if the reading file exists")
	gap := flag.Duration("gap", 3*time.Second, "sleep between upstream requests (be polite)")
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

	agg := marketdata.NewGovernmentBrokerAggregator(*dir)

	var okDays, emptyDays, failDays, skipDays int
	ctx := context.Background()
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
		reading, err := agg.AggregateDate(ctx, d)
		if err != nil {
			failDays++
			logging.Error("backfill-govflow", "day_failed", "date", dateStr, logging.Err(err))
		} else if reading == nil {
			emptyDays++
			fmt.Printf("%s no-data (holiday or not published)\n", dateStr)
		} else {
			okDays++
			fmt.Printf("%s ok total_net=%d source=%s\n", dateStr, reading.TotalNet, reading.Source)
		}
		time.Sleep(*gap)
	}
	fmt.Printf("\ndone: ok=%d empty=%d failed=%d skipped(existing)=%d\n", okDays, emptyDays, failDays, skipDays)
	if failDays > 0 {
		os.Exit(1)
	}
}
