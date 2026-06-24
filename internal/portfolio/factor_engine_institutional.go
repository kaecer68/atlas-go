package portfolio

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CalculateInstitutionalSentimentScore computes the institutional sentiment factor
// as a weighted combination of foreign / domestic / margin-balance / retail
// sub-scores from the macro FactorBridge input.
//
// Weights come from fe.params.Factor.InstitutionalSentimentWeights (4 keys).
// Default weights (per AGENTS.md §2.2): foreign=0.50, domestic=0.30,
// margin=0.20. Retail weight is currently a placeholder (0.0 by default).
//
// Score is clamped to [-1, 1].
func (fe *FactorEngine) CalculateInstitutionalSentimentScore(input FactorBridgeInput) domain.FactorScoreItem {
	weights := fe.params.Factor.InstitutionalSentimentWeights
	foreignWeight := weights["foreign"]
	domesticWeight := weights["domestic"]
	marginWeight := weights["margin"]
	retailWeight := weights["retail"]
	score := foreignWeight*input.ForeignFlowScore +
		domesticWeight*input.DomesticFlowScore +
		marginWeight*input.MarginBalanceScore +
		retailWeight*input.RetailSentimentScore
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: fmt.Sprintf("%.2f*ForeignFlowScore + %.2f*DomesticFlowScore + %.2f*MarginBalanceScore + %.2f*RetailSentimentScore", foreignWeight, domesticWeight, marginWeight, retailWeight),
		RawInputs: map[string]float64{
			"foreign_score":   input.ForeignFlowScore,
			"domestic_score":  input.DomesticFlowScore,
			"margin_score":    input.MarginBalanceScore,
			"retail_score":    input.RetailSentimentScore,
			"foreign_weight":  foreignWeight,
			"domestic_weight": domesticWeight,
			"margin_weight":   marginWeight,
			"retail_weight":   retailWeight,
		},
	}
}
