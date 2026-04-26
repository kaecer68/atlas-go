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
