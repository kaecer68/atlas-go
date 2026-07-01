package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kaecer68/atlas-go/internal/mcp/anomaly"
)

// Test_AnomalyStore_exposed_on_server verifies the Phase 4 T1.4 wiring:
// the server struct exposes its AnomalyStore via AnomalyStore() so the
// mcp_anomaly_ack tool (T1.5) can call Ack(...) on it.
//
// We construct the server field directly (not via Run) because Run
// launches the stdio MCP transport which reads from stdin — not
// usable from a unit test. The wiring of the AnomalyStore is the same
// in both paths.
func Test_AnomalyStore_exposed_on_server(t *testing.T) {
	srv := &server{anomalyStore: nil}
	require.Nil(t, srv.AnomalyStore(), "uninitialised server should return nil")

	store := anomaly.NewMemoryStore(100)
	srv.anomalyStore = store
	require.NotNil(t, srv.AnomalyStore(), "wired server should return non-nil store")
	require.Same(t, store, srv.AnomalyStore(), "wiring must return the same instance")
}

// Test_AnomalyStore_Save_then_Ack_end_to_end exercises the ack path that
// mcp_anomaly_ack (T1.5) will trigger: Save → Get → Ack → Get shows
// Acked=true. The same MemoryStore is the production default.
func Test_AnomalyStore_Save_then_Ack_end_to_end(t *testing.T) {
	store := anomaly.NewMemoryStore(100)
	ev := anomaly.AnomalyEvent{
		TenantID:    "tenant-a",
		AnomalyType: "burst",
		Score:       4.2,
		TS:          time.Now().UTC().Format(time.RFC3339),
	}
	sa, err := store.Save(ev)
	require.NoError(t, err)
	require.False(t, sa.Acked)

	require.NoError(t, store.Ack(sa.AnomalyID, "operator-1"))
	got, ok := store.Get(sa.AnomalyID)
	require.True(t, ok)
	require.True(t, got.Acked, "Acked flag must be set after Ack()")
	require.Equal(t, "operator-1", got.AckedBy)
}
