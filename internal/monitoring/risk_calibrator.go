package monitoring

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/risk"
)

type sessionCalibrationProvider struct {
	stateDir string
}

func NewSessionCalibrationProvider(stateDir string) risk.CalibrationProvider {
	return &sessionCalibrationProvider{stateDir: stateDir}
}

func (p *sessionCalibrationProvider) RecentSessions(ctx context.Context, limit int) ([]risk.SessionOutcome, error) {
	pattern := filepath.Join(p.stateDir, "sessions", "session-*")
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	type dirWithTime struct {
		path  string
		mtime time.Time
	}
	var sorted []dirWithTime
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			continue
		}
		sorted = append(sorted, dirWithTime{path: d, mtime: info.ModTime()})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].mtime.After(sorted[j].mtime)
	})

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}

	var sessions []risk.SessionOutcome
	for _, sd := range sorted {
		session, err := p.loadSession(sd.path)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (p *sessionCalibrationProvider) loadSession(dir string) (risk.SessionOutcome, error) {
	s := risk.SessionOutcome{
		SectorExposures: make(map[string]float64),
		PositionValues:  make(map[string]float64),
	}

	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return s, err
	}
	var summary struct {
		SessionID      string  `json:"session_id"`
		PortfolioValue float64 `json:"portfolio_value"`
		EndingCash     float64 `json:"ending_cash"`
		RecordedAt     string  `json:"recorded_at"`
	}
	if json.Unmarshal(data, &summary) != nil {
		return s, nil
	}

	s.SessionID = summary.SessionID
	s.PortfolioValue = summary.PortfolioValue
	s.EndingCash = summary.EndingCash
	if t, err := time.Parse(time.RFC3339, summary.RecordedAt); err == nil {
		s.Timestamp = t
	}

	data, err = os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		return s, nil
	}

	var forwardReturns []float64
	lines := splitJSONLLines(data)
	for _, line := range lines {
		var outcome struct {
			Symbol        string  `json:"symbol"`
			Side          string  `json:"side"`
			Price         float64 `json:"price"`
			Conviction    int     `json:"conviction"`
			ForwardReturn float64 `json:"forward_return"`
			Hit           bool    `json:"hit"`
		}
		if json.Unmarshal([]byte(line), &outcome) != nil {
			continue
		}
		if outcome.Symbol == "" {
			continue
		}

		notional := s.PortfolioValue * float64(outcome.Conviction) / 100.0 * 0.1
		if notional <= 0 {
			notional = s.PortfolioValue * 0.05
		}

		order := risk.HistoricOrder{
			Symbol:        outcome.Symbol,
			Side:          outcome.Side,
			Notional:      notional,
			Sector:        "unknown",
			ForwardReturn: outcome.ForwardReturn,
			Hit:           outcome.Hit,
		}
		if order.Side == "" {
			order.Side = "buy"
		}
		s.Orders = append(s.Orders, order)
		forwardReturns = append(forwardReturns, outcome.ForwardReturn)
	}

	if len(forwardReturns) > 0 {
		s.ForwardReturnAvg = avg(forwardReturns)
		s.ForwardReturnStdDev = stdev(forwardReturns, s.ForwardReturnAvg)
	}

	return s, nil
}

func splitJSONLLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdev(vals []float64, mean_ float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		d := v - mean_
		sum += d * d
	}
	return mathSqrt(sum / float64(len(vals)-1))
}

func mathSqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	s := x / 2
	for range 10 {
		s = (s + x/s) / 2
	}
	return s
}
