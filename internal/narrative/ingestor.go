package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// MacroIngestor fetches macro data, compares with previous snapshot, and emits events.
type MacroIngestor struct {
	provider    marketdata.MacroDataProvider
	snapshotDir string
}

// NewMacroIngestor creates an ingestor with a given provider and snapshot directory.
func NewMacroIngestor(provider marketdata.MacroDataProvider, snapshotDir string) *MacroIngestor {
	return &MacroIngestor{
		provider:    provider,
		snapshotDir: snapshotDir,
	}
}

func hitRateForTheme(theme string) float64 {
	for _, t := range DefaultTemplates() {
		if t.TriggerTheme == theme {
			return t.HistoricalHitRate
		}
	}
	return 0.0
}

// SnapshotDir returns the directory where snapshots are stored.
func (m *MacroIngestor) SnapshotDir() string {
	return m.snapshotDir
}

// Ingest fetches latest macro data, computes changes from previous snapshot, and returns events.
func (m *MacroIngestor) Ingest(ctx context.Context) ([]NarrativeEvent, marketdata.MacroDataSnapshot, error) {
	snap, err := m.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, snap, fmt.Errorf("fetch snapshot: %w", err)
	}

	prev, _ := m.loadLatestSnapshot()
	events := detectEventsFromSnapshot(snap, prev)

	if err := m.saveSnapshot(snap); err != nil {
		return events, snap, fmt.Errorf("save snapshot: %w", err)
	}
	return events, snap, nil
}

func (m *MacroIngestor) loadLatestSnapshot() (marketdata.MacroDataSnapshot, error) {
	path := filepath.Join(m.snapshotDir, "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, fmt.Errorf("read snapshot: %w", err)
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return marketdata.MacroDataSnapshot{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return snap, nil
}

func (m *MacroIngestor) saveSnapshot(snap marketdata.MacroDataSnapshot) error {
	if err := os.MkdirAll(m.snapshotDir, 0o755); err != nil {
		return fmt.Errorf("mkdir snapshot dir: %w", err)
	}
	path := filepath.Join(m.snapshotDir, "latest.json")
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	// Also save dated copy.
	dateStr := time.Now().UTC().Format("2006-01-02")
	datedPath := filepath.Join(m.snapshotDir, dateStr+".json")
	if err := os.WriteFile(datedPath, data, 0o644); err != nil {
		return fmt.Errorf("save dated snapshot: %w", err)
	}
	return nil
}

func detectEventsFromSnapshot(curr, prev marketdata.MacroDataSnapshot) []NarrativeEvent {
	var events []NarrativeEvent
	now := time.Now().UTC()

	if event := detectUSRatesEventFromSnapshot(curr.US10Y, prev.US10Y, now); event != nil {
		events = append(events, *event)
	}
	if event := detectJPYCarryUnwindEventFromSnapshot(curr.JPY, prev.JPY, curr.VIX, now); event != nil {
		events = append(events, *event)
	}
	if event := detectGeopoliticalRiskEventFromSnapshot(curr.Gold, curr.VIX, now); event != nil {
		events = append(events, *event)
	}
	if event := detectOilShockEventFromSnapshot(curr.Oil, now); event != nil {
		events = append(events, *event)
	}
	if event := detectRetailFrenzyEvent(curr); event != nil {
		events = append(events, *event)
	}
	if event := detectRetailFearEvent(curr); event != nil {
		events = append(events, *event)
	}
	// AI capex sentiment remains externally supplied for now.
	return events
}

func detectUSRatesEventFromSnapshot(curr, prev marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if curr.Symbol == "" {
		return nil
	}
	// ^TNX is stored as bps proxy in our provider.
	changeBps := curr.Value
	if prev.Symbol != "" {
		changeBps = curr.Value - prev.Value
	}
	if changeBps > 10 || curr.ChangePct > 1.5 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-us-rates-%d", now.UnixNano()),
			Theme:       "US_rates_up",
			Region:      "US",
			Sentiment:   -0.6,
			Confidence:  0.75,
			CapitalFlow: "flight_to_USD",
			TimeWindow:  "1_week",
			Timestamp:   now,
			SourceData: map[string]float64{
				"us10y_change_bps": changeBps,
				"us10y_level":      curr.Value,
			},
		}
	}
	return nil
}

func detectJPYCarryUnwindEventFromSnapshot(currJPY, prevJPY, currVIX marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if currJPY.Symbol == "" {
		return nil
	}
	jpyChange := currJPY.ChangePct
	if prevJPY.Symbol != "" && prevJPY.Value != 0 {
		jpyChange = (currJPY.Value - prevJPY.Value) / prevJPY.Value * 100
	}
	vixLevel := 0.0
	if currVIX.Symbol != "" {
		vixLevel = currVIX.Value
	}
	if jpyChange > 2.0 || vixLevel > 25 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-jpy-%d", now.UnixNano()),
			Theme:       "JPY_carry_unwind",
			Region:      "JP",
			Sentiment:   -0.6,
			Confidence:  0.65,
			CapitalFlow: "global_liquidity_drain",
			TimeWindow:  "immediate",
			Timestamp:   now,
			SourceData: map[string]float64{
				"jpy_change_pct": jpyChange,
				"vix_level":      vixLevel,
			},
		}
	}
	return nil
}

func detectGeopoliticalRiskEventFromSnapshot(currGold, currVIX marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	goldChange := 0.0
	if currGold.Symbol != "" {
		goldChange = currGold.ChangePct
	}
	// Use VIX spike as proxy for geopolitical risk if no GPR index.
	vixSpike := false
	if currVIX.Symbol != "" && currVIX.Value > 25 {
		vixSpike = true
	}
	if goldChange > 2.0 || vixSpike {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-geo-%d", now.UnixNano()),
			Theme:       "geopolitical_risk_spike",
			Region:      "Global",
			Sentiment:   -0.8,
			Confidence:  0.65,
			CapitalFlow: "risk_off",
			TimeWindow:  "immediate",
			Timestamp:   now,
			SourceData: map[string]float64{
				"gold_change_pct": goldChange,
				"vix_level":       currVIX.Value,
			},
		}
	}
	return nil
}

func detectOilShockEventFromSnapshot(currOil marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if currOil.Symbol == "" {
		return nil
	}
	if currOil.ChangePct > 5.0 || currOil.ChangePct < -5.0 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-oil-%d", now.UnixNano()),
			Theme:       "oil_price_shock",
			Region:      "Global",
			Sentiment:   -0.5,
			Confidence:  0.60,
			CapitalFlow: "inflation_reprice",
			TimeWindow:  "1_week",
			Timestamp:   now,
			SourceData: map[string]float64{
				"oil_change_pct": currOil.ChangePct,
			},
		}
	}
	return nil
}

func detectRetailFrenzyEvent(snap marketdata.MacroDataSnapshot) *NarrativeEvent {
	if snap.RetailSentiment.Symbol == "" {
		return nil
	}
	if snap.RetailSentiment.Value >= 0.8 {
		now := time.Now().UTC()
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-retail-frenzy-%d", now.UnixNano()),
			Theme:            "retail_frenzy",
			Region:           "TW",
			Sentiment:        1.0,
			Confidence:       snap.RetailSentiment.Value,
			ConfidenceSource: "retail_sentiment_90th_percentile",
			HitRate:          hitRateForTheme("retail_frenzy"),
			CapitalFlow:      "retail_chasing",
			TimeWindow:       "1-2_weeks",
			Timestamp:        now,
			SourceData: map[string]float64{
				"sentiment_score": snap.RetailSentiment.Value,
			},
		}
	}
	return nil
}

func detectRetailFearEvent(snap marketdata.MacroDataSnapshot) *NarrativeEvent {
	if snap.RetailSentiment.Symbol == "" {
		return nil
	}
	if snap.RetailSentiment.Value <= -0.8 {
		now := time.Now().UTC()
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-retail-fear-%d", now.UnixNano()),
			Theme:            "retail_fear",
			Region:           "TW",
			Sentiment:        -1.0,
			Confidence:       -snap.RetailSentiment.Value,
			ConfidenceSource: "retail_sentiment_10th_percentile",
			HitRate:          hitRateForTheme("retail_fear"),
			CapitalFlow:      "retail_fleeing",
			TimeWindow:       "1-2_weeks",
			Timestamp:        now,
			SourceData: map[string]float64{
				"sentiment_score": snap.RetailSentiment.Value,
			},
		}
	}
	return nil
}
