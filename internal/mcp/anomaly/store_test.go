package anomaly

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_Store_Add_and_Recent_returns_newest_first verifies the ring buffer
// returns the most recent events in descending order.
func Test_Store_Add_and_Recent_returns_newest_first(t *testing.T) {
	s := NewStore(10)
	s.Add(AnomalyEvent{TenantID: "t1", AnomalyType: "burst", Score: 1.0, TS: time.Now().Format(time.RFC3339)})
	s.Add(AnomalyEvent{TenantID: "t2", AnomalyType: "tool", Score: 2.0, TS: time.Now().Format(time.RFC3339)})

	recent := s.Recent(2)
	require.Len(t, recent, 2)
	require.Equal(t, "t2", recent[0].TenantID)
	require.Equal(t, "t1", recent[1].TenantID)
}

// Test_Store_Recent_empty_returns_empty_slice verifies that an empty store
// returns an empty slice, not an error or nil.
func Test_Store_Recent_empty_returns_empty_slice(t *testing.T) {
	s := NewStore(10)
	recent := s.Recent(5)
	require.Empty(t, recent)
	require.NotNil(t, recent)
}

// Test_Store_capacity_evicts_oldest verifies that the ring buffer drops oldest
// entries when capacity is exceeded.
func Test_Store_capacity_evicts_oldest(t *testing.T) {
	s := NewStore(2)
	s.Add(AnomalyEvent{TenantID: "old"})
	s.Add(AnomalyEvent{TenantID: "mid"})
	s.Add(AnomalyEvent{TenantID: "new"})

	recent := s.Recent(3)
	require.Len(t, recent, 2)
	require.Equal(t, "new", recent[0].TenantID)
	require.Equal(t, "mid", recent[1].TenantID)
}
