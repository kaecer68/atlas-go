package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
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

type MarginHistoryBackfiller struct {
	WorkDir       string
	Provider      *marketdata.TWSEMarginBalanceProvider
	LookbackDays  int
}

func NewMarginHistoryBackfiller(workDir string) *MarginHistoryBackfiller {
	marginDir := filepath.Join(workDir, DefaultMarginHistoryDir)
	return &MarginHistoryBackfiller{
		WorkDir:      workDir,
		Provider:     marketdata.NewTWSEMarginBalanceProvider(marginDir),
		LookbackDays: 30,
	}
}

func (b *MarginHistoryBackfiller) Backfill() error {
	marginDir := filepath.Join(b.WorkDir, DefaultMarginHistoryDir)
	if err := os.MkdirAll(marginDir, 0o755); err != nil {
		return fmt.Errorf("margin backfill: mkdir: %w", err)
	}

	existing, err := LoadMarginHistory(marginDir)
	if err != nil {
		return fmt.Errorf("margin backfill: load existing: %w", err)
	}

	existingDates := make(map[string]bool)
	for _, e := range existing {
		existingDates[e.Date] = true
	}

	ctx := context.Background()
	fetched := 0
	for i := 0; i < b.LookbackDays; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("20060102")
		if existingDates[date] {
			continue
		}
		if _, err := b.Provider.FetchSnapshot(ctx); err != nil {
			logging.Warn("margin_backfill", "fetch_failed", logging.Err(err), "date", date)
			continue
		}
		fetched++
	}

	logging.Info("margin_backfill", "complete", "fetched", fetched, "lookback_days", b.LookbackDays)
	return nil
}
