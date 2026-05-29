package feature

import (
	"math"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Func computes one feature value from a bar at position idx in a sorted bar slice.
type Func func(bar domain.DailyBar, idx int, bars []domain.DailyBar) float64

// Registry maps feature names to their computation functions.
var Registry = map[string]Func{
	"close": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		return b.Close
	},
	"volume": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Volume <= 0 {
			return 0
		}
		return math.Log(float64(b.Volume))
	},
	"return_1d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx > 0 && bars[idx-1].Close > 0 {
			return (b.Close - bars[idx-1].Close) / bars[idx-1].Close
		}
		return 0
	},
	"return_5d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx >= 5 && bars[idx-5].Close > 0 {
			return (b.Close - bars[idx-5].Close) / bars[idx-5].Close
		}
		return 0
	},
	"hl_ratio": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Close > 0 {
			return (b.High - b.Low) / b.Close
		}
		return 0
	},
	"ma_ratio": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 1.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += bars[j].Close
		}
		if sum > 0 {
			return b.Close / (sum / 20.0)
		}
		return 1.0
	},
	"volume_ratio": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 || b.Volume <= 0 {
			return 1.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += float64(bars[j].Volume)
		}
		avg := sum / 20.0
		if avg > 0 {
			return float64(b.Volume) / avg
		}
		return 1.0
	},
}

// Available returns all registered feature names in sorted order.
func Available() []string {
	n := make([]string, 0, len(Registry))
	for k := range Registry {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Validate returns any feature names not in the registry.
func Validate(names []string) []string {
	var u []string
	for _, n := range names {
		if _, ok := Registry[n]; !ok {
			u = append(u, n)
		}
	}
	return u
}

// ParseNames splits a comma-separated string, trimming whitespace.
func ParseNames(raw string) []string {
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	return names
}

// MakeExtractor returns a function that extracts the named features from a bar slice.
func MakeExtractor(names []string) func(bars []domain.DailyBar) [][]float64 {
	return func(bars []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(bars))
		for i, bar := range bars {
			row := make([]float64, len(names))
			for j, n := range names {
				row[j] = Registry[n](bar, i, bars)
			}
			f[i] = row
		}
		return f
	}
}

// ForwardReturnLabel returns a label extractor that computes forward 1-day returns.
func ForwardReturnLabel() func(bars []domain.DailyBar) []float64 {
	return func(bars []domain.DailyBar) []float64 {
		l := make([]float64, len(bars))
		for i := 0; i < len(bars)-1; i++ {
			if bars[i].Close > 0 {
				l[i] = (bars[i+1].Close - bars[i].Close) / bars[i].Close
			}
		}
		if len(bars) > 0 {
			l[len(bars)-1] = 0
		}
		return l
	}
}
