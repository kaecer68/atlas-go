package portfolio

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Sector represents a stock sector classification.
type Sector string

const (
	SectorSemiconductor Sector = "semiconductor"
	SectorFinancials    Sector = "financials"
	SectorElectronics   Sector = "electronics"
	SectorShipping      Sector = "shipping"
	SectorEnergy        Sector = "energy"
	SectorConsumer      Sector = "consumer"
	SectorIndustrial    Sector = "industrial"
	SectorOther         Sector = "other"
)

// FundamentalData holds per-symbol fundamentals.
type FundamentalData struct {
	PE            float64 `json:"PE"`
	PB            float64 `json:"PB"`
	PS            float64 `json:"PS"` // Price-to-Sales ratio
	DividendYield float64 `json:"DividendYield"`
	Sector        string  `json:"Sector"`
}

// FundamentalProvider loads fundamental metrics for symbols.
type FundamentalProvider struct {
	data map[string]FundamentalData
}

// NewFundamentalProvider creates an empty provider.
func NewFundamentalProvider() *FundamentalProvider {
	return &FundamentalProvider{data: make(map[string]FundamentalData)}
}

// LoadFromJSON reads fundamentals from a local JSON file.
// The expected format is: {"2330.TW": {"PE": 25.3, "PB": 6.1, "DividendYield": 1.5, "Sector": "semiconductor"}, ...}
func (fp *FundamentalProvider) LoadFromJSON(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open fundamentals: %w", err)
	}
	defer func() { _ = f.Close() }()

	var raw map[string]FundamentalData
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return fmt.Errorf("decode fundamentals: %w", err)
	}
	fp.data = raw
	return nil
}

// Get returns the fundamental data for a symbol (zero value if missing).
func (fp *FundamentalProvider) Get(symbol string) FundamentalData {
	return fp.data[symbol]
}

// HasSymbol returns true if the snapshot contains the canonical symbol
// (with `.TW` suffix normalized). Unlike Get(), which returns the zero
// value for both missing and all-zero entries, HasSymbol disambiguates
// presence vs data absence — essential for the stocktools coverage
// guard (See docs/manifests/2026-08-06-stock-coverage-notice.md).
// Safe on a nil receiver: returns false.
func (fp *FundamentalProvider) HasSymbol(canonical string) bool {
	if fp == nil {
		return false
	}
	_, ok := fp.data[canonical]
	return ok
}

// HasData returns true if any fundamentals were loaded.
func (fp *FundamentalProvider) HasData() bool {
	return len(fp.data) > 0
}

// SectorMedianPE calculates the median P/E for a given sector.
// Returns 0 if no valid P/E values exist for the sector.
func (fp *FundamentalProvider) SectorMedianPE(sector string) float64 {
	var pes []float64
	for _, data := range fp.data {
		if data.Sector == sector && data.PE > 0 {
			pes = append(pes, data.PE)
		}
	}
	if len(pes) == 0 {
		return 0
	}
	sort.Float64s(pes)
	if len(pes)%2 == 0 {
		return (pes[len(pes)/2-1] + pes[len(pes)/2]) / 2
	}
	return pes[len(pes)/2]
}

// GetSector returns the sector for a given symbol.
func (fp *FundamentalProvider) GetSector(symbol string) string {
	return fp.data[symbol].Sector
}
