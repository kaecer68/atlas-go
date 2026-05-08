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

// Side represents a trade direction.
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
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

// FactorScoreItem holds a single factor's score with its computation metadata.
type FactorScoreItem struct {
	Score      float64            `json:"score"`
	Weight     float64            `json:"weight,omitempty"`
	Formula    string             `json:"formula"`
	RawInputs  map[string]float64 `json:"raw_inputs"`
	IsFallback bool               `json:"is_fallback"`
}

// FactorScoreBreakdown contains the six-factor score decomposition.
type FactorScoreBreakdown struct {
	Momentum               FactorScoreItem `json:"momentum"`
	Value                  FactorScoreItem `json:"value"`
	Quality                FactorScoreItem `json:"quality"`
	Agent                  FactorScoreItem `json:"agent"`
	InstitutionalSentiment FactorScoreItem `json:"institutional_sentiment"`
	Liquidity              FactorScoreItem `json:"liquidity"`
	Total                  FactorScoreItem `json:"total"`
}

// FactorScores holds aggregate factor scores with optional breakdown.
type FactorScores struct {
	Momentum               float64               `json:"momentum"`
	Value                  float64               `json:"value"`
	Quality                float64               `json:"quality"`
	Agent                  float64               `json:"agent"`
	InstitutionalSentiment float64               `json:"institutional_sentiment"`
	Liquidity              float64               `json:"liquidity"`
	Total                  float64               `json:"total"`
	Breakdown              *FactorScoreBreakdown `json:"breakdown,omitempty"`
}

// ConvictionStep records one step in the conviction calculation chain.
type ConvictionStep struct {
	Rule   string `json:"rule"`
	Delta  int    `json:"delta"`
	Reason string `json:"reason"`
}

// ConvictionBreakdown holds the conviction computation trace.
type ConvictionBreakdown struct {
	Base  int              `json:"base"`
	Floor int              `json:"floor"`
	Final int              `json:"final"`
	Steps []ConvictionStep `json:"steps"`
}

// AgentLayer classifies an agent's position in the decision hierarchy.
type AgentLayer string

const (
	LayerContext       AgentLayer = "context"
	LayerMacro         AgentLayer = "macro"
	LayerSector        AgentLayer = "sector"
	LayerStyle         AgentLayer = "style"
	LayerSuperinvestor AgentLayer = "superinvestor"
	LayerControl       AgentLayer = "control"
)

// FlexTime supports flexible time parsing from JSON.
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
	return []byte(fmt.Sprintf("%q", ft.Format(time.RFC3339Nano))), nil
}
