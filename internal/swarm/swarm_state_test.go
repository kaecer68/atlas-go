package swarm

import (
	"context"
	"sync"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestNewSwarmState_NonNil(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	if s == nil {
		t.Fatal("NewSwarmState returned nil")
	}
}

func TestNewSwarmState_GetLatestStatus(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	if got := s.GetLatestStatus(); got != "deprecated" {
		t.Fatalf("GetLatestStatus = %q, want \"deprecated\"", got)
	}
}

func TestNewSwarmState_IsRunning(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	if s.IsRunning() {
		t.Fatal("IsRunning = true, want false (simulation engine disabled)")
	}
}

func TestNewSwarmState_GetLatestResult(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	res, ok := s.GetLatestResult()
	if ok {
		t.Fatal("GetLatestResult ok = true, want false")
	}
	if res.Consensus != nil {
		t.Fatal("GetLatestResult consensus = non-nil, want nil")
	}
}

func TestNewSwarmState_StartStop(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if s.IsRunning() {
		t.Fatal("IsRunning = true after Start, want false (simulation engine disabled)")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestNewSwarmState_UpdateScenario(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	// must not panic
	s.UpdateScenario()
}

func TestNewSwarmState_ConcurrentAccess(t *testing.T) {
	s := NewSwarmState(domain.SwarmConfig{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.GetLatestStatus()
			s.IsRunning()
			s.GetLatestResult()
		}()
	}
	wg.Wait()
}

func TestDeprecatedNewMiroFishSwarm_Exists(t *testing.T) {
	if DeprecatedNewMiroFishSwarm == nil {
		t.Fatal("DeprecatedNewMiroFishSwarm is nil, expected backward-compat alias")
	}
	s := DeprecatedNewMiroFishSwarm(domain.SwarmConfig{})
	if s == nil {
		t.Fatal("DeprecatedNewMiroFishSwarm returned nil")
	}
	if got := s.GetLatestStatus(); got != "deprecated" {
		t.Fatalf("DeprecatedNewMiroFishSwarm().GetLatestStatus() = %q, want \"deprecated\"", got)
	}
}
