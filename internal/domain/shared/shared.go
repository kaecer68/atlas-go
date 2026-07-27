package shared

import (
	"fmt"
	"time"
)

// Regime represents the market regime classification.
type Regime string

const (
	RegimeRiskOn  Regime = "RISK_ON"
	RegimeRiskOff Regime = "RISK_OFF"
	RegimeNeutral Regime = "NEUTRAL"
)

// MarketPeriod represents the seven-phase market cycle defined in
// docs/ATLAS_METHODOLOGY.md §3. This is the methodology-level classification
// that sits above the three-state domain.Regime (RISK_ON/RISK_OFF/NEUTRAL).
// Use PeriodToRegime() to map a MarketPeriod to the compatible Regime.
type MarketPeriod string

const (
	PeriodDownturn       MarketPeriod = "downturn"        // 低迷
	PeriodTurnaroundUp   MarketPeriod = "turnaround_up"   // 轉折開高
	PeriodBull           MarketPeriod = "bull"            // 上升／多頭
	PeriodPlateau        MarketPeriod = "plateau"         // 高原
	PeriodConsolidation  MarketPeriod = "consolidation"   // 盤整
	PeriodTurnaroundDown MarketPeriod = "turnaround_down" // 轉折下壓
	PeriodBlackSwan      MarketPeriod = "black_swan"      // 黑天鵝
)

// Side represents a trade direction.
type Side string

const (
	SideBuy    Side = "BUY"
	SideSell   Side = "SELL"
	SideReduce Side = "REDUCE"
)

// Quote represents a market data snapshot for a single symbol.
type Quote struct {
	Symbol     string    `json:"symbol"`
	Last       float64   `json:"last"`
	Open       float64   `json:"open"`
	High       float64   `json:"high"`
	Low        float64   `json:"low"`
	Volume     int64     `json:"volume"`
	Market     string    `json:"market"`
	AsOf       time.Time `json:"as_of"`
	IsTradable bool      `json:"is_tradable"`
	Source     string    `json:"source"`
}

type FactorScoreItem struct {
	Score      float64            `json:"score"`
	Weight     float64            `json:"weight,omitempty"`
	Formula    string             `json:"formula"`
	RawInputs  map[string]float64 `json:"raw_inputs"`
	IsFallback bool               `json:"is_fallback"`
}

type FactorScoreBreakdown struct {
	Momentum               FactorScoreItem `json:"momentum"`
	Value                  FactorScoreItem `json:"value"`
	Quality                FactorScoreItem `json:"quality"`
	Agent                  FactorScoreItem `json:"agent"`
	InstitutionalSentiment FactorScoreItem `json:"institutional_sentiment"`
	Liquidity              FactorScoreItem `json:"liquidity"`
	Narrative              FactorScoreItem `json:"narrative,omitempty"`
	IndustryCycle          FactorScoreItem `json:"industry_cycle,omitempty"`
	PreciousMetals         FactorScoreItem `json:"precious_metals,omitempty"`
	ETF                    FactorScoreItem `json:"etf,omitempty"`
	Linkage                FactorScoreItem `json:"linkage,omitempty"`
	TSMC                   FactorScoreItem `json:"tsmc,omitempty"`
	Total                  FactorScoreItem `json:"total"`
}

type FactorScores struct {
	Momentum               float64               `json:"momentum"`
	Value                  float64               `json:"value"`
	Quality                float64               `json:"quality"`
	Agent                  float64               `json:"agent"`
	InstitutionalSentiment float64               `json:"institutional_sentiment"`
	Liquidity              float64               `json:"liquidity"`
	Narrative              float64               `json:"narrative,omitempty"`
	IndustryCycle          float64               `json:"industry_cycle,omitempty"`
	PreciousMetals         float64               `json:"precious_metals,omitempty"`
	ETF                    float64               `json:"etf,omitempty"`
	Linkage                float64               `json:"linkage,omitempty"`
	TSMC                   float64               `json:"tsmc,omitempty"`
	Total                  float64               `json:"total"`
	Breakdown              *FactorScoreBreakdown `json:"breakdown,omitempty"`
}

type NarrativeFactorScore struct {
	Score      float64  `json:"score"`
	Theme      string   `json:"theme,omitempty"`
	HitRate    float64  `json:"hit_rate,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	EventIDs   []string `json:"event_ids,omitempty"`
}

type IndustryCycleFactorScore struct {
	Score      float64 `json:"score"`
	Phase      string  `json:"phase,omitempty"`
	PhaseScore float64 `json:"phase_score,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	IndustryID string  `json:"industry_id,omitempty"`
}

type LinkageFactorScore struct {
	Score              float64 `json:"score"`
	SystemicImportance float64 `json:"systemic_importance,omitempty"`
	ShockPropagation   float64 `json:"shock_propagation_speed,omitempty"`
	AvgCorrelation     float64 `json:"avg_correlation,omitempty"`
	IndustryID         string  `json:"industry_id,omitempty"`
}

type ConvictionStep struct {
	Rule        string   `json:"rule"`
	Delta       int      `json:"delta"`
	Reason      string   `json:"reason"`
	Source      string   `json:"source,omitempty"`
	ParamRef    string   `json:"param_ref,omitempty"`
	ParamValue  string   `json:"param_value,omitempty"`
	Sensitivity *float64 `json:"sensitivity,omitempty"`
}

type ConvictionBreakdown struct {
	Base  int              `json:"base"`
	Floor int              `json:"floor"`
	Final int              `json:"final"`
	Steps []ConvictionStep `json:"steps"`
}

type ParameterSnapshot struct {
	FactorWeights       map[string]float64 `json:"factor_weights,omitempty"`
	NarrativeHitRates   map[string]float64 `json:"narrative_hit_rates,omitempty"`
	IndustryPhaseScores map[string]float64 `json:"industry_phase_scores,omitempty"`
	ConfigVersion       string             `json:"config_version,omitempty"`
	CapturedAt          time.Time          `json:"captured_at"`
}

type AgentLayer string

const (
	LayerContext       AgentLayer = "context"
	LayerMacro         AgentLayer = "macro"
	LayerSector        AgentLayer = "sector"
	LayerStyle         AgentLayer = "style"
	LayerSuperinvestor AgentLayer = "superinvestor"
	LayerControl       AgentLayer = "control"
)

type FlexTime struct {
	time.Time
}

func (ft *FlexTime) UnmarshalJSON(data []byte) error {
	str := string(data)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	if str == "" || str == "null" {
		ft.Time = time.Time{}
		return nil
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05.999",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, str); err == nil {
			ft.Time = t
			return nil
		}
	}

	return fmt.Errorf("cannot parse time %q", str)
}

func (ft FlexTime) MarshalJSON() ([]byte, error) {
	if ft.IsZero() {
		return []byte("null"), nil
	}
	return fmt.Appendf(nil, "%q", ft.Format(time.RFC3339Nano)), nil
}
