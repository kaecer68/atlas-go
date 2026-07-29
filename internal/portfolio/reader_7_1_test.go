//go:build golden

package portfolio

import (
	"fmt"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestReader_7_1_CrossFile shows the SectorIndexReader output for 2026-07-01
// when both the 18-industry batch file (sector_indices_20260701_20260710.json)
// and the 8-industry single-day file (sector_indices_20260701_20260701.json)
// are present in the same directory. This documents the cross-file
// averaging behavior that the source-priority refactor in W1 will replace.
func TestReader_7_1_CrossFile(t *testing.T) {
	workDir := findWorkDir()
	sectorDir := workDir + "/data/state/sector_index"
	reader := marketdata.NewSectorIndexReader(sectorDir)
	returns, ok, err := reader.Get("2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("no data for 2026-07-01")
	}
	fmt.Printf("\n=== SectorIndexReader.Get(2026-07-01) — before W1 priority refactor ===\n")
	fmt.Printf("Cross-file average (8 產業 mapped into 18 產業 schema, averaged):\n")
	for _, ind := range []string{"electronics", "semiconductor", "financials", "machinery", "energy", "shipping"} {
		if v, ok := returns[ind]; ok {
			fmt.Printf("  %-20s = %.4f\n", ind, v)
		}
	}
}
