package ledger

import (
	"reflect"
	"strings"
	"testing"
)

// ── I6 (#1944 Batch 3): sector predictions have no landing type ──────────
//
// /api/events/prediction emits SectorDayPrediction rows, but the ledger
// landing type and store interface have no sector dimension. The rows are
// therefore recomputed on every request and can never be reconciled T+1.
// These two tests are the machine-readable form of that gap: they fail the
// moment a sector column is added, which is the signal to flip
// eventdriven.SectorPredictionPersisted to true, delete the corresponding
// entry in docs/reference/inert-registry.md, and remove these tests.

func TestEventFlowPredictionRecordHasNoSectorField(t *testing.T) {
	rt := reflect.TypeOf(EventFlowPredictionRecord{})
	for i := range rt.NumField() {
		f := rt.Field(i)
		jsonTag := strings.ToLower(f.Tag.Get("json"))
		if strings.Contains(strings.ToLower(f.Name), "sector") || strings.Contains(jsonTag, "sector") {
			t.Fatalf("EventFlowPredictionRecord.%s (json=%q) looks like a sector column; "+
				"the I6 gap is closed — update eventdriven.SectorPredictionPersisted and remove this test",
				f.Name, jsonTag)
		}
	}
}

func TestEventFlowPredictionStoreHasNoSectorAccessor(t *testing.T) {
	rt := reflect.TypeOf((*EventFlowPredictionStore)(nil)).Elem()
	for i := range rt.NumMethod() {
		name := rt.Method(i).Name
		if strings.Contains(strings.ToLower(name), "sector") {
			t.Fatalf("EventFlowPredictionStore.%s looks like a sector accessor; "+
				"update the I6 registry entry and this test", name)
		}
	}
}
