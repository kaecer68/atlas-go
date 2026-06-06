package eventlogic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

type rulesFile struct {
	SavedAt time.Time    `json:"saved_at"`
	Rules   []*EventRule `json:"rules"`
}

func (r *RuleRegistry) SaveToFile(path string) error {
	r.mu.RLock()
	snapshot := make([]*EventRule, 0, len(r.rules))
	for _, rule := range r.rules {
		snapshot = append(snapshot, rule)
	}
	r.mu.RUnlock()
	data, err := json.MarshalIndent(rulesFile{SavedAt: time.Now(), Rules: snapshot}, "", "  ")
	if err != nil {
		return fmt.Errorf("eventlogic: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("eventlogic: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("eventlogic: write: %w", err)
	}
	return os.Rename(tmp, path)
}

func (r *RuleRegistry) MustSave(path string) {
	if err := r.SaveToFile(path); err != nil {
		logging.Error("eventlogic", "save_failed", "path", path, "err", err.Error())
	}
}

func LoadRules(path string) (*RuleRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRegistry(), nil
		}
		return nil, fmt.Errorf("eventlogic: read: %w", err)
	}
	var saved rulesFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("eventlogic: unmarshal: %w", err)
	}
	reg := &RuleRegistry{rules: make(map[string]*EventRule, len(saved.Rules))}
	for _, rule := range saved.Rules {
		reg.rules[rule.ID] = rule
	}
	if reg.Count() == 0 {
		reg.seedRules()
	}
	return reg, nil
}

func LoadOrDefault(path string) *RuleRegistry {
	reg, err := LoadRules(path)
	if err != nil {
		logging.Error("eventlogic", "load_failed", "path", path, "err", err.Error())
		return NewRegistry()
	}
	return reg
}

type RuleSnapshot struct {
	RecordedAt time.Time `json:"recorded_at"`
	RuleID     string    `json:"rule_id"`
	Pattern    string    `json:"pattern"`
	HitRate    float64   `json:"hit_rate"`
	TotalTests int       `json:"total_tests"`
	TotalHits  int       `json:"total_hits"`
	Status     string    `json:"status"`
	Direction  string    `json:"direction"`
	Sectors    []string  `json:"sectors"`
}

type HistoryRecorder struct{ path string }

func NewHistoryRecorder(path string) *HistoryRecorder {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return &HistoryRecorder{path: path}
}

func (h *HistoryRecorder) RecordSnapshot(s RuleSnapshot) {
	line, _ := json.Marshal(s)
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

func (h *HistoryRecorder) SnapshotAll(reg *RuleRegistry) {
	now := time.Now()
	for _, rule := range reg.List() {
		h.RecordSnapshot(RuleSnapshot{
			RecordedAt: now, RuleID: rule.ID, Pattern: rule.Pattern,
			HitRate: rule.HitRate, TotalTests: rule.TotalTests, TotalHits: rule.TotalHits,
			Status: rule.Status, Direction: rule.Direction, Sectors: rule.AffectedSectors,
		})
	}
}
