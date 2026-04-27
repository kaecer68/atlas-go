package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func statusText(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "warn":
		return "延遲"
	case "error":
		return "異常"
	case "inactive":
		return "未啟用"
	default:
		return "未知"
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func parseFloatQuery(r *http.Request, key string, defaultValue float64) float64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultValue
	}
	return v
}

func parseLimit(r *http.Request, defaultValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultValue, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: must be integer")
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid limit: must be > 0")
	}
	if v > maxValue {
		return maxValue, nil
	}
	return v, nil
}

func sessionDateFromID(id string) time.Time {
	const prefix = "session-"
	if !strings.HasPrefix(id, prefix) {
		return time.Time{}
	}
	trimmed := strings.TrimPrefix(id, prefix)
	parts := strings.Split(trimmed, "-")
	if len(parts) < 1 {
		return time.Time{}
	}
	if d, err := time.Parse("20060102", parts[0]); err == nil {
		return d
	}
	return time.Time{}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func isStockPickingLayer(layer string) bool {
	return layer == "sector" || layer == "style" || layer == "superinvestor"
}

func isStockPickingLayerByID(agentID string, views []AgentUniverseView) bool {
	for _, v := range views {
		if v.AgentID == agentID {
			return isStockPickingLayer(v.Layer)
		}
	}
	return false
}

func buildMutationSummary(policy baseline.Policy, result domain.PromptExperimentResult) string {
	baselinePrompt := baseline.ResolvePromptOverride(policy, result.Experiment.TargetAgentID, result.Experiment.Skill)
	if baselinePrompt == "" {
		sourcePrompt, err := os.ReadFile(result.Brief.PromptFile)
		if err == nil {
			baselinePrompt = string(sourcePrompt)
		}
	}

	baselineCtrl, _ := domain.ExtractPromptControl(baselinePrompt)
	candidateBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		return result.Experiment.MutationType
	}
	candidateCtrl, _ := domain.ExtractPromptControl(string(candidateBytes))

	parts := make([]string, 0, 4)
	add := func(name string, base, cand int64) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %d→%d", name, base, cand))
		}
	}
	addInt := func(name string, base, cand int) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %d→%d", name, base, cand))
		}
	}
	addBool := func(name string, base, cand bool) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %t→%t", name, base, cand))
		}
	}

	add("volume_floor", baselineCtrl.VolumeFloor, candidateCtrl.VolumeFloor)
	addInt("volume_downgrade", baselineCtrl.VolumeDowngrade, candidateCtrl.VolumeDowngrade)
	addInt("close_strength_boost", baselineCtrl.CloseStrengthBoost, candidateCtrl.CloseStrengthBoost)
	add("hard_reject_volume", baselineCtrl.HardRejectVolume, candidateCtrl.HardRejectVolume)
	addInt("conviction_floor", baselineCtrl.ConvictionFloor, candidateCtrl.ConvictionFloor)
	addInt("volume_boost", baselineCtrl.VolumeBoost, candidateCtrl.VolumeBoost)
	addInt("neutral_penalty_reduction", baselineCtrl.NeutralPenaltyReduction, candidateCtrl.NeutralPenaltyReduction)
	addBool("require_trend", baselineCtrl.RequireTrend, candidateCtrl.RequireTrend)

	if len(parts) == 0 {
		return result.Experiment.MutationType
	}
	return strings.Join(parts, ", ")
}

func computePipelineTags(ds *replay.Dataset, symbol string, date time.Time) []string {
	if ds == nil {
		return nil
	}
	dateKey := date.Format("2006-01-02")
	bar, ok := ds.ByDate[dateKey][symbol]
	if !ok {
		return nil
	}
	var prevBar domain.DailyBar
	var hasPrev bool
	for i, d := range ds.Dates {
		if d.Format("2006-01-02") == dateKey && i > 0 {
			prevBar = ds.ByDate[ds.Dates[i-1].Format("2006-01-02")][symbol]
			hasPrev = prevBar.Close > 0
			break
		}
	}

	tags := make([]string, 0, 3)
	changePct := 0.0
	if bar.Open > 0 {
		changePct = (bar.Close - bar.Open) / bar.Open
	}
	if changePct > 0.035 {
		tags = append(tags, "長紅")
	} else if changePct < -0.035 {
		tags = append(tags, "長黑")
	}
	if hasPrev && prevBar.Volume > 0 && bar.Volume > int64(float64(prevBar.Volume)*1.5) {
		tags = append(tags, "放量")
	}

	high5 := bar.Close
	low5 := bar.Close
	for i, d := range ds.Dates {
		if d.Format("2006-01-02") == dateKey {
			start := i - 4
			if start < 0 {
				start = 0
			}
			for _, pd := range ds.Dates[start : i+1] {
				b := ds.ByDate[pd.Format("2006-01-02")][symbol]
				if b.Close > high5 {
					high5 = b.Close
				}
				if b.Close > 0 && (low5 == 0 || b.Close < low5) {
					low5 = b.Close
				}
			}
			break
		}
	}
	if bar.Close > 0 && bar.Close == high5 {
		tags = append(tags, "創5日高")
	}
	if bar.Close > 0 && low5 > 0 && bar.Close == low5 {
		tags = append(tags, "創5日低")
	}
	return tags
}

func fallbackPriceTargets(skill string, price float64) (float64, float64) {
	var targetMult, stopLossMult float64
	switch skill {
	case "semiconductor_desk":
		targetMult, stopLossMult = 1.06, 0.95
	case "ai_supply_chain_desk":
		targetMult, stopLossMult = 1.08, 0.95
	case "etf_rotation_desk":
		targetMult, stopLossMult = 1.04, 0.97
	case "financials_desk":
		targetMult, stopLossMult = 1.05, 0.96
	case "shipping_desk":
		targetMult, stopLossMult = 1.07, 0.94
	case "growth_momentum":
		targetMult, stopLossMult = 1.08, 0.95
	case "value_yield":
		targetMult, stopLossMult = 1.05, 0.96
	case "earnings_quality":
		targetMult, stopLossMult = 1.06, 0.95
	case "technical_breakout":
		targetMult, stopLossMult = 1.10, 0.94
	case "alpha_discovery":
		targetMult, stopLossMult = 1.06, 0.95
	default:
		targetMult, stopLossMult = 1.05, 0.95
	}
	return price * targetMult, price * stopLossMult
}
