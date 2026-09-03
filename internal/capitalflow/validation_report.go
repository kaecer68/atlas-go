package capitalflow

// Validation report schema and writers for the Phase 1 hypothesis
// validator (plan §3.2 PR-1b). The report is the auditable artifact a
// human gate reads before any eligible flip; the tool itself only
// produces the report and NEVER writes state or config.
//
// Schema contract (versioned — do not rename JSON fields):
//
//	{
//	  "version": "1.0",
//	  "run_at": "...",
//	  "workdir": "...",
//	  "data_coverage": {"oi_days": 244, ...},
//	  "hypotheses": [ {id, status, verdict, sample_count, metrics,
//	                   thresholds, notes, started_at}, ... ],
//	  "eligible_recommendation": true|false,
//	  "operator_notes": ["...", ...],
//	  "method": {"hcf05_layered_probability_map": "...", ...}
//	}
//
// eligible_recommendation is true only when EVERY hypothesis judged
// PASS or PASS(improved). It is a suggestion for a human PR review —
// the actual eligibility flip happens exclusively through a separate
// config PR (plan §3.3; CF-INV-13).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ValidationReportVersion is the report schema version emitted by
// this code. Bump when the schema changes shape.
const ValidationReportVersion = "1.0"

// ValidationReport is the full Phase 1 run artifact.
type ValidationReport struct {
	Version      string             `json:"version"`
	RunAt        time.Time          `json:"run_at"`
	Workdir      string             `json:"workdir,omitempty"`
	DataCoverage map[string]int     `json:"data_coverage,omitempty"`
	Hypotheses   []HypothesisResult `json:"hypotheses"`
	// EligibleRecommendation is true only when every hypothesis came
	// back PASS / PASS(improved). Suggestion only — see file doc.
	EligibleRecommendation bool     `json:"eligible_recommendation"`
	OperatorNotes          []string `json:"operator_notes,omitempty"`
	// Method documents the pre-registered probability mappings and
	// scoring conventions that a reader must know to reproduce the
	// numbers (plan §3.1 B3).
	Method map[string]string `json:"method,omitempty"`
}

// BuildValidationReport assembles the report from the CLI run.
// coverage keys are free-form data-coverage counters (e.g.
// "oi_days", "t86_days", "taiex_days", "adr_days", "calendar_days").
func BuildValidationReport(workdir string, coverage map[string]int, results []HypothesisResult) ValidationReport {
	eligible := len(results) > 0
	for _, r := range results {
		switch r.Status {
		case ValidationPass, ValidationPassImproved:
		default:
			eligible = false
		}
	}
	return ValidationReport{
		Version:                ValidationReportVersion,
		RunAt:                  time.Now().UTC(),
		Workdir:                workdir,
		DataCoverage:           coverage,
		Hypotheses:             results,
		EligibleRecommendation: eligible,
		OperatorNotes: []string{
			"eligible_recommendation=true 僅供人工 PR review；真正翻轉 eligible 是另一個 config PR（capitalflow.calibration_eligible_override / capitalflow.eventdriven_baseline_eligible），CLI 永不寫 config。",
			"每日方向命中事件有自相關，i.i.d. binomial p 偏樂觀；報告同步輸出 block=5 交易日的 block-bootstrap p 作揭露，門檻仍以 binomial 為準（plan §3.1）。",
			"資料不足 252 交易日時 INSUFFICIENT_DATA 是第一級合法結論，不得以少樣本硬判 PASS/FAIL。",
		},
		Method: map[string]string{
			"hcf01_lag_selection":           "訓練窗（前 2/3）選 k*=argmax|rho_k|，k∈{1,2,3}，tie 取較小 k；選定後鎖定。",
			"hcf01_verdict":                 "|rho_k*|≥0.10 且 OOS 命中率≥55% 且 binomial p≤0.05 且 3 折 rho 符號一致。",
			"hcf02_verdict":                 "整體命中率≥55% 且 binomial p≤0.05 且每一折命中率≥50%（tie 剔除）。",
			"hcf05_layered_model":           "E07 4 層投票：Direction=bullish→+1、bearish→−1、其他→0；缺層（Available=false）直接跳過不補中性；vote_sum=Σv_i；方向=sign(vote_sum)（0=abstain）。",
			"hcf05_layered_probability_map": "p(次日上漲)=sigmoid(vote_sum)；abstain 日 p=0.5。",
			"hcf05_ew_model":                "平權=sign(mean(z_1..z_7))，|meanZ|<0.1 為 abstain；無信心輸出，Brier 以三態 p∈{0,0.5,1} 誠實計分。",
			"hcf05_hit_rate_denominator":    "命中率分母只算非 abstain 日；abstain 日單獨記 abstain_days。",
			"hcf05_brier_scope":             "Brier 對所有評估日計分（abstain 日以 p=0.5 誠實計分）。",
			"hcf05_position":                "position=sign(vote_sum)（平權=sign(meanZ)）；abstain 日無倉位；maxDD 為累積 position×direction 路徑之最大回撤（點×100=pp）。",
			"hcf05_walk_forward":            "expanding window 自 126 評估日起；評估日 <252 → INSUFFICIENT_DATA。",
			"hcf05_verdict":                 "三指標不劣化：hit_L≥hit_EW−1pp 且 Brier_L≤Brier_EW+0.02 且 maxDD_L≤maxDD_EW+0.5pp；任一嚴格更優→PASS(improved)；任一超容忍→FAIL。",
		},
	}
}

// WriteValidationReportJSON writes the report as pretty-printed JSON,
// creating parent directories as needed.
func WriteValidationReportJSON(path string, report ValidationReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("validation report: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("validation report: mkdir: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteValidationReportMarkdown writes the human-readable review copy
// of the report. It is the document a human gate reads before
// drafting the eligibility config PR.
func WriteValidationReportMarkdown(path string, report ValidationReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("validation report: mkdir: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Capital Flow 假設驗證報告（Phase 1）\n\n")
	fmt.Fprintf(&b, "- schema version: `%s`\n", report.Version)
	fmt.Fprintf(&b, "- run at: `%s`\n", report.RunAt.Format(time.RFC3339))
	if report.Workdir != "" {
		fmt.Fprintf(&b, "- workdir: `%s`\n", report.Workdir)
	}
	if len(report.DataCoverage) > 0 {
		keys := make([]string, 0, len(report.DataCoverage))
		for k := range report.DataCoverage {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, report.DataCoverage[k]))
		}
		fmt.Fprintf(&b, "- data coverage: %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "- **eligible_recommendation: %t**（僅供人工 PR review；CLI 永不寫 config）\n", report.EligibleRecommendation)

	fmt.Fprintf(&b, "\n## 結論對照表\n\n")
	fmt.Fprintf(&b, "| ID | Status | Sample Count | Verdict |\n|---|---|---|---|\n")
	for _, r := range report.Hypotheses {
		fmt.Fprintf(&b, "| %s | `%s` | %d | %s |\n", r.ID, r.Status, r.SampleCount, strings.ReplaceAll(r.Verdict, "|", "\\|"))
	}

	for _, r := range report.Hypotheses {
		fmt.Fprintf(&b, "\n## %s — %s\n\n", r.ID, r.Status)
		if r.Verdict != "" {
			fmt.Fprintf(&b, "%s\n\n", r.Verdict)
		}
		if len(r.Metrics) > 0 {
			fmt.Fprintf(&b, "### Metrics\n\n| 指標 | 值 |\n|---|---|\n")
			keys := make([]string, 0, len(r.Metrics))
			for k := range r.Metrics {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "| `%s` | %.6g |\n", k, r.Metrics[k])
			}
			fmt.Fprintln(&b)
		}
		if len(r.Thresholds) > 0 {
			fmt.Fprintf(&b, "### Pre-registered Thresholds\n\n| 門檻 | 值 |\n|---|---|\n")
			keys := make([]string, 0, len(r.Thresholds))
			for k := range r.Thresholds {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "| `%s` | %.6g |\n", k, r.Thresholds[k])
			}
			fmt.Fprintln(&b)
		}
		if len(r.Notes) > 0 {
			fmt.Fprintf(&b, "### Notes\n\n")
			for _, n := range r.Notes {
				fmt.Fprintf(&b, "- %s\n", n)
			}
			fmt.Fprintln(&b)
		}
	}

	if len(report.Method) > 0 {
		fmt.Fprintf(&b, "## 方法（預註冊，事後不可改）\n\n")
		keys := make([]string, 0, len(report.Method))
		for k := range report.Method {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "- **%s**: %s\n", k, report.Method[k])
		}
		fmt.Fprintln(&b)
	}
	if len(report.OperatorNotes) > 0 {
		fmt.Fprintf(&b, "## Operator Notes\n\n")
		for _, n := range report.OperatorNotes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
		fmt.Fprintln(&b)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
