package autobacktest

import (
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

const (
	shortTermDays = 5
	longTermDays  = 20
)

type Comparison struct {
	ShortTermAvg float64
	LongTermAvg  float64
	Delta        float64
	DeltaPct     float64
}

type Comparator struct {
	store ledger.FullStore
}

func NewComparator(ledgerDir string) *Comparator {
	return &Comparator{store: ledger.NewStore(ledgerDir).(ledger.FullStore)}
}

func (c *Comparator) ComparePortfolio() (Comparison, error) {
	summaries, err := c.store.LoadSessionSummaries()
	if err != nil {
		return Comparison{}, err
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].RecordedAt.Before(summaries[j].RecordedAt)
	})

	recent := summaries
	if len(recent) > longTermDays {
		recent = recent[len(recent)-longTermDays:]
	}

	var shortSum, longSum float64
	for i, s := range recent {
		pv := s.PortfolioValue
		if pv == 0 {
			pv = s.EndingCash
		}
		if i >= len(recent)-shortTermDays {
			shortSum += pv
		}
		longSum += pv
	}

	shortAvg := 0.0
	longAvg := 0.0
	if shortTermDays > 0 {
		shortAvg = shortSum / float64(shortTermDays)
	}
	if longTermDays > 0 {
		longAvg = longSum / float64(longTermDays)
	}

	delta := shortAvg - longAvg
	deltaPct := 0.0
	if longAvg != 0 {
		deltaPct = delta / longAvg
	}

	return Comparison{
		ShortTermAvg: shortAvg,
		LongTermAvg:  longAvg,
		Delta:        delta,
		DeltaPct:     deltaPct,
	}, nil
}

func (c *Comparator) CompareSharpe() (Comparison, error) {
	scorecards, _, err := c.store.LoadAllSessionScorecards()
	if err != nil {
		return Comparison{}, err
	}

	sort.Slice(scorecards, func(i, j int) bool {
		return scorecards[i].LastUpdatedAt.Before(scorecards[j].LastUpdatedAt)
	})

	recent := scorecards
	if len(recent) > longTermDays {
		recent = recent[len(recent)-longTermDays:]
	}

	var shortSum, longSum float64
	for i, sc := range recent {
		if i >= len(recent)-shortTermDays {
			shortSum += sc.SharpeLike
		}
		longSum += sc.SharpeLike
	}

	shortAvg := 0.0
	longAvg := 0.0
	if shortTermDays > 0 {
		shortAvg = shortSum / float64(shortTermDays)
	}
	if longTermDays > 0 {
		longAvg = longSum / float64(longTermDays)
	}

	delta := shortAvg - longAvg
	deltaPct := 0.0
	if longAvg != 0 {
		deltaPct = delta / longAvg
	}

	return Comparison{
		ShortTermAvg: shortAvg,
		LongTermAvg:  longAvg,
		Delta:        delta,
		DeltaPct:     deltaPct,
	}, nil
}

func (c *Comparator) RecentRegimes() ([]domain.Regime, error) {
	summaries, err := c.store.LoadSessionSummaries()
	if err != nil {
		return nil, err
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].RecordedAt.Before(summaries[j].RecordedAt)
	})

	recent := summaries
	if len(recent) > longTermDays {
		recent = recent[len(recent)-longTermDays:]
	}

	regimes := make([]domain.Regime, len(recent))
	for i, s := range recent {
		regimes[i] = s.Regime
	}
	return regimes, nil
}
