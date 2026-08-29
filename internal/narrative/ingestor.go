package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
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
	PublishNarrativeEvent(eventID, theme, region string, sentiment, confidence float64, confidenceSource, hitRate, capitalFlow, timeWindow, explanation, sentimentExplanation string)
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
			prev, _ := m.loadLatestSnapshot() //nolint:errcheck
			snap = mergeWithPrev(snap, prev)
			snap = computeChangePct(snap, prev)
			if saveErr := m.saveSnapshot(snap); saveErr != nil {
				logging.Warn("ingestor", "partial_save_failed", logging.Err(saveErr))
			}
			events := detectEventsFromSnapshot(snap, prev, m.divergenceDetect)
			m.publishEvents(ctx, events)
			return events, snap, nil
		}
		prev, prevErr := m.loadLatestSnapshot()
		if prevErr == nil {
			prevPrev, _ := m.loadPreviousSnapshot(prev) //nolint:errcheck
			// Repair ChangePct from prevPrev if the previous run left it at zero
			// (first-run or all-providers-fail scenario).
			if prev.JPY.ChangePct == 0 && prev.JPY.Symbol != "" && prev.JPY.Value != 0 &&
				prevPrev.JPY.Symbol != "" && prevPrev.JPY.Value != 0 {
				prev.JPY.ChangePct = (prev.JPY.Value - prevPrev.JPY.Value) / prevPrev.JPY.Value * 100
			}
			if prev.USD_TWD.ChangePct == 0 && prev.USD_TWD.Symbol != "" && prev.USD_TWD.Value != 0 &&
				prevPrev.USD_TWD.Symbol != "" && prevPrev.USD_TWD.Value != 0 {
				prev.USD_TWD.ChangePct = (prev.USD_TWD.Value - prevPrev.USD_TWD.Value) / prevPrev.USD_TWD.Value * 100
			}
			events := detectEventsFromSnapshot(prev, prevPrev, m.divergenceDetect)
			m.publishEvents(ctx, events)
			return events, prev, nil
		}
		return nil, snap, fmt.Errorf("fetch snapshot: %w", err)
	}

	prev, _ := m.loadLatestSnapshot() //nolint:errcheck

	// Fallback: if prev has zero values for JPY or USD_TWD, try a deeper snapshot
	// so computeChangePct can calculate meaningful deltas (first-run or corrupted-data scenario).
	if prev.JPY.Value == 0 || prev.JPY.Symbol == "" || prev.USD_TWD.Value == 0 || prev.USD_TWD.Symbol == "" {
		if prevPrev, err := m.loadPreviousSnapshot(prev); err == nil {
			if prev.JPY.Value == 0 || prev.JPY.Symbol == "" {
				prev.JPY = prevPrev.JPY
			}
			if prev.USD_TWD.Value == 0 || prev.USD_TWD.Symbol == "" {
				prev.USD_TWD = prevPrev.USD_TWD
			}
		}
	}

	events := detectEventsFromSnapshot(snap, prev, m.divergenceDetect)
	m.publishEvents(ctx, events)

	snap = mergeWithPrev(snap, prev)
	snap = computeChangePct(snap, prev)

	if err := m.saveSnapshot(snap); err != nil {
		return events, snap, fmt.Errorf("save snapshot: %w", err)
	}
	return events, snap, nil
}

func (m *MacroIngestor) publishEvents(ctx context.Context, events []NarrativeEvent) {
	if m.eventBus == nil {
		return
	}
	for i := range events {
		e := &events[i]
		if m.lifecycle != nil {
			if m.lifecycle.IsThemeActive(e.Theme) {
				if existing := m.lifecycle.GetActiveByTheme(e.Theme); existing != nil {
					if e.Confidence > existing.Confidence {
						m.lifecycle.UpdateConfidence(existing.ID, e.Confidence)
					}
				}
				continue
			}
			m.lifecycle.AddEvent(e)
		}
		AnnotateEvent(ctx, e)
		m.eventBus.PublishNarrativeEvent(
			e.ID, e.Theme, e.Region,
			e.Sentiment, e.Confidence,
			e.ConfidenceSource, fmt.Sprintf("%.2f", e.HitRate),
			e.CapitalFlow, e.TimeWindow,
			e.Explanation, e.SentimentExplanation,
		)
	}
}

// computeChangePct calculates change_pct for indicators where the provider
// does not supply it (e.g., ExchangeRate-API gives only current rate).
func computeChangePct(curr, prev marketdata.MacroDataSnapshot) marketdata.MacroDataSnapshot {
	if curr.JPY.ChangePct == 0 && curr.JPY.Symbol != "" && prev.JPY.Symbol != "" && prev.JPY.Value != 0 {
		curr.JPY.ChangePct = (curr.JPY.Value - prev.JPY.Value) / prev.JPY.Value * 100
	}
	if curr.USD_TWD.ChangePct == 0 && curr.USD_TWD.Symbol != "" && prev.USD_TWD.Symbol != "" && prev.USD_TWD.Value != 0 {
		curr.USD_TWD.ChangePct = (curr.USD_TWD.Value - prev.USD_TWD.Value) / prev.USD_TWD.Value * 100
	}
	return curr
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
	if curr.MarginMaintenanceRatio.Symbol == "" {
		curr.MarginMaintenanceRatio = prev.MarginMaintenanceRatio
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
	if curr.RetailShortBalance.Symbol == "" {
		curr.RetailShortBalance = prev.RetailShortBalance
	}
	if curr.Bdi.Symbol == "" {
		curr.Bdi = prev.Bdi
	}
	if curr.TSMADR.Symbol == "" {
		curr.TSMADR = prev.TSMADR
	}
	if curr.SPXIndex.Symbol == "" {
		curr.SPXIndex = prev.SPXIndex
	}
	if curr.NDXIndex.Symbol == "" {
		curr.NDXIndex = prev.NDXIndex
	}
	if curr.DJIIndex.Symbol == "" {
		curr.DJIIndex = prev.DJIIndex
	}
	if curr.NVDA.Symbol == "" {
		curr.NVDA = prev.NVDA
	}
	if curr.AAPL.Symbol == "" {
		curr.AAPL = prev.AAPL
	}
	if curr.MSFT.Symbol == "" {
		curr.MSFT = prev.MSFT
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
		info, _ := e.Info() //nolint:errcheck
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
	slices.Sort(candidates)
	for _, candidate := range slices.Backward(candidates) {
		path := filepath.Join(m.snapshotDir, candidate)
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
	// Preserve current latest as previous.json before overwriting,
	// so the stress index calculator can compute day-over-day change_pct.
	latestPath := filepath.Join(m.snapshotDir, "latest.json")
	prevPath := filepath.Join(m.snapshotDir, "previous.json")
	if prevData, err := os.ReadFile(latestPath); err == nil {
		_ = os.WriteFile(prevPath, prevData, 0o644) //nolint:errcheck
	}
	path := latestPath
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
	params := config.GetParametersConfig().Narrative

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
		if math.Abs(curr.TSMCRevenue.ChangePct) > params.EarningsSurpriseThreshold.Value {
			if event := NewEarningsSurpriseEvent(curr.TSMCRevenue.ChangePct); event != nil {
				events = append(events, *event)
			}
		}
	}

	if curr.RetailMarginBalance.Symbol != "" {
		if event := detectRetailFrenzyEventFromSnapshot(curr.RetailMarginBalance, DefaultMarginHistoryDir, now); event != nil {
			events = append(events, *event)
		}
		if event := detectRetailFearEventFromSnapshot(curr.RetailMarginBalance, DefaultMarginHistoryDir, now); event != nil {
			events = append(events, *event)
		}
	}

	// P3-2: PM event detectors — trigger gold_rally/dollar_surge/inflation_spike for factor weight adjustment.
	if event := detectGoldRallyEventFromSnapshot(curr.Gold, now); event != nil {
		events = append(events, *event)
	}
	if event := detectDollarSurgeEventFromSnapshot(curr.DXY, now); event != nil {
		events = append(events, *event)
	}
	if event := detectInflationSpikeEventFromSnapshot(curr.VIX, curr.DXY, now); event != nil {
		events = append(events, *event)
	}
	if event := detectTariffShockEventFromSnapshot(curr.VIX, curr.DXY, curr.SPXIndex, now); event != nil {
		events = append(events, *event)
	}
	if event := detectDeepSeekEventFromSnapshot(curr.SOXIndex, curr.NVDA, curr.TSMADR, now); event != nil {
		events = append(events, *event)
	}

	return events
}

func detectRetailDivergenceEventFromSnapshot(foreignNet marketdata.MacroDataPoint, marginZScore float64, now time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if marginZScore > params.RetailMarginZScoreThreshold.Value && foreignNet.Value < 0 {
		confidence := computeDeviationConfidence(marginZScore, params.RetailMarginZScoreThreshold.Value, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-retail-div-%d", now.UnixNano()),
			Theme:            "retail_institutional_divergence",
			Region:           "TW",
			Sentiment:        -0.5,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
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
	params := config.GetParametersConfig().Narrative
	// ^TNX is stored as bps proxy in our provider.
	changeBps := curr.Value
	if prev.Symbol != "" {
		changeBps = curr.Value - prev.Value
	}
	if changeBps > params.US10YChangeBpsThreshold.Value || curr.ChangePct > params.DXYChangePctThreshold.Value {
		confidenceBps := computeDeviationConfidence(changeBps, params.US10YChangeBpsThreshold.Value, params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceDXY := computeDeviationConfidence(curr.ChangePct, params.DXYChangePctThreshold.Value, params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
		confidence := confidenceBps
		if confidenceDXY > confidence {
			confidence = confidenceDXY
		}
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-us-rates-%d", now.UnixNano()),
			Theme:            "US_rates_up",
			Region:           "US",
			Sentiment:        -0.6,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
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
	return buildJPYCarryUnwindEvent(jpyChange, vixLevel, now)
}

func detectGeopoliticalRiskEventFromSnapshot(currGold, currVIX, currUSDTWD marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	goldChange := 0.0
	if currGold.Symbol != "" {
		goldChange = currGold.ChangePct
	}
	// Use VIX spike as proxy for geopolitical risk if no GPR index.
	vixSpike := currVIX.Symbol != "" && currVIX.Value > params.VIXLevelThreshold.Value
	// NARR-05: Taiwan Stress Index proxy via USD/TWD depreciation.
	// Augmented with Taiwan-specific geopolitical context: Trump tariff rhetoric,
	// BIS export controls, DeepSeek AI disruption — all amplify the Taiwan
	// geopolitical risk premium beyond generic Gold/VIX signals.
	taiwanStress := currUSDTWD.Symbol != "" && currUSDTWD.ChangePct > params.TaiwanStressUSDTWDThreshold.Value
	if goldChange > params.GoldChangePctThreshold.Value || vixSpike || taiwanStress {
		confidenceGold := computeDeviationConfidence(goldChange, params.GoldChangePctThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceVIX := computeDeviationConfidence(currVIX.Value, params.VIXLevelThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceTW := computeDeviationConfidence(currUSDTWD.ChangePct, params.TaiwanStressUSDTWDThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
		confidence := confidenceGold
		if confidenceVIX > confidence {
			confidence = confidenceVIX
		}
		if confidenceTW > confidence {
			confidence = confidenceTW
		}
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-geo-%d", now.UnixNano()),
			Theme:            "geopolitical_risk_spike",
			Region:           "Global",
			Sentiment:        -0.8,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
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
	event := buildUSDTWDVolatilityEvent(changePct, now)
	if event != nil {
		event.SourceData["usd_twd_level"] = curr.Value
	}
	return event
}

func detectSemiconductorEventFromSnapshot(curr, prev marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if curr.Symbol == "" {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	// NARR-04: Detect semiconductor downturn from export data
	changePct := curr.ChangePct
	if prev.Symbol != "" && prev.Value != 0 {
		changePct = (curr.Value - prev.Value) / prev.Value * 100
	}
	if changePct < params.SemiconductorExportDropThreshold.Value {
		confidence := computeDeviationConfidence(math.Abs(changePct), math.Abs(params.SemiconductorExportDropThreshold.Value), params.ConfidenceBaseTSMCRevenue.Value, params.ConfidenceDeviationCeiling.Value)
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-semi-%d", now.UnixNano()),
			Theme:            "semiconductor_downturn",
			Region:           "TW",
			Sentiment:        -0.6,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
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
	return buildOilShockEvent(currOil.ChangePct, now)
}

func computeAICapexSentiment(tsmcYoYChangePct float64) float64 {
	params := config.GetParametersConfig().Narrative
	yoyThreshold := params.TSMCRevenueYoYThreshold.Value
	posThreshold := params.TSMCRevenuePositiveThreshold.Value
	fallback := params.AICapexFallbackSentiment.Value

	if tsmcYoYChangePct >= yoyThreshold {
		// Above YoY threshold: scale from 0.8 toward 1.0, capped at 1.0
		extra := min((tsmcYoYChangePct-yoyThreshold)/yoyThreshold, 1.0)
		return 0.8 + (0.2 * extra)
	}
	if tsmcYoYChangePct >= posThreshold {
		// Between thresholds: linear interpolation from 0.5 to 0.8
		ratio := (tsmcYoYChangePct - posThreshold) / (yoyThreshold - posThreshold)
		return 0.5 + (0.3 * ratio)
	}
	return fallback
}

func detectRetailFrenzyEventFromSnapshot(marginBalance marketdata.MacroDataPoint, marginDir string, now time.Time) *NarrativeEvent {
	if marginBalance.Symbol == "" {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	history, err := LoadMarginHistory(marginDir)
	if err == nil && marginHistoryAvailable(history) {
		percentile, ok := ComputeRollingPercentile(history, marginBalance.Value, 60)
		if ok && percentile >= params.RetailFrenzyPercentileThreshold.Value {
			accel, accelOK := ComputeRollingAcceleration(history, params.RetailAccelerationWindowDays.Value)
			medianAccel, medianOK := historicalMedianAcceleration(history, params.RetailAccelerationWindowDays.Value)
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

func detectRetailFearEventFromSnapshot(marginBalance marketdata.MacroDataPoint, marginDir string, now time.Time) *NarrativeEvent {
	if marginBalance.Symbol == "" {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	history, err := LoadMarginHistory(marginDir)
	if err == nil && marginHistoryAvailable(history) {
		percentile, ok := ComputeRollingPercentile(history, marginBalance.Value, 60)
		if ok && percentile <= params.RetailFearPercentileThreshold.Value {
			accel, accelOK := ComputeRollingAcceleration(history, params.RetailAccelerationWindowDays.Value)
			medianAccel, medianOK := historicalMedianAcceleration(history, params.RetailAccelerationWindowDays.Value)
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
	event := buildAICapexSurgeEvent(sentiment, now)
	if event == nil {
		return nil
	}
	if prevTSMC.Symbol != "" && prevTSMC.ChangePct > 0 {
		boosted := event.Confidence + 0.05
		if boosted > 0.95 {
			boosted = 0.95
		}
		event.Confidence = boosted
	}
	return event
}

// detectGoldRallyEventFromSnapshot triggers gold_rally when gold price surges above threshold.
// Used by P3-2 to feed FactorWeightEngine's PM event weight adjustments.
func detectGoldRallyEventFromSnapshot(currGold marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if currGold.Symbol == "" {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	if currGold.ChangePct <= params.GoldChangePctThreshold.Value {
		return nil
	}
	confidence := computeDeviationConfidence(currGold.ChangePct, params.GoldChangePctThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-gold-rally-%d", now.UnixNano()),
		Theme:            "gold_rally",
		Region:           "Global",
		Sentiment:        0.7,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("gold_rally"),
		CapitalFlow:      "flight_to_gold",
		TimeWindow:       "1_week",
		Timestamp:        now,
		Severity:         "high",
		SourceData: map[string]float64{
			"gold_change_pct": currGold.ChangePct,
			"gold_price":      currGold.Value,
		},
	}
}

// detectDollarSurgeEventFromSnapshot triggers dollar_surge when DXY strengthens beyond threshold.
// Used by P3-2 to feed FactorWeightEngine's PM event weight adjustments.
func detectDollarSurgeEventFromSnapshot(currDXY marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if currDXY.Symbol == "" {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	if currDXY.ChangePct <= params.DXYChangePctThreshold.Value {
		return nil
	}
	confidence := computeDeviationConfidence(currDXY.ChangePct, params.DXYChangePctThreshold.Value, params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-dollar-surge-%d", now.UnixNano()),
		Theme:            "dollar_surge",
		Region:           "US",
		Sentiment:        -0.5,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("dollar_surge"),
		CapitalFlow:      "flight_to_USD",
		TimeWindow:       "1_week",
		Timestamp:        now,
		Severity:         "high",
		SourceData: map[string]float64{
			"dxy_change_pct": currDXY.ChangePct,
			"dxy_level":      currDXY.Value,
		},
	}
}

// detectInflationSpikeEventFromSnapshot triggers inflation_spike when VIX and DXY both signal
// inflation repricing. Uses VIX as a volatility proxy for inflation uncertainty and DXY
// strengthening as confirmation. CPI data source is pending; this is a proxy-based v1 detector.
func detectInflationSpikeEventFromSnapshot(currVIX, currDXY marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	if currVIX.Symbol == "" {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	if currVIX.Value <= params.VIXLevelThreshold.Value {
		return nil
	}
	dxySignal := 0.0
	if currDXY.Symbol != "" && currDXY.ChangePct > 0 {
		dxySignal = 1.0
	}
	confidence := computeDeviationConfidence(currVIX.Value, params.VIXLevelThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
	if dxySignal > 0 {
		confidence = clampConfidence(confidence + 0.05)
	}
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-inflation-spike-%d", now.UnixNano()),
		Theme:            "inflation_spike",
		Region:           "US",
		Sentiment:        -0.6,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("inflation_spike"),
		CapitalFlow:      "inflation_reprice",
		TimeWindow:       "1_week",
		Timestamp:        now,
		Severity:         "high",
		SourceData: map[string]float64{
			"vix_level":      currVIX.Value,
			"dxy_change_pct": currDXY.ChangePct,
			"dxy_confirming": dxySignal,
		},
	}
}

func detectTariffShockEventFromSnapshot(currVIX, currDXY, currSPX marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if currVIX.Symbol == "" {
		return nil
	}

	vixThreshold := params.VIXLevelThreshold.Value
	if vixThreshold <= 0 {
		vixThreshold = 20.0
	}
	vixSpike := currVIX.Value > vixThreshold

	dxyChange := currDXY.ChangePct
	if dxyChange < 0 {
		dxyChange = -dxyChange
	}
	dxyThreshold := params.DXYChangePctThreshold.Value
	if dxyThreshold <= 0 {
		dxyThreshold = 1.5
	}
	dxyVolatile := currDXY.Symbol != "" && dxyChange > dxyThreshold

	spxSelloff := currSPX.Symbol != "" && currSPX.ChangePct < -2.0

	if !vixSpike && !dxyVolatile && !spxSelloff {
		return nil
	}

	confBase := params.ConfidenceBaseGeopolitical.Value
	confCeil := params.ConfidenceDeviationCeiling.Value
	vixConf := computeDeviationConfidence(currVIX.Value, vixThreshold, confBase, confCeil)
	dxyConf := computeDeviationConfidence(dxyChange, dxyThreshold, confBase, confCeil)

	confidence := vixConf
	if dxyConf > confidence {
		confidence = dxyConf
	}
	if spxSelloff {
		spxConf := computeDeviationConfidence(-currSPX.ChangePct, 2.0, confBase, confCeil)
		if spxConf > confidence {
			confidence = spxConf
		}
	}

	td := DefaultThemeDurations()
	dur, ok := td["tariff_shock"]
	if !ok {
		dur = 14 * 24 * time.Hour
	}

	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-tariff-shock-%d", now.UnixNano()),
		Theme:            "tariff_shock",
		Region:           "US",
		Sentiment:        -0.9,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("tariff_shock"),
		CapitalFlow:      "risk_off",
		TimeWindow:       "immediate",
		Duration:         dur,
		Severity:         "critical",
		Timestamp:        now,
		SourceData: map[string]float64{
			"vix_level":      currVIX.Value,
			"dxy_change_pct": dxyChange,
			"spx_change_pct": currSPX.ChangePct,
		},
	}
}

func clampConfidence(c float64) float64 {
	if c > 0.95 {
		return 0.95
	}
	return c
}

// detectDeepSeekEventFromSnapshot detects AI model disruption events when semiconductor
// and AI-exposed stocks suffer acute selloffs. Triggered by SOX drop >5% or NVDA/TSM ADR
// drop >10%. Models the DeepSeek Jan 27 2025 shock: SOX -9.15%, NVDA -16.97%, TSM ADR -13.33%.
func detectDeepSeekEventFromSnapshot(currSOX, currNVDA, currTSMADR marketdata.MacroDataPoint, now time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if currSOX.Symbol == "" {
		return nil
	}

	soxThreshold := -5.0
	soxCrash := currSOX.ChangePct < soxThreshold

	nvdaCrash := false
	tsmAdrCrash := false
	if currNVDA.Symbol != "" && currNVDA.ChangePct < -10.0 {
		nvdaCrash = true
	}
	if currTSMADR.Symbol != "" && currTSMADR.ChangePct < -10.0 {
		tsmAdrCrash = true
	}

	if !soxCrash && !nvdaCrash && !tsmAdrCrash {
		return nil
	}

	confBase := params.ConfidenceBaseGeopolitical.Value
	confCeil := params.ConfidenceDeviationCeiling.Value
	soxConf := computeDeviationConfidence(-currSOX.ChangePct, -soxThreshold, confBase, confCeil)

	confidence := soxConf
	if nvdaCrash {
		nvdaConf := computeDeviationConfidence(-currNVDA.ChangePct, 10.0, confBase, confCeil)
		if nvdaConf > confidence {
			confidence = nvdaConf
		}
	}
	if tsmAdrCrash {
		tsmConf := computeDeviationConfidence(-currTSMADR.ChangePct, 10.0, confBase, confCeil)
		if tsmConf > confidence {
			confidence = tsmConf
		}
	}

	td := DefaultThemeDurations()
	dur := td["AI_capex_surge"] // reuse AI capex duration (90d)
	if custom, ok := td["semiconductor_downturn"]; ok {
		dur = custom // prefer semiconductor_downturn duration (90d)
	}
	_ = dur

	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-deepseek-%d", now.UnixNano()),
		Theme:            "semiconductor_downturn",
		Region:           "US_TW",
		Sentiment:        -0.9,
		Confidence:       clampConfidence(confidence),
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("semiconductor_downturn"),
		CapitalFlow:      "risk_off",
		TimeWindow:       "immediate",
		Timestamp:        now,
		SourceData: map[string]float64{
			"sox_change_pct":    currSOX.ChangePct,
			"nvda_change_pct":   currNVDA.ChangePct,
			"tsmadr_change_pct": currTSMADR.ChangePct,
		},
	}
}
