package risk

// PortfolioRiskAssessment evaluates the risk contribution from portfolio-level
// concentration, sector exposure, and factor exposure.
type PortfolioRiskAssessment struct {
	ConcentrationScore float64            `json:"concentration_score"`
	SectorExposure     map[string]float64 `json:"sector_exposure"`
	FactorExposure     map[string]float64 `json:"factor_exposure"`
	TotalRiskScore     float64            `json:"total_risk_score"`
}

// PortfolioRiskProvider defines the interface for assessing portfolio-level risk.
type PortfolioRiskProvider interface {
	Assess() *PortfolioRiskAssessment
}
