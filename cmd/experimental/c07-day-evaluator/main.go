// Command c07-day-evaluator automates the Day 7 / Day 14 acceptance gate
// evaluation for C07 sector direction predictions. It reads the observation
// log, computes acceptance criteria per runbook §3, and generates a
// pass/fail report.
//
// Usage:
//
//	go run ./cmd/experimental/c07-day-evaluator [flags]
//
// Flags:
//
//	-obs-log    path to observation log (default docs/operations/sector-prediction-observation-log.md)
//	-day        evaluation day (7 or 14)
//	-output     output report path (default stdout)
//	-baseline   baseline hit-rate for Day 14 comparison (default 0.55)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultObsLog   = ".omo/evidence/sector-prediction-observation-log.md"
	defaultBaseline = 0.55
)

// obsRow represents one row in the observation log.
type obsRow struct {
	Date                 string
	SectorCount          int
	JSDAlertRate         float64
	LatencyP95Ms         int64
	ConfidenceViolations int
	PanicCount           int
	SpotCheckCount       int
	Notes                string
}

// evalResult holds the evaluation outcome.
type evalResult struct {
	Day       int
	Passed    bool
	Criteria  []criterionResult
	Summary   string
	Generated string
}

type criterionResult struct {
	Name     string
	Passed   bool
	Actual   string
	Expected string
	Severity string // "must" or "should"
	Note     string
}

func main() {
	var (
		obsLog   = flag.String("obs-log", defaultObsLog, "path to observation log")
		day      = flag.Int("day", 7, "evaluation day (7 or 14)")
		output   = flag.String("output", "", "output report path (default stdout)")
		baseline = flag.Float64("baseline", defaultBaseline, "baseline hit-rate for Day 14 comparison")
	)
	flag.Parse()

	if *day != 7 && *day != 14 {
		fmt.Fprintf(os.Stderr, "ERROR: day must be 7 or 14, got %d\n", *day)
		os.Exit(2)
	}

	rows, err := parseObsLog(*obsLog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parse obs log: %v\n", err)
		os.Exit(1)
	}

	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: no rows in obs log %s\n", *obsLog)
		os.Exit(1)
	}

	result := evaluate(rows, *day, *baseline)
	report := formatReport(result)

	if *output != "" {
		if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: mkdir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*output, []byte(report), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: write report: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to %s\n", *output)
	} else {
		fmt.Println(report)
	}

	if !result.Passed {
		os.Exit(1)
	}
	os.Exit(0)
}

// parseObsLog reads the observation log and parses table rows.
func parseObsLog(path string) ([]obsRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var rows []obsRow
	inRecords := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "## Records" {
			inRecords = true
			continue
		}
		if !inRecords || !strings.HasPrefix(line, "|") || strings.Contains(line, "日期") {
			continue
		}
		// Skip comment lines and empty lines.
		if strings.HasPrefix(line, "<!--") || line == "" {
			continue
		}

		cells := splitTableRow(line)
		if len(cells) < 8 {
			continue
		}

		row := obsRow{
			Date:                 strings.TrimSpace(cells[1]),
			SectorCount:          parseInt(cells[2]),
			JSDAlertRate:         parsePercent(cells[3]),
			LatencyP95Ms:         int64(parseInt(cells[4])),
			ConfidenceViolations: parseInt(cells[5]),
			PanicCount:           parseInt(cells[6]),
			SpotCheckCount:       parseInt(cells[7]),
			Notes:                strings.TrimSpace(cells[8]),
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// splitTableRow splits a markdown table row into cells.
func splitTableRow(line string) []string {
	var cells []string
	start := 0
	for i, c := range line {
		if c == '|' {
			cells = append(cells, line[start:i])
			start = i + 1
		}
	}
	cells = append(cells, line[start:])
	return cells
}

// parseInt parses an integer from a string, returning 0 on failure.
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s)
	return n
}

// parsePercent parses a percentage string like "5.0%" into 0.05.
func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	f, _ := strconv.ParseFloat(s, 64)
	return f / 100
}

// evaluate computes acceptance criteria for the given day.
func evaluate(rows []obsRow, day int, baseline float64) *evalResult {
	result := &evalResult{
		Day:       day,
		Generated: fmt.Sprintf("Day %d evaluation", day),
	}

	// Use the most recent row for evaluation.
	latest := rows[len(rows)-1]

	// Day 7 criteria (must all pass).
	result.Criteria = append(
		result.Criteria,
		criterionResult{
			Name:     "jsd.alert_rate < 5%",
			Passed:   latest.JSDAlertRate < 0.05,
			Actual:   fmt.Sprintf("%.1f%%", latest.JSDAlertRate*100),
			Expected: "< 5%",
			Severity: "must",
			Note:     "超標 → 檢查 macro weight 與 cycle shift",
		},
		criterionResult{
			Name:     "latency_p95 < 200ms",
			Passed:   latest.LatencyP95Ms < 200,
			Actual:   fmt.Sprintf("%dms", latest.LatencyP95Ms),
			Expected: "< 200ms",
			Severity: "must",
			Note:     "超標 → 改為 cron 預計算",
		},
		criterionResult{
			Name:     "confidence.floor_violations = 0",
			Passed:   latest.ConfidenceViolations == 0,
			Actual:   fmt.Sprintf("%d", latest.ConfidenceViolations),
			Expected: "= 0",
			Severity: "must",
			Note:     "違反 → 立即排查 (invariant I7)",
		},
		criterionResult{
			Name:     "sector.count_per_day = 20",
			Passed:   latest.SectorCount == 20,
			Actual:   fmt.Sprintf("%d", latest.SectorCount),
			Expected: "= 20",
			Severity: "must",
			Note:     "違反 → 檢查 industry.L1Sectors()",
		},
		criterionResult{
			Name:     "panic_count = 0",
			Passed:   latest.PanicCount == 0,
			Actual:   fmt.Sprintf("%d", latest.PanicCount),
			Expected: "= 0",
			Severity: "must",
			Note:     "觸發 → 立即 rollback",
		},
		criterionResult{
			Name:     "spot_check_count >= 15",
			Passed:   latest.SpotCheckCount >= 15,
			Actual:   fmt.Sprintf("%d", latest.SpotCheckCount),
			Expected: ">= 15",
			Severity: "must",
			Note:     "不足 → 延長觀察至 day 14",
		},
	)

	// Day 14 additional criteria.
	if day == 14 {
		// Hit-rate comparison (requires baseline).
		// Note: This is a placeholder — actual hit-rate computation requires
		// historical outcome data which is deferred per PR #1200.
		result.Criteria = append(
			result.Criteria,
			criterionResult{
				Name:     "hit-rate >= baseline (Δ >= -3%)",
				Passed:   true, // deferred — no historical data yet
				Actual:   "deferred",
				Expected: fmt.Sprintf(">= %.1f%%", baseline*100),
				Severity: "should",
				Note:     "需歷史板塊報酬才能計算；標記為未來升級條件",
			},
			criterionResult{
				Name:     "driver explainability >= 20 spot-checks",
				Passed:   latest.SpotCheckCount >= 20,
				Actual:   fmt.Sprintf("%d", latest.SpotCheckCount),
				Expected: ">= 20",
				Severity: "must",
				Note:     "每筆 driver 至少引用 1 個具體 macro/cycle/event",
			},
			criterionResult{
				Name:     "rollback verified",
				Passed:   false, // manual verification required
				Actual:   "manual",
				Expected: "verified",
				Severity: "must",
				Note:     "至少一次手動測試把 flag 翻回未設置",
			},
		)
	}

	// Compute overall pass/fail.
	allMustPassed := true
	for _, c := range result.Criteria {
		if c.Severity == "must" && !c.Passed {
			allMustPassed = false
			break
		}
	}
	result.Passed = allMustPassed

	if result.Passed {
		result.Summary = fmt.Sprintf("Day %d acceptance: ALL MUST criteria PASSED", day)
	} else {
		result.Summary = fmt.Sprintf("Day %d acceptance: SOME MUST criteria FAILED", day)
	}

	return result
}

// formatReport generates a markdown report from the evaluation result.
func formatReport(r *evalResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# C07 Sector Prediction — %s\n\n", r.Generated)
	fmt.Fprintf(&b, "**Date**: %s\n\n", timeNow())
	fmt.Fprintf(&b, "**Result**: %s\n\n", r.Summary)

	b.WriteString("## Criteria\n\n")
	b.WriteString("| Criterion | Actual | Expected | Severity | Result | Note |\n")
	b.WriteString("|-----------|--------|----------|----------|--------|------|\n")
	for _, c := range r.Criteria {
		status := "✅ PASS"
		if !c.Passed {
			status = "❌ FAIL"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			c.Name, c.Actual, c.Expected, c.Severity, status, c.Note)
	}

	b.WriteString("\n## Next Steps\n\n")
	if r.Passed {
		if r.Day == 7 {
			b.WriteString("- Continue observation to Day 14\n")
			b.WriteString("- Day 14 evaluation: `go run ./cmd/experimental/c07-day-evaluator -day 14`\n")
		} else {
			b.WriteString("- Promotion gate passed — proceed to runbook §5 Promotion Procedure\n")
		}
	} else {
		b.WriteString("- Trigger runbook §4 Rollback Procedure\n")
		b.WriteString("- File follow-up issue with root cause analysis\n")
		b.WriteString("- Do NOT re-enable flag until issue is resolved\n")
	}

	return b.String()
}

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05 MST")
}
