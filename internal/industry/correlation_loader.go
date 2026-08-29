package industry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func LoadIndustryReturnsFromReplay(replayPath, sectorSymbolsPath string) (map[string][]float64, error) {
	sectorSymbols, err := loadSectorSymbolsJSON(sectorSymbolsPath)
	if err != nil {
		return nil, fmt.Errorf("load sector symbols: %w", err)
	}

	symbolToIndustries := invertSectorSymbols(sectorSymbols)

	dataset, err := loadReplayData(replayPath)
	if err != nil {
		return nil, fmt.Errorf("load replay data: %w", err)
	}

	stockReturns := computeStockReturns(dataset)

	industryDateReturns := aggregateIndustryDateReturns(stockReturns, symbolToIndustries)

	result := extractSortedArrays(industryDateReturns)

	filtered := filterMinObservations(result, 15)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no industry has at least 15 observations")
	}
	return filtered, nil
}

func loadSectorSymbolsJSON(path string) (map[string][]string, error) {
	if path == "" {
		cfg := config.Load()
		path = cfg.WorkDir + "/configs/sector_symbols.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sector symbols file %s: %w", path, err)
	}

	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse sector symbols from %s: %w", path, err)
	}

	result := make(map[string][]string, len(raw))
	for k, v := range raw {
		result[k] = v
	}
	return result, nil
}

func invertSectorSymbols(sectorSymbols map[string][]string) map[string][]string {
	mapping := make(map[string][]string)
	for industryID, symbols := range sectorSymbols {
		for _, sym := range symbols {
			mapping[sym] = append(mapping[sym], industryID)
		}
	}
	return mapping
}

func loadReplayData(path string) (*replay.Dataset, error) {
	if strings.HasSuffix(path, ".jsonl") {
		return loadJSONLDataset(path)
	}
	return replay.LoadTWSEOpenDataCSV(path)
}

func computeStockReturns(dataset *replay.Dataset) map[string]map[string]float64 {
	stockPrices := make(map[string]map[string]float64)

	for dateStr, stocks := range dataset.ByDate {
		for symbol, bar := range stocks {
			if bar.Close <= 0 {
				continue
			}
			if stockPrices[symbol] == nil {
				stockPrices[symbol] = make(map[string]float64)
			}
			stockPrices[symbol][dateStr] = bar.Close
		}
	}

	stockReturns := make(map[string]map[string]float64)

	for symbol, datePrices := range stockPrices {
		sortedDates := make([]string, 0, len(datePrices))
		for d := range datePrices {
			sortedDates = append(sortedDates, d)
		}
		sort.Strings(sortedDates)

		stockReturns[symbol] = make(map[string]float64)
		for i := len(sortedDates) - 1; i > 0; i-- {
			curr := sortedDates[i]
			prev := sortedDates[i-1]
			currPrice := datePrices[curr]
			prevPrice := datePrices[prev]
			if prevPrice > 0 {
				stockReturns[symbol][curr] = (currPrice - prevPrice) / prevPrice
			}
		}
	}

	return stockReturns
}

func aggregateIndustryDateReturns(
	stockReturns map[string]map[string]float64,
	symbolToIndustries map[string][]string,
) map[string]map[string]float64 {
	type accum struct {
		sum   float64
		count int
	}

	industryAccum := make(map[string]map[string]*accum)

	for symbol, dateReturns := range stockReturns {
		industryIDs := symbolToIndustries[symbol]
		if len(industryIDs) == 0 {
			continue
		}
		for _, industryID := range industryIDs {
			if industryAccum[industryID] == nil {
				industryAccum[industryID] = make(map[string]*accum)
			}
			for date, ret := range dateReturns {
				a, ok := industryAccum[industryID][date]
				if !ok {
					industryAccum[industryID][date] = &accum{sum: ret, count: 1}
				} else {
					a.sum += ret
					a.count++
				}
			}
		}
	}

	result := make(map[string]map[string]float64)
	for industryID, dateAccums := range industryAccum {
		result[industryID] = make(map[string]float64)
		for date, a := range dateAccums {
			result[industryID][date] = a.sum / float64(a.count)
		}
	}
	return result
}

func extractSortedArrays(industryDateReturns map[string]map[string]float64) map[string][]float64 {
	result := make(map[string][]float64)

	for industryID, dateReturns := range industryDateReturns {
		dates := make([]string, 0, len(dateReturns))
		for d := range dateReturns {
			dates = append(dates, d)
		}
		sort.Strings(dates)

		returns := make([]float64, 0, len(dates))
		for _, d := range dates {
			returns = append(returns, dateReturns[d])
		}
		result[industryID] = returns
	}

	return result
}

func filterMinObservations(industryReturns map[string][]float64, minObs int) map[string][]float64 {
	result := make(map[string][]float64)
	for id, returns := range industryReturns {
		if len(returns) >= minObs {
			result[id] = returns
		}
	}
	return result
}

// jsonlField holds parsed fields from a JSONL row, supporting both
// PascalCase (tw_extended_90days) and lowercase (finmind, tw_combined) keys.
type jsonlField struct {
	Date   string
	Symbol string
	Name   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
	Source string
}

func parseJSONLRow(line string) (jsonlField, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return jsonlField{}, fmt.Errorf("unmarshal JSONL row: %w", err)
	}

	getStr := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				if s, ok := v.(string); ok {
					return s
				}
			}
		}
		return ""
	}
	getFloat := func(keys ...string) float64 {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
				switch n := v.(type) {
				case float64:
					return n
				case string:
					f, _ := strconv.ParseFloat(n, 64)
					return f
				}
			}
		}
		return 0
	}

	return jsonlField{
		Date:   getStr("Date", "date"),
		Symbol: getStr("Symbol", "symbol"),
		Name:   getStr("Name", "name"),
		Open:   getFloat("Open", "open"),
		High:   getFloat("High", "high"),
		Low:    getFloat("Low", "low"),
		Close:  getFloat("Close", "close"),
		Volume: int64(getFloat("Volume", "volume")),
		Source: getStr("Source", "source"),
	}, nil
}

func loadJSONLDataset(path string) (*replay.Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open JSONL: %w", err)
	}
	defer f.Close()

	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{},
		Dates:  make([]time.Time, 0),
	}
	seenDates := map[string]time.Time{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		row, err := parseJSONLRow(line)
		if err != nil {
			return nil, err
		}
		date, err := time.Parse(time.RFC3339, row.Date)
		if err != nil {
			date, err = time.Parse("2006-01-02", row.Date)
			if err != nil {
				return nil, fmt.Errorf("parse date %s: %w", row.Date, err)
			}
		}
		dateKey := date.Format("2006-01-02")
		if _, ok := ds.ByDate[dateKey]; !ok {
			ds.ByDate[dateKey] = map[string]domain.DailyBar{}
			seenDates[dateKey] = date
		}
		ds.ByDate[dateKey][row.Symbol] = domain.DailyBar{
			Date:   date,
			Symbol: row.Symbol,
			Name:   row.Name,
			Open:   row.Open,
			High:   row.High,
			Low:    row.Low,
			Close:  row.Close,
			Volume: row.Volume,
			Source: row.Source,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan JSONL: %w", err)
	}

	for _, date := range seenDates {
		ds.Dates = append(ds.Dates, date)
	}
	sort.Slice(ds.Dates, func(i, j int) bool {
		return ds.Dates[i].Before(ds.Dates[j])
	})
	return ds, nil
}
