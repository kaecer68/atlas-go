package monitoring

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
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

func BuildMutationSummary(policy baseline.Policy, result domain.PromptExperimentResult) string {
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

func GetSymbolSector(symbol string, symMap map[string]string) string {
	if s, ok := symMap[symbol]; ok {
		return s
	}
	return "other"
}

func ComputeSectorFactorExposure(outcomes []domain.RecommendationOutcome, portfolioValue float64, symSectorMap map[string]string) ([]SectorExposure, FactorExposureInline) {
	sectorLabelMap := map[string]string{
		"semiconductor":   "半導體",
		"ai_supply_chain": "AI供應鏈",
		"robotics":        "機器人",
		"financials":      "金融",
		"shipping":        "航運",
		"energy":          "能源",
		"electronics":     "電子",
		"consumer":        "消費",
		"industrial":      "工業",
		"other":           "其他",
	}

	type secAgg struct {
		count                        int
		absReturn                    float64
		avgM, avgV, avgQ, avgA, avgT float64
	}
	secMap := make(map[string]*secAgg)

	var totalM, totalV, totalQ, totalA, totalT float64
	var totalAbsReturn float64
	var cnt int

	for _, oc := range outcomes {
		if !oc.PassedGuards || oc.Symbol == "" {
			continue
		}
		sec := GetSymbolSector(oc.Symbol, symSectorMap)
		if secMap[sec] == nil {
			secMap[sec] = &secAgg{}
		}
		s := secMap[sec]
		s.count++
		s.absReturn += math.Abs(oc.ForwardReturn)
		totalAbsReturn += math.Abs(oc.ForwardReturn)
		s.avgM += oc.FactorScores.Momentum
		s.avgV += oc.FactorScores.Value
		s.avgQ += oc.FactorScores.Quality
		s.avgA += oc.FactorScores.Agent
		s.avgT += oc.FactorScores.Total

		totalM += oc.FactorScores.Momentum
		totalV += oc.FactorScores.Value
		totalQ += oc.FactorScores.Quality
		totalA += oc.FactorScores.Agent
		totalT += oc.FactorScores.Total
		cnt++
	}

	var sectorExp []SectorExposure
	for sec, s := range secMap {
		weight := 0.0
		if totalAbsReturn > 0 {
			weight = s.absReturn / totalAbsReturn
		}
		sectorExp = append(sectorExp, SectorExposure{
			Sector:      sec,
			SectorLabel: sectorLabelMap[sec],
			Weight:      weight,
			EstValue:    weight * portfolioValue,
		})
	}

	var fe FactorExposureInline
	if cnt > 0 {
		fe = FactorExposureInline{
			Momentum: totalM / float64(cnt),
			Value:    totalV / float64(cnt),
			Quality:  totalQ / float64(cnt),
			Agent:    totalA / float64(cnt),
			Total:    totalT / float64(cnt),
		}
	}

	return sectorExp, fe
}

func PromotionHistoryToAPI(history []baseline.PromotionRecordWithVersion) []map[string]any {
	result := make([]map[string]any, len(history))
	for i, h := range history {
		result[i] = map[string]any{
			"experiment_id":   h.ExperimentID,
			"target_agent_id": h.TargetAgentID,
			"target_skill":    h.TargetSkill,
			"mutation_type":   h.MutationType,
			"candidate_path":  h.CandidatePath,
			"promoted_at":     h.PromotedAt,
			"status":          h.Status,
			"version_after":   h.VersionAfter,
			"version":         h.Version,
		}
	}
	return result
}

func LoadSessionSummary(ledgerDir, sessionID string) (*domain.SessionSummary, error) {
	sessionsDir := filepath.Join(ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]domain.SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if sessionID != "" && entry.Name() != sessionID {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	if sessionID != "" {
		selected := summaries[0]
		return &selected, nil
	}

	slices.SortFunc(summaries, func(a, b domain.SessionSummary) int {
		aDate := sessionDateFromID(a.SessionID)
		bDate := sessionDateFromID(b.SessionID)
		switch {
		case aDate.After(bDate):
			return -1
		case aDate.Before(bDate):
			return 1
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})
	latest := summaries[0]
	return &latest, nil
}
