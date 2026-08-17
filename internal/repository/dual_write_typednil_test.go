package repository_test

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/repository"
)

// TestDualWriteRepository_TypedNilAsExportStatsSaver guards the typed-nil
// interface pitfall from PR #1581: when the PG pool is unavailable,
// InitRepository returns a typed nil *DualWriteRepository (bootstrap
// bootstrapper.go), and passing it as the marketdata.ExportStatsSaver
// interface yields a NON-nil interface wrapping a nil pointer. SaveExportStats
// must treat that as JSON-only mode (pgUsable guards r == nil) instead of
// panicking on the r.pg dereference — otherwise every export_statistics fetch
// in a DATABASE_URL-less deployment crashes.
func TestDualWriteRepository_TypedNilAsExportStatsSaver(t *testing.T) {
	var repo *repository.DualWriteRepository // typed nil: pool unavailable
	saver := marketdata.ExportStatsSaver(repo)
	// The conversion of a typed nil to an interface is provably non-nil
	// (SA4023) — that is exactly the hazard: callers believe the interface is
	// nil, but SaveExportStats dispatches onto the nil receiver and must
	// degrade to JSON-only mode instead of panicking.
	if err := saver.SaveExportStats(context.Background(), 114, 3, 52000, 47000, 5000); err != nil {
		t.Fatalf("SaveExportStats on a typed-nil receiver must not panic: %v", err)
	}
}
