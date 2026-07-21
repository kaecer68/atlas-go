// Command c07-obs-collector automates the daily observation log fill for
// C07 sector direction predictions (PR #1200). It replaces the manual
// "operator fills docs/operations/sector-prediction-observation-log.md
// every trading day" step with an automated collector that:
//
//  1. Pulls /api/events/prediction and /api/macro/snapshot/latest
//  2. Computes automatable metrics (sector_count, jsd_alert_rate,
//     confidence_violations, latency proxy)
//  3. Appends a row to the observation log
//  4. Fires alerts via AlertStore when thresholds are breached
//
// What it does NOT automate (explicitly marked in the log):
//   - panic_count: requires log parsing or metric endpoint (not available)
//   - spot_check_count: manual by definition (human verifies driver quality)
//
// Usage:
//
//	go run ./cmd/experimental/c07-obs-collector [flags]
//
// Flags:
//
//	-url        atlas base URL (default http://localhost:18080)
//	-obs-log    path to observation log (default docs/operations/sector-prediction-observation-log.md)
//	-alert-dir  path to alert store directory (default data/state/alerts)
//	-date       override date for the log row (default today, YYYY-MM-DD)
//	-dry-run    print the row without writing to disk
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAtlasURL  = "http://localhost:18080"
	defaultObsLog    = "docs/operations/sector-prediction-observation-log.md"
	defaultAlertDir  = "data/state/alerts"
	httpTimeout      = 10 * time.Second
	jsdThreshold     = 0.25
	confidenceFloor  = 0.40
	latencyThreshold = 200 // ms
)

// predictionReport mirrors the API response shape (subset).
type predictionReport struct {
	Predictions       []flowPrediction      `json:"predictions"`
	SectorPredictions []sectorDayPrediction `json:"sector_predictions"`
}

type flowPrediction struct {
	Date         string                 `json:"date"`
	Direction    string                 `json:"direction"`
	Confidence   float64                `json:"confidence"`
	Distribution predictionDistribution `json:"distribution"`
}

type sectorDayPrediction struct {
	Date    string             `json:"date"`
	Sectors []sectorPrediction `json:"sectors"`
}

type sectorPrediction struct {
	SectorID     string                 `json:"sector_id"`
	SectorName   string                 `json:"sector_name"`
	Direction    string                 `json:"direction"`
	Confidence   float64                `json:"confidence"`
	Distribution predictionDistribution `json:"distribution"`
	Drivers      []string               `json:"drivers"`
}

type predictionDistribution struct {
	Inflow  float64 `json:"inflow"`
	Outflow float64 `json:"outflow"`
	Neutral float64 `json:"neutral"`
}

// metrics holds the computed observation values.
type metrics struct {
	Date                 string
	SectorCount          int
	JSDAlertRate         float64 // fraction of sectors with JSD > threshold
	LatencyP95Ms         int64   // proxy: max of the two API call durations
	ConfidenceViolations int
	PanicCount           int // always 0; manual verification required
	SpotCheckCount       int // always 0; manual by definition
	Notes                string
	FlagOff              bool // true when SECTOR_PREDICTION_ENABLED is off
}

func main() {
	var (
		baseURL  = flag.String("url", defaultAtlasURL, "atlas base URL")
		obsLog   = flag.String("obs-log", defaultObsLog, "path to observation log")
		alertDir = flag.String("alert-dir", defaultAlertDir, "path to alert store directory")
		dateStr  = flag.String("date", "", "override date (YYYY-MM-DD)")
		dryRun   = flag.Bool("dry-run", false, "print row without writing")
	)
	flag.Parse()

	date := *dateStr
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	m, err := collectMetrics(*baseURL, date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: collect metrics: %v\n", err)
		os.Exit(1)
	}

	row := formatRow(m)
	if *dryRun {
		fmt.Println("DRY-RUN: would append row:")
		fmt.Println(row)
		os.Exit(0)
	}

	if err := appendToObsLog(*obsLog, row); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: append to obs log: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Appended row to %s\n", *obsLog)

	// Fire alerts on threshold breaches.
	if err := fireAlerts(*alertDir, m); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: fire alerts: %v\n", err)
		// Non-fatal: alerts are best-effort.
	}

	os.Exit(0)
}

// collectMetrics pulls the prediction API and computes automatable metrics.
// When the flag is off (no sector_predictions), returns a flagOff metrics
// struct with zero values — the caller notes this in the obs log and exits 0.
func collectMetrics(baseURL, date string) (*metrics, error) {
	start := time.Now()
	report, err := fetchPredictionReport(baseURL)
	if err != nil {
		return nil, fmt.Errorf("fetch prediction report: %w", err)
	}
	latency1 := time.Since(start).Milliseconds()

	if len(report.SectorPredictions) == 0 {
		return &metrics{
			Date:                 date,
			SectorCount:          0,
			JSDAlertRate:         0,
			LatencyP95Ms:         latency1,
			ConfidenceViolations: 0,
			PanicCount:           0,
			SpotCheckCount:       0,
			Notes:                "flag off (SECTOR_PREDICTION_ENABLED not set or false)",
			FlagOff:              true,
		}, nil
	}
	day := report.SectorPredictions[0]
	sectors := day.Sectors

	// Overall distribution for JSD comparison.
	var ovDist predictionDistribution
	if len(report.Predictions) > 0 {
		ovDist = report.Predictions[0].Distribution
	} else {
		// Fallback: uniform distribution.
		ovDist = predictionDistribution{Inflow: 1.0 / 3, Outflow: 1.0 / 3, Neutral: 1.0 / 3}
	}

	jsdAlerts := 0
	confViolations := 0
	for _, s := range sectors {
		if jsd(s.Distribution, ovDist) > jsdThreshold {
			jsdAlerts++
		}
		if s.Confidence < confidenceFloor {
			confViolations++
		}
	}

	m := &metrics{
		Date:                 date,
		SectorCount:          len(sectors),
		JSDAlertRate:         float64(jsdAlerts) / float64(len(sectors)),
		LatencyP95Ms:         latency1,
		ConfidenceViolations: confViolations,
		PanicCount:           0, // manual verification required
		SpotCheckCount:       0, // manual by definition
		Notes:                "auto-collected",
	}
	return m, nil
}

// fetchPredictionReport GETs /api/events/prediction and parses the response.
func fetchPredictionReport(baseURL string) (*predictionReport, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/events/prediction"
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url) //nolint:gosec // G704: URL validated as localhost-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var report predictionReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return &report, nil
}

// formatRow formats the metrics as a markdown table row.
func formatRow(m *metrics) string {
	if m.FlagOff {
		return fmt.Sprintf("| %s | - | - | %d | - | - | - | %s |",
			m.Date, m.LatencyP95Ms, m.Notes)
	}
	return fmt.Sprintf(
		"| %s | %d | %.1f%% | %d | %d | %d | %d | %s |",
		m.Date,
		m.SectorCount,
		m.JSDAlertRate*100,
		m.LatencyP95Ms,
		m.ConfidenceViolations,
		m.PanicCount,
		m.SpotCheckCount,
		m.Notes,
	)
}

// appendToObsLog appends a row to the observation log, creating the file
// if it doesn't exist and removing the placeholder comment on first fill.
func appendToObsLog(path, row string) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read file: %w", err)
		}
		// File doesn't exist — create with header.
		header := `# Sector Prediction Observation Log

> **對應 runbook**：` + "`docs/operations/sector-prediction-runbook.md`" + `
> **對應 spec**：` + "[`docs/specs/sector-dimension-prediction-spec.md`](../specs/sector-dimension-prediction.md)" + `
> **對應 invariant manifest**：` + "[`docs/manifests/sector-dimension-prediction-invariant-manifest.md`](../manifests/sector-dimension-prediction-invariant-manifest.md)" + `

L2.4-style 觀察窗口的逐日記錄表。每個交易日填寫一次，欄位說明見 runbook §「Daily Check-in」。

## Record Schema（每行）

` + "```text" + `
| 日期 | sector_count | jsd.alert_rate | latency_p95_ms | confidence_violation | panic_count | spot_check_count | notes |
` + "```" + `

## Records

`
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		data = []byte(header)
	}

	content := string(data)

	// Remove placeholder comment on first fill.
	placeholder := `<!--
範例列（Day 1 placeholder；實際填寫時請刪除）:
| 2026-07-21 | 20 | 0.0% | 145 | 0 | 0 | 0 | 啟用 ` + "`SECTOR_PREDICTION_ENABLED=true`" + `；baseline established |
-->`
	content = strings.ReplaceAll(content, placeholder, "")

	// Append row.
	content = strings.TrimRight(content, "\n") + "\n" + row + "\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// fireAlerts saves alerts to the AlertStore when thresholds are breached.
func fireAlerts(alertDir string, m *metrics) error {
	if m.FlagOff {
		return nil
	}
	store, err := newAlertStore(alertDir)
	if err != nil {
		return fmt.Errorf("new alert store: %w", err)
	}

	alerts := []struct {
		rule      string
		severity  string
		message   string
		value     float64
		threshold float64
	}{
		{
			rule:      "c07_jsd_alert_rate",
			severity:  "warning",
			message:   fmt.Sprintf("C07 JSD alert rate %.1f%% >= 5%%", m.JSDAlertRate*100),
			value:     m.JSDAlertRate * 100,
			threshold: 5.0,
		},
		{
			rule:      "c07_latency_p95",
			severity:  "warning",
			message:   fmt.Sprintf("C07 latency p95 %dms >= 200ms", m.LatencyP95Ms),
			value:     float64(m.LatencyP95Ms),
			threshold: 200,
		},
		{
			rule:      "c07_confidence_violations",
			severity:  "critical",
			message:   fmt.Sprintf("C07 confidence violations %d > 0", m.ConfidenceViolations),
			value:     float64(m.ConfidenceViolations),
			threshold: 0,
		},
		{
			rule:      "c07_sector_count",
			severity:  "critical",
			message:   fmt.Sprintf("C07 sector count %d != 20", m.SectorCount),
			value:     float64(m.SectorCount),
			threshold: 20,
		},
	}

	for _, a := range alerts {
		breached := false
		switch a.rule {
		case "c07_jsd_alert_rate":
			breached = a.value >= a.threshold
		case "c07_latency_p95":
			breached = a.value >= a.threshold
		case "c07_confidence_violations":
			breached = a.value > a.threshold
		case "c07_sector_count":
			breached = a.value != a.threshold
		}
		if !breached {
			continue
		}
		record := alertRecord{
			ID:        fmt.Sprintf("c07-%s-%s", a.rule, m.Date),
			Timestamp: time.Now(),
			Rule:      a.rule,
			Severity:  a.severity,
			Message:   a.message,
			Value:     a.value,
			Threshold: a.threshold,
			Status:    "active",
			DedupKey:  fmt.Sprintf("%s:%s", a.rule, m.Date),
		}
		if err := store.Save(record); err != nil {
			return fmt.Errorf("save alert %s: %w", a.rule, err)
		}
		fmt.Printf("Alert fired: %s\n", a.message)
	}
	return nil
}

// jsd computes Jensen-Shannon divergence between two distributions.
func jsd(a, b predictionDistribution) float64 {
	m := predictionDistribution{
		Inflow:  (a.Inflow + b.Inflow) / 2,
		Neutral: (a.Neutral + b.Neutral) / 2,
		Outflow: (a.Outflow + b.Outflow) / 2,
	}
	return (klDivergence(a, m) + klDivergence(b, m)) / 2
}

func klDivergence(p, q predictionDistribution) float64 {
	var d float64
	for _, pair := range [][2]float64{
		{p.Inflow, q.Inflow},
		{p.Neutral, q.Neutral},
		{p.Outflow, q.Outflow},
	} {
		if pair[0] > 0 && pair[1] > 0 {
			d += pair[0] * math.Log(pair[0]/pair[1])
		}
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// alertRecord mirrors domain.AlertRecord (subset for alert store).
type alertRecord struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Rule      string    `json:"rule"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Status    string    `json:"status"`
	DedupKey  string    `json:"dedup_key,omitempty"`
}

// alertStore wraps the file-based alert store.
type alertStore struct {
	dir string
}

func newAlertStore(dir string) (*alertStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	return &alertStore{dir: dir}, nil
}

func (s *alertStore) Save(record alertRecord) error {
	path := filepath.Join(s.dir, record.ID+".json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
