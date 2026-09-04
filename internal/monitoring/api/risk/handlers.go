package risk

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type Handlers struct {
	LedgerDir          string
	correlationMatrix  *industry.CorrelationMatrix
	classificationTree *industry.ClassificationTree
	RiskGate           *risk.RiskGate // optional, set via WithRiskGate()
	// store backs every L-cold read of these handlers through the
	// backend-aware ledger factory (PG-first on production):
	//   - HandleRiskMetrics session-summary history (P1-2: replaces the raw
	//     os.ReadDir over LedgerDir/sessions),
	//   - HandleRiskCommentary's persisted-summary fallback (P0-2).
	// When nil, handlers fall back to the JSONL ledger store (tests/legacy).
	store ledger.OutcomeStore
}

func NewHandlers(ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir}
}

// WithStore wires the backend-aware ledger store used for L-cold reads
// (risk-metrics session history + risk-commentary persisted fallback).
// When nil, handlers fall back to the JSONL ledger store.
func (h *Handlers) WithStore(store ledger.OutcomeStore) *Handlers {
	h.store = store
	return h
}

// WithCorrelationMatrix sets an optional correlation matrix provider.
// When nil, HandleCorrelationMatrix falls back to DefaultCorrelationMatrix().
func (h *Handlers) WithCorrelationMatrix(cm *industry.CorrelationMatrix) *Handlers {
	h.correlationMatrix = cm
	return h
}

// WithClassificationTree sets an optional classification tree for industry label lookup.
// When nil, industryLabel returns raw IDs.
func (h *Handlers) WithClassificationTree(ct *industry.ClassificationTree) *Handlers {
	h.classificationTree = ct
	return h
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/risk", shared.Get(h.HandleRiskMetrics))
	mux.Handle("GET /api/dashboard/correlation-matrix", shared.Get(h.HandleCorrelationMatrix))
	mux.Handle("GET /api/dashboard/risk-calibration", shared.Get(h.HandleRiskCalibration))
	mux.Handle("GET /api/risk/commentary", shared.Get(h.HandleRiskCommentary))
}

func (h *Handlers) HandleRiskMetrics(r *http.Request) (int, any) {
	if h.RiskGate == nil {
		return serviceUnavailable("risk_gate_not_injected", "RiskGate 尚未注入, 請檢查 cmd/atlas main.go 的 DI chain")
	}

	// SSOT (P1-2): session-summary history comes from the backend-aware store
	// (PG-first on production via NewReportOutcomeStore) instead of a raw
	// os.ReadDir over LedgerDir/sessions. This removes the duplicated VaR
	// threshold logic the old scan implied: the store path and the
	// risk-exposure endpoint now read the same PG history.
	store := h.summariesStore()
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		logging.Warn("risk_handler", "summaries_unreadable", logging.Err(err))
		return http.StatusOK, map[string]any{
			"risk_snapshot": map[string]float64{
				"var_95":            0,
				"var_99":            0,
				"cvar_95":           0,
				"max_drawdown_pct":  0,
				"data_points":       0,
				"insufficient_data": 1,
			},
			"session_count": 0,
			"gate_mode":     "",
		}
	}

	points := service.BuildHistoryPoints(summaries)
	portfolioValues := service.PortfolioValues(points)
	dailyReturns := service.SessionReturns(points)

	var snap map[string]float64
	if len(dailyReturns) >= risk.MinObservationsForVaR {
		computed := risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
		snap = map[string]float64{
			"var_95":           computed.VaR95,
			"var_99":           computed.VaR99,
			"cvar_95":          computed.CVaR95,
			"max_drawdown_pct": computed.MaxDrawdownPct,
			"data_points":      float64(len(dailyReturns)),
		}
	} else if len(dailyReturns) == 0 {
		snap = map[string]float64{
			"var_95":            0,
			"var_99":            0,
			"cvar_95":           0,
			"max_drawdown_pct":  0,
			"data_points":       0,
			"insufficient_data": 1,
		}
	} else {
		// P0-1 (SSOT Phase 0): fewer than 252 observations — never surface a
		// provisional VaR value. The 30..251 range used to return the raw
		// percentile (e.g. -0.3214) alongside insufficient_data=1, so a
		// consumer that only checked the data-availability flag still painted
		// "-32.1%" while risk-exposure correctly showed "觀察期中". Zero the
		// VaR/CVaR fields and keep the drawdown (a 1..251-observation
		// historical fact, not a sample-size-gated estimate).
		dd := risk.CalculateMaxDrawdown(portfolioValues)
		snap = map[string]float64{
			"var_95":            0,
			"var_99":            0,
			"cvar_95":           0,
			"max_drawdown_pct":  dd,
			"data_points":       float64(len(dailyReturns)),
			"insufficient_data": 1,
		}
	}

	gateMode := ""
	if h.RiskGate != nil {
		gateMode = string(h.RiskGate.Mode())
	}

	resp := map[string]any{
		"risk_snapshot": snap,
		"session_count": len(portfolioValues),
		"gate_mode":     gateMode,
	}
	if len(dailyReturns) < risk.MinObservationsForVaR {
		resp["var_gate"] = "light" // <252 obs — provisional VaR estimate
	}
	// P1-3: label the L-cold history source so consumers can show a degraded
	// badge when the SSoT backend (PG) was unavailable and JSONL served.
	if d, ok := store.(interface{ Degraded() bool }); ok {
		resp["degraded"] = d.Degraded()
	}
	if s, ok := store.(interface{ SourceBackend() string }); ok {
		resp["source"] = s.SourceBackend()
	}
	return http.StatusOK, resp
}

// summariesStore resolves the store used for L-cold summary reads: the
// injected backend-aware store when present, the JSONL ledger otherwise.
func (h *Handlers) summariesStore() ledger.OutcomeStore {
	if h.store != nil {
		return h.store
	}
	return ledger.NewStore(h.LedgerDir)
}

// CorrelationMatrixResponse is the response for GET /api/dashboard/correlation-matrix.
type CorrelationMatrixResponse struct {
	Symbols []string    `json:"symbols"`
	Labels  []string    `json:"labels"`
	Matrix  [][]float64 `json:"matrix"`
}

// HandleCorrelationMatrix returns the industry correlation matrix.
func (h *Handlers) HandleCorrelationMatrix(r *http.Request) (int, any) {
	cm := h.correlationMatrix
	if cm == nil {
		cm = industry.DefaultCorrelationMatrix()
	}

	allCorrs := cm.GetAllCorrelations()

	symbols := make([]string, 0, len(allCorrs))
	for k := range allCorrs {
		symbols = append(symbols, k)
	}
	sort.Strings(symbols)

	n := len(symbols)

	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
		matrix[i][i] = 1.0
	}

	for i := range n {
		for j := i + 1; j < n; j++ {
			corr, ok := cm.GetCorrelation(symbols[i], symbols[j])
			if ok {
				matrix[i][j] = corr
				matrix[j][i] = corr
			}
		}
	}

	labels := make([]string, n)
	for i, s := range symbols {
		labels[i] = h.industryLabel(s)
	}

	return http.StatusOK, CorrelationMatrixResponse{
		Symbols: symbols,
		Labels:  labels,
		Matrix:  matrix,
	}
}

// WithRiskGate sets an optional RiskGate for serving calibration reports.
func (h *Handlers) WithRiskGate(rg *risk.RiskGate) *Handlers {
	h.RiskGate = rg
	return h
}

// HandleRiskCalibration serves the latest risk gate calibration report.
func (h *Handlers) HandleRiskCalibration(r *http.Request) (int, any) {
	if h.RiskGate == nil {
		return serviceUnavailable("risk_gate_not_injected", "")
	}
	report := h.RiskGate.LastCalibrationReport()
	if report == nil {
		return http.StatusOK, map[string]any{
			"status":    "not_available",
			"message":   "no calibration report available yet",
			"generated": time.Now().Format(time.RFC3339),
		}
	}
	return http.StatusOK, map[string]any{
		"report":    report,
		"generated": time.Now().Format(time.RFC3339),
	}
}

// HandleRiskCommentary returns the latest risk gate decision commentary.
//
// P0-2 (SSOT Phase 0): RiskGate decisions live only in memory, so after a
// service restart LastDecision is empty and the commentary surface goes blank
// even though the latest session's commentary was already persisted in PG
// session_summaries.risk_commentary. When the in-memory gate has no decision
// yet, fall back to the newest persisted summary commentary (pure read-side
// change — the write path is untouched).
func (h *Handlers) HandleRiskCommentary(r *http.Request) (int, any) {
	if h.RiskGate == nil {
		return serviceUnavailable("risk_gate_not_injected", "")
	}
	dec := h.RiskGate.LastDecision()
	if !dec.Recorded.IsZero() {
		return http.StatusOK, map[string]any{
			"phase":   string(dec.Phase),
			"verdict": string(dec.Verdict),
			"reason":  dec.Reason,
			// SSOT Phase 2 field-contract --strict：risk.js 讀 reason_zh；
			// 欄位就位先行（暫與 reason 同值），值的中文化屬計劃 A-1 後續。
			"reason_zh":             dec.Reason,
			"action_type":           string(dec.Action.Type),
			"action_description":    dec.Action.Description,
			"mode":                  dec.Mode,
			"symbol":                dec.Symbol,
			"recorded_at":           dec.Recorded.Format(time.RFC3339),
			"confidence_commentary": dec.ConfidenceCommentary,
			"source":                "risk_gate_memory",
			"generated":             true,
		}
	}

	// Fallback: read the latest session summary that carries a persisted
	// risk_commentary from the backend-aware store (PG-first on production).
	if h.store != nil {
		if summaries, err := h.store.LoadSessionSummaries(); err == nil {
			var latest *domain.SessionSummary
			for i := range summaries {
				s := &summaries[i]
				if strings.TrimSpace(s.RiskCommentary) == "" {
					continue
				}
				if latest == nil || s.SessionID > latest.SessionID {
					latest = s
				}
			}
			if latest != nil {
				recorded := latest.RecordedAt
				if recorded.IsZero() {
					recorded = domain.SessionDateFromID(latest.SessionID)
				}
				commentary := strings.TrimSpace(latest.RiskCommentary)
				return http.StatusOK, map[string]any{
					"phase":                 "session_summary",
					"verdict":               "UNKNOWN",
					"reason":                commentary,
					"reason_zh":             commentary,
					"action_type":           "none",
					"action_description":    "RiskGate 重啟後尚無新決策，此評語來自最近一次已持久化之 session_summary（risk_commentary）",
					"mode":                  "unknown",
					"symbol":                "",
					"recorded_at":           recorded.Format(time.RFC3339),
					"confidence_commentary": commentary,
					"source":                "session_summaries_pg",
					"session_id":            latest.SessionID,
					"generated":             true,
				}
			}
		} else {
			logging.Warn("risk_handler", "commentary_fallback_summaries_failed", "err", err.Error())
		}
	}

	return http.StatusOK, map[string]any{
		"status":    "not_available",
		"message":   "no risk decision recorded yet",
		"generated": false,
	}
}

var legacyIndustryLabels = map[string]string{
	"semiconductor":   "半導體",
	"ai_supply_chain": "AI 供應鏈",
	"robotics":        "機器人",
	"foundry":         "晶圓代工",
	"electronics":     "電子零組件",
	"shipping":        "航運",
	"financials":      "金融",
	"energy":          "能源",
	"industrial":      "工業",
	"consumer":        "消費",
	"cooling":         "散熱",
	"server_assembly": "伺服器組裝",
	"mining":          "礦業/貴金屬",
}

func (h *Handlers) industryLabel(id string) string {
	if h.classificationTree != nil {
		if seg, ok := h.classificationTree.GetSegment(id); ok {
			return seg.Name
		}
		if l, ok := legacyIndustryLabels[id]; ok {
			return l
		}
	}
	return id
}
