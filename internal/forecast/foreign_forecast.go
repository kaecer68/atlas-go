// Package forecast provides transparent, rule-based forward-looking signal
// scoring for capital flow movements (manifest #E03). All weights, scales,
// and gates are documented in docs/specs/foreign-flow-forecast-spec.md and
// follow the §8 calibration philosophy: no black-box ML, every coefficient
// must be explainable to a retail user.
package forecast

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ForeignDirection is the predicted/actual next-day foreign spot net direction.
type ForeignDirection string

const (
	ForeignDirectionBullish ForeignDirection = "bullish"
	ForeignDirectionBearish ForeignDirection = "bearish"
	ForeignDirectionNeutral ForeignDirection = "neutral"
)

// SignificanceThresholdTWD: |actual net| must exceed this for a non-neutral
// outcome (30 億台幣 — see docs/specs/foreign-flow-forecast-spec.md §5).
const SignificanceThresholdTWD int64 = 3_000_000_000

// MinSamplesForCalibration: 90-day rolling window per §6 gate.
const MinSamplesForCalibration = 90

// MinHitRateForCalibration: 55% per §6 gate.
const MinHitRateForCalibration = 0.55

// Input carries the daily normalized features the scorecard consumes.
// All numeric features are in their raw units; scaling happens inside Score.
type Input struct {
	ForeignFuturesOIZ  float64 // 60-day Z-score of TAIFEX foreign futures OI net
	ForeignSpot5DSlope float64 // 過去 5 個交易日外資現貨淨額的線性回歸斜率 (億/日)
	TSMADRChangePct    float64
	SPXChangePct       float64
	NDXChangePct       float64
	USDTWDChangePct    float64 // 正 = 台幣升值
	VIX                float64
}

// Result is the prediction produced by Score.
type Result struct {
	Date        string
	Direction   ForeignDirection
	Probability float64 // 0..1
	Score       float64 // raw pre-squash score in [-1, 1]
}

// Score computes the v1 weighted scorecard. Returns direction + probability.
func Score(date string, in Input) Result {
	r := Result{Date: date}

	// Each component: weight * tanh(feature / scale). scale is the
	// "typical significant move" — at ±scale the feature saturates.
	r.Score += 0.30 * math.Tanh(in.ForeignFuturesOIZ/1.0)
	r.Score += 0.20 * math.Tanh(in.ForeignSpot5DSlope/15.0)
	r.Score += 0.15 * math.Tanh(in.TSMADRChangePct/2.0)
	r.Score += 0.15 * math.Tanh(in.SPXChangePct/1.5)
	r.Score += 0.10 * math.Tanh(in.NDXChangePct/2.0)
	// USD/TWD is inverted: stronger TWD (positive change) is bullish.
	r.Score += 0.10 * math.Tanh(-in.USDTWDChangePct/0.5)
	if in.VIX > 25 {
		r.Score -= 0.10 * math.Min((in.VIX-25)/10.0, 1.0)
	}

	r.Probability = clamp(0.5+0.5*r.Score, 0, 1)
	switch {
	case r.Probability >= 0.60:
		r.Direction = ForeignDirectionBullish
	case r.Probability <= 0.40:
		r.Direction = ForeignDirectionBearish
	default:
		r.Direction = ForeignDirectionNeutral
	}
	return r
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------------
// Ledger
// ---------------------------------------------------------------------------

// Record is one row in data/state/foreign_forecast/YYYYMMDD.json.
type Record struct {
	Date        string           `json:"date"`
	Direction   ForeignDirection `json:"predicted_direction"`
	Probability float64          `json:"probability"`
	Score       float64          `json:"score"`
	// Filled in on T+1:
	ActualOutcome ForeignDirection `json:"actual_outcome,omitempty"`
	ActualNet     int64            `json:"actual_net,omitempty"`
	Correct       *bool            `json:"correct,omitempty"` // nil = pending
}

// Ledger stores daily prediction records as YYYYMMDD.json files in dir.
type Ledger struct {
	dir string
}

// NewLedger creates a ledger rooted at dir (will be created on first write).
func NewLedger(dir string) *Ledger { return &Ledger{dir: dir} }

// Write persists a prediction record.
func (l *Ledger) Write(r Record) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("forecast mkdir: %w", err)
	}
	path := filepath.Join(l.dir, r.Date+".json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("forecast marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("forecast write: %w", err)
	}
	return nil
}

// Load reads a single record by date.
func (l *Ledger) Load(date string) (Record, error) {
	data, err := os.ReadFile(filepath.Join(l.dir, date+".json"))
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, err
	}
	return r, nil
}

// List returns the most recent n records sorted oldest-first.
func (l *Ledger) List(n int) ([]Record, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	dates := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || len(name) != len("YYYYMMDD.json") {
			continue
		}
		dates = append(dates, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(dates)
	if n > 0 && len(dates) > n {
		dates = dates[len(dates)-n:]
	}
	out := make([]Record, 0, len(dates))
	for _, d := range dates {
		r, err := l.Load(d)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Calibration gate
// ---------------------------------------------------------------------------

// CalibrationStatus summarizes the §6 gate for external exposure.
type CalibrationStatus struct {
	Calibrated bool
	Samples    int
	HitRate    float64
	Reason     string
}

// Calibrate evaluates the §6 gate from the ledger history.
func Calibrate(records []Record) CalibrationStatus {
	completed := 0
	hits := 0
	for _, r := range records {
		if r.Correct == nil {
			continue
		}
		completed++
		if *r.Correct {
			hits++
		}
	}
	if completed < MinSamplesForCalibration {
		return CalibrationStatus{
			Calibrated: false,
			Samples:    completed,
			HitRate:    safeRate(hits, completed),
			Reason:     fmt.Sprintf("校準中（樣本 %d/%d）", completed, MinSamplesForCalibration),
		}
	}
	hr := safeRate(hits, completed)
	if hr < MinHitRateForCalibration {
		return CalibrationStatus{
			Calibrated: false,
			Samples:    completed,
			HitRate:    hr,
			Reason:     fmt.Sprintf("校準中（命中率 %.1f%% < %.0f%%）", hr*100, MinHitRateForCalibration*100),
		}
	}
	return CalibrationStatus{
		Calibrated: true,
		Samples:    completed,
		HitRate:    hr,
		Reason:     "校準通過",
	}
}

// Judge updates a previous day's record with the actual outcome of T+1.
// `actualNetTWD` is the realized foreign spot net (positive=buy, negative=sell).
// Returns the updated record.
func Judge(prev Record, actualNetTWD int64) Record {
	actual := ForeignDirectionNeutral
	if actualNetTWD > SignificanceThresholdTWD {
		actual = ForeignDirectionBullish
	} else if actualNetTWD < -SignificanceThresholdTWD {
		actual = ForeignDirectionBearish
	}
	prev.ActualOutcome = actual
	prev.ActualNet = actualNetTWD
	correct := prev.Direction == actual
	prev.Correct = &correct
	return prev
}

func safeRate(hits, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// ---------------------------------------------------------------------------
// Convenience helpers
// ---------------------------------------------------------------------------

// TodayDate returns today's date in YYYYMMDD (Taipei timezone).
func TodayDate() string {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		tz = time.UTC
	}
	return time.Now().In(tz).Format("20060102")
}
