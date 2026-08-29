package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

type CalibratedOrder struct {
	Symbol        string
	Side          string
	ForwardReturn float64
	FactorScores  map[FactorType]float64
	AgentID       string // executor/agent that produced this recommendation
	Skill         string // executor skill name (e.g. "momentum", "value")
}

type WeightCalibrationReport struct {
	Timestamp       time.Time      `json:"timestamp"`
	OrdersEvaluated int            `json:"orders_evaluated"`
	BaselineScore   float64        `json:"baseline_score"`
	OptimizedScore  float64        `json:"optimized_score"`
	ImprovementPct  float64        `json:"improvement_pct"`
	Verdict         string         `json:"verdict"`
	Changes         []WeightChange `json:"changes"`
	Summary         string         `json:"summary"`
}

type WeightChange struct {
	Factor     FactorType `json:"factor"`
	Before     float64    `json:"before"`
	After      float64    `json:"after"`
	DeltaPct   float64    `json:"delta_pct"`
	Confidence string     `json:"confidence"`
}

func CalibrateWeights(ctx context.Context, orders []CalibratedOrder) (*WeightCalibrationReport, error) {
	if len(orders) == 0 {
		return nil, fmt.Errorf("calibrate: no orders")
	}
	current := currentFactorWeights()

	trainEnd := len(orders) * 8 / 10
	train := orders[:trainEnd]
	valid := orders[trainEnd:]
	if len(train) < 5 {
		train = orders
		valid = nil
	}

	baseline := evalWeights(train, current)
	optimal, optScore, err := searchWeights(train, current)
	improvement := (optScore - baseline) / math.Abs(baseline+1e-10) * 100

	verdict := "stable"
	summary := ""
	var changes []WeightChange

	if err != nil {
		summary = "optimization failed"
	} else if len(valid) >= 2 {
		baseV := evalWeights(valid, current)
		optV := evalWeights(valid, optimal)
		vDelta := (optV - baseV) / math.Abs(baseV+1e-10) * 100
		switch {
		case vDelta > 3.0 && improvement > 5.0:
			osc := detectOscillation(current, optimal)
			applied := optimal
			if osc.Detected {
				applied = dampenWeights(optimal, current, osc.DampingFactor)
				verdict = "oscillation_dampened"
			} else {
				verdict = "applied"
			}
			summary = fmt.Sprintf("valid=+%.1f%% train=+%.1f%%", vDelta, improvement)
			if osc.Detected {
				summary += " oscillation=" + osc.Reason
			}
			changes = buildWeightChanges(current, applied, len(orders))
			applyFactorWeights(applied)
		case vDelta < -2.0:
			verdict = "degraded"
			summary = fmt.Sprintf("valid=%.1f%% (train=+%.1f%%)", vDelta, improvement)
		default:
			summary = fmt.Sprintf("valid=+%.1f%% stable", vDelta)
		}
	} else if improvement > 15.0 {
		osc := detectOscillation(current, optimal)
		applied := optimal
		if osc.Detected {
			applied = dampenWeights(optimal, current, osc.DampingFactor)
			verdict = "oscillation_dampened"
		} else {
			verdict = "applied"
		}
		summary = fmt.Sprintf("train=+%.1f%%", improvement)
		if osc.Detected {
			summary += " oscillation=" + osc.Reason
		}
		changes = buildWeightChanges(current, applied, len(orders))
		applyFactorWeights(applied)
	} else {
		summary = fmt.Sprintf("train=+%.1f%% stable", improvement)
	}

	return &WeightCalibrationReport{
		Timestamp: time.Now(), OrdersEvaluated: len(orders),
		BaselineScore: baseline, OptimizedScore: optScore,
		ImprovementPct: improvement, Verdict: verdict,
		Changes: changes, Summary: summary,
	}, nil
}

func currentFactorWeights() map[FactorType]float64 {
	w := make(map[FactorType]float64)
	params := config.GetParametersConfig()
	if params == nil || params.FactorWeight.BaseWeights.Value == nil {
		maps.Copy(w, defaultBaseWeights())
		return w
	}
	for k, v := range params.FactorWeight.BaseWeights.Value {
		w[FactorType(k)] = v
	}
	return w
}

func applyFactorWeights(w map[FactorType]float64) {
	params := config.GetParametersConfig()
	if params == nil {
		return
	}
	if params.FactorWeight.BaseWeights.Value == nil {
		params.FactorWeight.BaseWeights.Value = make(map[string]float64)
	}
	for ft, v := range w {
		params.FactorWeight.BaseWeights.Value[string(ft)] = v
	}
	now := time.Now()
	params.FactorWeight.BaseWeights.LastCalibrated = &now
	params.FactorWeight.BaseWeights.CalibrationMethod = "bayesian_search"
	if p := config.GetParametersConfigPath(); p != "" {
		if err := config.SnapshotToBackup(p); err != nil {
			fmt.Printf("calibrator: snapshot_to_backup failed: %v\n", err)
		}
		_ = params.LockedSaveWithRollback(p)
	}
}

func searchWeights(orders []CalibratedOrder, current map[FactorType]float64) (map[FactorType]float64, float64, error) {
	names := sortedNames(current)
	bounds := make([][2]float64, len(names))
	for i := range bounds {
		bounds[i] = [2]float64{0.02, 0.50}
	}
	eval := func(x []float64) (float64, error) {
		return evalWeights(orders, vecToWeights(x, names, current)), nil
	}
	cfg := config.DefaultOptimizerConfig()
	cfg.InitialPoints, cfg.Iterations, cfg.LengthScale = 10, 20, 0.3
	opt := config.NewBayesianOptimizer(bounds, eval, cfg)
	r, err := opt.Optimize()
	if err != nil {
		return current, evalWeights(orders, current), err
	}
	return vecToWeights(r.BestX, names, current), r.BestScore, nil
}

func buildWeightChanges(before, after map[FactorType]float64, n int) []WeightChange {
	var cs []WeightChange
	for ft, nw := range after {
		ow := before[ft]
		d := (nw - ow) / math.Abs(ow+1e-10) * 100
		if math.Abs(d) < 1.0 {
			continue
		}
		conf := "low"
		if n >= 20 && math.Abs(d) > 5 {
			conf = "high"
		} else if n >= 10 && math.Abs(d) > 3 {
			conf = "medium"
		}
		cs = append(cs, WeightChange{Factor: ft, Before: ow, After: nw, DeltaPct: d, Confidence: conf})
	}
	return cs
}

func evalWeights(orders []CalibratedOrder, weights map[FactorType]float64) float64 {
	var pnl float64
	var buys int
	for _, o := range orders {
		if weightedScore(o.FactorScores, weights) > 0 {
			pnl += o.ForwardReturn
			buys++
		}
	}
	n := float64(len(orders))
	cov := float64(buys) / n
	if cov < 0.05 || cov > 0.95 {
		return -1.0
	}
	return pnl/math.Sqrt(n) + cov*0.5
}

func weightedScore(scores map[FactorType]float64, weights map[FactorType]float64) float64 {
	var t float64
	for ft, w := range weights {
		if s, ok := scores[ft]; ok {
			t += s * w
		}
	}
	return t
}

func sortedNames(w map[FactorType]float64) []string {
	ns := make([]string, 0, len(w))
	for ft := range w {
		ns = append(ns, string(ft))
	}
	sort.Strings(ns)
	return ns
}

func vecToWeights(x []float64, names []string, fallback map[FactorType]float64) map[FactorType]float64 {
	raw := make(map[FactorType]float64, len(names))
	for i, name := range names {
		if i < len(x) {
			raw[FactorType(name)] = x[i]
		}
	}
	var total float64
	for _, v := range raw {
		total += v
	}
	if total > 0 {
		for k := range raw {
			raw[k] /= total
		}
	}
	for k, v := range fallback {
		if _, ok := raw[k]; !ok {
			raw[k] = v
		}
	}
	return raw
}

func LoadOrdersFromJSONL(path string) ([]CalibratedOrder, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var orders []CalibratedOrder
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var raw struct {
			Symbol        string             `json:"symbol"`
			Side          string             `json:"side"`
			ForwardReturn float64            `json:"forward_return"`
			FactorScores  map[string]float64 `json:"factor_scores"`
			AgentID       string             `json:"agent_id"`
			Skill         string             `json:"skill"`
		}
		if json.Unmarshal([]byte(line), &raw) != nil || raw.Symbol == "" {
			continue
		}
		scores := make(map[FactorType]float64)
		for k, v := range raw.FactorScores {
			scores[FactorType(k)] = v
		}
		orders = append(orders, CalibratedOrder{
			Symbol: raw.Symbol, Side: raw.Side,
			ForwardReturn: raw.ForwardReturn, FactorScores: scores,
			AgentID: raw.AgentID, Skill: raw.Skill,
		})
	}
	return orders, nil
}

func splitLines(s string) []string {
	var ls []string
	st := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			ls = append(ls, s[st:i])
			st = i + 1
		}
	}
	if st < len(s) {
		ls = append(ls, s[st:])
	}
	return ls
}
