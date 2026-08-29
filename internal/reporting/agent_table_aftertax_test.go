package reporting

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

//go:fix inline
func p(v float64) *float64 { return new(v) }

func TestAgentPerformanceRow_HasAfterTaxPnLField(t *testing.T) {
	row := AgentPerformanceRow{
		AgentID:     "test_agent",
		HitRate:     0.65,
		SharpeLike:  1.5,
		MaxDrawdown: 0.15,
		Weight:      1.0,
		AfterTaxPnL: p(9500),
	}
	if row.AfterTaxPnL == nil || *row.AfterTaxPnL != 9500 {
		t.Errorf("AfterTaxPnL should be 9500")
	}
}

func TestAgentPerformanceRow_NilAfterTaxPnL(t *testing.T) {
	row := AgentPerformanceRow{
		AgentID: "test_agent",
	}
	if row.AfterTaxPnL != nil {
		t.Errorf("AfterTaxPnL should be nil by default")
	}
}

func TestRenderAgentPerformanceTable_AfterTaxPnL_Nil(t *testing.T) {
	rows := []AgentPerformanceRow{
		{
			AgentID:     "test_agent",
			Layer:       "L1",
			WindowCount: 10,
			HitRate:     0.65,
			SharpeLike:  1.5,
			MaxDrawdown: 0.15,
			Weight:      1.0,
			AfterTaxPnL: nil,
		},
	}
	table := RenderAgentPerformanceTable(rows)
	if !strings.Contains(table, "N/A") {
		t.Errorf("nil AfterTaxPnL should render as N/A, got:\n%s", table)
	}
}

func TestRenderAgentPerformanceTable_AfterTaxPnL_Zero(t *testing.T) {
	rows := []AgentPerformanceRow{
		{
			AgentID:     "test_agent",
			Layer:       "L1",
			WindowCount: 10,
			HitRate:     0.65,
			SharpeLike:  1.5,
			MaxDrawdown: 0.15,
			Weight:      1.0,
			AfterTaxPnL: p(0),
		},
	}
	table := RenderAgentPerformanceTable(rows)
	if !strings.Contains(table, "N/A") {
		t.Errorf("zero AfterTaxPnL should render as N/A (same as nil), got:\n%s", table)
	}
}

func TestRenderAgentPerformanceTable_AfterTaxPnL_NonZero(t *testing.T) {
	rows := []AgentPerformanceRow{
		{
			AgentID:     "test_agent",
			Layer:       "L1",
			WindowCount: 10,
			HitRate:     0.65,
			SharpeLike:  1.5,
			MaxDrawdown: 0.15,
			Weight:      1.0,
			AfterTaxPnL: p(15000),
		},
	}
	table := RenderAgentPerformanceTable(rows)
	if !strings.Contains(table, "15000") {
		t.Errorf("non-zero AfterTaxPnL should render as formatted number, got:\n%s", table)
	}
}

func TestBuildAgentRows_PassesAfterTaxPnLDirectly(t *testing.T) {
	afterTax := 5000.0
	sc := []domain.Scorecard{
		{
			AgentID:     "test_agent",
			Skill:       "growth_momentum",
			Layer:       "L1",
			AfterTaxPnL: &afterTax,
		},
	}
	rows := BuildAgentRows(sc, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AfterTaxPnL == nil {
		t.Fatal("AfterTaxPnL should not be nil when Scorecard has it")
	}
	if *rows[0].AfterTaxPnL != 5000 {
		t.Errorf("AfterTaxPnL = %f, want 5000", *rows[0].AfterTaxPnL)
	}
}

func TestBuildAgentRows_AfterTaxPnL_NilWhenScorecardNil(t *testing.T) {
	sc := []domain.Scorecard{
		{
			AgentID:     "test_agent",
			Skill:       "growth_momentum",
			Layer:       "L1",
			AfterTaxPnL: nil,
		},
	}
	rows := BuildAgentRows(sc, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AfterTaxPnL != nil {
		t.Error("AfterTaxPnL should be nil when Scorecard.AfterTaxPnL is nil")
	}
}

func TestBuildAgentRows_AfterTaxPnL_PreservesZero(t *testing.T) {
	afterTax := 0.0
	sc := []domain.Scorecard{
		{
			AgentID:     "test_agent",
			Skill:       "growth_momentum",
			Layer:       "L1",
			AfterTaxPnL: &afterTax,
		},
	}
	rows := BuildAgentRows(sc, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].AfterTaxPnL == nil {
		t.Fatal("AfterTaxPnL should not be nil when Scorecard has explicit 0")
	}
	if *rows[0].AfterTaxPnL != 0 {
		t.Errorf("AfterTaxPnL = %f, want 0", *rows[0].AfterTaxPnL)
	}
}

func TestFormatAfterTaxPnL_Nil(t *testing.T) {
	if got := formatAfterTaxPnL(nil); got != "N/A" {
		t.Errorf("formatAfterTaxPnL(nil) = %q, want \"N/A\"", got)
	}
}

func TestFormatAfterTaxPnL_Zero(t *testing.T) {
	got := formatAfterTaxPnL(p(0))
	if got != "N/A" {
		t.Errorf("formatAfterTaxPnL(0) = %q, want \"N/A\"", got)
	}
}

func TestFormatAfterTaxPnL_Positive(t *testing.T) {
	got := formatAfterTaxPnL(p(12345))
	if got != "12345" {
		t.Errorf("formatAfterTaxPnL(12345) = %q, want \"12345\"", got)
	}
}

func TestFormatAfterTaxPnL_Negative(t *testing.T) {
	got := formatAfterTaxPnL(p(-500))
	if got != "-500" {
		t.Errorf("formatAfterTaxPnL(-500) = %q, want \"-500\"", got)
	}
}
