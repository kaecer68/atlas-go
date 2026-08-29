// Package marketdata provides market data provider abstractions and adapters
// for Taiwan stock market data sources.
//
// SymbolIndustryMapper maps stock symbols to their industry classification using
// a three-layer fallback strategy:
//  1. Exact match: FinMind industry_category against ClassificationTree segment names
//  2. Fuzzy match: Levenshtein distance ≤ 2 against all segment names
//  3. Representative stocks: ClassifyBySymbol reverse lookup as last resort
//
// Results are cached in-memory and persisted to disk (JSON, 24h TTL) for
// fast subsequent lookups without re-fetching from FinMind.
//
// Note: Due to circular import constraints (industry ↔ marketdata), this package
// defines its own local segment/classification types and accesses the
// ClassificationTree through an interface. Callers with an industry.ClassificationTree
// pass it via the ClassificationTreeAccessor interface; industry.ClassifyBySymbol is
// wired through the classifyFunc constructor parameter.
package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// ---------------------------------------------------------------------------
// Local industry types (mirror industry.IndustrySegment / IndustryClassification
// to avoid circular import between marketdata ↔ industry).
// ---------------------------------------------------------------------------

// MapperIndustryLevel mirrors industry.IndustryLevel.
type MapperIndustryLevel int

const (
	MapperLevel1 MapperIndustryLevel = 1
	MapperLevel2 MapperIndustryLevel = 2
	MapperLevel3 MapperIndustryLevel = 3
)

// MapperIndustrySegment mirrors industry.IndustrySegment with the fields
// needed for classification lookups and cache serialization.
type MapperIndustrySegment struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	NameEN               string   `json:"name_en"`
	Level                int      `json:"level"`
	ParentID             string   `json:"parent_id,omitempty"`
	RepresentativeStocks []string `json:"representative_stocks,omitempty"`
}

// MapperIndustryClassification mirrors industry.IndustryClassification.
type MapperIndustryClassification struct {
	Symbol    string                `json:"symbol"`
	Level1    MapperIndustrySegment `json:"level1"`
	Level2    MapperIndustrySegment `json:"level2"`
	Level3    MapperIndustrySegment `json:"level3"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// ClassificationTreeAccessor abstracts access to the industry classification
// tree. The concrete industry.ClassificationTree satisfies this interface,
// allowing callers to pass it without marketdata importing industry directly.
type ClassificationTreeAccessor interface {
	GetSegment(id string) (*MapperIndustrySegment, bool)
	GetChildren(parentID string) []*MapperIndustrySegment
	GetLevel1() []*MapperIndustrySegment
	GetPath(segmentID string) []*MapperIndustrySegment
	GetAllSegments() []*MapperIndustrySegment
}

// ---------------------------------------------------------------------------
// SymbolIndustryMapper
// ---------------------------------------------------------------------------

// SymbolIndustryMapper maps stock symbols to their industry classification using
// FinMind data as the primary source with ClassificationTree fallback matching.
type SymbolIndustryMapper struct {
	finMindClient *FinMindClient
	tree          ClassificationTreeAccessor
	cache         map[string]MapperIndustryClassification
	cachePath     string
	mu            sync.RWMutex

	// classifyFunc is the Tier-3 fallback: symbol → industry name.
	// Typically wired to industry.ClassifyBySymbol by the caller.
	classifyFunc func(symbol string) string
}

// NewSymbolIndustryMapper creates a new mapper.
//
//   - client: FinMindClient for fetching TaiwanStockInfo
//   - tree: classification tree accessor (typically an adapter around
//     industry.ClassificationTree)
//   - cachePath: disk cache file path (JSON); empty to skip disk caching
//   - classifyFunc: Tier-3 fallback function, typically industry.ClassifyBySymbol
//
// If cachePath is non-empty, existing cache data is loaded from disk on
// construction (errors are logged but not fatal).
func NewSymbolIndustryMapper(
	client *FinMindClient,
	tree ClassificationTreeAccessor,
	cachePath string,
	classifyFunc func(string) string,
) *SymbolIndustryMapper {
	m := &SymbolIndustryMapper{
		finMindClient: client,
		tree:          tree,
		cache:         make(map[string]MapperIndustryClassification),
		cachePath:     cachePath,
		classifyFunc:  classifyFunc,
	}

	if cachePath != "" {
		if err := m.loadFromDisk(); err != nil {
			logging.Warn("mapper", "cache_load_failed",
				logging.FStr("path", cachePath),
				logging.Err(err))
		} else if len(m.cache) > 0 {
			logging.Info("mapper", "cache_loaded",
				logging.FInt("entries", len(m.cache)),
				logging.FStr("path", cachePath))
		}
	}

	return m
}

// BuildMapping fetches stock info from FinMind and resolves industry
// classification for every stock. Results are stored in the in-memory cache
// and persisted to disk (if cachePath is set).
func (m *SymbolIndustryMapper) BuildMapping(ctx context.Context) error {
	infos, err := m.finMindClient.GetStockInfo(ctx)
	if err != nil {
		return fmt.Errorf("mapper: fetch stock info: %w", err)
	}

	logging.Info("mapper", "build_mapping_start", logging.FInt("total_stocks", len(infos)))

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	matched := 0
	unmatched := 0

	for _, info := range infos {
		symbol := normalizeSymbol(info.StockID)
		if symbol == "" {
			continue
		}

		seg := m.matchIndustry(info.IndustryCategory, info.StockName, symbol)
		if seg == nil {
			logging.Warn("mapper", "match_failed",
				logging.FStr("symbol", symbol),
				logging.FStr("finmind_category", info.IndustryCategory),
				logging.FStr("stock_name", info.StockName))
			unmatched++
			continue
		}

		class := m.classifyForSymbol(symbol, seg, now)
		m.cache[symbol] = class
		matched++
	}

	logging.Info("mapper", "build_mapping_done",
		logging.FInt("matched", matched),
		logging.FInt("unmatched", unmatched))

	if m.cachePath != "" {
		if err := m.atomicWrite(); err != nil {
			logging.Warn("mapper", "cache_write_failed", logging.Err(err))
		}
	}

	return nil
}

// GetClassification returns the cached industry classification for a symbol.
// Symbol is normalized (".TW" suffix stripped) before lookup.
func (m *SymbolIndustryMapper) GetClassification(symbol string) (MapperIndustryClassification, bool) {
	symbol = normalizeSymbol(symbol)

	m.mu.RLock()
	defer m.mu.RUnlock()

	class, ok := m.cache[symbol]
	return class, ok
}

// GetSymbolsByIndustry returns all cached symbols that belong to the given
// Level 1 industry name. Matching is case-insensitive.
func (m *SymbolIndustryMapper) GetSymbolsByIndustry(industryLevel1Name string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	target := strings.ToLower(industryLevel1Name)

	for symbol, class := range m.cache {
		if strings.ToLower(class.Level1.Name) == target {
			result = append(result, symbol)
		}
	}
	return result
}

// GetSymbolsByLevel returns all cached symbols that match a specific
// industry level and segment ID. For example, level=MapperLevel2 and
// segmentID="semi" returns all semiconductor stocks.
func (m *SymbolIndustryMapper) GetSymbolsByLevel(level MapperIndustryLevel, segmentID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string

	for symbol, class := range m.cache {
		var segID string
		switch level {
		case MapperLevel1:
			segID = class.Level1.ID
		case MapperLevel2:
			segID = class.Level2.ID
		case MapperLevel3:
			segID = class.Level3.ID
		}
		if segID == segmentID {
			result = append(result, symbol)
		}
	}
	return result
}

// LoadCache reads the JSON cache file from disk and populates the in-memory
// cache map. Existing entries are replaced.
func (m *SymbolIndustryMapper) LoadCache(path string) error {
	m.cachePath = path

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.loadFromDisk()
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

// matchIndustry resolves a stock to an industry segment using three-tier matching.
//
// Tier 1 (Exact): Match finMindCategory against segment.Name or segment.NameEN
// (case-insensitive). Also tries stripping trailing "業" from the FinMind
// category for better alignment with the ClassificationTree.
//
// Tier 2 (Fuzzy): Compute Levenshtein distance ≤ 2 against all segment names
// (both Name and NameEN). Shortest distance wins; ties pick the first match.
//
// Tier 3 (Representative): Use the classifyFunc (typically industry.ClassifyBySymbol)
// as a reverse lookup to get an industry name, then exact-match that name
// against segments.
//
// Returns nil if no tier succeeds.
func (m *SymbolIndustryMapper) matchIndustry(finMindCategory, stockName, symbol string) *MapperIndustrySegment {
	segments := m.tree.GetAllSegments()

	// Tier 1: Exact match (case-insensitive)
	if seg := exactMatch(finMindCategory, segments); seg != nil {
		return seg
	}

	// Tier 2: Fuzzy match (Levenshtein ≤ 2)
	if seg := fuzzyMatch(finMindCategory, segments); seg != nil {
		return seg
	}

	// Tier 3: Representative stocks fallback
	if m.classifyFunc != nil {
		industryName := m.classifyFunc(symbol)
		if industryName != "" {
			if seg := exactMatch(industryName, segments); seg != nil {
				return seg
			}
		}
	}

	return nil
}

// exactMatch searches segments for an exact (case-insensitive) match against
// Name or NameEN. Also tries the query without a trailing "業" suffix to
// handle FinMind categories like "半導體業" → "半導體".
func exactMatch(query string, segments []*MapperIndustrySegment) *MapperIndustrySegment {
	if query == "" {
		return nil
	}

	queries := []string{query}
	if before, ok := strings.CutSuffix(query, "業"); ok {
		queries = append(queries, before)
	}

	for _, q := range queries {
		qlower := strings.ToLower(q)
		for _, seg := range segments {
			if strings.ToLower(seg.Name) == qlower || strings.ToLower(seg.NameEN) == qlower {
				return seg
			}
		}
	}

	return nil
}

// fuzzyMatch finds the best segment using Levenshtein distance ≤ 2.
// Both Name and NameEN are checked. Shortest distance wins; ties pick the first.
func fuzzyMatch(query string, segments []*MapperIndustrySegment) *MapperIndustrySegment {
	if query == "" {
		return nil
	}

	qlower := strings.ToLower(query)
	var best *MapperIndustrySegment
	bestDist := 999

	for _, seg := range segments {
		for _, name := range []string{seg.Name, seg.NameEN} {
			if name == "" {
				continue
			}
			d := levenshtein(qlower, strings.ToLower(name))
			if d <= 2 && d < bestDist {
				bestDist = d
				best = seg
			}
		}
	}

	return best
}

// levenshtein computes the edit distance between two strings.
// Self-contained inline implementation, no external library dependency.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// classifyForSymbol builds a full MapperIndustryClassification from a matched
// segment by walking up the tree to resolve all three levels.
func (m *SymbolIndustryMapper) classifyForSymbol(
	symbol string,
	seg *MapperIndustrySegment,
	now time.Time,
) MapperIndustryClassification {
	class := MapperIndustryClassification{
		Symbol:    symbol,
		UpdatedAt: now,
	}

	path := m.tree.GetPath(seg.ID)
	for _, p := range path {
		switch MapperIndustryLevel(p.Level) {
		case MapperLevel1:
			class.Level1 = *p
		case MapperLevel2:
			class.Level2 = *p
		case MapperLevel3:
			class.Level3 = *p
		}
	}

	return class
}

// normalizeSymbol strips the ".TW" suffix from a symbol for internal storage.
// All internal symbol lookups use the suffix-free form.
func normalizeSymbol(symbol string) string {
	return strings.TrimSuffix(strings.ToUpper(symbol), ".TW")
}

// ---------------------------------------------------------------------------
// Disk cache (JSON, atomic write)
// ---------------------------------------------------------------------------

// loadFromDisk reads the cache file from disk. Caller must hold m.mu.
// Expired caches (older than 24h) are silently skipped.
func (m *SymbolIndustryMapper) loadFromDisk() error {
	if m.cachePath == "" {
		return nil
	}

	f, err := os.Open(m.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("mapper: open cache: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("mapper: stat cache: %w", err)
	}
	if time.Since(info.ModTime()) > 24*time.Hour {
		logging.Info("mapper", "cache_expired",
			logging.FStr("path", m.cachePath),
			logging.FStr("age", time.Since(info.ModTime()).String()))
		return nil
	}

	decoded := make(map[string]MapperIndustryClassification)
	if err := json.NewDecoder(f).Decode(&decoded); err != nil {
		return fmt.Errorf("mapper: decode cache: %w", err)
	}

	m.cache = decoded
	return nil
}

// atomicWrite persists the cache to disk atomically (write temp → rename).
// Caller must hold m.mu.
func (m *SymbolIndustryMapper) atomicWrite() error {
	if m.cachePath == "" {
		return nil
	}

	dir := filepath.Dir(m.cachePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mapper: mkdir cache dir: %w", err)
	}

	tmpPath := m.cachePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("mapper: create temp cache: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m.cache); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("mapper: encode cache: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("mapper: close temp cache: %w", err)
	}

	if err := os.Rename(tmpPath, m.cachePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("mapper: rename cache: %w", err)
	}

	logging.Info("mapper", "cache_saved",
		logging.FInt("entries", len(m.cache)),
		logging.FStr("path", m.cachePath))

	return nil
}
