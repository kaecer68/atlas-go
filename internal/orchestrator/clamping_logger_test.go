package orchestrator

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestClampingLogger_Append(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/clamping.jsonl"
	l := newClampingLogger(path)

	l.Append(eventbus.ClampingEventPayload{
		AgentID: "a1", RawWeight: 3.0, FinalWeight: 2.5, Boundary: "max", Timestamp: time.Now(),
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), `"agent_id":"a1"`) {
		t.Errorf("expected agent_id in output, got: %s", string(data))
	}
}

func TestClampingLogger_Append_Nil(t *testing.T) {
	var l *clampingLogger
	l.Append(eventbus.ClampingEventPayload{AgentID: "a1"}) // should not panic
}

func TestClampingLogger_AppendConvictionEvents(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/conviction.jsonl"
	l := newClampingLogger(path)

	events := []portfolio.ConvictionClampingEvent{
		{AgentID: "a1", Symbol: "2330.TW", RawConviction: 140, FinalConviction: 100, Weight: 1.5, Boundary: "max"},
		{AgentID: "a2", Symbol: "2317.TW", RawConviction: 50, FinalConviction: 60, Weight: 0.8, Boundary: "min"},
	}
	l.AppendConvictionEvents(events)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(string(data), "2330.TW") {
		t.Error("expected 2330.TW in output")
	}
	if !strings.Contains(string(data), "2317.TW") {
		t.Error("expected 2317.TW in output")
	}
}

func TestClampingLogger_AppendConvictionEvents_Nil(t *testing.T) {
	var l *clampingLogger
	l.AppendConvictionEvents([]portfolio.ConvictionClampingEvent{{AgentID: "a1"}}) // should not panic
}

func TestClampingLogger_AppendConvictionEvents_Empty(t *testing.T) {
	dir := t.TempDir()
	l := newClampingLogger(dir + "/empty.jsonl")
	l.AppendConvictionEvents(nil) // should not panic, no file created
}
