//go:build golden

package portfolio

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

// TestChangedDayDetail shows the full PeriodAssessment (period, confidence,
// conditions_hit/total, all 5 triggered_indicators) for every date that
// differs from the period_history DB. This is the E3-b deliverable.
func TestChangedDayDetail(t *testing.T) {
	workDir := findWorkDir()
	snapshotDir := filepath.Join(workDir, "data", "state", "macro")
	dbPath := filepath.Join(workDir, "data", "state", "atlas.db")
	sectorIndexDir := filepath.Join(workDir, "data", "state", "sector_index")
	govFlowDir := filepath.Join(workDir, "data", "state", "government_flow")

	snapshots := loadSnapshots(t, snapshotDir)
	if len(snapshots) == 0 {
		t.Fatal("no snapshots found")
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	periodHistory := loadPeriodHistory(t, db)

	calc := NewCalculator()
	detector := NewPeriodDetectorWithDefaults()

	dates := make([]string, 0, len(snapshots))
	for d := range snapshots {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	fmt.Printf("\n========== Changed day detail (assessment) ==========\n")
	for _, d := range dates {
		snap := snapshots[d]
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
		if err := calc.EnrichFromDir(&ind, d, snapshotDir); err != nil {
			t.Logf("warn: enrich %s: %v", d, err)
		}
		calc.EnrichBatch3(&ind, d, sectorIndexDir, govFlowDir)
		newPeriod := detector.DetectPeriod(ind)
		oldPeriod, hasOld := periodHistory[d]
		if hasOld && oldPeriod != "" && string(newPeriod) != oldPeriod {
			assess, _ := detector.DetectAssessment(ind)
			fmt.Printf("\n--- Date: %s ---\n", d)
			fmt.Printf("Old Period (DB): %s\n", oldPeriod)
			fmt.Printf("New Period:       %s\n", assess.MarketPeriod)
			fmt.Printf("Confidence:       %.2f (hits %d/%d)\n", assess.Confidence, assess.ConditionsHit, assess.ConditionsTotal)
			fmt.Printf("\n%-4s %-28s %-14s %-14s %-10s %s\n", "Hit", "Name", "Value", "Threshold", "Relation", "InputAvail")
			for _, ti := range assess.TriggeredIndicators {
				hitMark := " "
				if ti.Hit {
					hitMark = "YES"
				}
				fmt.Printf("%-4s %-28s %-14.4f %-14.4f %-10s %v\n",
					hitMark, ti.Name, ti.Value, ti.Threshold, ti.Relation, ti.InputAvailable)
			}
		}
	}
	_ = json.Marshal // keep import
}
