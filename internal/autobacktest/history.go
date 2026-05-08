package autobacktest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type AutoSnapshot struct {
	Date          time.Time `json:"date"`
	PortfolioVal  float64   `json:"portfolio_value"`
	VaR95         float64   `json:"var_95"`
	SharpeShort   float64   `json:"sharpe_short"`
	SharpeLong    float64   `json:"sharpe_long"`
	DrawdownPct   float64   `json:"drawdown_pct"`
	SignalCount   int       `json:"signal_count"`
	ActiveSignals []string  `json:"active_signals"`
	ShortTermAvg  float64   `json:"short_term_avg"`
	LongTermAvg   float64   `json:"long_term_avg"`
	DeltaPct      float64   `json:"delta_pct"`
}

type History struct {
	baseDir string
}

func NewHistory(baseDir string) *History {
	return &History{baseDir: baseDir}
}

func (h *History) Append(snapshot AutoSnapshot) error {
	dir := filepath.Join(h.baseDir, "autobacktest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	filename := filepath.Join(dir, "snapshots.jsonl")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if _, err := fmt.Fprintln(f, string(data)); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}

func (h *History) LatestN(n int) ([]AutoSnapshot, error) {
	filename := filepath.Join(h.baseDir, "autobacktest", "snapshots.jsonl")
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var snapshots []AutoSnapshot
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var s AutoSnapshot
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			continue
		}
		snapshots = append(snapshots, s)
	}

	if len(snapshots) <= n {
		return snapshots, nil
	}
	return snapshots[len(snapshots)-n:], nil
}
