package baseline

import (
	"encoding/json"
	"os"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Manager struct {
	path string
}

func NewManager(path string) *Manager {
	return &Manager{path: path}
}

func (m *Manager) PromoteResult(resultPath string) (Policy, error) {
	policy, err := Load(m.path)
	if err != nil {
		return Policy{}, err
	}
	result, err := loadExperimentResult(resultPath)
	if err != nil {
		return Policy{}, err
	}
	candidate, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		return Policy{}, err
	}
	next, err := Promote(policy, result, string(candidate))
	if err != nil {
		return Policy{}, err
	}
	if err := SaveWithLock(m.path, next); err != nil {
		return Policy{}, err
	}
	return next, nil
}

func loadExperimentResult(path string) (domain.PromptExperimentResult, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}
	var result domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	return result, nil
}
