package marketdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/sectormap"
)

// canonicalSectorIDs is the stable L1 sector universe used by downstream
// consumers (period detector / calculator). Issue #1943: it is derived from the
// shared canonical list instead of a hand-copied 18-entry set, which silently
// excluded chemicals and tourism.
//
// Note (2026-09-24, verified against the live TWSE MI_INDEX response): the two
// missing IDs were unreachable in practice, not merely rejected on read — the
// TWSE vocabulary spells them 化學類指數 and 觀光餐旅類指數, while the provider
// table declared the non-existent names 化學工業類指數 / 觀光類指數. Widening
// this set alone therefore recovers no data; the names were added to
// internal/sectormap (NamespaceTWSESectorIndex) in the same change.
var canonicalSectorIDs = func() map[string]bool {
	ids := sectormap.CanonicalL1IDs()
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}()

// canonicalSectorID normalizes a raw sector ID from a sector_index file to a
// canonical L1 ID. Legacy 8-industry IDs (ai_supply_chain, robotics) resolve
// through the declared namespace table in internal/sectormap — the same table
// the writing side uses, so the two can no longer disagree. Unknown IDs return
// an empty string and are dropped by the reader.
func canonicalSectorID(raw string) string {
	if canonicalSectorIDs[raw] {
		return raw
	}
	if l1, ok := sectormap.ResolveL1(sectormap.NamespaceSectorIndexReader, raw); ok {
		return l1
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
//
// W1 (B5 Batch 3): source-priority policy — when both the 18-industry native
// schema and the 8-industry legacy schema are present for the same date and
// canonical industry, the 18-industry value wins. The 8-industry value is
// used only to fill canonical IDs not covered by the 18-industry data.
//
// Files are classified by their parsed key count: 18 keys -> 18-industry
// native (priority); otherwise -> 8-industry legacy (fallback).
func (r *SectorIndexReader) ReadRange(startDate, endDate time.Time) (map[string]map[string]float64, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read sector_index dir: %w", err)
	}

	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	result := make(map[string]map[string]float64)

	// Pre-load all files; classify into native (18-industry) vs legacy (other).
	type loaded struct {
		path     string
		isNative bool
		data     map[string][]SectorIndexData
	}
	var natives []loaded
	var legacys []loaded
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
		if len(data) >= 18 {
			natives = append(natives, loaded{path, true, data})
		} else {
			legacys = append(legacys, loaded{path, false, data})
		}
	}

	// Phase 1: fill from native (18-industry) sources.
	for _, src := range natives {
		for rawIndustry, series := range src.data {
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
				result[item.Date][industry] = item.ReturnPct
			}
		}
	}

	// Phase 2: aggregate canonical collisions within each legacy file, then fill
	// only IDs not already covered by native data or an earlier legacy file.
	type aggregate struct {
		sum   float64
		count int
	}
	for _, src := range legacys {
		fileAggregates := make(map[string]map[string]aggregate)
		for rawIndustry, series := range src.data {
			industry := canonicalSectorID(rawIndustry)
			if industry == "" {
				continue
			}
			for _, item := range series {
				if item.Date < startStr || item.Date > endStr {
					continue
				}
				if fileAggregates[item.Date] == nil {
					fileAggregates[item.Date] = make(map[string]aggregate)
				}
				value := fileAggregates[item.Date][industry]
				value.sum += item.ReturnPct
				value.count++
				fileAggregates[item.Date][industry] = value
			}
		}

		for date, industries := range fileAggregates {
			if result[date] == nil {
				result[date] = make(map[string]float64)
			}
			for industry, value := range industries {
				if _, alreadyCovered := result[date][industry]; alreadyCovered {
					continue
				}
				result[date][industry] = value.sum / float64(value.count)
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
