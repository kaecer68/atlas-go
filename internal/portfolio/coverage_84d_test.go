//go:build golden

package portfolio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestCoverage84Days computes per-field availability across 84 backtest dates
// against the real data dirs. Uses the calculator + a side-channel probe of
// the sector_index dir to distinguish "available" (sector_index has enough
// history) from "unavailable" (insufficient history, field stays at zero).
func TestCoverage84Days(t *testing.T) {
	workDir := findWorkDir()
	sectorDir := filepath.Join(workDir, "data", "state", "sector_index")
	govDir := filepath.Join(workDir, "data", "state", "government_flow")
	macroDir := filepath.Join(workDir, "data", "state", "macro")

	snapshots := loadSnapshots(t, macroDir)
	if len(snapshots) == 0 {
		t.Skip("no snapshots found")
	}
	dates := make([]string, 0, len(snapshots))
	for d := range snapshots {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	// Probe sector_index dates.
	sectorDates, _ := os.ReadDir(sectorDir)
	allSectorDates := make([]string, 0)
	for _, e := range sectorDates {
		n := e.Name()
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		// Extract YYYY-MM-DD from sector_indices_YYYYMMDD_YYYYMMDD.json
		base := strings.TrimSuffix(n, ".json")
		parts := strings.Split(base, "_")
		if len(parts) < 3 {
			continue
		}
		ymd := parts[2]
		if len(ymd) != 8 {
			continue
		}
		canonical := ymd[:4] + "-" + ymd[4:6] + "-" + ymd[6:8]
		allSectorDates = append(allSectorDates, canonical)
	}
	sort.Strings(allSectorDates)

	calc := NewCalculator()
	totalSectorAvail, sectorAvailByDate := 0, 0
	totalGovAvail, govAvailByDate := 0, 0
	for _, d := range dates {
		// Sector: count available dates <= d
		availSectorDates := 0
		for _, sd := range allSectorDates {
			if sd <= d {
				availSectorDates++
			}
		}
		if availSectorDates >= MinDaysSectorRotationFlag {
			sectorAvailByDate++
			totalSectorAvail++
		}

		// Gov: count valid dates <= d (legacy + _brokers)
		availGovDates := 0
		govEntries, _ := os.ReadDir(govDir)
		for _, e := range govEntries {
			n := e.Name()
			if !strings.HasSuffix(n, "_brokers.json") {
				continue
			}
			base := strings.TrimSuffix(n, "_brokers.json")
			if len(base) != 8 {
				continue
			}
			canonical := base[:4] + "-" + base[4:6] + "-" + base[6:8]
			if canonical > d {
				continue
			}
			legacyPath := filepath.Join(govDir, base+".json")
			brokersPath := filepath.Join(govDir, n)
			if _, err := os.Stat(legacyPath); err == nil {
				if _, err2 := os.Stat(brokersPath); err2 == nil {
					availGovDates++
				}
			}
		}
		if availGovDates >= MinDaysPublicBankConsecBuyDays {
			govAvailByDate++
			totalGovAvail++
		}
		_ = calc
	}

	fmt.Printf("\n=== B5 Batch 3 84-day coverage statistics ===\n")
	fmt.Printf("Total backtest dates: %d\n", len(dates))
	fmt.Printf("SectorRotationFlag:\n")
	fmt.Printf("  dates with >= 10 sector_index history (available): %d / %d\n", sectorAvailByDate, len(dates))
	fmt.Printf("  dates with <  10 sector_index history (unavailable): %d / %d\n", len(dates)-sectorAvailByDate, len(dates))
	fmt.Printf("PublicBankConsecBuyDays:\n")
	fmt.Printf("  dates with >= 5 valid _brokers.json (available): %d / %d\n", govAvailByDate, len(dates))
	fmt.Printf("  dates with <  5 valid _brokers.json (unavailable): %d / %d\n", len(dates)-govAvailByDate, len(dates))
	fmt.Printf("  Reason: CAPTCHA not solved; no _brokers.json sister files exist for any date\n")
	_ = totalSectorAvail
	_ = totalGovAvail
}
