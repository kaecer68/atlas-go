package marketdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// canonicalSectorIDs is the stable L1 sector universe used by downstream
// consumers (period detector / calculator). It matches the mapping produced by
// TWSESectorIndexProvider.canonicalL1SectorID.
var canonicalSectorIDs = map[string]bool{
	"auto":              true,
	"biotech":           true,
	"cement":            true,
	"construction":      true,
	"electronics":       true,
	"energy":            true,
	"financials":        true,
	"food":              true,
	"machinery":         true,
	"optoelectronics":   true,
	"other_electronics": true,
	"plastics":          true,
	"retail":            true,
	"semiconductor":     true,
	"shipping":          true,
	"steel":             true,
	"telecom":           true,
	"textiles":          true,
}

// sectorIndexLegacyToCanonical maps the old 8-industry schema IDs to the L1
// canonical IDs. Other IDs are kept as-is if they are already canonical.
var sectorIndexLegacyToCanonical = map[string]string{
	"ai_supply_chain": "electronics",
	"robotics":        "machinery",
}

// canonicalSectorID normalizes a raw sector ID to the canonical L1 set. Unknown
// IDs return an empty string and are dropped by the reader.
func canonicalSectorID(raw string) string {
	id := raw
	if mapped, ok := sectorIndexLegacyToCanonical[id]; ok {
		id = mapped
	}
	if canonicalSectorIDs[id] {
		return id
	}
	return ""
}

// SectorIndexReader reads the sector_index files written by
// TWSESectorIndexProvider and exposes a canonical date×industry×return view.
// It is a file-only reader; it never calls upstream APIs, so it is safe to use
// from period_calculator without circuit-breaker side effects.
type SectorIndexReader struct {
	dir string
}

// NewSectorIndexReader creates a reader rooted at dir.
func NewSectorIndexReader(dir string) *SectorIndexReader {
	return &SectorIndexReader{dir: dir}
}

// ReadRange returns a date-keyed map of canonical industry daily returns for
// all dates in [startDate, endDate] that have data. Missing dates are omitted
// (not zero-filled), so callers can distinguish "no data" from "return = 0".
func (r *SectorIndexReader) ReadRange(startDate, endDate time.Time) (map[string]map[string]float64, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read sector_index dir: %w", err)
	}

	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	result := make(map[string]map[string]float64)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "sector_indices_") || !strings.HasSuffix(name, ".json") {
			continue
		}

		path := filepath.Join(r.dir, name)
		data, err := loadSectorIndexFile(path)
		if err != nil {
			// Logically skip unreadable files; a single corrupt file should not
			// block the rest of the history.
			continue
		}

		for rawIndustry, series := range data {
			industry := canonicalSectorID(rawIndustry)
			if industry == "" {
				continue
			}
			for _, item := range series {
				if item.Date < startStr || item.Date > endStr {
					continue
				}
				if result[item.Date] == nil {
					result[item.Date] = make(map[string]float64)
				}
				// When two raw industries map to the same canonical ID on the same
				// date (e.g. 8-industry schema has both ai_supply_chain and
				// electronics mapping to electronics), average the returns.
				if existing, ok := result[item.Date][industry]; ok {
					result[item.Date][industry] = (existing + item.ReturnPct) / 2
				} else {
					result[item.Date][industry] = item.ReturnPct
				}
			}
		}
	}

	return result, nil
}

// Get returns the canonical industry return map for a single date (YYYY-MM-DD).
// The second return value is false if no data exists for that date.
func (r *SectorIndexReader) Get(date string) (map[string]float64, bool, error) {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, false, fmt.Errorf("parse date: %w", err)
	}
	all, err := r.ReadRange(d, d)
	if err != nil {
		return nil, false, err
	}
	returns, ok := all[date]
	return returns, ok, nil
}

// AvailableDates returns the sorted list of YYYY-MM-DD dates present in the data.
func (r *SectorIndexReader) AvailableDates() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read sector_index dir: %w", err)
	}

	dateSet := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "sector_indices_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(r.dir, name)
		data, err := loadSectorIndexFile(path)
		if err != nil {
			continue
		}
		for _, series := range data {
			for _, item := range series {
				dateSet[item.Date] = struct{}{}
			}
		}
	}

	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	return dates, nil
}

// loadSectorIndexFile loads a single sector_index file. It accepts both the
// single-day `sector_indices_YYYYMMDD_YYYYMMDD.json` and the batch file format.
func loadSectorIndexFile(path string) (map[string][]SectorIndexData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	var parsed map[string][]SectorIndexData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode file: %w", err)
	}
	return parsed, nil
}
