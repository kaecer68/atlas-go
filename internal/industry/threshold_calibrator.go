package industry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
)

type revenueRecord struct {
	Date         string  `json:"date"`
	StockID      string  `json:"stock_id"`
	Revenue      float64 `json:"revenue"`
	RevenueMonth int     `json:"revenue_month"`
	RevenueYear  int     `json:"revenue_year"`
}

type CalibrationResult struct {
	IndustryID string
	P25        float64
	P50        float64
	P75        float64
	SampleSize int
}

func CalibrateThresholdsFromFile(revenuePath string) ([]CalibrationResult, error) {
	records, err := loadRevenueFile(revenuePath)
	if err != nil {
		return nil, fmt.Errorf("calibrate: load revenue: %w", err)
	}
	tree := DefaultClassification()
	return computeThresholds(records, tree), nil
}

func loadRevenueFile(path string) ([]revenueRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []revenueRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec revenueRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Revenue <= 0 {
			continue
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

func computeThresholds(records []revenueRecord, tree *ClassificationTree) []CalibrationResult {
	var results []CalibrationResult
	type monthKey struct {
		symbol string
		year   int
		month  int
	}

	for _, seg := range tree.GetLevel1() {
		if len(seg.RepresentativeStocks) == 0 {
			continue
		}
		symSet := make(map[string]bool, len(seg.RepresentativeStocks))
		for _, s := range seg.RepresentativeStocks {
			symSet[s] = true
		}

		byMonth := make(map[monthKey]float64)
		for _, r := range records {
			if !symSet[r.StockID] || r.RevenueYear == 0 || r.RevenueMonth == 0 {
				continue
			}
			key := monthKey{symbol: r.StockID, year: r.RevenueYear, month: r.RevenueMonth}
			byMonth[key] = r.Revenue
		}

		var growths []float64
		for key, current := range byMonth {
			priorKey := monthKey{symbol: key.symbol, year: key.year - 1, month: key.month}
			prior, ok := byMonth[priorKey]
			if !ok || prior == 0 {
				continue
			}
			g := (current - prior) / math.Abs(prior)
			g = math.Max(g, -1.0)
			g = math.Min(g, 5.0)
			growths = append(growths, g)
		}

		if len(growths) == 0 {
			continue
		}

		sorted := make([]float64, len(growths))
		copy(sorted, growths)
		sort.Float64s(sorted)

		results = append(results, CalibrationResult{
			IndustryID: seg.ID,
			P25:        sorted[int(float64(len(sorted))*0.25)],
			P50:        sorted[int(float64(len(sorted))*0.50)],
			P75:        sorted[int(float64(len(sorted))*0.75)],
			SampleSize: len(sorted),
		})
	}
	return results
}
