package risk

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type Handlers struct {
	LedgerDir         string
	correlationMatrix *industry.CorrelationMatrix
	RiskGate          *risk.RiskGate // optional, set via WithRiskGate()
}

func NewHandlers(ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir}
}

// WithCorrelationMatrix sets an optional correlation matrix provider.
// When nil, HandleCorrelationMatrix falls back to DefaultCorrelationMatrix().
func (h *Handlers) WithCorrelationMatrix(cm *industry.CorrelationMatrix) *Handlers {
	h.correlationMatrix = cm
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
	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		logging.Warn("risk_handler", "sessions_dir_unreadable", logging.Err(err))
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

	type sessionEntry struct {
		name  string
		value float64
	}
	sessions := make([]sessionEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			logging.Warn("risk_handler", "corrupted_summary_skipped", logging.Err(err))
			continue
		}
		sessions = append(sessions, sessionEntry{name: entry.Name(), value: summary.PortfolioValue})
	}

	slices.SortFunc(sessions, func(a, b sessionEntry) int {
		return strings.Compare(a.name, b.name)
	})

	portfolioValues := make([]float64, len(sessions))
	for i, s := range sessions {
		portfolioValues[i] = s.value
	}

	dailyReturns := make([]float64, 0, max(0, len(portfolioValues)-1))
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

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
	} else {
		snap = map[string]float64{
			"var_95":            0,
			"var_99":            0,
			"cvar_95":           0,
			"max_drawdown_pct":  0,
			"data_points":       float64(len(dailyReturns)),
			"insufficient_data": 1,
		}
	}

	gateMode := ""
	if h.RiskGate != nil {
		gateMode = string(h.RiskGate.Mode())
	}

	return http.StatusOK, map[string]any{
		"risk_snapshot": snap,
		"session_count": len(portfolioValues),
		"gate_mode":     gateMode,
	}
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

	for i := 0; i < n; i++ {
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
		labels[i] = industryLabel(s)
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
func (h *Handlers) HandleRiskCommentary(r *http.Request) (int, any) {
	if h.RiskGate == nil {
		return serviceUnavailable("risk_gate_not_injected", "")
	}
	dec := h.RiskGate.LastDecision()
	if dec.Recorded.IsZero() {
		return http.StatusOK, map[string]any{
			"status":    "not_available",
			"message":   "no risk decision recorded yet",
			"generated": false,
		}
	}
	return http.StatusOK, map[string]any{
		"phase":                 string(dec.Phase),
		"verdict":               string(dec.Verdict),
		"reason":                dec.Reason,
		"action_type":           string(dec.Action.Type),
		"action_description":    dec.Action.Description,
		"mode":                  dec.Mode,
		"symbol":                dec.Symbol,
		"recorded_at":           dec.Recorded.Format(time.RFC3339),
		"confidence_commentary": dec.ConfidenceCommentary,
		"generated":             true,
	}
}

func industryLabel(id string) string {
	m := map[string]string{
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
	if l, ok := m[id]; ok {
		return l
	}
	return id
}
