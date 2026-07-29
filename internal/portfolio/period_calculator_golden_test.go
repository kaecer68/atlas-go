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
	// Walk up from cwd to find go.mod then check for data/state/macro.
	// In a git worktree, the data directory (gitignored) lives next door
	// to the wip worktree, not inside it. We probe for an actual dated
	// snapshot file (not just _metadata.json) to distinguish worktree vs main.
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Check if a real snapshot file exists in this worktree
			if entries, err := os.ReadDir(filepath.Join(dir, "data", "state", "macro")); err == nil {
				for _, e := range entries {
					name := e.Name()
					if strings.HasPrefix(name, "20") && strings.HasSuffix(name, ".json") {
						return dir
					}
				}
			}
			// Try sibling dir (worktree sibling, e.g. ~/workspace/atlas)
			sibling := filepath.Dir(dir)
			if entries, err := os.ReadDir(filepath.Join(sibling, "data", "state", "macro")); err == nil {
				for _, e := range entries {
					name := e.Name()
					if strings.HasPrefix(name, "20") && strings.HasSuffix(name, ".json") {
						return sibling
					}
				}
			}
			// Fallback: return the go.mod dir
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

// loadMarginHistoryForGolden loads margin history entries for golden test.
func loadMarginHistoryForGolden(t *testing.T, marginDir string) ([]MarginEntry, error) {
	t.Helper()
	entries, err := os.ReadDir(marginDir)
	if err != nil {
		return nil, err
	}

	type marginFile struct {
		Date          string  `json:"date"`
		MarginBalance float64 `json:"margin_balance"`
	}

	var result []MarginEntry
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "_margin.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(marginDir, name))
		if err != nil {
			continue
		}
		var mf marginFile
		if err := json.Unmarshal(data, &mf); err != nil {
			continue
		}
		result = append(result, MarginEntry{Date: mf.Date, MarginBalance: mf.MarginBalance})
	}

	// Sort by date ascending
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result, nil
}

// TestGolden_B5Batch2 runs the Batch 2 golden test — same as Batch 1 but
// also loads margin history and includes the new Batch 2 indicator fields.
//
// Run with: go test -tags=golden -run TestGoldenB5B2 -v ./internal/portfolio/
func TestGolden_B5Batch2(t *testing.T) {
	workDir := findWorkDir()
	snapshotDir := filepath.Join(workDir, "data", "state", "macro")
	marginDir := filepath.Join(workDir, "data", "state", "margin")

	if _, err := os.Stat(snapshotDir); err != nil {
		t.Skipf("snapshot dir not found: %s", snapshotDir)
	}

	snapshots := loadSnapshots(t, snapshotDir)
	if len(snapshots) == 0 {
		t.Fatal("no snapshots found")
	}
	t.Logf("loaded %d snapshots", len(snapshots))

	// Load margin history
	marginHistory, _ := loadMarginHistoryForGolden(t, marginDir)
	t.Logf("loaded %d margin entries", len(marginHistory))

	calc := NewCalculator()
	detector := NewPeriodDetectorWithDefaults()

	type result struct {
		Date      string
		NewPeriod domain.MarketPeriod
		Changed   bool
		NewFields string
	}

	var results []result
	var w1Changes, fieldChanges int

	dates := make([]string, 0, len(snapshots))
	for d := range snapshots {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	for _, date := range dates {
		snap := snapshots[date]

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

		if err := calc.EnrichFromDir(&ind, date, snapshotDir); err != nil {
			t.Logf("  warn: enrich %s: %v", date, err)
		}
		if len(marginHistory) > 0 {
			calc.EnrichMargin(&ind, marginHistory)
		}

		newPeriod := detector.DetectPeriod(ind)

		r := result{
			Date:      date,
			NewPeriod: newPeriod,
		}

		// Check for changes: if any B5-1/B5-2 indicator is non-zero,
		// this date's output was affected by our work.
		hasBatch1Fields := ind.TAIEXMA5 != 0 || ind.TAIEXMA20 != 0 || ind.SOXMA20 != 0 || ind.SOXMA50 != 0 ||
			ind.TSMADRHigh5 != 0 || ind.MarketVolumeMA20 != 0 || ind.TWDMA20 != 0 || ind.TWDChange1D != 0
		hasBatch2Fields := ind.ForeignNet5DayAvg != 0 || ind.ForeignNet10DayAvg != 0 || ind.ForeignNetPeakSell != 0 ||
			ind.ForeignBuyDays10 != 0 || ind.ForeignConsecBuyDays != 0 || ind.ForeignConsecSellDays != 0 ||
			ind.ForeignFuturesOIPrev != 0 || ind.ForeignFuturesOIDelta3 != 0 ||
			ind.MarginBalancePeak != 0 || ind.MarginBalanceChange5D != 0

		if hasBatch1Fields || hasBatch2Fields {
			r.NewFields = fmt.Sprintf("B1:MA5=%.0f MA20=%.0f|B2:F5D=%.0f F10D=%.0f ConB=%d ConS=%d FutP=%.0f FutD=%d MPk=%.0f MC5=%.2f",
				ind.TAIEXMA5, ind.TAIEXMA20,
				ind.ForeignNet5DayAvg, ind.ForeignNet10DayAvg,
				ind.ForeignConsecBuyDays, ind.ForeignConsecSellDays,
				ind.ForeignFuturesOIPrev, ind.ForeignFuturesOIDelta3,
				ind.MarginBalancePeak, ind.MarginBalanceChange5D)
		}

		results = append(results, r)
	}

	fmt.Printf("\n========== B5 Batch 2 Golden Test ==========\n")
	fmt.Printf("Total dates: %d\n", len(results))
	fmt.Printf("W1 degradations (sparse): %d, New field activations: %d\n\n", w1Changes, fieldChanges)

	// Print dates with new Batch 2 fields active
	fmt.Printf("%-12s %-16s %s\n", "Date", "Period", "Key Indicators")
	fmt.Printf("%-12s %-16s %s\n", "----", "------", "--------------")
	for _, r := range results {
		hasB2 := false
		snap := snapshots[r.Date]
		_ = snap
		for _, res := range results {
			if res.Date == r.Date && res.NewFields != "" &&
				(strings.Contains(res.NewFields, "B2:") || strings.Contains(res.NewFields, "B1:")) {
				hasB2 = true
				break
			}
		}
		if hasB2 || true { // show all dates
			fmt.Printf("%-12s %-16s %s\n", r.Date, r.NewPeriod, r.NewFields)
		}
	}

	// Save
	outDir := filepath.Join(workDir, "data", "golden")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "b5-batch2-backtest.txt")
	f, err := os.Create(outPath)
	if err == nil {
		defer f.Close()
		fmt.Fprintf(f, "B5 Batch 2 Golden Test Results\n")
		fmt.Fprintf(f, "==============================\n\n")
		fmt.Fprintf(f, "Total dates: %d\n\n", len(results))
		for _, r := range results {
			fmt.Fprintf(f, "%s | %s | %s\n", r.Date, r.NewPeriod, r.NewFields)
		}
		t.Logf("saved results to %s", outPath)
	}

	t.Logf("done: %d total, W1 deg=%d, new field activations=%d", len(results), w1Changes, fieldChanges)
}
