package baseline

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Policy struct {
	Version         int
	PromptOverrides map[string]string
	Constraints     domain.SimulationConstraints
	ExecutionPolicy domain.ExecutionPolicy
	Promotions      []PromotionRecord
	RevertHistory   []RevertRecord
	LastUpdatedAt   time.Time
}

type PromotionRecord struct {
	ExperimentID  string
	TargetAgentID string
	TargetSkill   string
	MutationType  string
	CandidatePath string
	PromotedAt    time.Time
	Status        string
	VersionAfter  int
}

type RevertRecord struct {
	FromVersion         int
	ToVersion           int
	RevertedExperiments []string
	Reason              string
	RevertedAt          time.Time
}

func DefaultPolicy() Policy {
	constraints := domain.SimulationConstraints{
		StartingCash:                3000000,
		MaxPositionWeight:           0.18,
		MaxOpenPositions:            5,
		MinTradableVolume:           1000000,
		MinRecommendationConviction: 60,
		RequireCROPass:              true,
		TransactionCostBPS:          1.425,
		SlippageBPS:                 4,
		ReserveCashFraction:         0.1,
	}
	return Policy{
		Version:         1,
		PromptOverrides: map[string]string{},
		Constraints:     constraints,
		ExecutionPolicy: ExecutionPolicyFromConstraints(constraints),
		Promotions:      []PromotionRecord{},
	}
}

func Load(path string) (Policy, error) {
	if path == "" {
		return DefaultPolicy(), nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPolicy(), nil
		}
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return Policy{}, err
	}
	if policy.PromptOverrides == nil {
		policy.PromptOverrides = map[string]string{}
	}
	if policy.Version == 0 {
		policy.Version = 1
	}
	if policy.ExecutionPolicy == (domain.ExecutionPolicy{}) {
		policy.ExecutionPolicy = ExecutionPolicyFromConstraints(policy.Constraints)
	}
	return policy, nil
}

func Save(path string, policy Policy) error {
	if path == "" {
		return errors.New("baseline policy path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	policy.LastUpdatedAt = time.Now()
	bytes, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

func ExecutionPolicyFromConstraints(constraints domain.SimulationConstraints) domain.ExecutionPolicy {
	floor := constraints.MinRecommendationConviction
	if floor <= 0 {
		floor = 50
	}
	return domain.ExecutionPolicy{
		ConvictionFloor: floor,
		RequireCROPass:  constraints.RequireCROPass,
	}
}

func ResolvePromptOverride(policy Policy, agentID, skill string) string {
	if override, ok := policy.PromptOverrides[agentID]; ok && override != "" {
		return override
	}
	if override, ok := policy.PromptOverrides[skill]; ok && override != "" {
		return override
	}
	return ""
}

func ApplyConstraintCandidate(base domain.SimulationConstraints, candidate string) domain.SimulationConstraints {
	lines := strings.Split(candidate, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "conviction_floor:"):
			if v, ok := parseIntValue(line); ok {
				base.MinRecommendationConviction = v
			}
		case strings.HasPrefix(line, "liquidity_floor:"):
			if v, ok := parseInt64Value(line); ok {
				base.MinTradableVolume = v
			}
		case strings.HasPrefix(line, "max_position_weight:"):
			if v, ok := parseFloatValue(line); ok {
				base.MaxPositionWeight = v
			}
		case strings.HasPrefix(line, "reserve_cash_fraction:"):
			if v, ok := parseFloatValue(line); ok {
				base.ReserveCashFraction = v
			}
		case strings.HasPrefix(line, "require_cro_pass:"):
			if v, ok := parseBoolValue(line); ok {
				base.RequireCROPass = v
			}
		case strings.HasPrefix(line, "stop_loss_pct:"):
			if v, ok := parsePctFloatValue(line); ok {
				base.StopLossPct = v
			}
		case strings.HasPrefix(line, "take_profit_pct:"):
			if v, ok := parsePctFloatValue(line); ok {
				base.TakeProfitPct = v
			}
		case strings.HasPrefix(line, "max_open_positions:"):
			if v, ok := parseIntValue(line); ok {
				base.MaxOpenPositions = v
			}
		case strings.HasPrefix(line, "transaction_cost_bps:"):
			if v, ok := parseFloatValue(line); ok {
				base.TransactionCostBPS = v
			}
		case strings.HasPrefix(line, "slippage_bps:"):
			if v, ok := parseFloatValue(line); ok {
				base.SlippageBPS = v
			}
		}
	}
	return base
}

func Promote(policy Policy, result domain.PromptExperimentResult, candidate string) (Policy, error) {
	if result.Experiment.Status != domain.ExperimentAccepted {
		return policy, errors.New("only accepted experiments can be promoted")
	}
	next := policy
	if next.PromptOverrides == nil {
		next.PromptOverrides = map[string]string{}
	}

	switch result.Experiment.MutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		next.Constraints = ApplyConstraintCandidate(next.Constraints, candidate)
		next.ExecutionPolicy = ExecutionPolicyFromConstraints(next.Constraints)
	default:
		next.PromptOverrides[result.Experiment.TargetAgentID] = candidate
	}

	next.Version++
	next.Promotions = append(next.Promotions, PromotionRecord{
		ExperimentID:  result.Experiment.ID,
		TargetAgentID: result.Experiment.TargetAgentID,
		TargetSkill:   result.Experiment.Skill,
		MutationType:  result.Experiment.MutationType,
		CandidatePath: result.CandidatePrompt,
		PromotedAt:    time.Now(),
		Status:        string(result.Experiment.Status),
	})
	return next, nil
}

func parseIntValue(line string) (int, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseInt64Value(line string) (int64, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseFloatValue(line string) (float64, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parsePctFloatValue(line string) (float64, bool) {
	v, ok := parseFloatValue(line)
	if !ok {
		return 0, false
	}
	if v > 1 {
		v = v / 100
	}
	return v, true
}

func parseBoolValue(line string) (bool, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return false, false
	}
	v, err := strconv.ParseBool(strings.TrimSpace(parts[1]))
	if err != nil {
		return false, false
	}
	return v, true
}
