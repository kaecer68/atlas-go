package narrative

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type MacroRiskLevel int

const (
	MacroRiskGreen MacroRiskLevel = iota
	MacroRiskYellow
	MacroRiskOrange
	MacroRiskRed
)

func (l MacroRiskLevel) String() string {
	switch l {
	case MacroRiskGreen:
		return "green"
	case MacroRiskYellow:
		return "yellow"
	case MacroRiskOrange:
		return "orange"
	case MacroRiskRed:
		return "red"
	default:
		return "unknown"
	}
}

type MacroRiskAssessment struct {
	Level              MacroRiskLevel `json:"level"`
	ForeignOutflowProb float64        `json:"foreign_outflow_probability"`
	PrimaryFlow        string         `json:"primary_flow"`
	FavoredSectors     []string       `json:"favored_sectors,omitempty"`
	AvoidedSectors     []string       `json:"avoided_sectors,omitempty"`
	StructuralOverride bool           `json:"structural_override"`
	Confidence         float64        `json:"confidence"`
	Rationale          string         `json:"rationale"`
	Timestamp          time.Time      `json:"timestamp"`
}

type MacroRiskAssessmentEngine struct {
	cfg config.MacroRiskConfig
}

func NewMacroRiskAssessmentEngine() *MacroRiskAssessmentEngine {
	return NewMacroRiskAssessmentEngineWithConfig(config.GetEngineConfig().MacroRisk)
}

func NewMacroRiskAssessmentEngineWithConfig(cfg config.MacroRiskConfig) *MacroRiskAssessmentEngine {
	return &MacroRiskAssessmentEngine{cfg: cfg}
}

func (e *MacroRiskAssessmentEngine) Assess(data MacroDataSnapshot) *MacroRiskAssessment {
	assessment := &MacroRiskAssessment{
		Timestamp: time.Now(),
	}

	riskFactors := e.evaluateRiskFactors(data)

	assessment.Level = e.determineRiskLevel(riskFactors)
	assessment.ForeignOutflowProb = e.calculateOutflowProbability(riskFactors)
	assessment.PrimaryFlow = e.determinePrimaryFlow(riskFactors)
	assessment.FavoredSectors, assessment.AvoidedSectors = e.determineSectorRotation(riskFactors)
	assessment.Rationale = e.buildRationale(riskFactors, assessment.Level)
	assessment.Confidence = e.calculateConfidence(riskFactors)

	return assessment
}

type RiskFactor struct {
	Type     string
	Severity float64
	Details  string
}

func (e *MacroRiskAssessmentEngine) evaluateRiskFactors(data MacroDataSnapshot) []RiskFactor {
	var factors []RiskFactor

	// Carry Trade Unwind: USD/JPY below threshold indicates JPY strengthening
	// This is the key signal for carry trade unwind (e.g., Aug 5, 2024 event)
	if data.JPY.Value > 0 && data.JPY.Value < e.cfg.CarryTradeUnwindThreshold {
		severity := 1.0 - (data.JPY.Value / e.cfg.CarryTradeUnwindThreshold)
		if severity > 1.0 {
			severity = 1.0
		}
		factors = append(factors, RiskFactor{
			Type:     "carry_trade",
			Severity: severity,
			Details:  fmt.Sprintf("USD/JPY at %.2f, below threshold %.2f", data.JPY.Value, e.cfg.CarryTradeUnwindThreshold),
		})
	}

	if data.VIX.Value > e.cfg.VIXThreshold {
		severity := (data.VIX.Value - e.cfg.VIXThreshold) / 20.0
		if severity > 1.0 {
			severity = 1.0
		}
		factors = append(factors, RiskFactor{
			Type:     "market_stress",
			Severity: severity,
			Details:  fmt.Sprintf("VIX at %.2f, above threshold %.2f", data.VIX.Value, e.cfg.VIXThreshold),
		})
	}

	if data.US10Y.Value > e.cfg.US10YThreshold {
		severity := (data.US10Y.Value - e.cfg.US10YThreshold) / 2.0
		if severity > 1.0 {
			severity = 1.0
		}
		factors = append(factors, RiskFactor{
			Type:     "rates",
			Severity: severity,
			Details:  fmt.Sprintf("US 10Y at %.2f, above threshold %.2f", data.US10Y.Value, e.cfg.US10YThreshold),
		})
	}

	if data.Gold.ChangePct > e.cfg.GoldSurgeThresholdPct && data.VIX.Value > e.cfg.VIXThreshold-5 {
		factors = append(factors, RiskFactor{
			Type:     "geopolitical",
			Severity: 0.8,
			Details:  fmt.Sprintf("Gold +%.2f%% with VIX %.2f, classic risk-off pattern", data.Gold.ChangePct, data.VIX.Value),
		})
	}

	if data.Oil.ChangePct > e.cfg.OilShockThresholdPct && data.Gold.ChangePct < e.cfg.GoldSurgeThresholdPct-3 {
		factors = append(factors, RiskFactor{
			Type:     "energy_crisis",
			Severity: 0.7,
			Details:  fmt.Sprintf("Oil +%.2f%% but Gold only +%.2f%%, energy crisis pattern", data.Oil.ChangePct, data.Gold.ChangePct),
		})
	}

	if data.USD_TWD.ChangePct > e.cfg.TWDStressThresholdPct {
		factors = append(factors, RiskFactor{
			Type:     "taiwan_stress",
			Severity: 0.6,
			Details:  fmt.Sprintf("USD/TWD +%.2f%%, Taiwan stress signal", data.USD_TWD.ChangePct),
		})
	}

	return factors
}

func (e *MacroRiskAssessmentEngine) determineRiskLevel(factors []RiskFactor) MacroRiskLevel {
	if len(factors) == 0 {
		return MacroRiskGreen
	}

	highSeverityCount := 0
	maxSeverity := 0.0
	for _, f := range factors {
		if f.Severity > 0.7 {
			highSeverityCount++
		}
		if f.Severity > maxSeverity {
			maxSeverity = f.Severity
		}
	}

	switch {
	case highSeverityCount >= 2 || maxSeverity > 0.9:
		return MacroRiskRed
	case highSeverityCount == 1 || len(factors) >= 2:
		return MacroRiskOrange
	case len(factors) == 1:
		return MacroRiskYellow
	default:
		return MacroRiskGreen
	}
}

func (e *MacroRiskAssessmentEngine) calculateOutflowProbability(factors []RiskFactor) float64 {
	if len(factors) == 0 {
		return e.cfg.OutflowProbBase / 3.5 // Base probability when no factors
	}

	totalSeverity := 0.0
	for _, f := range factors {
		switch f.Type {
		case "carry_trade":
			totalSeverity += f.Severity * 1.5
		case "geopolitical":
			totalSeverity += f.Severity * 1.3
		case "taiwan_stress":
			totalSeverity += f.Severity * 1.2
		default:
			totalSeverity += f.Severity
		}
	}

	prob := (totalSeverity / float64(len(factors))) * 100
	if prob > e.cfg.OutflowProbMax {
		prob = e.cfg.OutflowProbMax
	}
	return prob
}

func (e *MacroRiskAssessmentEngine) determinePrimaryFlow(factors []RiskFactor) string {
	// Priority: energy_crisis > geopolitical > carry_trade > market_stress
	// Energy crisis takes precedence because it signals sector rotation opportunity
	// Geopolitical takes precedence over carry_trade because it's more systemic
	for _, f := range factors {
		if f.Type == "energy_crisis" {
			return "sector_rotation"
		}
	}

	for _, f := range factors {
		if f.Type == "geopolitical" {
			return "risk_off"
		}
	}

	for _, f := range factors {
		if f.Type == "carry_trade" {
			return "carry_trade_unwind"
		}
	}

	for _, f := range factors {
		if f.Type == "market_stress" {
			return "risk_off"
		}
	}

	return "mixed"
}

func (e *MacroRiskAssessmentEngine) determineSectorRotation(factors []RiskFactor) (favored, avoided []string) {
	// Priority: energy_crisis > geopolitical > carry_trade > market_stress
	for _, f := range factors {
		if f.Type == "energy_crisis" {
			favored = []string{"energy", "oil_services", "alternative_energy", "shipping"}
			avoided = []string{"high_valuation_tech", "rate_sensitive"}
			return
		}
	}

	for _, f := range factors {
		if f.Type == "geopolitical" {
			favored = []string{"gold", "defensive_financials", "high_dividend", "utilities"}
			avoided = []string{"ai_supply_chain", "small_cap", "emerging_market"}
			return
		}
	}

	for _, f := range factors {
		if f.Type == "carry_trade" {
			favored = []string{"cash", "short_term_bonds", "jpy"}
			avoided = []string{"all_equities", "tech", "financials"}
			return
		}
	}

	for _, f := range factors {
		if f.Type == "market_stress" {
			favored = []string{"gold", "defensive_financials", "high_dividend", "utilities"}
			avoided = []string{"ai_supply_chain", "small_cap", "emerging_market"}
			return
		}
	}

	favored = []string{"diversified"}
	avoided = []string{"concentrated"}
	return
}

func (e *MacroRiskAssessmentEngine) buildRationale(factors []RiskFactor, level MacroRiskLevel) string {
	rationale := fmt.Sprintf("Macro risk level: %s. ", level.String())

	if len(factors) == 0 {
		rationale += "No significant risk factors detected."
		return rationale
	}

	rationale += fmt.Sprintf("Detected %d risk factor(s): ", len(factors))
	for i, f := range factors {
		if i > 0 {
			rationale += "; "
		}
		rationale += fmt.Sprintf("%s (severity: %.2f): %s", f.Type, f.Severity, f.Details)
	}

	return rationale
}

func (e *MacroRiskAssessmentEngine) calculateConfidence(factors []RiskFactor) float64 {
	if len(factors) == 0 {
		return 0.9 // High confidence in no risk
	}

	// Confidence decreases with more factors (more complex = less certain)
	baseConfidence := 0.85
	penalty := float64(len(factors)-1) * 0.05
	if penalty > 0.2 {
		penalty = 0.2
	}
	return baseConfidence - penalty
}

type MacroDataSnapshot = marketdata.MacroDataSnapshot
type MacroDataPoint = marketdata.MacroDataPoint
