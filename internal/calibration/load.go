package calibration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/pricegate"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// LoadReturns loads a return series from a JSONL or CSV replay file.
func LoadReturns(dataPath string) ([]float64, int, error) {
	ext := strings.ToLower(filepath.Ext(dataPath))

	var returns []float64

	if ext == ".jsonl" || ext == ".json" {
		returns, _ = LoadReturnsFromJSONL(dataPath)
	}

	if len(returns) == 0 {
		var err error
		returns, _, err = LoadReturnsFromCSV(dataPath)
		if err != nil {
			return nil, 0, fmt.Errorf("load from %s: %w", dataPath, err)
		}
	}

	if len(returns) < 30 {
		return nil, len(returns), fmt.Errorf("insufficient data: got %d returns, need at least 30", len(returns))
	}

	return returns, len(returns), nil
}

// LoadReturnsFromJSONL reads return values from a JSONL file where each line
// contains {"return": <value>}. Invalid lines and non-finite values are skipped.
func LoadReturnsFromJSONL(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	type Outcome struct {
		Return float64 `json:"return"`
	}

	var returns []float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var out Outcome
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			continue
		}
		if !math.IsNaN(out.Return) && !math.IsInf(out.Return, 0) {
			returns = append(returns, out.Return)
		}
	}
	return returns, nil
}

// LoadReturnsFromCSV reads TWSE open-data CSV and computes daily returns for
// the symbol with the most complete price history.
func LoadReturnsFromCSV(path string) ([]float64, int, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(path)
	if err != nil {
		return nil, 0, err
	}

	var bestSym string
	var bestCount int
	if len(ds.Dates) > 0 {
		for sym := range ds.ByDate[ds.Dates[0].Format("2006-01-02")] {
			count := 0
			for _, d := range ds.Dates {
				bar, ok := ds.ByDate[d.Format("2006-01-02")][sym]
				if ok && bar.Close > 0 {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestSym = sym
			}
		}
	}

	if bestSym == "" {
		return nil, 0, fmt.Errorf("no valid bars found")
	}

	bestCount = 0
	for _, d := range ds.Dates {
		bar, ok := ds.ByDate[d.Format("2006-01-02")][bestSym]
		if ok && bar.Close > 0 {
			bestCount++
		}
	}

	var returns []float64
	var prevClose float64
	// obs counts the symbol's usable closes seen so far, which is the position
	// used for the IPO exemption: date gaps must not turn a new listing's first
	// sessions into "not exempt" (k3 decision 3-1).
	obs := 0
	for _, date := range ds.Dates {
		bar, ok := ds.ByDate[date.Format("2006-01-02")][bestSym]
		// Non-positive/non-finite closes are missing observations, not prices
		// (FU-20260930-04): `== 0` alone let a negative close through and
		// produced a nonsensical label.
		if !ok || !shared.IsUsablePrice(bar.Close) {
			continue
		}
		obs++
		if prevClose > 0 {
			// The gate also drops a corrupted multiple (a 10x close), which
			// both closes being usable prices cannot reveal (k3 decision 3-1).
			if pricegate.Check("calibration.label_return", bestSym, prevClose, bar.Close, obs-1, 1) {
				returns = append(returns, (bar.Close-prevClose)/prevClose)
			}
		}
		prevClose = bar.Close
	}

	return returns, bestCount, nil
}
