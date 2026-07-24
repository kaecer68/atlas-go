package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: validate-stress-index [--help]")
		fmt.Println("Computes the Taiwan market stress index from replay data.")
		os.Exit(0)
	}

	replayPath := config.GetReplayDataPath("")
	f, err := os.Open(replayPath)
	if err != nil {
		fmt.Println("Error opening CSV:", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	header, err := reader.Read()
	if err != nil {
		fmt.Println("Error reading CSV header:", err)
		os.Exit(1)
	}
	idx := make(map[string]int)
	for i, h := range header {
		idx[h] = i
	}

	type dayData struct {
		date   time.Time
		close  float64
		change float64
	}
	var days []dayData

	for {
		rec, err := reader.Read()
		if err != nil {
			break
		}
		if rec[idx["Code"]] != "0050" {
			continue
		}
		date, _ := time.Parse("2006-01-02", rec[idx["Date"]])
		close, _ := strconv.ParseFloat(rec[idx["Close"]], 64)
		open, _ := strconv.ParseFloat(rec[idx["Open"]], 64)
		days = append(days, dayData{
			date:   date,
			close:  close,
			change: (close - open) / open,
		})
	}

	calc := narrative.NewTaiwanStressCalculator(nil, "")
	geo := geopolitical.GeopoliticalRiskScore{Intensity: 30}

	type result struct {
		date   time.Time
		stress float64
		regime string
		change float64
	}
	var results []result

	for _, d := range days {
		// Generate synthetic macro data that varies by day-of-month to
		// produce a broad spread of stress index outputs, so we can
		// validate that the calculator correctly bins days into
		// low/alert/high/crisis regimes.
		//
		// Design rationale for each synthetic factor:
		//   DXY: V-shape peaking at day 15 (range 0-30% change)
		//        -> low stress early/late month, peaks mid-month
		//        math.Abs(dayOffset-15) * 2.0
		//   US10Y: Sinusoidal oscillation around 6% (range 5-7%)
		//          -> moderate baseline with smooth variation
		//          6.0 + math.Sin(dayOffset*0.5)
		//   VIX: V-shape peaking at day 15 (range 10-60+)
		//        -> drives alert/high regime classification mid-month
		//        10.0 + math.Abs(dayOffset-15)*3.5
		//   ForeignInvestorNet: Negative linear trend offset
		//                       -> foreign selling accelerates through month
		//                       -(dayOffset - 5) * 1.5
		dayOffset := float64(d.date.Day())
		snap := marketdata.MacroDataSnapshot{
			DXY:                marketdata.MacroDataPoint{ChangePct: math.Abs(dayOffset-15) * 2.0},
			US10Y:              marketdata.MacroDataPoint{Value: 6.0 + math.Sin(dayOffset*0.5)},
			VIX:                marketdata.MacroDataPoint{Value: 10.0 + math.Abs(dayOffset-15)*3.5},
			ForeignInvestorNet: marketdata.MacroDataPoint{Value: -(dayOffset - 5) * 1.5},
		}
		idx := calc.Calculate(snap, marketdata.MacroDataSnapshot{}, geo)
		results = append(results, result{
			date:   d.date,
			stress: idx.Score,
			regime: idx.Regime,
			change: d.change,
		})
	}

	buckets := map[string][]float64{
		"low":    {},
		"alert":  {},
		"high":   {},
		"crisis": {},
	}
	for _, r := range results {
		buckets[r.regime] = append(buckets[r.regime], r.change)
	}

	fmt.Println("=== Taiwan Stress Index Validation ===")
	fmt.Printf("Days analyzed: %d\n\n", len(results))
	for _, name := range []string{"low", "alert", "high", "crisis"} {
		vals := buckets[name]
		if len(vals) == 0 {
			fmt.Printf("Regime %-6s: 0 days\n", name)
			continue
		}
		up := countPositive(vals)
		fmt.Printf(
			"Regime %-6s: %2d days | up: %2d (%5.1f%%) | down: %2d (%5.1f%%) | avg return: %+.2f%%\n",
			name, len(vals), up, float64(up)/float64(len(vals))*100,
			len(vals)-up, float64(len(vals)-up)/float64(len(vals))*100,
			mean(vals)*100,
		)
	}
	fmt.Println()
	fmt.Println("Correlation interpretation:")
	fmt.Println("  - 'high' / 'crisis' regimes should show lower up-ratio than 'low'.")
	fmt.Println("  - If not, the scoring weights or macro inputs may need recalibration.")
}

func countPositive(v []float64) int {
	c := 0
	for _, x := range v {
		if x > 0 {
			c++
		}
	}
	return c
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
