package baseline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type Policy struct {
	Version         int                          `json:"version"`
	PromptOverrides map[string]string            `json:"prompt_overrides"`
	Constraints     domain.SimulationConstraints `json:"constraints"`
	ExecutionPolicy domain.ExecutionPolicy       `json:"execution_policy"`
	Promotions      []PromotionRecord            `json:"promotions"`
	RevertHistory   []RevertRecord               `json:"revert_history"`
	LastUpdatedAt   time.Time                    `json:"last_updated_at"`
}

// ErrBaselineNotLoaded is returned when the baseline policy file cannot be found
// and strict loading is required.
var ErrBaselineNotLoaded = errors.New("baseline policy not loaded")

func (p *Policy) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal raw policy: %w", err)
	}

	if _, ok := raw["version"]; ok {
		type alias Policy
		var current alias
		if err := json.Unmarshal(data, &current); err != nil {
			return fmt.Errorf("unmarshal current policy: %w", err)
		}
		*p = Policy(current)
		return nil
	}

	type legacyPromotionRecord struct {
		ExperimentID  string    `json:"ExperimentID"`
		TargetAgentID string    `json:"TargetAgentID"`
		TargetSkill   string    `json:"TargetSkill"`
		MutationType  string    `json:"MutationType"`
		CandidatePath string    `json:"CandidatePath"`
		PromotedAt    time.Time `json:"PromotedAt"`
		Status        string    `json:"Status"`
		VersionAfter  int       `json:"VersionAfter"`
	}

	type legacyRevertRecord struct {
		FromVersion         int       `json:"FromVersion"`
		ToVersion           int       `json:"ToVersion"`
		RevertedExperiments []string  `json:"RevertedExperiments"`
		Reason              string    `json:"Reason"`
		RevertedAt          time.Time `json:"RevertedAt"`
	}

	type legacySimulationConstraints struct {
		StartingCash                float64 `json:"StartingCash"`
		MaxPositionWeight           float64 `json:"MaxPositionWeight"`
		MaxOpenPositions            int     `json:"MaxOpenPositions"`
		MinTradableVolume           int64   `json:"MinTradableVolume"`
		MinRecommendationConviction int     `json:"MinRecommendationConviction"`
		RequireCROPass              bool    `json:"RequireCROPass"`
		TransactionCostBPS          float64 `json:"TransactionCostBPS"`
		SlippageBPS                 float64 `json:"SlippageBPS"`
		ReserveCashFraction         float64 `json:"ReserveCashFraction"`
		StopLossPct                 float64 `json:"StopLossPct"`
		TakeProfitPct               float64 `json:"TakeProfitPct"`
		MaxHoldingDays              int     `json:"MaxHoldingDays"`
	}

	type legacyExecutionPolicy struct {
		ConvictionFloor               int  `json:"ConvictionFloor"`
		RequireCROPass                bool `json:"RequireCROPass"`
		MomentumCrashProtection       bool `json:"MomentumCrashProtection"`
		EnableConvictionNormalization bool `json:"EnableConvictionNormalization"`
	}

	type legacyPolicy struct {
		Version         int                         `json:"Version"`
		PromptOverrides map[string]string           `json:"PromptOverrides"`
		Constraints     legacySimulationConstraints `json:"Constraints"`
		ExecutionPolicy legacyExecutionPolicy       `json:"ExecutionPolicy"`
		Promotions      []legacyPromotionRecord     `json:"Promotions"`
		RevertHistory   []legacyRevertRecord        `json:"RevertHistory"`
		LastUpdatedAt   time.Time                   `json:"LastUpdatedAt"`
	}

	var legacy legacyPolicy
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("unmarshal legacy policy: %w", err)
	}

	promotions := make([]PromotionRecord, len(legacy.Promotions))
	for i, lp := range legacy.Promotions {
		promotions[i] = PromotionRecord{
			ExperimentID:  lp.ExperimentID,
			TargetAgentID: lp.TargetAgentID,
			TargetSkill:   lp.TargetSkill,
			MutationType:  lp.MutationType,
			CandidatePath: lp.CandidatePath,
			PromotedAt:    lp.PromotedAt,
			Status:        lp.Status,
			VersionAfter:  lp.VersionAfter,
		}
	}

	revertHistory := make([]RevertRecord, len(legacy.RevertHistory))
	for i, lr := range legacy.RevertHistory {
		revertHistory[i] = RevertRecord(lr)
	}

	*p = Policy{
		Version:         legacy.Version,
		PromptOverrides: legacy.PromptOverrides,
		Constraints: domain.SimulationConstraints{
			StartingCash:                legacy.Constraints.StartingCash,
			MaxPositionWeight:           legacy.Constraints.MaxPositionWeight,
			MaxOpenPositions:            legacy.Constraints.MaxOpenPositions,
			MinTradableVolume:           legacy.Constraints.MinTradableVolume,
			MinRecommendationConviction: legacy.Constraints.MinRecommendationConviction,
			RequireCROPass:              legacy.Constraints.RequireCROPass,
			TransactionCostBPS:          legacy.Constraints.TransactionCostBPS,
			SlippageBPS:                 legacy.Constraints.SlippageBPS,
			ReserveCashFraction:         legacy.Constraints.ReserveCashFraction,
			StopLossPct:                 legacy.Constraints.StopLossPct,
			TakeProfitPct:               legacy.Constraints.TakeProfitPct,
			MaxHoldingDays:              legacy.Constraints.MaxHoldingDays,
		},
		ExecutionPolicy: domain.ExecutionPolicy{
			ConvictionFloor:               legacy.ExecutionPolicy.ConvictionFloor,
			RequireCROPass:                legacy.ExecutionPolicy.RequireCROPass,
			MomentumCrashProtection:       legacy.ExecutionPolicy.MomentumCrashProtection,
			EnableConvictionNormalization: legacy.ExecutionPolicy.EnableConvictionNormalization,
		},
		Promotions:    promotions,
		RevertHistory: revertHistory,
		LastUpdatedAt: legacy.LastUpdatedAt,
	}
	return nil
}

type PromotionRecord struct {
	ExperimentID        string                        `json:"experiment_id"`
	TargetAgentID       string                        `json:"target_agent_id"`
	TargetSkill         string                        `json:"target_skill"`
	MutationType        string                        `json:"mutation_type"`
	CandidatePath       string                        `json:"candidate_path"`
	PromotedAt          time.Time                     `json:"promoted_at"`
	Status              string                        `json:"status"`
	VersionAfter        int                           `json:"version_after"`
	ConstraintsSnapshot *domain.SimulationConstraints `json:"constraints_snapshot,omitempty"`
	PromptSnapshot      string                        `json:"prompt_snapshot,omitempty"`
}

type RevertRecord struct {
	FromVersion         int       `json:"from_version"`
	ToVersion           int       `json:"to_version"`
	RevertedExperiments []string  `json:"reverted_experiments"`
	Reason              string    `json:"reason"`
	RevertedAt          time.Time `json:"reverted_at"`
}

func DefaultPolicy() Policy {
	cfg := config.GetParametersConfig().Baseline
	constraints := domain.SimulationConstraints{
		StartingCash:                cfg.StartingCash.Value,
		MaxPositionWeight:           cfg.MaxPositionWeight.Value,
		MaxOpenPositions:            cfg.MaxOpenPositions.Value,
		MinTradableVolume:           int64(cfg.MinTradableVolume.Value),
		MinRecommendationConviction: cfg.MinRecommendationConviction.Value,
		RequireCROPass:              cfg.RequireCROPass.Value,
		TransactionCostBPS:          cfg.TransactionCostBPS.Value,
		DiscountedCommissionBps:     cfg.DiscountedCommissionBps.Value,
		CommissionDiscountThreshold: cfg.CommissionDiscountThreshold.Value,
		SlippageBPS:                 cfg.SlippageBPS.Value,
		ReserveCashFraction:         cfg.ReserveCashFraction.Value,
		MaxHoldingDays:              0,
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
		logging.Warn("baseline", "using_default_policy", "reason", "empty_path")
		return DefaultPolicy(), nil
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Warn("baseline", "using_default_policy", "reason", "file_not_found", "path", path)
			return DefaultPolicy(), nil
		}
		return Policy{}, fmt.Errorf("read policy file: %w", err)
	}
	var policy Policy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return Policy{}, fmt.Errorf("unmarshal policy: %w", err)
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

// LoadStrict loads a baseline policy from the given path.
// Unlike Load, it returns ErrBaselineNotLoaded if the file does not exist
// instead of returning DefaultPolicy.
func LoadStrict(path string) (Policy, error) {
	if path == "" {
		return Policy{}, fmt.Errorf("baseline policy path must not be empty")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Policy{}, fmt.Errorf("%w: %s", ErrBaselineNotLoaded, path)
		}
		return Policy{}, fmt.Errorf("read policy file: %w", err)
	}
	var policy Policy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return Policy{}, fmt.Errorf("unmarshal policy: %w", err)
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
		return fmt.Errorf("mkdir: %w", err)
	}
	policy.LastUpdatedAt = time.Now()
	bytes, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}
	return os.WriteFile(path, bytes, 0o644)
}

func SaveWithLock(path string, policy Policy) error {
	if path == "" {
		return errors.New("baseline policy path must not be empty")
	}
	policy.LastUpdatedAt = time.Now()
	bytes, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}
	return config.LockedWriteFileWithRollback(path, bytes)
}

func ExecutionPolicyFromConstraints(constraints domain.SimulationConstraints) domain.ExecutionPolicy {
	floor := constraints.MinRecommendationConviction
	if floor <= 0 {
		floor = config.GetParametersConfig().Baseline.MinRecommendationConviction.Value
	}
	return domain.ExecutionPolicy{
		ConvictionFloor:               floor,
		RequireCROPass:                constraints.RequireCROPass,
		MomentumCrashProtection:       true,
		EnableConvictionNormalization: true,
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
	lines := strings.SplitSeq(candidate, "\n")
	for line := range lines {
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
		case strings.HasPrefix(line, "max_holding_days:"):
			if v, ok := parseIntValue(line); ok {
				base.MaxHoldingDays = v
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
	record := PromotionRecord{
		ExperimentID:  result.Experiment.ID,
		TargetAgentID: result.Experiment.TargetAgentID,
		TargetSkill:   result.Experiment.Skill,
		MutationType:  result.Experiment.MutationType,
		CandidatePath: result.CandidatePrompt,
		PromotedAt:    time.Now(),
		Status:        string(result.Experiment.Status),
		VersionAfter:  next.Version,
	}
	switch result.Experiment.MutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		snapshot := next.Constraints
		record.ConstraintsSnapshot = &snapshot
	case "prompt_tightening", "":
		record.PromptSnapshot = candidate
	}
	next.Promotions = append(next.Promotions, record)
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
