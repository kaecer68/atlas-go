package experiment

import (
	"context"
	"testing"
)

func TestAutoExperimentNilSystemReturnsError(t *testing.T) {
	err := AutoExperiment(context.Background(), AutoExperimentConfig{})
	if err == nil {
		t.Fatal("expected error when System is nil, got nil")
	}
}

type testMonitor struct {
	lastLevel   string
	lastMessage string
}

func (m *testMonitor) Alert(level, category, message string, details map[string]any) {
	m.lastLevel = level
	m.lastMessage = message
}

func TestAutoExperimentMonitorInterface(t *testing.T) {
	m := &testMonitor{}
	cfg := AutoExperimentConfig{Monitor: m}
	_ = cfg
	var _ AutoExperimentMonitor = m // compile-time interface check
}
