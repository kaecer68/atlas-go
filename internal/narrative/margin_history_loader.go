package narrative

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultMarginHistoryDir = "data/state/margin"

type MarginHistoryEntry struct {
	Date          string
	MarginBalance float64
	ChangePct     float64
}

type marginHistoryFile struct {
	Date          string  `json:"date"`
	MarginBalance float64 `json:"margin_balance"`
	ChangePct     float64 `json:"change_pct"`
}

func LoadMarginHistory(dataDir string) ([]MarginHistoryEntry, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("read margin history dir: %w", err)
	}

	var history []MarginHistoryEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_margin.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dataDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read margin history file %s: %w", entry.Name(), err)
		}
		var file marginHistoryFile
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("unmarshal margin history file %s: %w", entry.Name(), err)
		}
		history = append(history, MarginHistoryEntry(file))
	}

	sort.Slice(history, func(i, j int) bool { return history[i].Date < history[j].Date })
	return history, nil
}

func ComputeRollingPercentile(history []MarginHistoryEntry, currentValue float64, windowDays int) (float64, bool) {
	if windowDays <= 0 || len(history) < windowDays {
		return 0, false
	}
	window := history[len(history)-windowDays:]
	values := make([]float64, 0, len(window))
	for _, entry := range window {
		values = append(values, entry.MarginBalance)
	}
	sort.Float64s(values)
	if len(values) == 0 {
		return 0, false
	}
	countBelow := 0
	countAtOrBelow := 0
	for _, v := range values {
		if v < currentValue {
			countBelow++
		}
		if v <= currentValue {
			countAtOrBelow++
		}
	}
	percentile := (float64(countBelow) + float64(countAtOrBelow)) / (2 * float64(len(values))) * 100
	return percentile, true
}

func ComputeRollingAcceleration(history []MarginHistoryEntry, windowDays int) (float64, bool) {
	if windowDays <= 0 || len(history) <= windowDays {
		return 0, false
	}
	current := history[len(history)-1].MarginBalance
	prior := history[len(history)-1-windowDays].MarginBalance
	return (current - prior) / float64(windowDays), true
}

func historicalMedianAcceleration(history []MarginHistoryEntry, windowDays int) (float64, bool) {
	if windowDays <= 0 || len(history) <= windowDays {
		return 0, false
	}
	var accels []float64
	for i := windowDays; i < len(history); i++ {
		accels = append(accels, (history[i].MarginBalance-history[i-windowDays].MarginBalance)/float64(windowDays))
	}
	if len(accels) == 0 {
		return 0, false
	}
	sort.Float64s(accels)
	mid := len(accels) / 2
	if len(accels)%2 == 1 {
		return accels[mid], true
	}
	return (accels[mid-1] + accels[mid]) / 2, true
}

func marginHistoryAvailable(history []MarginHistoryEntry) bool {
	return len(history) >= 30
}

func marginPercentileConfidence(percentile float64) float64 {
	if percentile <= 90 {
		return 0.45
	}
	confidence := 0.45 + math.Min(0.35, (percentile-90)/10*0.35)
	if confidence > 0.9 {
		confidence = 0.9
	}
	return confidence
}

func marginStage2Confirmed(accel, median float64, positive bool) bool {
	if !positive {
		return accel < 0 && median < 0 && accel < 2*median
	}
	return accel > 0 && median > 0 && accel > 2*median
}

func isMarginHistoryError(err error) bool { return err != nil }
