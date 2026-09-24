// Per-symbol flow file storage for the stock_flows store.
//
// The on-disk format is stockpicker.FlowFile
// ({"symbol":"2330","flows":[{"date":"2026-01-05","foreign_net":1500},...]});
// the type lives in internal/stockpicker so the JSON tags can never drift
// from what the backtest panel and the win-rate gate read.
package stockflows

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// Dir returns the per-symbol flow store directory under workDir.
func Dir(workDir string) string {
	return filepath.Join(workDir, "data", "state", "stock_flows")
}

// LoadFile reads a symbol flow file; a missing file yields an empty file so
// a first-time merge is a plain create.
func LoadFile(path string) (stockpicker.FlowFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return stockpicker.FlowFile{}, nil
		}
		return stockpicker.FlowFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	var f stockpicker.FlowFile
	if err := json.Unmarshal(data, &f); err != nil {
		return stockpicker.FlowFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return f, nil
}

// MergeSymbolFile loads <symbol>.json, merges one day of flows keyed by date
// (idempotent: an existing date is overwritten, never duplicated), sorts by
// date, and atomically rewrites the file. It returns the number of newly
// added flow points and the file's total flow-point count.
func MergeSymbolFile(flowsDir, symbol string, dayFlows []marketdata.SymbolFlow) (added, total int, err error) {
	path := filepath.Join(flowsDir, symbol+".json")
	existing, err := LoadFile(path)
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
	out := stockpicker.FlowFile{Symbol: symbol, Flows: flows}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return 0, 0, fmt.Errorf("marshal %s: %w", symbol, err)
	}
	if err := AtomicWrite(path, data); err != nil {
		return 0, 0, fmt.Errorf("write %s: %w", path, err)
	}
	return added, len(flows), nil
}

// VerifyFiles re-reads up to three written symbol files and checks they
// parse, carry the right symbol, and keep strictly ascending dates. Each
// verified file is reported through report (nil is allowed).
func VerifyFiles(flowsDir string, written map[string]bool, report func(format string, args ...any)) error {
	if len(written) == 0 {
		return nil
	}
	syms := SortedKeys(written)
	n := min(3, len(syms))
	for _, sym := range syms[:n] {
		f, err := LoadFile(filepath.Join(flowsDir, sym+".json"))
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
		if report != nil {
			report("verify %s.json: flows=%d sorted ok\n", sym, len(f.Flows))
		}
	}
	return nil
}

// CountFlowPoints sums the flow points across all symbol files on disk.
func CountFlowPoints(flowsDir string) (int, error) {
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		return 0, fmt.Errorf("read flows dir: %w", err)
	}
	total := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		f, err := LoadFile(filepath.Join(flowsDir, e.Name()))
		if err != nil {
			return 0, err
		}
		total += len(f.Flows)
	}
	return total, nil
}

// NewestStoredDate returns the newest flow date stored across every symbol
// file in flowsDir; ok is false when the store is empty or missing. The
// daily refresh uses it to fetch only the missing sessions, so a gap left by
// a failed run heals on the next tick instead of growing.
func NewestStoredDate(flowsDir string) (newest time.Time, ok bool, err error) {
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("read flows dir: %w", err)
	}
	newestISO := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		f, err := LoadFile(filepath.Join(flowsDir, e.Name()))
		if err != nil {
			return time.Time{}, false, err
		}
		latest, ok := f.Newest()
		if !ok {
			continue
		}
		if latest.Date > newestISO {
			newestISO = latest.Date
		}
	}
	if newestISO == "" {
		return time.Time{}, false, nil
	}
	d, ok := stockpicker.FlowDate(newestISO)
	if !ok {
		return time.Time{}, false, fmt.Errorf("newest stored flow date %q is malformed", newestISO)
	}
	return d, true, nil
}

// AtomicWrite writes data to path via a temp file + rename so a crash never
// leaves a half-written symbol file.
func AtomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SortedKeys returns the keys of m in ascending order.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// toISODate converts a provider YYYYMMDD date to the store's YYYY-MM-DD
// layout; any other shape is passed through unchanged.
func toISODate(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return yyyymmdd
	}
	return yyyymmdd[0:4] + "-" + yyyymmdd[4:6] + "-" + yyyymmdd[6:8]
}
