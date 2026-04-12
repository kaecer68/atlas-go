package portfolio

import (
	"encoding/json"
	"fmt"
	"os"
)

// FundamentalData holds per-symbol fundamentals.
type FundamentalData struct {
	PE            float64 `json:"PE"`
	PB            float64 `json:"PB"`
	DividendYield float64 `json:"DividendYield"`
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
// The expected format is: {"2330.TW": {"PE": 25.3, "PB": 6.1, "DividendYield": 1.5}, ...}
func (fp *FundamentalProvider) LoadFromJSON(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open fundamentals: %w", err)
	}
	defer f.Close()

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

// HasData returns true if any fundamentals were loaded.
func (fp *FundamentalProvider) HasData() bool {
	return len(fp.data) > 0
}
