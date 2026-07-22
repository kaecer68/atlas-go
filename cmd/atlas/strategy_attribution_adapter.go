// Package main — attributionAdapter bridges *apistrategies.FeedbackStore
// to autobacktest.AttributionWriter. The fields are identical so this
// is a pure type-assertion shim; kept here (rather than in
// monitoring/api/strategies) to preserve the import direction that
// monitoring → autobacktest.
package main

import (
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/autobacktest"
)

type strategyAttributionAdapter struct {
	fb *apistrategies.FeedbackStore
}

func (a strategyAttributionAdapter) Write(r autobacktest.AttributionRecord) error {
	return a.fb.Write(apistrategies.Record{
		StrategyID: r.StrategyID,
		TotalTests: r.TotalTests,
		TotalHits:  r.TotalHits,
		HitRate:    r.HitRate,
		Status:     r.Status,
	})
}
