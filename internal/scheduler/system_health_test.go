package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestSystemHealthMonitor_AllClear(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_allclear.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "a2", Enabled: true, Layer: domain.LayerSector, Skill: "finance"},
		},
	})

	// Seed positive returns so RollingSharpe > 0 (>=8 unique values each).
	for i := range 60 {
		dw.RecordOutcome("a1", 0.02+float64(i%10)*0.001, true)
		dw.RecordOutcome("a2", 0.015+float64(i%10)*0.001, true)
	}

	hm := portfolio.NewAgentHealthManager()
	monitor := NewSystemHealthMonitor(dw, hm)

	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts for healthy system, got %d: %+v", len(alerts), alerts)
	}
}

func TestSystemHealthMonitor_SharpeTrendDeclining(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_sharpe.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	hm := portfolio.NewAgentHealthManager()
	monitor := NewSystemHealthMonitor(dw, hm)

	// Build 10 days of high Sharpe history.
	// Use varied returns so stddev > 0 and Sharpe is non-zero.
	for day := range 10 {
		n := 6
		if day == 0 {
			n = 60
		}
		for i := 0; i < n; i++ {
			ret := 0.04 + float64(i%10)*0.005 // 0.04 ~ 0.085, 10 unique
			dw.RecordOutcome("a1", ret, true)
		}
		_, _ = monitor.RunDaily(context.Background())
	}

	// Now degrade: negative returns with variance drop Sharpe significantly.
	dw.Reset()
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for i := range 60 {
		// 10 unique values so the degenerate-window guard does not zero Sharpe.
		dw.RecordOutcome("a1", -0.03-float64(i%10)*0.001, false)
	}

	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Category == "sharpe_trend" && a.Severity == "WARNING" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sharpe_trend WARNING, got alerts: %+v", alerts)
	}
}

func TestSystemHealthMonitor_NegativeSharpeCritical(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_neg.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Inject negative returns so RollingSharpe < -0.5 (10 unique values).
	for i := range 60 {
		dw.RecordOutcome("a1", -0.03-float64(i%10)*0.001, false)
	}

	hm := portfolio.NewAgentHealthManager()
	monitor := NewSystemHealthMonitor(dw, hm)

	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Category == "sharpe" && a.Severity == "CRITICAL" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected negative-sharpe CRITICAL alert, got: %+v", alerts)
	}
}

func TestSystemHealthMonitor_AgentPopulationMuted30(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_muted30.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "a2", Enabled: true, Layer: domain.LayerSector, Skill: "finance"},
			{ID: "a3", Enabled: true, Layer: domain.LayerSector, Skill: "energy"},
		},
	})

	// Mute a1 via 5 consecutive losses (default mute threshold).
	hm := portfolio.NewAgentHealthManager()
	for range 5 {
		hm.RecordOutcome("a1", false, -1.0, 0.0)
	}

	monitor := NewSystemHealthMonitor(dw, hm)
	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Category == "agent_population" && a.Severity == "WARNING" {
			found = true
			if a.Value < 0.3 || a.Value > 0.4 {
				t.Errorf("muted pct expected ~0.33, got %f", a.Value)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected agent_population WARNING, got: %+v", alerts)
	}
}

func TestSystemHealthMonitor_AgentPopulationMuted50(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_muted50.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "a2", Enabled: true, Layer: domain.LayerSector, Skill: "finance"},
		},
	})

	// Mute both agents.
	hm := portfolio.NewAgentHealthManager()
	for range 5 {
		hm.RecordOutcome("a1", false, -1.0, 0.0)
		hm.RecordOutcome("a2", false, -1.0, 0.0)
	}

	monitor := NewSystemHealthMonitor(dw, hm)
	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Category == "agent_population" && a.Severity == "CRITICAL" {
			found = true
			if a.Value != 1.0 {
				t.Errorf("muted pct expected 1.0, got %f", a.Value)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected agent_population CRITICAL, got: %+v", alerts)
	}
}

func TestSystemHealthMonitor_WeightDistributionStuck(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_stuck.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
			{ID: "a2", Enabled: true, Layer: domain.LayerSector, Skill: "finance"},
		},
	})

	// Set both agents to min weight (0.3).
	dw.SetWeight("a1", 0.3)
	dw.SetWeight("a2", 0.3)

	hm := portfolio.NewAgentHealthManager()
	monitor := NewSystemHealthMonitor(dw, hm)

	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	found := false
	for _, a := range alerts {
		if a.Category == "weight_distribution" && a.Severity == "WARNING" {
			found = true
			if a.Value != 1.0 {
				t.Errorf("stuck pct expected 1.0, got %f", a.Value)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected weight_distribution WARNING, got: %+v", alerts)
	}
}

func TestSystemHealthMonitor_BurnInStillRuns(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_burnin.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Set weight to min to trigger an alert even in burn-in.
	dw.SetWeight("a1", 0.3)

	hm := portfolio.NewAgentHealthManager()
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC()) // burn-in (0 days)
	monitor := NewSystemHealthMonitor(dw, hm).WithMaturityTracker(tr)

	alerts, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	// Burn-in should NOT skip health checks.
	found := false
	for _, a := range alerts {
		if a.Category == "weight_distribution" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected health checks to run during burn-in, got: %+v", alerts)
	}
}

func TestSystemHealthMonitor_EventBusPublish(t *testing.T) {
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_health_eventbus.json")
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Set agent to min weight to trigger a weight_distribution alert.
	dw.SetWeight("a1", 0.3)

	hm := portfolio.NewAgentHealthManager()
	eb := eventbus.NewChannelEventBus(16)
	monitor := NewSystemHealthMonitor(dw, hm).WithEventBus(eb)

	// Subscribe to health alert events.
	var mu sync.Mutex
	var received []eventbus.HealthAlertPayload
	eb.Subscribe(eventbus.EventHealthAlert, func(_ context.Context, ev eventbus.BusEvent) error {
		if payload, ok := ev.Payload.(eventbus.HealthAlertPayload); ok {
			mu.Lock()
			received = append(received, payload)
			mu.Unlock()
		}
		return nil
	})

	_, err := monitor.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	// Allow event bus to process.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	n := len(received)
	cat := ""
	sev := ""
	if n > 0 {
		cat = received[0].Category
		sev = received[0].Severity
	}
	mu.Unlock()

	if n != 1 {
		t.Fatalf("expected 1 health alert event, got %d", n)
	}
	if cat != "weight_distribution" {
		t.Errorf("expected category=weight_distribution, got %s", cat)
	}
	if sev != "WARNING" {
		t.Errorf("expected severity=WARNING, got %s", sev)
	}
}
