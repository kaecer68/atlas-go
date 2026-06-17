package reporting

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// AgentPerformanceRow is a flattened view for table rendering.
type AgentPerformanceRow struct {
	AgentID      string
	Skill        string
	Layer        string
	WindowCount  int
	Observations int
	HitRate      float64
	SharpeLike   float64
	MaxDrawdown  float64
	Weight       float64
	AfterTaxPnL  float64
}

// RenderAgentPerformanceTable generates a Markdown table from agent rows.
func RenderAgentPerformanceTable(rows []AgentPerformanceRow) string {
	if len(rows) == 0 {
		return "_No agent performance data available._\n"
	}

	var sb strings.Builder
	sb.WriteString("| Agent | Layer | Windows | Hit Rate | Sharpe | Max DD | After-Tax P&L | Weight |\n")
	sb.WriteString("|-------|-------|---------|----------|--------|--------|--------------|--------|\n")

	for _, r := range rows {
		fmt.Fprintf(
			&sb, "| %s | %s | %d | %.1f%% | %.3f | %.2f%% | %.0f | %.2f |\n",
			truncate(r.AgentID, 20),
			r.Layer,
			r.WindowCount,
			r.HitRate*100,
			r.SharpeLike,
			r.MaxDrawdown*100,
			r.AfterTaxPnL,
			r.Weight,
		)
	}
	return sb.String()
}

// BuildAgentRows converts scorecards and optional weight map into rows.
func BuildAgentRows(scorecards []domain.Scorecard, weights map[string]float64) []AgentPerformanceRow {
	rows := make([]AgentPerformanceRow, 0, len(scorecards))
	for _, sc := range scorecards {
		w := 1.0
		if weights != nil {
			if vw, ok := weights[sc.AgentID]; ok {
				w = vw
			}
		}
		rows = append(rows, AgentPerformanceRow{
			AgentID:      sc.AgentID,
			Skill:        sc.Skill,
			Layer:        string(sc.Layer),
			WindowCount:  sc.WindowCount,
			Observations: sc.Observations,
			HitRate:      sc.HitRate,
			SharpeLike:   sc.SharpeLike,
			MaxDrawdown:  sc.MaxDrawdown,
			Weight:       w,
		})
	}
	return rows
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
