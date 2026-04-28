package narrative

import (
	"sync"
	"time"
)

type EventLifecycleManager struct {
	mu        sync.RWMutex
	events    map[string]*NarrativeEvent
	durations map[string]time.Duration
}

func NewEventLifecycleManager() *EventLifecycleManager {
	return &EventLifecycleManager{
		events:    make(map[string]*NarrativeEvent),
		durations: defaultDurations(),
	}
}

func defaultDurations() map[string]time.Duration {
	return map[string]time.Duration{
		"AI_capex_surge":          90 * 24 * time.Hour,
		"US_rates_up":             7 * 24 * time.Hour,
		"JPY_carry_unwind":        14 * 24 * time.Hour,
		"geopolitical_risk_spike": 30 * 24 * time.Hour,
		"oil_price_shock":         15 * 24 * time.Hour,
		"Fed_emergency_cut":       3 * 24 * time.Hour,
		"earnings_surprise":       10 * 24 * time.Hour,
	}
}

func (m *EventLifecycleManager) AddEvent(event *NarrativeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if event.Duration == 0 {
		if d, ok := m.durations[event.Theme]; ok {
			event.Duration = d
		} else {
			event.Duration = 7 * 24 * time.Hour
		}
	}
	if event.ExpiresAt.IsZero() {
		event.ExpiresAt = event.Timestamp.Add(event.Duration)
	}
	if event.Status == "" {
		event.Status = "active"
	}
	m.events[event.ID] = event
}

func (m *EventLifecycleManager) GetActiveEvents() []*NarrativeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*NarrativeEvent
	for _, e := range m.events {
		if e.Status == "active" || e.Status == "confirmed" {
			result = append(result, e)
		}
	}
	return result
}

func (m *EventLifecycleManager) UpdateStatuses() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, e := range m.events {
		if e.Status == "expired" {
			continue
		}
		if now.After(e.ExpiresAt) {
			e.Status = "expired"
			continue
		}
		fadeThreshold := e.Timestamp.Add(time.Duration(float64(e.Duration) * 0.8))
		if now.After(fadeThreshold) && e.Status == "active" {
			e.Status = "faded"
		}
	}
}

func (m *EventLifecycleManager) ConfirmEvent(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.events[id]; ok {
		e.Status = "confirmed"
	}
}

func (m *EventLifecycleManager) ExpireEvent(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.events[id]; ok {
		e.Status = "expired"
	}
}

func (m *EventLifecycleManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = make(map[string]*NarrativeEvent)
}
