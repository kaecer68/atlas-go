package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
)

// ---------- janusPlugin ----------

func TestJanusPlugin_Name_Cov(t *testing.T) {
	p := &janusPlugin{}
	if got := p.Name(); got != "janus" {
		t.Errorf("expected janus, got %q", got)
	}
}

func TestJanusPlugin_Attach_Cov(t *testing.T) {
	p := &janusPlugin{}
	mock := &mockServiceRegistry{}
	p.Attach(mock)
	if p.core == nil {
		t.Error("expected core attached")
	}
}

func TestJanusPlugin_ProcessRecommendations_Cov(t *testing.T) {
	// nil engine: pass-through
	p := &janusPlugin{}
	recs := []domain.Recommendation{{Symbol: "2330.TW"}}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, recs); len(got) != 1 {
		t.Errorf("nil engine should pass through, got %d", len(got))
	}

	// nil recs with engine: return nil
	engine := janus.NewEngineWithConfig(janus.JANUSConfig{})
	p = &janusPlugin{engine: engine}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, nil); got != nil {
		t.Error("nil recs should return nil")
	}
}

func TestJanusPlugin_PostSimulation_Cov(t *testing.T) {
	// nil engine: no-op
	p := &janusPlugin{}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())

	// engine + core path
	mock := &mockServiceRegistry{}
	engine := janus.NewEngineWithConfig(janus.JANUSConfig{})
	p = &janusPlugin{engine: engine, core: mock}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

// ---------- swarmPlugin ----------

func TestSwarmPlugin_Name_Cov(t *testing.T) {
	p := &swarmPlugin{}
	if got := p.Name(); got != "swarm" {
		t.Errorf("expected swarm, got %q", got)
	}
}

func TestSwarmPlugin_Attach_Cov(t *testing.T) {
	p := &swarmPlugin{}
	mock := &mockServiceRegistry{}
	p.Attach(mock) // no-op for swarm
}

func TestSwarmPlugin_SetController_Cov(t *testing.T) {
	p := &swarmPlugin{}
	ctrl := &Phase3Controller{}
	p.SetController(ctrl)
	if p.controller != ctrl {
		t.Error("controller not set")
	}
}

func TestSwarmPlugin_ProcessRecommendations_Cov(t *testing.T) {
	// nil recs
	p := &swarmPlugin{}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, nil); got != nil {
		t.Error("nil recs should return nil")
	}

	// no controller, no swarm: pass-through
	recs := []domain.Recommendation{{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50}}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, recs); len(got) != 1 {
		t.Errorf("expected pass-through, got %d", len(got))
	}
}

func TestSwarmPlugin_PostSimulation_Cov(t *testing.T) {
	p := &swarmPlugin{}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now()) // no-op
}

// ---------- prismPlugin ----------

func TestPrismPlugin_Name_Cov(t *testing.T) {
	p := &prismPlugin{}
	if got := p.Name(); got != "prism" {
		t.Errorf("expected prism, got %q", got)
	}
}

func TestPrismPlugin_SetController_Cov(t *testing.T) {
	p := &prismPlugin{}
	ctrl := &Phase3Controller{}
	p.SetController(ctrl)
	if p.controller != ctrl {
		t.Error("controller not set")
	}
}

func TestPrismPlugin_Attach_Cov(t *testing.T) {
	// nil manager: no-op
	p := &prismPlugin{}
	mock := &mockServiceRegistry{}
	p.Attach(mock)

	// with manager
	pm := prism.NewPRISMManager(prism.PRISMConfig{})
	p = &prismPlugin{manager: pm}
	p.Attach(mock)
}

func TestPrismPlugin_ProcessRecommendations_Cov(t *testing.T) {
	// nil controller: pass-through
	p := &prismPlugin{}
	recs := []domain.Recommendation{{Symbol: "2330.TW"}}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, recs); len(got) != 1 {
		t.Errorf("nil controller should pass through, got %d", len(got))
	}

	// with controller
	ctrl := &Phase3Controller{}
	p = &prismPlugin{controller: ctrl}
	got := p.ProcessRecommendations(domain.RegimeNeutral, recs)
	if len(got) != 1 {
		t.Errorf("expected one rec, got %d", len(got))
	}
}

func TestPrismPlugin_PostSimulation_Cov(t *testing.T) {
	// no manager, no controller: no-op
	p := &prismPlugin{}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())

	// with manager + core
	pm := prism.NewPRISMManager(prism.PRISMConfig{})
	mock := &mockServiceRegistry{}
	p = &prismPlugin{manager: pm, core: mock}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())
}

// ---------- spawningPlugin ----------

func TestSpawningPlugin_Name_Cov(t *testing.T) {
	p := &spawningPlugin{}
	if got := p.Name(); got != "spawning" {
		t.Errorf("expected spawning, got %q", got)
	}
}

func TestSpawningPlugin_Attach_Cov(t *testing.T) {
	p := &spawningPlugin{}
	mock := &mockServiceRegistry{}
	p.Attach(mock) // no-op for spawning
}

func TestSpawningPlugin_SetController_Cov(t *testing.T) {
	p := &spawningPlugin{}
	ctrl := &Phase3Controller{}
	p.SetController(ctrl)
	if p.controller != ctrl {
		t.Error("controller not set")
	}
}

func TestSpawningPlugin_ProcessRecommendations_Cov(t *testing.T) {
	p := &spawningPlugin{}
	recs := []domain.Recommendation{{Symbol: "2330.TW"}}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, recs); len(got) != 1 {
		t.Errorf("should pass through, got %d", len(got))
	}
}

func TestSpawningPlugin_PostSimulation_Cov(t *testing.T) {
	// nil manager: no-op
	p := &spawningPlugin{}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())

	// with manager
	sm := spawning.NewSpawningManager(&domain.AgentRegistry{}, spawning.DefaultSpawningConfig())
	p = &spawningPlugin{manager: sm}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())
}

// ---------- phase3Plugin ----------

func TestPhase3Plugin_Name_Cov(t *testing.T) {
	p := &phase3Plugin{}
	if got := p.Name(); got != "phase3" {
		t.Errorf("expected phase3, got %q", got)
	}
}

func TestPhase3Plugin_Attach_Cov(t *testing.T) {
	// nil controller
	p := &phase3Plugin{}
	mock := &mockServiceRegistry{}
	p.Attach(mock)

	// with controller
	p = &phase3Plugin{controller: &Phase3Controller{}}
	p.Attach(mock)
}

func TestPhase3Plugin_ProcessRecommendations_Cov(t *testing.T) {
	p := &phase3Plugin{}
	recs := []domain.Recommendation{{Symbol: "2330.TW"}}
	if got := p.ProcessRecommendations(domain.RegimeNeutral, recs); len(got) != 1 {
		t.Errorf("should pass through, got %d", len(got))
	}
}

func TestPhase3Plugin_PostSimulation_Cov(t *testing.T) {
	// nil controller: no-op
	p := &phase3Plugin{}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())

	// with controller
	quotes := []domain.Quote{{Symbol: "2330.TW", Last: 600, High: 610, Low: 590, Volume: 1_000_000}}
	p = &phase3Plugin{controller: &Phase3Controller{}}
	p.PostSimulation(quotes, domain.RegimeNeutral, time.Now())
}
