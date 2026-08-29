package replay

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ErrReplayDataMissing is returned when the replay CSV file cannot be found.
var ErrReplayDataMissing = errors.New("replay data missing")

type Dataset struct {
	ByDate map[string]map[string]domain.DailyBar
	Dates  []time.Time
}

func LoadTWSEOpenDataCSV(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrReplayDataMissing, path)
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}

	index := make(map[string]int, len(header))
	for i, col := range header {
		index[strings.TrimSpace(col)] = i
	}

	required := []string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"}
	for _, col := range required {
		if _, ok := index[col]; !ok {
			return nil, fmt.Errorf("missing column %s", col)
		}
	}

	ds := &Dataset{
		ByDate: map[string]map[string]domain.DailyBar{},
		Dates:  make([]time.Time, 0),
	}
	seenDates := map[string]time.Time{}

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		date, err := time.Parse("2006-01-02", value(record, index, "Date"))
		if err != nil {
			return nil, fmt.Errorf("parse date: %w", err)
		}
		dateKey := date.Format("2006-01-02")
		if _, ok := ds.ByDate[dateKey]; !ok {
			ds.ByDate[dateKey] = map[string]domain.DailyBar{}
			seenDates[dateKey] = date
		}

		symbol := strings.TrimSpace(value(record, index, "Code")) + ".TW"
		bar := domain.DailyBar{
			Date:   date,
			Symbol: symbol,
			Name:   strings.TrimSpace(value(record, index, "Name")),
			Open:   parseFloat(value(record, index, "Open")),
			High:   parseFloat(value(record, index, "High")),
			Low:    parseFloat(value(record, index, "Low")),
			Close:  parseFloat(value(record, index, "Close")),
			Volume: parseInt(value(record, index, "TradeVolume")),
			Source: "twse_open_data_csv",
		}
		ds.ByDate[dateKey][symbol] = bar
	}

	for _, date := range seenDates {
		ds.Dates = append(ds.Dates, date)
	}
	sort.Slice(ds.Dates, func(i, j int) bool {
		return ds.Dates[i].Before(ds.Dates[j])
	})

	return ds, nil
}

func (d *Dataset) QuotesForDate(date time.Time, symbols []string) []domain.Quote {
	day := d.ByDate[date.Format("2006-01-02")]
	quotes := make([]domain.Quote, 0, len(symbols))
	for _, symbol := range symbols {
		bar, ok := day[symbol]
		if !ok {
			continue
		}
		quotes = append(quotes, domain.Quote{
			Symbol:     bar.Symbol,
			Last:       bar.Close,
			Open:       bar.Open,
			High:       bar.High,
			Low:        bar.Low,
			Volume:     bar.Volume,
			Market:     "TW",
			AsOf:       bar.Date,
			IsTradable: bar.Close > 0 && bar.Volume > 0,
			Source:     bar.Source,
		})
	}
	return quotes
}

func (d *Dataset) ForwardReturn(symbol string, currentDate time.Time, window int) (float64, bool) {
	currentKey := currentDate.Format("2006-01-02")
	currentBar, ok := d.ByDate[currentKey][symbol]
	if !ok || currentBar.Close == 0 {
		return 0, false
	}

	nextDate, ok := d.NextDate(currentDate, window)
	if !ok {
		return 0, false
	}
	nextBar, ok := d.ByDate[nextDate.Format("2006-01-02")][symbol]
	if !ok || nextBar.Close == 0 {
		return 0, false
	}

	// Reject stale/duplicated OHLCV: if both bars share the same Open/High/Low/Close/Volume,
	// the data source is likely backfilled with the same row across consecutive dates.
	// Return ok=false so downstream code (orchestrator, judge) falls back to the
	// synthetic forward-return path instead of recording a fabricated 0% real return.
	if currentBar.Open == nextBar.Open &&
		currentBar.High == nextBar.High &&
		currentBar.Low == nextBar.Low &&
		currentBar.Close == nextBar.Close &&
		currentBar.Volume == nextBar.Volume {
		return 0, false
	}

	return (nextBar.Close - currentBar.Close) / currentBar.Close, true
}

func (d *Dataset) NextDate(currentDate time.Time, offset int) (time.Time, bool) {
	for i, date := range d.Dates {
		if date.Format("2006-01-02") == currentDate.Format("2006-01-02") {
			target := i + offset
			if target >= 0 && target < len(d.Dates) {
				return d.Dates[target], true
			}
			return time.Time{}, false
		}
	}
	return time.Time{}, false
}

func value(record []string, index map[string]int, key string) string {
	i := index[key]
	if i >= len(record) {
		return ""
	}
	return record[i]
}

func parseFloat(v string) float64 {
	v = strings.ReplaceAll(strings.TrimSpace(v), ",", "")
	if v == "" || v == "--" {
		return 0
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func parseInt(v string) int64 {
	v = strings.ReplaceAll(strings.TrimSpace(v), ",", "")
	if v == "" || v == "--" {
		return 0
	}
	i, _ := strconv.ParseInt(v, 10, 64)
	return i
}

// jsonlRow is the expected JSON shape of each line in a FinMind-style JSONL replay file.
type jsonlRow struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// GetLatestDate detects the file format by extension and returns the latest
// date present in the replay data file without loading the entire dataset.
//
//   - .jsonl  → reads the last non-empty line, parses the "date" field.
//   - .csv    → delegates to LoadTWSEOpenDataCSV and returns Dates[len-1].
func GetLatestDate(path string) (string, error) {
	if strings.HasSuffix(path, ".jsonl") {
		return latestDateJSONL(path)
	}
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		return "", err
	}
	if len(ds.Dates) == 0 {
		return "", fmt.Errorf("no dates found in %s", path)
	}
	return ds.Dates[len(ds.Dates)-1].Format("2006-01-02"), nil
}

func latestDateJSONL(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrReplayDataMissing, path)
	}
	defer func() { _ = f.Close() }()

	const chunkSize = 4096
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	fileSize := stat.Size()
	if fileSize == 0 {
		return "", fmt.Errorf("file is empty: %s", path)
	}

	var lastNonEmptyLine string
	pos := fileSize
	buf := make([]byte, chunkSize)
	for pos > 0 {
		readSize := min(pos, int64(chunkSize))
		pos -= readSize
		_, err := f.ReadAt(buf[:readSize], pos)
		if err != nil && err != io.EOF {
			return "", err
		}

		chunk := string(buf[:readSize])
		lines := strings.Split(chunk, "\n")

		for _, line := range slices.Backward(lines) {
			if strings.TrimSpace(line) != "" {
				lastNonEmptyLine = line
				goto parse
			}
		}
	}

parse:
	if lastNonEmptyLine == "" {
		return "", fmt.Errorf("no data found in %s", path)
	}

	var row jsonlRow
	if err := json.Unmarshal([]byte(lastNonEmptyLine), &row); err != nil {
		return "", fmt.Errorf("parse jsonl last line: %w", err)
	}
	if row.Date == "" {
		return "", fmt.Errorf("missing date field in last line of %s", path)
	}
	return row.Date, nil
}
