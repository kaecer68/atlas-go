package bootstrap

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	monitoring "github.com/kaecer68/atlas-go/internal/monitoring"
)

func StartChannelHealthSyncLoop(ctx context.Context, workDir string, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				healthStore := monitoring.NewChannelHealthStoreWithPool(filepath.Join(workDir, "data/state"), pool)
				if err := healthStore.SyncAllToDB(); err != nil {
					logging.Error("channel_health", "background_sync_failed", "err", err)
				}
			}
		}
	}()
}

func StartAutoBackfill(ctx context.Context, workDir, replayDataPath string) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}

		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			absWorkDir = workDir
		}

		latestDate, err := getLatestReplayDate(replayDataPath)
		if err != nil {
			logging.Error("bootstrap", "replay_csv_read_failed", "err", err)
			return
		}

		now := time.Now()
		if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
			now = now.In(tz)
		}

		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
			end = end.AddDate(0, 0, -1)
		}

		start := latestDate.AddDate(0, 0, 1)
		for start.Weekday() == time.Saturday || start.Weekday() == time.Sunday {
			start = start.AddDate(0, 0, 1)
		}
		for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
			end = end.AddDate(0, 0, -1)
		}

		if start.After(end) {
			logging.Info("bootstrap", "no_backfill_gap_detected", "latest_date", latestDate.Format("2006-01-02"), "target_date", end.Format("2006-01-02"))
			return
		}

		startStr := start.Format("2006-01-02")
		endStr := end.Format("2006-01-02")
		logging.Info("bootstrap", "backfill_gap_detected", "start_date", startStr, "end_date", endStr)

		bgCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		var cmd *exec.Cmd
		binaryPath := filepath.Join(absWorkDir, "daily-replay-sync")
		if _, err := os.Stat(binaryPath); err == nil {
			cmd = exec.CommandContext(bgCtx, binaryPath, "-csv", replayDataPath, "-backfill-start", startStr, "-backfill-end", endStr)
			cmd.Dir = absWorkDir
		} else if _, err := exec.LookPath("go"); err == nil {
			cmd = exec.CommandContext(bgCtx, "go", "run", "./cmd/daily-replay-sync", "-csv", replayDataPath, "-backfill-start", startStr, "-backfill-end", endStr)
			cmd.Dir = absWorkDir
		} else {
			logging.Warn("bootstrap", "backfill_binary_not_found")
			return
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			logging.Error("bootstrap", "backfill_failed", "err", err, "output", string(out))
			return
		}
		logging.Info("bootstrap", "backfill_success", "output", string(out))
	}()
}

func StartAutoCapitalFlowFetch(ctx context.Context, workDir string) {
	go func() {
		// Initial fetch after 5 seconds.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow"))

		// Fetch immediately on startup.
		doFetch := func() {
			bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			_, err := capFlowProvider.FetchSnapshot(bgCtx)
			if err != nil {
				logging.Error("bootstrap", "capital_flow_fetch_failed", "err", err)
				return
			}
			logging.Info("bootstrap", "capital_flow_fetch_succeeded")
		}
		doFetch()

		// Periodic fetch every 30 minutes during market hours (09:00-15:30 CST).
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
					now = now.In(tz)
				}
				// Only fetch during Taiwan market hours to avoid unnecessary API calls.
				if now.Weekday() != time.Saturday && now.Weekday() != time.Sunday {
					hour := now.Hour()
					if hour >= 9 && hour < 16 {
						doFetch()
					}
				}
			}
		}
	}()
}

func getLatestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	var latest time.Time
	_, _ = reader.Read()
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if len(row) == 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		if d.After(latest) {
			latest = d
		}
	}
	if latest.IsZero() {
		return time.Time{}, errors.New("no valid dates found")
	}
	return latest, nil
}
