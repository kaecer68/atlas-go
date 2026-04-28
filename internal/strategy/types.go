package strategy

import "github.com/kaecer68/atlas-go/internal/domain"

type RiskAppetite int

const (
	RiskAppetiteConservative RiskAppetite = 1
	RiskAppetiteBalanced     RiskAppetite = 2
	RiskAppetiteAggressive   RiskAppetite = 3
)

type Strategy struct {
	ID           string
	Name         string
	Description  string
	Enabled      bool
	Agents       []string
	Filters      []string
	Priority     int
	RiskAppetite RiskAppetite
	RegimePrefs  []domain.Regime
}

type StrategyComparison struct {
	Date           string
	StrategyID     string
	DailyReturn    float64
	SharpeRatio    float64
	MaxDrawdown    float64
	WinRate        float64
	Outperformance float64
}

type ComparisonResult struct {
	Date           string
	Comparisons    []*StrategyComparison
	BestByReturn   string
	BestBySharpe   string
	BestByDrawdown string
}
