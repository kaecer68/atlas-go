package bootstrap

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
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
					log.Printf("[ChannelHealth] background sync to DB failed: %v", err)
				}
			}
		}
	}()
}

func StartAutoBackfill(ctx context.Context, workDir string) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}

		csvPath := filepath.Join(workDir, "data/replay/tw_extended_90days.csv")
		latestDate, err := getLatestReplayDate(csvPath)
		if err != nil {
			log.Printf("[AutoBackfill] cannot read replay csv: %v", err)
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
			log.Printf("[AutoBackfill] no gap detected (latest=%s, target=%s)", latestDate.Format("2006-01-02"), end.Format("2006-01-02"))
			return
		}

		startStr := start.Format("2006-01-02")
		endStr := end.Format("2006-01-02")
		log.Printf("[AutoBackfill] detected gap %s ~ %s. Triggering backfill...", startStr, endStr)

		bgCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		var cmd *exec.Cmd
		binaryPath := filepath.Join(workDir, "backfill-replay")
		if _, err := os.Stat(binaryPath); err == nil {
			cmd = exec.CommandContext(bgCtx, binaryPath, "-csv", csvPath, "-start", startStr, "-end", endStr)
		} else if _, err := exec.LookPath("go"); err == nil {
			cmd = exec.CommandContext(bgCtx, "go", "run", "./cmd/backfill-replay", "-csv", csvPath, "-start", startStr, "-end", endStr)
			cmd.Dir = workDir
		} else {
			log.Printf("[AutoBackfill] backfill-replay binary not found and go not in PATH; skipping auto-backfill")
			return
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[AutoBackfill] backfill failed: %v\noutput: %s", err, string(out))
			return
		}
		log.Printf("[AutoBackfill] success:\n%s", string(out))
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
		healthStore := monitoring.NewChannelHealthStoreWithPool(filepath.Join(workDir, "data/state"), nil)

		// Fetch immediately on startup.
		doFetch := func() {
			bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			_, err := capFlowProvider.FetchSnapshot(bgCtx)
			if err != nil {
				log.Printf("[AutoCapitalFlow] fetch failed: %v", err)
				_ = healthStore.Record("twse_capital_flow", "error", err.Error())
				return
			}
			log.Printf("[AutoCapitalFlow] fetch succeeded")
			_ = healthStore.Record("twse_capital_flow", "ok", "")
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

// StartEncodingDaemon periodically checks for PascalCase summary.json files and auto-fixes them.
func StartEncodingDaemon(ctx context.Context, ledgerDir string) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}

		// Run immediately, then every 5 minutes
		doCheck := func() {
			sessionsDir := filepath.Join(ledgerDir, "sessions")
			entries, err := os.ReadDir(sessionsDir)
			if err != nil {
				return
			}
			for _, e := range entries {
				if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
					continue
				}
				sp := filepath.Join(sessionsDir, e.Name(), "summary.json")
				data, err := os.ReadFile(sp)
				if err != nil {
					continue
				}
				if !strings.Contains(string(data[:200]), `"SessionID"`) {
					continue
				}
				// PascalCase detected — auto-fix
				log.Printf("[EncodingDaemon] fixing PascalCase: %s", e.Name())
				var raw map[string]any
				if json.Unmarshal(data, &raw) != nil {
					continue
				}
				fixed := fixKeys(raw)
				newData, _ := json.MarshalIndent(fixed, "", "  ")
				tmp := sp + ".tmp"
				if err := os.WriteFile(tmp, newData, 0o644); err != nil {
					continue
				}
				_ = os.Rename(tmp, sp)
			}
		}

		doCheck()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				doCheck()
			}
		}
	}()
}

func fixKeys(obj any) any {
	switch v := obj.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, val := range v {
			result[keyMap(k)] = fixKeys(val)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = fixKeys(val)
		}
		return result
	default:
		return v
	}
}

func keyMap(k string) string {
	m := map[string]string{
		"SessionID": "session_id", "Regime": "regime", "OrderCount": "order_count",
		"PositionCount": "position_count", "EndingCash": "ending_cash",
		"PortfolioValue": "portfolio_value", "OutcomeCount": "outcome_count",
		"RecordedAt": "recorded_at", "BrokerRuntime": "broker_runtime",
		"GuardOutcomes": "guard_outcomes", "Mode": "mode", "Adapter": "adapter",
		"Signer": "signer", "SignerVersion": "signer_version", "KeyID": "key_id",
		"MaxRetries": "max_retries", "HTTPTimeoutSec": "http_timeout_sec",
		"HTTPAttempts": "http_attempts", "RetryStatusCodes": "retry_status_codes",
		"MaxClockSkewSec": "max_clock_skew_sec", "NonceTTLSec": "nonce_ttl_sec",
		"NonceStore": "nonce_store", "NonceStorePath": "nonce_store_path",
		"NonceRedisPrefix": "nonce_redis_prefix",
		"GuardID": "guard_id", "GuardSkill": "guard_skill", "Severity": "severity",
		"Passed": "passed", "Reason": "reason", "InputCount": "input_count",
		"OutputCount": "output_count", "NextExperimentAgentID": "next_experiment_agent_id",
		"ProposalID": "proposal_id", "CommitID": "commit_id", "ApprovalID": "approval_id",
	}
	if n, ok := m[k]; ok {
		return n
	}
	return k
}
