package domain

type ExecutionInput struct {
	Regime               Regime
	RawRecommendations   []Recommendation
	FinalRecommendations []Recommendation
	GuardOutcomes        []GuardOutcome
	DeterminedBy         string
}
