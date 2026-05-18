package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// MacroIngestor fetches macro data, compares with previous snapshot, and emits events.
type MacroIngestor struct {
	provider         marketdata.MacroDataProvider
	snapshotDir      string
	divergenceDetect *DivergenceDetector
	eventBus         eventbusPublisher
	lifecycle        *EventLifecycleManager
}

// eventbusPublisher abstracts the event bus for testing.
type eventbusPublisher interface {
	PublishNarrativeEvent(eventID, theme, region string, sentiment, confidence float64, confidenceSource, hitRate, capitalFlow, timeWindow string) error
}

// NewMacroIngestor creates an ingestor with a given provider and snapshot directory.
func NewMacroIngestor(provider marketdata.MacroDataProvider, snapshotDir string) *MacroIngestor {
	return &MacroIngestor{
		provider:         provider,
		snapshotDir:      snapshotDir,
		divergenceDetect: NewDivergenceDetector(),
	}
}

// SnapshotDir returns the directory where snapshots are stored.
func (m *MacroIngestor) SnapshotDir() string {
	return m.snapshotDir
}

func (m *MacroIngestor) SetEventBus(bus eventbusPublisher) {
	m.eventBus = bus
}

func (m *MacroIngestor) SetLifecycleManager(lm *EventLifecycleManager) {
	m.lifecycle = lm
}

// Ingest fetches latest macro data, computes changes from previous snapshot, and returns events.
func (m *MacroIngestor) Ingest(ctx context.Context) ([]NarrativeEvent, marketdata.MacroDataSnapshot, error) {
	if m.lifecycle != nil {
		m.lifecycle.UpdateStatuses()
	}

	snap, err := m.provider.FetchSnapshot(ctx)
	if err != nil {
		// Partial success: save valid fields and merge with previous snapshot.
		if hasValidYahooData(snap) {
			prev, _ := m.loadLatestSnapshot()
			snap = mergeWithPrev(snap, prev)
			if saveErr := m.saveSnapshot(snap); saveErr != nil {
				logging.Warn("ingestor", "partial_save_failed", logging.Err(saveErr))
			}
			events := detectEventsFromSnapshot(snap, prev, m.divergenceDetect)
			m.publishEvents(events)
			return events, snap, nil
		}
		prev, prevErr := m.loadLatestSnapshot()
		if prevErr == nil {
			prevPrev, _ := m.loadPreviousSnapshot(prev)
			events := detectEventsFromSnapshot(prev, prevPrev, m.divergenceDetect)
			m.publishEvents(events)
			return events, prev, nil
		}
		return nil, snap, fmt.Errorf("fetch snapshot: %w", err)
	}

	prev, _ := m.loadLatestSnapshot()
	events := detectEventsFromSnapshot(snap, prev, m.divergenceDetect)
	m.publishEvents(events)

	snap = mergeWithPrev(snap, prev)

	if err := m.saveSnapshot(snap); err != nil {
		return events, snap, fmt.Errorf("save snapshot: %w", err)
	}
	return events, snap, nil
}

func (m *MacroIngestor) publishEvents(events []NarrativeEvent) {
	if m.eventBus == nil {
		return
	}
	for i := range events {
		e := &events[i]
		if m.lifecycle != nil {
			if m.lifecycle.IsThemeActive(e.Theme) {
				if existing := m.lifecycle.GetActiveByTheme(e.Theme); existing != nil {
					m.lifecycle.UpdateConfidence(existing.ID, e.Confidence)
				}
				continue
			}
			m.lifecycle.AddEvent(e)
		}
		m.eventBus.PublishNarrativeEvent(
			e.ID, e.Theme, e.Region,
			e.Sentiment, e.Confidence,
			e.ConfidenceSource, fmt.Sprintf("%.2f", e.HitRate),
			e.CapitalFlow, e.TimeWindow,
		)
	}
}

func mergeWithPrev(curr, prev marketdata.MacroDataSnapshot) marketdata.MacroDataSnapshot {
	if curr.US10Y.Symbol == "" {
		curr.US10Y = prev.US10Y
	}
	if curr.DXY.Symbol == "" {
		curr.DXY = prev.DXY
	}
	if curr.VIX.Symbol == "" {
		curr.VIX = prev.VIX
	}
	if curr.USD_TWD.Symbol == "" {
		curr.USD_TWD = prev.USD_TWD
	}
	if curr.Oil.Symbol == "" {
		curr.Oil = prev.Oil
	}
	if curr.Gold.Symbol == "" {
		curr.Gold = prev.Gold
	}
	if curr.JPY.Symbol == "" {
		curr.JPY = prev.JPY
	}
	if curr.ForeignInvestorNet.Symbol == "" {
		curr.ForeignInvestorNet = prev.ForeignInvestorNet
	}
	if curr.DomesticFundNet.Symbol == "" {
		curr.DomesticFundNet = prev.DomesticFundNet
	}
	if curr.DealerNet.Symbol == "" {
		curr.DealerNet = prev.DealerNet
	}
	if curr.ExportElectronics.Symbol == "" {
		curr.ExportElectronics = prev.ExportElectronics
	}
	if curr.RetailMarginBalance.Symbol == "" {
		curr.RetailMarginBalance = prev.RetailMarginBalance
	}
	if curr.TSMCRevenue.Symbol == "" {
		curr.TSMCRevenue = prev.TSMCRevenue
	}
	if curr.SOXIndex.Symbol == "" {
		curr.SOXIndex = prev.SOXIndex
	}
	if curr.CoWoSUtilization.Symbol == "" {
		curr.CoWoSUtilization = prev.CoWoSUtilization
	}
	if curr.CapexGrowth.Symbol == "" {
		curr.CapexGrowth = prev.CapexGrowth
	}
	return curr
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
	if hasValidYahooData(snap) {
		return snap, nil
	}
	dated, err := m.loadFallbackDatedSnapshot()
	if err != nil {
		return snap, nil
	}
	return dated, nil
}

func hasValidYahooData(snap marketdata.MacroDataSnapshot) bool {
	return snap.US10Y.Symbol != "" || snap.DXY.Symbol != "" || snap.VIX.Symbol != "" || snap.JPY.Symbol != ""
}

func (m *MacroIngestor) loadFallbackDatedSnapshot() (marketdata.MacroDataSnapshot, error) {
	entries, err := os.ReadDir(m.snapshotDir)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	var latest string
	var latestTime int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "latest.json" {
			continue
		}
		info, _ := e.Info()
		if info != nil && info.ModTime().Unix() > latestTime {
			latestTime = info.ModTime().Unix()
			latest = e.Name()
		}
	}
	if latest == "" {
		return marketdata.MacroDataSnapshot{}, fmt.Errorf("no dated snapshots")
	}
	data, err := os.ReadFile(filepath.Join(m.snapshotDir, latest))
	if err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	return snap, nil
}

func (m *MacroIngestor) loadPreviousSnapshot(curr marketdata.MacroDataSnapshot) (marketdata.MacroDataSnapshot, error) {
	entries, err := os.ReadDir(m.snapshotDir)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, fmt.Errorf("read snapshot dir: %w", err)
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "latest.json" {
			continue
		}
		candidates = append(candidates, e.Name())
	}
	if len(candidates) == 0 {
		return marketdata.MacroDataSnapshot{}, fmt.Errorf("no previous snapshots")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i] < candidates[j]
	})
	for i := len(candidates) - 1; i >= 0; i-- {
		path := filepath.Join(m.snapshotDir, candidates[i])
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		if snap.RecordedAt < curr.RecordedAt {
			return snap, nil
		}
	}
	return marketdata.MacroDataSnapshot{}, fmt.Errorf("no previous snapshot found")
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

var templateHitRates = func() map[string]float64 {
	m := make(map[string]float64)
	for _, t := range DefaultTemplates() {
		m[t.TriggerTheme] = t.HistoricalHitRate
	}
	return m
}()

func hitRateForTheme(theme string) float64 {
	if r, ok := templateHitRates[theme]; ok {
		return r
	}
	return 0.0
}

func detectEventsFromSnapshot(curr, prev marketdata.MacroDataSnapshot, div *DivergenceDetector) []NarrativeEvent {
	var events []NarrativeEvent
	now := time.Now().UTC()

	if event := detectUSRatesEventFromSnapshot(curr.US10Y, prev.US10Y, now); event != nil {
		events = append(events, *event)
	}
	if event := detectJPYCarryUnwindEventFromSnapshot(curr.JPY, prev.JPY, curr.VIX, now); event != nil {
		events = append(events, *event)
	}
	if event := detectGeopoliticalRiskEventFromSnapshot(curr.Gold, curr.VIX, curr.USD_TWD, now); event != nil {
		events = append(events, *event)
	}
	if event := detectOilShockEventFromSnapshot(curr.Oil, now); event != nil {
		events = append(events, *event)
	}
	if event := detectUSDTWDEventFromSnapshot(curr.USD_TWD, prev.USD_TWD, now); event != nil {
		events = append(events, *event)
	}
	if event := detectSemiconductorEventFromSnapshot(curr.ExportElectronics, prev.ExportElectronics, now); event != nil {
		events = append(events, *event)
	}

	if div != nil && curr.RetailMarginBalance.Symbol != "" && curr.ForeignInvestorNet.Symbol != "" {
		div.Update(curr.RetailMarginBalance.Value, prev.RetailMarginBalance.Value, curr.ForeignInvestorNet.Value, prev.ForeignInvestorNet.Value)
		_, marginZ := div.RetailDivergenceAndMarginZScore(curr.RetailMarginBalance.Value, curr.ForeignInvestorNet.Value)
		if event := detectRetailDivergenceEventFromSnapshot(curr.ForeignInvestorNet, marginZ, now); event != nil {
			events = append(events, *event)
		}
	}

	if curr.TSMCRevenue.Symbol != "" && curr.TSMCRevenue.ChangePct > 0 {
		sentiment := computeAICapexSentiment(curr.TSMCRevenue.ChangePct)
		if event := detectAICapexEventFromSnapshot(sentiment, prev.TSMCRevenue, now); event != nil {
			events = append(events, *event)
		}
	}

	if curr.RetailMarginBalance.Symbol != "" {
		if event := detectRetailFrenzyEventFromSnapshot(curr.RetailMarginBalance, now); event != nil {
			events = append(events, *event)
		}
		if event := detectRetailFearEventFromSnapshot(curr.RetailMarginBalance, now); event != nil {
			events = append(events, *event)
		}
	}

	return events
}

func detectRetailDivergenceEventFromSnapshot(foreignNet marketdata.MacroDataPoint, marginZScore float64, now time.Time) *NarrativeEvent {
	if marginZScore > 1.5 && foreignNet.Value < 0 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-retail-div-%d", now.UnixNano()),
			Theme:            "retail_institutional_divergence",
			Region:           "TW",
			Sentiment:        -0.5,
			Confidence:       0.60,
			ConfidenceSource: "divergence_zscore_v1",
			HitRate:          hitRateForTheme("retail_institutional_divergence"),
			CapitalFlow:      "crowding_risk",
			TimeWindow:       "immediate",
			Timestamp:        now,
			SourceData: map[string]float64{
				"margin_zscore": marginZScore,
			},
		}
	}
	return nil
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
			ID:               fmt.Sprintf("evt-us-rates-%d", now.UnixNano()),
			Theme:            "US_rates_up",
			Region:           "US",
			Sentiment:        -0.6,
			Confidence:       0.75,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("US_rates_up"),
			CapitalFlow:      "flight_to_USD",
			TimeWindow:       "1_week",
			Timestamp:        now,
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
			ID:               fmt.Sprintf("evt-jpy-%d", now.UnixNano()),
			Theme:            "JPY_carry_unwind",
			Region:           "JP",
			Sentiment:        -0.6,
			Confidence:       0.65,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("JPY_carry_unwind"),
			CapitalFlow:      "global_liquidity_drain",
			TimeWindow:       "immediate",
			Timestamp:        now,
			SourceData: map[string]float64{
				"jpy_change_pct": jpyChange,
				"vix_level":      vixLevel,
			},
		}
	}
	return nil
}

func detectGeopoliticalRiskEventFromSnapshot(currGold, currVIX, currUSDTWD marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	goldChange := 0.0
	if currGold.Symbol != "" {
		goldChange = currGold.ChangePct
	}
	// Use VIX spike as proxy for geopolitical risk if no GPR index.
	vixSpike := false
	if currVIX.Symbol != "" && currVIX.Value > 25 {
		vixSpike = true
	}
	// NARR-05: Taiwan Stress Index proxy via USD/TWD depreciation
	taiwanStress := false
	if currUSDTWD.Symbol != "" && currUSDTWD.ChangePct > 1.0 {
		taiwanStress = true
	}
	if goldChange > 2.0 || vixSpike || taiwanStress {
		confidence := 0.65
		if taiwanStress {
			confidence = 0.70
		}
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-geo-%d", now.UnixNano()),
			Theme:            "geopolitical_risk_spike",
			Region:           "Global",
			Sentiment:        -0.8,
			Confidence:       confidence,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("geopolitical_risk_spike"),
			CapitalFlow:      "risk_off",
			TimeWindow:       "immediate",
			Timestamp:        now,
			SourceData: map[string]float64{
				"gold_change_pct": goldChange,
				"vix_level":       currVIX.Value,
				"usd_twd_change":  currUSDTWD.ChangePct,
				"taiwan_stress":   boolToFloat(taiwanStress),
			},
		}
	}
	return nil
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func detectUSDTWDEventFromSnapshot(curr, prev marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if curr.Symbol == "" {
		return nil
	}
	changePct := curr.ChangePct
	if prev.Symbol != "" && prev.Value != 0 {
		changePct = (curr.Value - prev.Value) / prev.Value * 100
	}
	if math.Abs(changePct) > 1.0 {
		sentiment := -0.5
		if changePct > 0 {
			sentiment = -0.7 // USD strengthening against TWD is negative for Taiwan exports
		}
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-usd-twd-%d", now.UnixNano()),
			Theme:            "USD_TWD_volatility",
			Region:           "TW",
			Sentiment:        sentiment,
			Confidence:       0.60,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("USD_TWD_volatility"),
			CapitalFlow:      "fx_driven_outflow",
			TimeWindow:       "1_week",
			Timestamp:        now,
			SourceData: map[string]float64{
				"usd_twd_change_pct": changePct,
				"usd_twd_level":      curr.Value,
			},
		}
	}
	return nil
}

func detectSemiconductorEventFromSnapshot(curr, prev marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if curr.Symbol == "" {
		return nil
	}
	// NARR-04: Detect semiconductor downturn from export data
	changePct := curr.ChangePct
	if prev.Symbol != "" && prev.Value != 0 {
		changePct = (curr.Value - prev.Value) / prev.Value * 100
	}
	if changePct < -5.0 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-semi-%d", now.UnixNano()),
			Theme:            "semiconductor_downturn",
			Region:           "TW",
			Sentiment:        -0.6,
			Confidence:       0.55,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("semiconductor_downturn"),
			CapitalFlow:      "tech_capex_slowdown",
			TimeWindow:       "1_month",
			Timestamp:        now,
			SourceData: map[string]float64{
				"export_electronics_change_pct": changePct,
				"export_electronics_level":      curr.Value,
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
			ID:               fmt.Sprintf("evt-oil-%d", now.UnixNano()),
			Theme:            "oil_price_shock",
			Region:           "Global",
			Sentiment:        -0.5,
			Confidence:       0.60,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("oil_price_shock"),
			CapitalFlow:      "inflation_reprice",
			TimeWindow:       "1_week",
			Timestamp:        now,
			SourceData: map[string]float64{
				"oil_change_pct": currOil.ChangePct,
			},
		}
	}
	return nil
}

func computeAICapexSentiment(tsmcYoYChangePct float64) float64 {
	if tsmcYoYChangePct > 10 {
		return 0.8
	}
	if tsmcYoYChangePct > 0 {
		return 0.5
	}
	return -0.3
}

func detectRetailFrenzyEventFromSnapshot(marginBalance marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if marginBalance.Symbol == "" {
		return nil
	}
	history, err := LoadMarginHistory(DefaultMarginHistoryDir)
	if err == nil && marginHistoryAvailable(history) {
		percentile, ok := ComputeRollingPercentile(history, marginBalance.Value, 60)
		if ok && percentile >= 90 {
			accel, accelOK := ComputeRollingAcceleration(history, 5)
			medianAccel, medianOK := historicalMedianAcceleration(history, 5)
			confirmed := accelOK && medianOK && marginStage2Confirmed(accel, medianAccel, true)
			confidence := 0.45
			if confirmed {
				confidence = marginPercentileConfidence(percentile)
			}
			return &NarrativeEvent{
				ID:               fmt.Sprintf("evt-retail-frenzy-%d", now.UnixNano()),
				Theme:            "retail_frenzy",
				Region:           "TW",
				Sentiment:        1.0,
				Confidence:       confidence,
				ConfidenceSource: "margin_history_percentile",
				HitRate:          hitRateForTheme("retail_frenzy"),
				CapitalFlow:      "retail_chasing",
				TimeWindow:       "1-2_weeks",
				Timestamp:        now,
				SourceData: map[string]float64{
					"margin_balance": marginBalance.Value,
					"percentile":     percentile,
					"acceleration":   accel,
					"median_accel":   medianAccel,
				},
			}
		}
		return nil
	}
	if err == nil || isMarginHistoryError(err) {
		return nil
	}
	return nil
}

func detectRetailFearEventFromSnapshot(marginBalance marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if marginBalance.Symbol == "" {
		return nil
	}
	history, err := LoadMarginHistory(DefaultMarginHistoryDir)
	if err == nil && marginHistoryAvailable(history) {
		percentile, ok := ComputeRollingPercentile(history, marginBalance.Value, 60)
		if ok && percentile <= 10 {
			accel, accelOK := ComputeRollingAcceleration(history, 5)
			medianAccel, medianOK := historicalMedianAcceleration(history, 5)
			confirmed := accelOK && medianOK && marginStage2Confirmed(accel, medianAccel, false)
			confidence := 0.45
			if confirmed {
				confidence = marginPercentileConfidence(100 - percentile)
			}
			return &NarrativeEvent{
				ID:               fmt.Sprintf("evt-retail-fear-%d", now.UnixNano()),
				Theme:            "retail_fear",
				Region:           "TW",
				Sentiment:        -1.0,
				Confidence:       confidence,
				ConfidenceSource: "margin_history_percentile",
				HitRate:          hitRateForTheme("retail_fear"),
				CapitalFlow:      "retail_fleeing",
				TimeWindow:       "1-2_weeks",
				Timestamp:        now,
				SourceData: map[string]float64{
					"margin_balance": marginBalance.Value,
					"percentile":     percentile,
					"acceleration":   accel,
					"median_accel":   medianAccel,
				},
			}
		}
		return nil
	}
	if err == nil || isMarginHistoryError(err) {
		return nil
	}
	return nil
}

func detectAICapexEventFromSnapshot(sentiment float64, prevTSMC marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if sentiment <= 0.5 {
		return nil
	}
	confidence := 0.70
	if prevTSMC.Symbol != "" && prevTSMC.ChangePct > 0 {
		confidence = 0.75
	}
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-ai-capex-%d", now.UnixNano()),
		Theme:            "AI_capex_surge",
		Region:           "US",
		Sentiment:        0.8,
		Confidence:       confidence,
		ConfidenceSource: "tsmc_revenue_yoy_v1",
		HitRate:          hitRateForTheme("AI_capex_surge"),
		CapitalFlow:      "tech_capex_inflow",
		TimeWindow:       "1_month",
		Timestamp:        now,
		SourceData: map[string]float64{
			"ai_capex_sentiment": sentiment,
		},
	}
}
