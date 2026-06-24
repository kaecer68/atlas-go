package service

import (
	"testing"
)

type mockChannelHealthStore struct {
	ids       []string
	latencies map[string]int64
}

func (m *mockChannelHealthStore) ChannelIDs() []string {
	return m.ids
}

func (m *mockChannelHealthStore) ChannelLatencyMs(channelID string) int64 {
	return m.latencies[channelID]
}

func TestChannelHealthIngestionLagProvider_P99LatencySeconds(t *testing.T) {
	store := &mockChannelHealthStore{
		ids: []string{"a", "b", "c", "d", "e"},
		latencies: map[string]int64{
			"a": 1000,
			"b": 2000,
			"c": 3000,
			"d": 4000,
			"e": 5000,
		},
	}

	provider := NewChannelHealthIngestionLagProvider(store)
	got := provider.P99LatencySeconds()
	want := 5.0
	if got != want {
		t.Errorf("p99 latency = %f, want %f", got, want)
	}
}

func TestChannelHealthIngestionLagProvider_P99LatencySeconds_IgnoresZeroOrMissing(t *testing.T) {
	store := &mockChannelHealthStore{
		ids: []string{"a", "b", "c"},
		latencies: map[string]int64{
			"a": 1000,
			"b": 0,
		},
	}

	provider := NewChannelHealthIngestionLagProvider(store)
	got := provider.P99LatencySeconds()
	want := 1.0
	if got != want {
		t.Errorf("p99 latency = %f, want %f", got, want)
	}
}

func TestChannelHealthIngestionLagProvider_NilStore(t *testing.T) {
	provider := NewChannelHealthIngestionLagProvider(nil)
	if got := provider.P99LatencySeconds(); got != 0 {
		t.Errorf("nil store should return 0, got %f", got)
	}
}

func TestChannelHealthIngestionLagProvider_P99LatencySeconds_NoChannels(t *testing.T) {
	store := &mockChannelHealthStore{ids: []string{}}
	provider := NewChannelHealthIngestionLagProvider(store)
	if got := provider.P99LatencySeconds(); got != 0 {
		t.Errorf("empty channels should return 0, got %f", got)
	}
}

func TestChannelHealthIngestionLagProvider_P99LatencySeconds_FewerThan100Channels(t *testing.T) {
	// With only 10 channels, p99 resolves to the last (highest) latency.
	store := &mockChannelHealthStore{
		ids: []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9", "c10"},
		latencies: map[string]int64{
			"c1":  100,
			"c2":  200,
			"c3":  300,
			"c4":  400,
			"c5":  500,
			"c6":  600,
			"c7":  700,
			"c8":  800,
			"c9":  900,
			"c10": 1000,
		},
	}

	provider := NewChannelHealthIngestionLagProvider(store)
	got := provider.P99LatencySeconds()
	want := 1.0
	if got != want {
		t.Errorf("p99 latency = %f, want %f", got, want)
	}
}
