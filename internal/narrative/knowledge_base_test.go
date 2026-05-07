package narrative

import (
	"testing"
	"time"
)

func TestDetectSeasonalEvent_WithExpectationGap(t *testing.T) {
	now := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	data := MarketNarrativeData{}

	event := detectSeasonalEventAt(now, data)
	if event == nil {
		t.Fatal("expected spring_festival_season event in January")
	}

	if event.Theme != "spring_festival_season" {
		t.Errorf("expected spring_festival_season, got %s", event.Theme)
	}
}

func TestDetectSeasonalEvent_ExpectationGapAdjustment(t *testing.T) {
	now := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	data := MarketNarrativeData{
		SpringFestivalExpectation: &SeasonalExpectation{
			HistoricalAvgReturn: 0.05,
			CurrentReturn:       0.02,
			ExpectationGap:      0.03,
			AlreadyPricedIn:     false,
			SurprisePotential:   0.6,
			Confidence:          0.7,
		},
	}

	event := detectSeasonalEventAt(now, data)
	if event == nil {
		t.Fatal("expected seasonal event")
	}

	if event.Theme != "spring_festival_season" {
		t.Errorf("expected theme spring_festival_season, got %s", event.Theme)
	}

	if event.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %f", event.Confidence)
	}
}

func TestDetectSeasonalEvent_AlreadyPricedIn(t *testing.T) {
	now := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	data := MarketNarrativeData{
		SpringFestivalExpectation: &SeasonalExpectation{
			HistoricalAvgReturn: 0.03,
			CurrentReturn:       0.05,
			ExpectationGap:      -0.02,
			AlreadyPricedIn:     true,
			SurprisePotential:   0.0,
			Confidence:          0.7,
		},
	}

	event := detectSeasonalEventAt(now, data)
	if event == nil {
		t.Fatal("expected seasonal event")
	}

	if event.Confidence >= 0.65 {
		t.Errorf("expected suppressed confidence for priced-in event, got %f", event.Confidence)
	}
}

func TestDetectSeasonalEvent_PostElectionRelief(t *testing.T) {
	now := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	data := MarketNarrativeData{}
	event := detectSeasonalEventAt(now, data)

	if event == nil {
		t.Fatal("expected event")
	}

	if event.Theme != "post_election_relief" {
		t.Errorf("expected post_election_relief, got %s", event.Theme)
	}
}

func TestDetectSeasonalEvent_NoEventInApril(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	data := MarketNarrativeData{}
	event := detectSeasonalEventAt(now, data)

	if event != nil {
		t.Errorf("expected no event in April, got %s", event.Theme)
	}
}
