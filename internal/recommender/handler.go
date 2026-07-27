package recommender

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/subscription"
)

// TierRecommendation wraps tier-specific recommendation content.
type TierRecommendation struct {
	Tier       string                  `json:"tier"`
	Market     MarketLight             `json:"market"`
	Strategies *StrategyRecommendation `json:"strategies,omitempty"`
	Signals    any                     `json:"signals,omitempty"`
	Warning    string                  `json:"warning,omitempty"`
}

// RankedBriefEntry carries a strategy identity and category for the
// RankedBrief payload. This is a local type intentionally separate
// from config.RankedBriefEntry so the recommender and config packages
// evolve independently (E5a vs E5b).
type RankedBriefEntry struct {
	ID       string `json:"id"`
	Category string `json:"category"`
}

// StrategyRecommendation is the structured strategies payload.
type StrategyRecommendation struct {
	Active      string             `json:"active,omitempty"`
	Available   []string           `json:"available,omitempty"`
	Ranked      []string           `json:"ranked,omitempty"`
	RankedBrief []RankedBriefEntry `json:"ranked_brief,omitempty"`
	EntrySignal string             `json:"entry_signal,omitempty"`
	StopLoss    string             `json:"stop_loss,omitempty"`
}

// MarketLight is the free/public tier market overview.
type MarketLight struct {
	Regime            string             `json:"regime"`
	RegimeLabel       string             `json:"regime_label"`
	StressIndex       float64            `json:"stress_index"`
	CapitalFlow       string             `json:"capital_flow_summary"`
	CapitalFlowDetail *CapitalFlowDetail `json:"capital_flow_detail,omitempty"`
	EventsToday       []string           `json:"events_today"`
}

// CapitalFlowDetail is the structured counterpart to CapitalFlow string.
// Derived from the same capitalflow.DailyReport used for the summary string,
// avoiding a second provider snapshot fetch. New consumers should prefer this
// when present; the string field is kept for backward compatibility.
type CapitalFlowDetail struct {
	Date          string  `json:"date"`
	QualityLabel  string  `json:"quality_label"`
	QualityScore  float64 `json:"quality_score"`
	ResonanceDir  string  `json:"resonance_dir"`
	DominantForce string  `json:"dominant_force"`
}

// Handler serves tier-based recommendations.
type Handler struct {
	subStore           subscription.Store
	jwtMgr             *subscription.JWTManager
	narrative          NarrativeProvider
	capitalFlow        CapitalFlowProvider
	eventPredictor     EventPredictor
	strategyComp       ComparisonEngine
	methodologyAdvisor *methodology.Advisor
	regimeListener     RegimeChangeListener
	lastSeenRegime     string
	devMode            bool
	regimeMu           sync.Mutex
}

// NewHandler creates a recommendation handler with optional JWT verification.
func NewHandler(store subscription.Store, jwtMgr *subscription.JWTManager) *Handler {
	return &Handler{subStore: store, jwtMgr: jwtMgr}
}

func (h *Handler) WithDevMode(enabled bool) *Handler {
	h.devMode = enabled
	return h
}

// NewHandlerWithServices constructs a Handler with all Sprint 2 T8-T9 service deps.
func NewHandlerWithServices(
	store subscription.Store,
	jwtMgr *subscription.JWTManager,
	narrative NarrativeProvider,
	capitalFlow CapitalFlowProvider,
	eventPredictor EventPredictor,
	strategy ComparisonEngine,
	methodologyAdvisor *methodology.Advisor,
) *Handler {
	return &Handler{
		subStore:           store,
		jwtMgr:             jwtMgr,
		narrative:          narrative,
		capitalFlow:        capitalFlow,
		eventPredictor:     eventPredictor,
		strategyComp:       strategy,
		methodologyAdvisor: methodologyAdvisor,
	}
}

// WithRegimeListener attaches a callback fired on regime change.
func (h *Handler) WithRegimeListener(l RegimeChangeListener) *Handler {
	h.regimeListener = l
	return h
}

// HandleRecommendations returns tier-appropriate recommendations.
func (h *Handler) HandleRecommendations(r *http.Request) (int, any) {
	var warnings []string
	tier := subscription.TierFree
	authenticated := false

	if h.jwtMgr != nil {
		if token := subscription.ExtractToken(r); token != "" {
			if claims, err := h.jwtMgr.Verify(token); err == nil {
				if user, err := h.subStore.GetByEmail(claims.Email); err == nil {
					tier = user.EffectiveTier()
					authenticated = true
				}
			}
		}
	}

	if !authenticated && devModeEnabled(h) {
		if email := r.Header.Get("X-User-Email"); email != "" {
			if user, err := h.subStore.GetByEmail(email); err == nil {
				tier = user.EffectiveTier()
				authenticated = true
			}
		}
	}

	if !authenticated && !devModeEnabled(h) && r.Header.Get("X-User-Email") != "" {
		return http.StatusUnauthorized, map[string]string{
			"error": "X-User-Email header not allowed in production",
		}
	}

	var capitalFlowReport *capitalflow.DailyReport
	var capitalFlowErr error
	if h.capitalFlow != nil {
		report, err := h.capitalFlow.LatestDaily(r.Context())
		capitalFlowErr = err
		if err == nil {
			capitalFlowReport = &report
		}
	}
	assessmentCalibrating := capitalFlowReport != nil &&
		capitalFlowReport.Assessment.CalibrationStatus == capitalflow.CalibrationCalibrating

	rec := TierRecommendation{
		Tier: string(tier),
		Market: MarketLight{
			Regime:            regimeFromNarrative(h.narrative, &warnings),
			RegimeLabel:       "盤勢中性",
			StressIndex:       stressIndexFromNarrative(h.narrative, &warnings),
			CapitalFlow:       capitalFlowFromCapitalFlow(capitalFlowReport, capitalFlowErr, &warnings),
			CapitalFlowDetail: capitalFlowDetailFromCapitalFlow(capitalFlowReport, capitalFlowErr, &warnings),
			EventsToday:       eventsFromPredictor(h.eventPredictor, &warnings),
		},
	}
	if assessmentCalibrating {
		warnings = append(warnings, "capital_flow_assessment_calibrating")
	}

	h.detectRegimeChange(rec.Market.Regime)

	switch tier {
	case subscription.TierFree:
		applyWarning(&rec, &warnings)
		return http.StatusOK, rec

	case subscription.TierRegistered:
		rec.Strategies = &StrategyRecommendation{
			Active:    "all_weather",
			Available: []string{"all_weather", "defensive"},
		}
		applyWarning(&rec, &warnings)
		return http.StatusOK, rec

	case subscription.TierPremium:
		rec.Strategies = buildPremiumStrategy(h.strategyComp, rec.Market.Regime, h.methodologyAdvisor, &warnings)
		applyWarning(&rec, &warnings)
		return http.StatusOK, rec

	default:
		applyWarning(&rec, &warnings)
		return http.StatusOK, rec
	}
}

// applyWarning joins the warnings slice and sets rec.Warning.
func applyWarning(rec *TierRecommendation, warnings *[]string) {
	if len(*warnings) > 0 {
		rec.Warning = strings.Join(*warnings, "; ")
	}
}

// RegisterRoutes registers recommendation endpoints with optional JWT verification.
// HandlerDeps groups optional service dependencies for /api/recommendations.
// Any nil field falls back to a hardcoded safe default at request time
type HandlerDeps struct {
	Narrative          NarrativeProvider
	CapitalFlow        CapitalFlowProvider
	EventPredictor     EventPredictor
	StrategyComp       ComparisonEngine
	MethodologyAdvisor *methodology.Advisor
}

// RegisterRoutes wires /api/recommendations via NewHandler (no services).
// Call RegisterRoutesWithDeps from main.go to enable live data integration.
func RegisterRoutes(mux *http.ServeMux, store subscription.Store, jwtMgr *subscription.JWTManager) {
	RegisterRoutesWithDeps(mux, store, jwtMgr, HandlerDeps{}, false)
}

// RegisterRoutesWithDeps is the production entry point. It instantiates the
// handler with optional service deps and mounts /api/recommendations.
//
// devMode=true enables X-User-Email fallback (legacy/dev only — see Q1 decision);
// main.go should pass config.LoadBool("ATLAS_DEV_MODE") here, NOT call os.Getenv
// from this package (per internal/apigateway/CONSTITUTION.md Art.1).
func RegisterRoutesWithDeps(
	mux *http.ServeMux,
	store subscription.Store,
	jwtMgr *subscription.JWTManager,
	deps HandlerDeps,
	devMode bool,
) {
	h := NewHandlerWithServices(
		store, jwtMgr,
		deps.Narrative, deps.CapitalFlow,
		deps.EventPredictor, deps.StrategyComp,
		deps.MethodologyAdvisor,
	).WithDevMode(devMode)
	mux.HandleFunc("GET /api/recommendations", func(w http.ResponseWriter, r *http.Request) {
		code, data := h.HandleRecommendations(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("[Recommender] encode response: %v", err)
		}
	})
}

// =====================================================================
// Helpers — read real producer types into MarketLight / StrategyRecommendation
// =====================================================================

// regimeFromNarrative reads NarrativeProvider.GetCurrentStressIndex() and
// returns the Regime string, or "NEUTRAL" fallback.
func regimeFromNarrative(p NarrativeProvider, w *[]string) string {
	if p == nil {
		*w = append(*w, "regime_unavailable")
		return "NEUTRAL"
	}
	tcsi := p.GetCurrentStressIndex()
	if tcsi.Regime == "" {
		*w = append(*w, "regime_unavailable")
		return "NEUTRAL"
	}
	return tcsi.Regime
}

// stressIndexFromNarrative reads NarrativeProvider.GetCurrentStressIndex()
// and returns the score value, or 0.0 fallback.
func stressIndexFromNarrative(p NarrativeProvider, w *[]string) float64 {
	if p == nil {
		*w = append(*w, "stress_index_unavailable")
		return 0.0
	}
	tcsi := p.GetCurrentStressIndex()
	// Treat zero score as "no data" (historical stress has non-zero baseline).
	if tcsi.Score == 0 {
		*w = append(*w, "stress_index_unavailable")
		return 0.0
	}
	return tcsi.Score
}

// capitalFlowFromCapitalFlow derives the legacy summary string from the one
// DailyReport fetched by HandleRecommendations. It never calls the provider.
func capitalFlowFromCapitalFlow(report *capitalflow.DailyReport, fetchErr error, w *[]string) string {
	if report == nil || fetchErr != nil || report.Summary == "" {
		*w = append(*w, "capital_flow_unavailable")
		return "資金流向均衡"
	}
	return report.Summary
}

// capitalFlowDetailFromCapitalFlow derives the structured legacy detail from
// the same DailyReport used for CapitalFlow. It never calls Summary or
// LatestAssessment, so one recommendation request consumes one macro snapshot.
func capitalFlowDetailFromCapitalFlow(report *capitalflow.DailyReport, fetchErr error, w *[]string) *CapitalFlowDetail {
	if report == nil {
		if fetchErr != nil {
			*w = append(*w, "capital_flow_detail_unavailable")
		}
		return nil
	}
	if fetchErr != nil || report.QualityLabel == "" {
		*w = append(*w, "capital_flow_detail_unavailable")
		return nil
	}
	dominant := report.DominantActor
	if dominant == "" {
		dominant = report.DominantSignal
	}
	if dominant == "" {
		dominant = capitalflow.ForceRetail
	}
	return &CapitalFlowDetail{
		Date:          report.Date.Format("2006-01-02"),
		QualityLabel:  report.QualityLabel,
		QualityScore:  report.QualityScore,
		ResonanceDir:  report.Resonance.Direction,
		DominantForce: string(dominant),
	}
}

// eventsFromPredictor reads EventPredictor.PredictToday() + NextNDays(4)
// and returns 5 short strings for the MarketLight.EventsToday array.
func eventsFromPredictor(p EventPredictor, w *[]string) []string {
	if p == nil {
		return nil
	}
	today, err := p.PredictToday()
	if err != nil || today.Direction == "" {
		*w = append(*w, "events_unavailable")
		return nil
	}
	return []string{
		"today:" + today.Direction,
	}
}

// buildPremiumStrategy composes the StrategyRecommendation for premium tier
// from real ComparisonEngine score + methodology regime filtering.
//
// When methodologyAdvisor is non-nil, ranked strategies are filtered to only
// those allowed in the current regime (per methodology_rules.yaml). The
// regime string is mapped to a MarketPeriod via methodology.RegimeToPeriod.
func buildPremiumStrategy(e ComparisonEngine, regime string, advisor *methodology.Advisor, w *[]string) *StrategyRecommendation {
	// Normalize regime string to the canonical vocabulary (RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL).
	// This handles both stress-index vocabulary (low/alert/high/crisis) and
	// regime-detector vocabulary transparently.
	normalized := narrative.NormalizeRegime(regime)

	// Determine the allowed strategy set for this regime.
	var allowedSet map[string]bool
	if advisor != nil {
		period := methodology.RegimeToPeriod(domainRegimeFromString(normalized))
		allowed := advisor.AllowedStrategies(period)
		if len(allowed) > 0 {
			allowedSet = make(map[string]bool, len(allowed))
			for _, id := range allowed {
				allowedSet[id] = true
			}
		}
	}

	if e == nil {
		// fallback 僅得含 YAML 六策略 id；"defensive" 屬 strategy registry
		// 宇宙，主路徑 filteredRank 會由 allowedSet 濾除，移除對齊兩路徑行為。
		ranked := []string{"growth", "momentum", "all_weather", "value"}
		if allowedSet != nil {
			ranked = filterRanked(ranked, allowedSet)
			if len(ranked) == 0 {
				ranked = []string{"all_weather"} // safe fallback
			}
		}
		return &StrategyRecommendation{
			Active:      ranked[0],
			Ranked:      ranked,
			RankedBrief: buildRankedBrief(ranked, advisor),
			EntrySignal: "等待回測支撐區間",
			StopLoss:    "-5%",
		}
	}
	// F06: use real shadow ranking when available.
	ranked, err := e.RankedStrategies()
	if err != nil || len(ranked) == 0 {
		*w = append(*w, "ranking_warming_up")
		score, _ := e.GetScore("growth")
		// fallback 僅得含 YAML 六策略 id；"defensive" 屬 strategy registry
		// 宇宙，主路徑 filteredRank 會由 allowedSet 濾除，移除對齊兩路徑行為。
		ranked = []string{"growth", "momentum", "all_weather", "value"}
		if allowedSet != nil {
			ranked = filterRanked(ranked, allowedSet)
			if len(ranked) == 0 {
				ranked = []string{"all_weather"}
			}
		}
		return &StrategyRecommendation{
			Active:      ranked[0],
			Ranked:      ranked,
			RankedBrief: buildRankedBrief(ranked, advisor),
			EntrySignal: fmt.Sprintf("排名暖機中 (%.2f)", score),
			StopLoss:    "-5%",
		}
	}
	// Filter ranked strategies to only those allowed in current regime.
	if allowedSet != nil {
		ranked = filterRanked(ranked, allowedSet)
		if len(ranked) == 0 {
			*w = append(*w, "no_strategies_allowed_in_regime")
			ranked = []string{"all_weather"}
		}
	}
	active := ranked[0]
	if len(ranked) > 5 {
		ranked = ranked[:5]
	}
	score, _ := e.GetScore(active)
	return &StrategyRecommendation{
		Active:      active,
		Ranked:      ranked,
		RankedBrief: buildRankedBrief(ranked, advisor),
		EntrySignal: fmt.Sprintf("Score=%.2f — 排名第1", score),
		StopLoss:    "-5%",
	}
}

// buildRankedBrief maps ranked strategy IDs to StrategyBrief entries.
// advisor=nil or unknown IDs produce Category="" without skipping.
func buildRankedBrief(ranked []string, advisor *methodology.Advisor) []RankedBriefEntry {
	briefs := make([]RankedBriefEntry, len(ranked))
	for i, id := range ranked {
		cat := ""
		if advisor != nil {
			cat = advisor.StrategyCategory(id)
		}
		briefs[i] = RankedBriefEntry{ID: id, Category: cat}
	}
	return briefs
}

// filterRanked keeps only IDs present in allowedSet, preserving order.
func filterRanked(ids []string, allowed map[string]bool) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if allowed[id] {
			result = append(result, id)
		}
	}
	return result
}

// domainRegimeFromString converts a regime string (RISK_ON/RISK_OFF/NEUTRAL)
// to a domain.Regime for use with methodology.RegimeToPeriod.
func domainRegimeFromString(s string) domain.Regime {
	switch s {
	case "RISK_ON":
		return domain.RegimeRiskOn
	case "RISK_OFF":
		return domain.RegimeRiskOff
	default:
		return domain.RegimeNeutral
	}
}

// =====================================================================
// Regime change detection
// =====================================================================

func (h *Handler) detectRegimeChange(newRegime string) {
	if h.regimeListener == nil || newRegime == "" {
		return
	}
	h.regimeMu.Lock()
	defer h.regimeMu.Unlock()
	if h.lastSeenRegime == newRegime {
		return
	}
	h.regimeListener(h.lastSeenRegime, newRegime)
	h.lastSeenRegime = newRegime
}

// need this import for narrative.MarketNarrativeData — keeping for future use.
var _ = narrative.MarketNarrativeData{}
