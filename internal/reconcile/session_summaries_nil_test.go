package reconcile

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestSummariesConflict_NilVsEmptySlice (#1783): the PG round-trip stores
// tax_snapshots as an empty array while the JSONL flat file omits the field
// (omitempty) - semantically identical, must not report a conflict.
func TestSummariesConflict_NilVsEmptySlice(t *testing.T) {
	base := domain.SessionSummary{
		SessionID:      "session-20260101-daily",
		RecordedAt:     time.Date(2026, 4, 13, 23, 17, 25, 514093000, time.UTC),
		OrderCount:     5,
		PortfolioValue: 2998577.80,
		OutcomeCount:   1747,
	}
	nilTax := base
	emptyTax := base
	emptyTax.TaxSnapshots = []domain.TaxSnapshot{}
	if summariesConflict(nilTax, emptyTax) {
		t.Error("nil vs empty TaxSnapshots must not be a conflict")
	}
	if summariesConflict(emptyTax, nilTax) {
		t.Error("empty vs nil TaxSnapshots must not be a conflict")
	}
	other := base
	other.PortfolioValue = 12345
	if !summariesConflict(base, other) {
		t.Error("real content drift must still be a conflict")
	}
	tzShifted := base
	tzShifted.RecordedAt = base.RecordedAt.In(time.FixedZone("CST", 8*3600))
	if summariesConflict(base, tzShifted) {
		t.Error("same instant in different zones must not be a conflict")
	}
}
