package reporting

import (
	"fmt"
	"strings"
	"time"
)

// BacktestReportData is the input container for report generation.
type BacktestReportData struct {
	WindowID              string
	StartDate             time.Time
	EndDate               time.Time
	SessionCount          int
	OutcomeCount          int
	EquityCurve           []float64
	AgentRows             []AgentPerformanceRow
	MutationStats         MutationStats
	WorstAgentID          string
	WorstAgentSkill       string
	WorstAgentLayer       string
	WorstAgentWindowCount int
	WorstSharpeLike       float64
	RegimeCounts          map[string]int
}

// RenderMarkdown assembles a full Markdown backtest report.
func RenderMarkdown(data BacktestReportData) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Backtest Report: %s\n\n", data.WindowID)
	fmt.Fprintf(&sb, "**Period:** %s to %s  \n", data.StartDate.Format("2006-01-02"), data.EndDate.Format("2006-01-02"))
	fmt.Fprintf(&sb, "**Sessions:** %d | **Outcomes:** %d\n\n", data.SessionCount, data.OutcomeCount)

	if len(data.EquityCurve) > 0 {
		startVal := data.EquityCurve[0]
		endVal := data.EquityCurve[len(data.EquityCurve)-1]
		ret := 0.0
		if startVal > 0 {
			ret = (endVal - startVal) / startVal
		}
		fmt.Fprintf(&sb, "**Portfolio Return:** %.2f%%  \n", ret*100)
		fmt.Fprintf(&sb, "**Starting Value:** %.0f | **Ending Value:** %.0f\n\n", startVal, endVal)
	}

	sb.WriteString("## Equity Curve\n\n")
	sb.WriteString("```\n")
	sb.WriteString(RenderASCIIChart(data.EquityCurve, 50, 10))
	sb.WriteString("```\n\n")

	sb.WriteString("## Agent Performance\n\n")
	sb.WriteString(RenderAgentPerformanceTable(data.AgentRows))
	sb.WriteString("\n")

	sb.WriteString("## Mutation Summary\n\n")
	sb.WriteString(RenderMutationSummary(data.MutationStats))
	sb.WriteString("\n")

	sb.WriteString("## Regime Distribution\n\n")
	if len(data.RegimeCounts) == 0 {
		sb.WriteString("_No regime data available._\n")
	} else {
		sb.WriteString("| Regime | Sessions |\n")
		sb.WriteString("|--------|----------|\n")
		for regime, count := range data.RegimeCounts {
			fmt.Fprintf(&sb, "| %s | %d |\n", regime, count)
		}
	}
	sb.WriteString("\n")

	sb.WriteString("## Experiment Candidate\n\n")
	if data.WorstAgentID != "" {
		fmt.Fprintf(&sb, "- **Weakest Agent:** `%s` (%s)\n", data.WorstAgentID, data.WorstAgentSkill)
		if data.WorstAgentLayer != "" {
			fmt.Fprintf(&sb, "- **Layer:** %s\n", data.WorstAgentLayer)
		}
		if data.WorstAgentWindowCount > 0 {
			fmt.Fprintf(&sb, "- **Window Count:** %d\n", data.WorstAgentWindowCount)
		}
		fmt.Fprintf(&sb, "- **Sharpe-like Score:** %.4f\n", data.WorstSharpeLike)
	} else {
		sb.WriteString("_No experiment candidate identified._\n")
	}
	sb.WriteString("\n")

	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "*Generated at %s*\n", time.Now().Format(time.RFC3339))

	return sb.String()
}
