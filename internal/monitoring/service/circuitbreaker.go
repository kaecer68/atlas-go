package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

type CircuitBreakerService struct {
	workDir string
	cb      *live.CircuitBreaker
}

func NewCircuitBreakerService(workDir string) *CircuitBreakerService {
	statePath := filepath.Join(workDir, livestore.DefaultCircuitBreakerStatePath)
	logPath := filepath.Join(workDir, "data/state/circuit_breaker_log.jsonl")
	cb := live.NewCircuitBreaker(logPath, statePath)
	return &CircuitBreakerService{
		workDir: workDir,
		cb:      cb,
	}
}

type CircuitBreakerStateResponse struct {
	State          string                     `json:"state"`
	StateChangedAt string                     `json:"state_changed_at"`
	ConsecutiveSL  int                        `json:"consecutive_sl"`
	CooldownUntil  string                     `json:"cooldown_until"`
	IntradayPeak   float64                    `json:"intraday_peak"`
	DayStartValue  float64                    `json:"day_start_value"`
	Events         []live.CircuitBreakerEvent `json:"events"`
	Initialized    bool                       `json:"initialized"`
}

func (s *CircuitBreakerService) GetCircuitBreakerState() (*CircuitBreakerStateResponse, error) {
	events, err := s.cb.LoadEvents()
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []live.CircuitBreakerEvent{}
	}

	stateData, err := os.ReadFile(filepath.Join(s.workDir, livestore.DefaultCircuitBreakerStatePath))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	initialized := err == nil

	var cbState struct {
		State          string    `json:"state"`
		StateChangedAt time.Time `json:"state_changed_at"`
		ConsecutiveSL  int       `json:"consecutive_sl"`
		CooldownUntil  time.Time `json:"cooldown_until"`
		IntradayPeak   float64   `json:"intraday_peak"`
		DayStartValue  float64   `json:"day_start_value"`
	}
	if initialized {
		if err := json.Unmarshal(stateData, &cbState); err != nil && len(stateData) > 0 {
			initialized = false
		}
	}

	resp := &CircuitBreakerStateResponse{
		Initialized:   initialized,
		State:         string(s.cb.State()),
		ConsecutiveSL: cbState.ConsecutiveSL,
		IntradayPeak:  cbState.IntradayPeak,
		DayStartValue: cbState.DayStartValue,
		Events:        events,
	}
	if !initialized {
		resp.State = "uninitialized"
		resp.StateChangedAt = ""
		resp.CooldownUntil = ""
	}
	if !cbState.StateChangedAt.IsZero() {
		resp.StateChangedAt = cbState.StateChangedAt.Format(time.RFC3339)
	}
	if !cbState.CooldownUntil.IsZero() {
		resp.CooldownUntil = cbState.CooldownUntil.Format(time.RFC3339)
	}
	return resp, nil
}

func (s *CircuitBreakerService) ResetCircuitBreaker(reason string) error {
	return s.cb.Reset(reason)
}

func (s *CircuitBreakerService) GetCircuitBreakerEvents() ([]live.CircuitBreakerEvent, error) {
	return s.cb.LoadEvents()
}
