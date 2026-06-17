package reporting

import (
	"strings"
	"testing"
)

func TestAgentPerformanceRow_HasAfterTaxPnLField(t *testing.T) {
	row := AgentPerformanceRow{
		AgentID:      "test_agent",
		HitRate:      0.65,
		SharpeLike:   1.5,
		MaxDrawdown:  0.15,
		Weight:       1.0,
		AfterTaxPnL:  9500.0,
	}
	if row.AfterTaxPnL != 9500.0 {
		t.Errorf("AfterTaxPnL = %f, want 9500.0", row.AfterTaxPnL)
	}
}

func TestRenderAgentPerformanceTable_IncludesAfterTaxPnLColumn(t *testing.T) {
	rows := []AgentPerformanceRow{
		{
			AgentID:     "test_agent",
			Layer:       "L1",
			WindowCount: 10,
			HitRate:     0.65,
			SharpeLike:  1.5,
			MaxDrawdown: 0.15,
			Weight:      1.0,
			AfterTaxPnL: 9500.0,
		},
	}
	table := RenderAgentPerformanceTable(rows)
	if !strings.Contains(table, "After-Tax P&L") {
		t.Errorf("table should include After-Tax P&L column header, got:\n%s", table)
	}
	if !strings.Contains(table, "9500") {
		t.Errorf("table should include AfterTaxPnL value 9500, got:\n%s", table)
	}
}
