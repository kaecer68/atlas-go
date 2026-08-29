package marketdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DailyBenchmark represents a single day's benchmark return.
type DailyBenchmark struct {
	Date   string  `json:"date"`
	Close  float64 `json:"close"`
	Return float64 `json:"return"`
}

// TAIEXBenchmarkProvider returns the daily TAIEX benchmark return.
type TAIEXBenchmarkProvider interface {
	DailyReturn(ctx any, tradingDate time.Time) (DailyBenchmark, error)
}

// FileTAIEXBenchmarkProvider reads TAIEX daily returns from a JSON file.
// During the shadow observation window, it returns unavailable until
// real benchmark data is loaded.
type FileTAIEXBenchmarkProvider struct {
	path string
	data map[string]DailyBenchmark
}

// NewFileTAIEXBenchmarkProvider loads benchmark data from the given file.
// Returns a provider in "unavailable" state if the file doesn't exist.
func NewFileTAIEXBenchmarkProvider(macroDir string) *FileTAIEXBenchmarkProvider {
	p := &FileTAIEXBenchmarkProvider{
		path: filepath.Join(macroDir, "taiex_daily.json"),
		data: make(map[string]DailyBenchmark),
	}
	raw, err := os.ReadFile(p.path)
	if err != nil {
		return p
	}
	var entries []DailyBenchmark
	if err := json.Unmarshal(raw, &entries); err != nil {
		return p
	}
	for _, e := range entries {
		p.data[e.Date] = e
	}
	return p
}

// DailyReturn returns the benchmark for the given trading date.
// Returns an unavailable benchmark when data is missing.
func (p *FileTAIEXBenchmarkProvider) DailyReturn(ctx any, tradingDate time.Time) (DailyBenchmark, error) {
	dateStr := tradingDate.Format("2006-01-02")
	b, ok := p.data[dateStr]
	if !ok {
		return DailyBenchmark{Date: dateStr}, fmt.Errorf("taiex benchmark: date %s not found", dateStr)
	}
	if b.Close <= 0 {
		return DailyBenchmark{Date: dateStr}, fmt.Errorf("taiex benchmark: invalid close for %s", dateStr)
	}
	b.Date = dateStr
	return b, nil
}
