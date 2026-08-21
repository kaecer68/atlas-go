package experiment

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func ValidateReplayData(windowStart, windowEnd time.Time, replayPath string) (*domain.ReplayDataMetadata, error) {
	info, err := os.Stat(replayPath)
	if err != nil {
		return nil, fmt.Errorf("無法存取 replay 數據: %w", err)
	}

	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		return nil, fmt.Errorf("無法載入 replay 數據: %w", err)
	}

	if len(ds.Dates) == 0 {
		return nil, fmt.Errorf("replay 數據為空")
	}

	latestDate := ds.Dates[len(ds.Dates)-1]
	earliestDate := ds.Dates[0]

	meta := &domain.ReplayDataMetadata{
		SourcePath:     replayPath,
		DateRangeStart: earliestDate,
		DateRangeEnd:   latestDate,
		LastModified:   info.ModTime(),
		RecordCount:    len(ds.Dates),
	}

	meta.CoversWindow = !latestDate.Before(windowStart) && !earliestDate.After(windowEnd)

	if latestDate.Before(windowEnd) {
		delay := int(windowEnd.Sub(latestDate).Hours() / 24)
		return meta, fmt.Errorf(
			"數據不足：replay 數據最後日期為 %s，但實驗窗口結束於 %s（缺少 %d 天數據）。請先執行：go run ./cmd/daily-replay-sync",
			latestDate.Format("2006-01-02"),
			windowEnd.Format("2006-01-02"),
			delay,
		)
	}

	daysDelayed := int(time.Since(latestDate).Hours() / 24)
	meta.DaysDelayed = daysDelayed

	return meta, nil
}

// ReplayLatestDate returns the most recent trading date present in the replay
// dataset at replayPath. It shares the CSV loading path with ValidateReplayData
// so freshness checks and window validation always observe the same data
// (Phase B1: avoid loading the CSV twice for the freshness gate).
func ReplayLatestDate(replayPath string) (time.Time, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("無法載入 replay 數據: %w", err)
	}
	if len(ds.Dates) == 0 {
		return time.Time{}, fmt.Errorf("replay 數據為空")
	}
	return ds.Dates[len(ds.Dates)-1], nil
}

// resolveExperimentWindow applies the Phase B1 replay freshness gate before a
// window ID is built. When the replay dataset lags "now" by exactly one
// calendar day, the window is deferred (shifted) to end at the replay's latest
// date so the experiment runs against available data instead of failing with
// ValidateReplayData's 數據不足 error. The deferred window is
// [latestDate-6d, latestDate] — a constant 7-day window (7 distinct dates)
// that is never shortened. A lag of two or more days returns an error that
// directs operators to daily-replay-sync (B4 alerting covers the standing
// failure). A fresh dataset keeps the default trailing 7-day window unchanged.
func resolveExperimentWindow(now time.Time, replayPath string) (start, end time.Time, deferred bool, err error) {
	if replayPath == "" {
		// No replay path configured: leave the window untouched and let the
		// executor's validation surface the standard error.
		return now.Add(-7 * 24 * time.Hour), now, false, nil
	}
	latestDate, err := ReplayLatestDate(replayPath)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("replay 新鮮度檢查失敗: %w", err)
	}
	// Compare calendar dates (latestDate is always UTC midnight) so the lag is
	// measured in whole days regardless of the current time of day.
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch delayDays := int(nowDate.Sub(latestDate).Hours() / 24); {
	case delayDays >= 2:
		return time.Time{}, time.Time{}, false, fmt.Errorf(
			"replay 數據落後 %d 天（最後日期 %s，實驗窗口結束於 %s）：需人工檢查 daily-replay-sync 是否正常執行",
			delayDays, latestDate.Format("2006-01-02"), nowDate.Format("2006-01-02"))
	case delayDays == 1:
		return latestDate.Add(-6 * 24 * time.Hour), latestDate, true, nil
	default:
		// Fresh (same calendar day) or ahead: keep the default trailing window.
		return now.Add(-7 * 24 * time.Hour), now, false, nil
	}
}

func parseWindowDates(windowID string) (time.Time, time.Time, error) {
	parts := strings.Split(windowID, "-")
	if len(parts) != 3 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid window ID format: %s", windowID)
	}

	startDate, err := time.Parse("20060102", parts[1])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse window start date: %w", err)
	}

	endDate, err := time.Parse("20060102", parts[2])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse window end date: %w", err)
	}

	return startDate, endDate, nil
}
