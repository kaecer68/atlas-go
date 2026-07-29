//go:build golden

package portfolio

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestGolden_BacktestAllDates reads all macro snapshots, enriches them with
// the Batch 1 calculator, runs period detection, and compares against the
// period_history table in data/state/atlas.db.
//
// Run with: go test -tags=golden -run TestGolden -v ./internal/portfolio/
//
// Output is a comparison table printed to stdout and saved to
// data/golden/b5-batch1-backtest.txt.
func TestGolden_BacktestAllDates(t *testing.T) {
	workDir := findWorkDir()
	snapshotDir := filepath.Join(workDir, "data", "state", "macro")
	dbPath := filepath.Join(workDir, "data", "state", "atlas.db")

	if _, err := os.Stat(snapshotDir); err != nil {
		t.Skipf("snapshot dir not found: %s", snapshotDir)
	}

	// Load all snapshot dates.
	snapshots := loadSnapshots(t, snapshotDir)
	if len(snapshots) == 0 {
		t.Fatal("no snapshots found")
	}
	t.Logf("loaded %d snapshots", len(snapshots))

	// Open SQLite and load period_history.
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	periodHistory := loadPeriodHistory(t, db)
	t.Logf("loaded %d period_history rows", len(periodHistory))

	// Compute periods for each date that has enough history.
	calc := NewCalculator()
	detector := NewPeriodDetectorWithDefaults()

	type result struct {
		Date      string
		NewPeriod domain.MarketPeriod
		OldPeriod string
		Changed   bool
		NewFields string // key indicator values
	}

	var results []result
	changeCount := 0
	unknownCount := 0

	// Sort dates for stable output.
	dates := make([]string, 0, len(snapshots))
	for d := range snapshots {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	for _, date := range dates {
		snap := snapshots[date]

		// Build period indicators from single snapshot.
		ind := PeriodIndicators{
			VIX:                    snap.VIX.Value,
			DXY:                    snap.DXY.Value,
			US10Y:                  snap.US10Y.Value,
			SOXPrice:               snap.SOXIndex.Value,
			TSMADRPrice:            snap.TSMADR.Value,
			TAIEXPrice:             snap.TAIEX.Value,
			ForeignSingleDayNet:    snap.ForeignInvestorNet.Value,
			ForeignFuturesOI:       snap.ForeignFuturesOINet.Value,
			MarginBalance:          snap.RetailMarginBalance.Value,
			MarginMaintenanceRatio: snap.MarginMaintenanceRatio.Value,
			MarketVolume:           snap.MarketVolume.Value,
			DayTradeRatio:          snap.DayTradeRatio.Value,
		}

		// Enrich with historical data.
		if err := calc.EnrichFromDir(&ind, date, snapshotDir); err != nil {
			t.Logf("  warn: enrich %s: %v", date, err)
		}

		// Detect period.
		newPeriod := detector.DetectPeriod(ind)

		// Get old period.
		oldPeriod, hasOld := periodHistory[date]

		r := result{
			Date:      date,
			NewPeriod: newPeriod,
			OldPeriod: oldPeriod,
		}

		if hasOld && oldPeriod != "" && string(newPeriod) != oldPeriod {
			r.Changed = true
			changeCount++
			r.NewFields = fmt.Sprintf("MA5=%.0f MA20=%.0f S20=%.0f S50=%.0f THigh5=%.1f VMA20=%.0f TWDMA20=%.2f TWD1D=%.2f TWD5D=%.2f",
				ind.TAIEXMA5, ind.TAIEXMA20, ind.SOXMA20, ind.SOXMA50,
				ind.TSMADRHigh5, ind.MarketVolumeMA20, ind.TWDMA20,
				ind.TWDChange1D, ind.TWDChange5D)
		}
		if !hasOld || oldPeriod == "" {
			unknownCount++
		}
		results = append(results, r)
	}

	// Print comparison table.
	fmt.Printf("\n========== B5 Batch 1 Golden Test ==========\n")
	fmt.Printf("Total dates: %d\n", len(results))
	fmt.Printf("Changed: %d, Unknown (no history): %d, Unchanged: %d\n\n",
		changeCount, unknownCount, len(results)-changeCount-unknownCount)

	if changeCount > 0 {
		fmt.Printf("%-12s %-16s %-16s %s\n", "Date", "Old Period", "New Period", "Indicators")
		fmt.Printf("%-12s %-16s %-16s %s\n", "----", "----------", "----------", "----------")
		for _, r := range results {
			if r.Changed {
				fmt.Printf("%-12s %-16s %-16s %s\n", r.Date, r.OldPeriod, r.NewPeriod, r.NewFields)
			}
		}
	} else {
		fmt.Println("No period changes detected — all periods match period_history DB.")
	}

	// Also print the last 5 dates for spot check.
	fmt.Printf("\n--- Last 5 Dates (Spot Check) ---\n")
	start := len(results) - 5
	if start < 0 {
		start = 0
	}
	for _, r := range results[start:] {
		changed := ""
		if r.Changed {
			changed = " ** CHANGED **"
		}
		old := r.OldPeriod
		if old == "" {
			old = "(none)"
		}
		fmt.Printf("  %s  old=%-12s  new=%-12s%s\n", r.Date, old, r.NewPeriod, changed)
	}

	// Save to file.
	outDir := filepath.Join(workDir, "data", "golden")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "b5-batch1-backtest.txt")
	f, err := os.Create(outPath)
	if err == nil {
		defer f.Close()
		fmt.Fprintf(f, "B5 Batch 1 Golden Test Results\n")
		fmt.Fprintf(f, "==============================\n\n")
		fmt.Fprintf(f, "Total dates: %d\nChanged: %d\nUnknown: %d\n\n", len(results), changeCount, unknownCount)
		for _, r := range results {
			if r.Changed {
				fmt.Fprintf(f, "%s | %s → %s | %s\n", r.Date, r.OldPeriod, r.NewPeriod, r.NewFields)
			}
		}
		t.Logf("saved results to %s", outPath)
	}

	t.Logf("done: %d total, %d changed, %d unknown", len(results), changeCount, unknownCount)
}

// ── helpers ──

func findWorkDir() string {
	// Walk up from cwd to find go.mod.
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

func loadSnapshots(t *testing.T, dir string) map[string]marketdata.MacroDataSnapshot {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	result := make(map[string]marketdata.MacroDataSnapshot)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "20") || !strings.HasSuffix(name, ".json") {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if len(date) != 10 || date[4] != '-' || date[7] != '-' {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		result[date] = snap
	}
	return result
}

func loadPeriodHistory(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query("SELECT date, period FROM period_history ORDER BY date")
	if err != nil {
		t.Logf("warn: query period_history: %v (table may not exist yet)", err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var date, period string
		if err := rows.Scan(&date, &period); err != nil {
			t.Logf("warn: scan row: %v", err)
			continue
		}
		result[date] = period
	}
	return result
}
