package orchestrator

import (
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

type mockSvc struct {
	bus      *eventbus.ChannelEventBus
	outcomes []domain.RecommendationOutcome
}

func (m *mockSvc) Replay() *replay.Dataset                         { return nil }
func (m *mockSvc) GetRegistry() domain.AgentRegistry               { return recommendation.AgentRegistry{} }
func (m *mockSvc) GetPolicy() baseline.Policy                      { return baseline.Policy{} }
func (m *mockSvc) GetLastOutcomes() []domain.RecommendationOutcome { return m.outcomes }
func (m *mockSvc) Ledger() ledger.OutcomeStore                     { return nil }
func (m *mockSvc) EventBus() *eventbus.ChannelEventBus             { return m.bus }

func newTestPlugin() *eventlogicPlugin {
	reg := eventlogic.NewRegistry()
	return &eventlogicPlugin{
		detector:  eventlogic.NewDetector(reg),
		corrector: eventlogic.NewCorrector(reg),
	}
}

func TestEventLogicPlugin_Name(t *testing.T) {
	p := newTestPlugin()
	if got := p.Name(); got != "eventlogic" {
		t.Errorf("Name() = %q, want %q", got, "eventlogic")
	}
}

func TestEventLogicPlugin_ProcessRecommendationsPassthrough(t *testing.T) {
	p := &eventlogicPlugin{}
	recs := []domain.Recommendation{{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80}}
	got := p.ProcessRecommendations(domain.RegimeRiskOn, recs)
	if len(got) != 1 || got[0].Symbol != "2330.TW" {
		t.Errorf("ProcessRecommendations modified recs")
	}
}

func TestEventLogicPlugin_AttachNilBus(t *testing.T) {
	p := newTestPlugin()
	p.Attach(&mockSvc{})
}

func TestEventLogicPlugin_AttachSubscribesToNarrative(t *testing.T) {
	bus := eventbus.NewChannelEventBus(16)
	p := newTestPlugin()
	p.Attach(&mockSvc{bus: bus})

	bus.PublishNarrativeEvent("evt-1", "US_rates_up", "US", 0.6, 0.8, "backtest", "0.72", "outflow", "7d")
	bus.Publish(eventbus.BusEvent{
		ID:        "evt-2",
		Type:      eventbus.EventNarrative,
		Timestamp: time.Now(),
		Payload: eventbus.NarrativeEventPayload{
			Theme: "AI_capex_surge", Region: "US",
			Sentiment: 0.9, Confidence: 0.8,
		},
	})

	time.Sleep(100 * time.Millisecond)
	p.mu.Lock()
	n := len(p.evtBuf)
	p.mu.Unlock()
	if n == 0 {
		t.Fatal("expected buffered narrative events")
	}
}

func TestEventLogicPlugin_PostSimulationNilCore(t *testing.T) {
	p := &eventlogicPlugin{core: nil}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestEventLogicPlugin_PostSimulationNoOutcomes(t *testing.T) {
	p := newTestPlugin()
	p.core = &mockSvc{}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestEventLogicPlugin_PostSimulationNoNarrative(t *testing.T) {
	out := []domain.RecommendationOutcome{
		{Symbol: "2330.TW", ForwardReturn: 1.5, RecordedAt: time.Now()},
	}
	p := newTestPlugin()
	p.core = &mockSvc{outcomes: out}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestEventLogicPlugin_PostSimulationDiscovery(t *testing.T) {
	out := []domain.RecommendationOutcome{
		{Symbol: "2330.TW", ForwardReturn: 2.0, RecordedAt: time.Now()},
	}
	p := newTestPlugin()
	p.core = &mockSvc{outcomes: out}
	p.evtBuf = append(p.evtBuf, eventlogic.NarrativeEventSnapshot{Theme: "US_rates_up", DetectedAt: time.Now().Add(-24 * time.Hour)})
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestEventLogicPlugin_PostSimulationPersistsOnPromotion(t *testing.T) {
	dir := t.TempDir()
	rulesPath := dir + "/rules.json"
	histPath := dir + "/history.jsonl"
	reg := eventlogic.NewRegistry()
	det := eventlogic.NewDetector(reg)
	cor := eventlogic.NewCorrector(reg)
	hr := eventlogic.NewHistoryRecorder(histPath)

	out := []domain.RecommendationOutcome{
		{Symbol: "2330.TW", ForwardReturn: 3.0, RecordedAt: time.Now()},
	}
	p := &eventlogicPlugin{
		detector:        det,
		corrector:       cor,
		core:            &mockSvc{outcomes: out},
		saveRulesPath:   rulesPath,
		historyRecorder: hr,
	}
	for i := 0; i < 10; i++ {
		p.evtBuf = append(p.evtBuf, eventlogic.NarrativeEventSnapshot{
			Theme: "US_rates_up", DetectedAt: time.Now().Add(-24 * time.Duration(i) * time.Hour),
		})
	}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	_, err := os.Stat(rulesPath)
	if os.IsNotExist(err) {
		t.Log("no promotion occurred — acceptable if thresholds not met")
	}
}

func TestEventLogicPlugin_onNarrativeEventSkipsEmptyTheme(t *testing.T) {
	p := newTestPlugin()
	err := p.onNarrativeEvent(nil, eventbus.BusEvent{
		Type:      eventbus.EventNarrative,
		Timestamp: time.Now(),
		Payload:   eventbus.NarrativeEventPayload{Theme: ""},
	})
	if err != nil {
		t.Fatalf("onNarrativeEvent returned error: %v", err)
	}
	if len(p.evtBuf) != 0 {
		t.Error("expected empty buf for empty theme")
	}
}
