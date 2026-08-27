// Per-symbol flow file storage for backfill-stockpicker-flows.
//
// The on-disk format mirrors cmd/run-stockpicker-backtest/panel.go:
// {"symbol":"2330","flows":[{"date":"2026-01-05","foreign_net":1500},...]}
// with FlowPoint shared from internal/stockpicker so JSON tags can never
// drift from what the backtest panel reads.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// flowFile is the on-disk shape of data/state/stock_flows/<symbol>.json.
type flowFile struct {
	Symbol string                  `json:"symbol"`
	Flows  []stockpicker.FlowPoint `json:"flows"`
}

// loadFlowFile reads a symbol flow file; a missing file yields an empty file.
func loadFlowFile(path string) (flowFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return flowFile{}, nil
		}
		return flowFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	var f flowFile
	if err := json.Unmarshal(data, &f); err != nil {
		return flowFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return f, nil
}

// mergeSymbolFile loads <symbol>.json, merges one day of flows keyed by
// date (idempotent: an existing date is overwritten, never duplicated),
// sorts by date, and atomically rewrites the file. It returns the number
// of newly added flow points and the file's total flow-point count.
func mergeSymbolFile(flowsDir, symbol string, dayFlows []marketdata.SymbolFlow) (added, total int, err error) {
	path := filepath.Join(flowsDir, symbol+".json")
	existing, err := loadFlowFile(path)
	if err != nil {
		return 0, 0, err
	}
	byDate := make(map[string]float64, len(existing.Flows)+len(dayFlows))
	for _, f := range existing.Flows {
		byDate[f.Date] = f.ForeignNet
	}
	for _, f := range dayFlows {
		d := toISODate(f.Date)
		if _, ok := byDate[d]; !ok {
			added++
		}
		byDate[d] = f.ForeignInvestorNet
	}

	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	flows := make([]stockpicker.FlowPoint, 0, len(dates))
	for _, d := range dates {
		flows = append(flows, stockpicker.FlowPoint{Date: d, ForeignNet: byDate[d]})
	}
	out := flowFile{Symbol: symbol, Flows: flows}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return 0, 0, fmt.Errorf("marshal %s: %w", symbol, err)
	}
	if err := atomicWrite(path, data); err != nil {
		return 0, 0, fmt.Errorf("write %s: %w", symbol, err)
	}
	return added, len(flows), nil
}

// verifyFiles re-reads up to three written symbol files and checks they
// parse, carry the right symbol, and keep strictly ascending dates.
func verifyFiles(flowsDir string, written map[string]bool) error {
	if len(written) == 0 {
		return nil
	}
	syms := sortedMapKeys(written)
	n := min(3, len(syms))
	for _, sym := range syms[:n] {
		f, err := loadFlowFile(filepath.Join(flowsDir, sym+".json"))
		if err != nil {
			return fmt.Errorf("verify %s: %w", sym, err)
		}
		if f.Symbol != sym || len(f.Flows) == 0 {
			return fmt.Errorf("verify %s: symbol=%q flows=%d", sym, f.Symbol, len(f.Flows))
		}
		for i := 1; i < len(f.Flows); i++ {
			if f.Flows[i-1].Date >= f.Flows[i].Date {
				return fmt.Errorf("verify %s: flows not strictly sorted at %s", sym, f.Flows[i].Date)
			}
		}
		fmt.Printf("verify %s.json: flows=%d sorted ok\n", sym, len(f.Flows))
	}
	return nil
}

// countFlowPoints sums the flow points across all symbol files on disk.
func countFlowPoints(flowsDir string) (int, error) {
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		return 0, fmt.Errorf("read flows dir: %w", err)
	}
	total := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		f, err := loadFlowFile(filepath.Join(flowsDir, e.Name()))
		if err != nil {
			return 0, err
		}
		total += len(f.Flows)
	}
	return total, nil
}

// atomicWrite writes data to path via a temp file + rename so a crash never
// leaves a half-written symbol file.
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// toISODate converts the provider's YYYYMMDD date to the file's YYYY-MM-DD.
func toISODate(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return yyyymmdd
	}
	return yyyymmdd[0:4] + "-" + yyyymmdd[4:6] + "-" + yyyymmdd[6:8]
}
