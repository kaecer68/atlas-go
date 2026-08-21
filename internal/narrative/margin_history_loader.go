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

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// DefaultMarginHistoryDir is an alias for constants.StateMargin to preserve
// backward compatibility for external callers.
const DefaultMarginHistoryDir = constants.StateMargin

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
	WorkDir      string
	Provider     *marketdata.TWSEMarginBalanceProvider
	LookbackDays int
	// StartDate bounds the backfill window (inclusive). Zero means the
	// legacy default window: EndDate - LookbackDays + 1.
	StartDate time.Time
	// EndDate bounds the backfill window (inclusive). Zero means time.Now().
	EndDate time.Time
	// MaxRetries is the number of fetch retries per date after the first
	// failure. Zero means 3; values above 3 are capped at 3.
	MaxRetries int
}

func NewMarginHistoryBackfiller(workDir string) *MarginHistoryBackfiller {
	marginDir := filepath.Join(workDir, DefaultMarginHistoryDir)
	return &MarginHistoryBackfiller{
		WorkDir:      workDir,
		Provider:     marketdata.NewTWSEMarginBalanceProvider(marginDir),
		LookbackDays: 30,
	}
}

// Backfill fetches margin balance snapshots for every trading day in the
// [StartDate, EndDate] window (defaults: EndDate = now, StartDate =
// EndDate-LookbackDays+1), skipping existing files, weekends, and failed
// dates after MaxRetries attempts. Non-trading holidays are handled by the
// provider's internal 7-day backward scan, so a holiday does not create a
// file for its own date. Per-date failures are logged, not fatal; the only
// fatal error is an invalid window (StartDate after EndDate).
func (b *MarginHistoryBackfiller) Backfill(ctx context.Context) error {
	marginDir := filepath.Join(b.WorkDir, DefaultMarginHistoryDir)
	if err := os.MkdirAll(marginDir, 0o750); err != nil {
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

	end := b.EndDate
	if end.IsZero() {
		end = time.Now()
	}
	start := b.StartDate
	if start.IsZero() {
		start = end.AddDate(0, 0, -(b.LookbackDays - 1))
	}
	if start.After(end) {
		return fmt.Errorf("margin backfill: start %s after end %s",
			start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	maxRetries := b.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if maxRetries > 3 {
		maxRetries = 3
	}

	fetched := 0
	skipped := 0
	failed := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		dateStr := d.Format("20060102")
		if existingDates[dateStr] {
			skipped++
			continue
		}
		if b.fetchWithRetry(ctx, d, dateStr, maxRetries) {
			existingDates[dateStr] = true
			fetched++
		} else {
			failed++
		}
	}

	logging.Info("margin_backfill", "complete",
		logging.FStr("start", start.Format("2006-01-02")),
		logging.FStr("end", end.Format("2006-01-02")),
		logging.FInt("fetched", fetched),
		logging.FInt("skipped_existing", skipped),
		logging.FInt("failed", failed),
		logging.FInt("max_retries", maxRetries))
	return nil
}

// fetchWithRetry fetches a single date, retrying up to maxRetries times after
// the first failure. Returns true on success.
func (b *MarginHistoryBackfiller) fetchWithRetry(ctx context.Context, d time.Time, dateStr string, maxRetries int) bool {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if _, err := b.Provider.FetchSnapshotForDate(ctx, d); err == nil {
			return true
		} else {
			logging.Warn("margin_backfill", "fetch_failed",
				logging.Err(err), logging.FStr("date", dateStr),
				logging.FInt("attempt", attempt+1), logging.FInt("max_attempts", maxRetries+1))
		}
	}
	return false
}
