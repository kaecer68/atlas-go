// Package schemas_test provides table-driven JSON round-trip tests for all
// capability input/output struct pairs defined in internal/llm/schemas.
package schemas

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

func TestRationaleGenerationRoundTrip(t *testing.T) {
	input := RationaleGenerationInput{
		EnglishText: "Hello, world!",
		DataClass:   llm.DataClassNonRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed RationaleGenerationInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.EnglishText != input.EnglishText {
		t.Errorf("EnglishText: got %q, want %q", parsed.EnglishText, input.EnglishText)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}

	resp := RationaleGenerationResponse{TranslatedText: "你好，世界！"}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp RationaleGenerationResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.TranslatedText != resp.TranslatedText {
		t.Errorf("TranslatedText: got %q, want %q", parsedResp.TranslatedText, resp.TranslatedText)
	}
}

func TestStrategySummaryRoundTrip(t *testing.T) {
	frame := strategy_techniques.StrategyFrame{
		ID:        "strat-001",
		Name:      "momentum-breakout",
		Layer:     "primary",
		Summary:   "Breakout detection on tech stocks",
		Rationale: "Historical patterns suggest momentum continuation",
		Conditions: []strategy_techniques.Condition{
			{Field: "volume", Operator: "gt", Value: 100000},
		},
		Themes:          []string{"momentum"},
		Regimes:         []string{"Bull"},
		Sectors:         []string{"Technology"},
		Direction:       "long",
		Risk:            "medium",
		Source:          "manual",
		Status:          "active",
		AttributionMode: "rule_based",
		Attribution:     []string{"factor_A"},
		HitRate:         0.65,
		TotalTests:      100,
		TotalHits:       65,
		CreatedAt:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	input := StrategySummaryInput{
		Frame:     frame,
		DataClass: llm.DataClassRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed StrategySummaryInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}
	if parsed.Frame.ID != frame.ID {
		t.Errorf("Frame.ID: got %q, want %q", parsed.Frame.ID, frame.ID)
	}
	if parsed.Frame.HitRate != frame.HitRate {
		t.Errorf("Frame.HitRate: got %v, want %v", parsed.Frame.HitRate, frame.HitRate)
	}
	if len(parsed.Frame.Conditions) != 1 {
		t.Errorf("Frame.Conditions length: got %d, want 1", len(parsed.Frame.Conditions))
	}

	resp := StrategySummaryResponse{
		Summary:       "Momentum breakout strategy summary",
		KeyConditions: []string{"volume > 100k", "price > 20dma"},
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp StrategySummaryResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.Summary != resp.Summary {
		t.Errorf("Summary: got %q, want %q", parsedResp.Summary, resp.Summary)
	}
	if !reflect.DeepEqual(parsedResp.KeyConditions, resp.KeyConditions) {
		t.Errorf("KeyConditions: got %v, want %v", parsedResp.KeyConditions, resp.KeyConditions)
	}
}

func TestPromptLintRoundTrip(t *testing.T) {
	input := PromptLintInput{
		PromptContent: "You are a trading assistant. Analyze the following data.",
		PromptPath:    "prompts/trading/analysis.md",
		DataClass:     llm.DataClassNonRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed PromptLintInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.PromptContent != input.PromptContent {
		t.Errorf("PromptContent: got %q, want %q", parsed.PromptContent, input.PromptContent)
	}
	if parsed.PromptPath != input.PromptPath {
		t.Errorf("PromptPath: got %q, want %q", parsed.PromptPath, input.PromptPath)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}

	resp := PromptLintResponse{
		Issues: []LintIssue{
			{Line: 3, Severity: "warning", Message: "Consider adding examples"},
			{Line: 7, Severity: "error", Message: "Missing required context"},
		},
		Pass: false,
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp PromptLintResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.Pass != resp.Pass {
		t.Errorf("Pass: got %v, want %v", parsedResp.Pass, resp.Pass)
	}
	if len(parsedResp.Issues) != 2 {
		t.Fatalf("Issues length: got %d, want 2", len(parsedResp.Issues))
	}
	if parsedResp.Issues[0].Line != 3 {
		t.Errorf("Issues[0].Line: got %d, want 3", parsedResp.Issues[0].Line)
	}
	if parsedResp.Issues[1].Severity != "error" {
		t.Errorf("Issues[1].Severity: got %q, want %q", parsedResp.Issues[1].Severity, "error")
	}
}

func TestScenarioSimulationRoundTrip(t *testing.T) {
	result := prism.TrainingResult{
		HitRate:      0.72,
		SharpeRatio:  1.8,
		MaxDrawdown:  -0.12,
		TotalReturn:  0.35,
		SignalsCount: 200,
		WinCount:     144,
		LossCount:    56,
		Error:        "",
		Duration:     time.Hour,
		Synthetic:    false,
	}
	input := ScenarioSimulationInput{
		Result:    result,
		Regime:    prism.RegimeRiskOn,
		DataClass: llm.DataClassRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed ScenarioSimulationInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}
	if parsed.Regime != input.Regime {
		t.Errorf("Regime: got %v, want %v", parsed.Regime, input.Regime)
	}
	if parsed.Result.HitRate != result.HitRate {
		t.Errorf("Result.HitRate: got %v, want %v", parsed.Result.HitRate, result.HitRate)
	}
	if parsed.Result.SharpeRatio != result.SharpeRatio {
		t.Errorf("Result.SharpeRatio: got %v, want %v", parsed.Result.SharpeRatio, result.SharpeRatio)
	}
	if parsed.Result.SignalsCount != result.SignalsCount {
		t.Errorf("Result.SignalsCount: got %d, want %d", parsed.Result.SignalsCount, result.SignalsCount)
	}

	resp := ScenarioSimulationResponse{
		Insight:       "Strategy performs well in bullish regimes with low volatility",
		CohortSummary: "Cohort of 200 signals across 5 regimes",
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp ScenarioSimulationResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.Insight != resp.Insight {
		t.Errorf("Insight: got %q, want %q", parsedResp.Insight, resp.Insight)
	}
	if parsedResp.CohortSummary != resp.CohortSummary {
		t.Errorf("CohortSummary: got %q, want %q", parsedResp.CohortSummary, resp.CohortSummary)
	}
}

func TestRiskSurfaceExtractionRoundTrip(t *testing.T) {
	gap := spawning.KnowledgeGap{
		ID:          "gap-001",
		Type:        spawning.GapTypeSector,
		Severity:    spawning.GapSeverityHigh,
		Description: "Missing coverage in energy sector",
		Sector:      "Energy",
		Style:       "momentum",
		MarketCap:   "large",
		Evidence: []spawning.GapEvidence{
			{Metric: "coverage_pct", Value: 0.15, Threshold: 0.20, Context: "energy sector"},
		},
		DetectedAt: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		Status:     "open",
	}
	input := RiskSurfaceExtractionInput{
		Gap:       gap,
		DataClass: llm.DataClassRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed RiskSurfaceExtractionInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}
	if parsed.Gap.ID != gap.ID {
		t.Errorf("Gap.ID: got %q, want %q", parsed.Gap.ID, gap.ID)
	}
	if parsed.Gap.Type != gap.Type {
		t.Errorf("Gap.Type: got %q, want %q", parsed.Gap.Type, gap.Type)
	}
	if parsed.Gap.Severity != gap.Severity {
		t.Errorf("Gap.Severity: got %q, want %q", parsed.Gap.Severity, gap.Severity)
	}
	if len(parsed.Gap.Evidence) != 1 {
		t.Errorf("Gap.Evidence length: got %d, want 1", len(parsed.Gap.Evidence))
	}

	resp := RiskSurfaceExtractionResponse{
		EnrichedDescription: "Energy sector gap detected with critical coverage loss",
		Coverage:            0.15,
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp RiskSurfaceExtractionResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.EnrichedDescription != resp.EnrichedDescription {
		t.Errorf("EnrichedDescription: got %q, want %q", parsedResp.EnrichedDescription, resp.EnrichedDescription)
	}
	if parsedResp.Coverage != resp.Coverage {
		t.Errorf("Coverage: got %v, want %v", parsedResp.Coverage, resp.Coverage)
	}
}

func TestRegimeExplanationRoundTrip(t *testing.T) {
	event := narrative.NarrativeEvent{
		ID:          "evt-001",
		Theme:       "US_rates_up",
		Region:      "US",
		Sentiment:   -0.5,
		Confidence:  0.85,
		HitRate:     0.70,
		CapitalFlow: "flight_to_USD",
		TimeWindow:  "immediate",
		Timestamp:   time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC),
		SourceData:  map[string]float64{"dxy": 105.3, "vix": 18.5},
		Duration:    time.Minute * 30,
		ExpiresAt:   time.Date(2024, 5, 15, 1, 0, 0, 0, time.UTC),
		Severity:    "medium",
		Status:      "active",
	}
	input := RegimeExplanationInput{
		Event:     event,
		DataClass: llm.DataClassRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed RegimeExplanationInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}
	if parsed.Event.ID != event.ID {
		t.Errorf("Event.ID: got %q, want %q", parsed.Event.ID, event.ID)
	}
	if parsed.Event.Theme != event.Theme {
		t.Errorf("Event.Theme: got %q, want %q", parsed.Event.Theme, event.Theme)
	}
	if parsed.Event.Sentiment != event.Sentiment {
		t.Errorf("Event.Sentiment: got %v, want %v", parsed.Event.Sentiment, event.Sentiment)
	}
	if parsed.Event.Confidence != event.Confidence {
		t.Errorf("Event.Confidence: got %v, want %v", parsed.Event.Confidence, event.Confidence)
	}
	if !reflect.DeepEqual(parsed.Event.SourceData, event.SourceData) {
		t.Errorf("Event.SourceData: got %v, want %v", parsed.Event.SourceData, event.SourceData)
	}

	resp := RegimeExplanationResponse{
		Headline: "US interest rate hike triggers flight to USD",
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp RegimeExplanationResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.Headline != resp.Headline {
		t.Errorf("Headline: got %q, want %q", parsedResp.Headline, resp.Headline)
	}
}

func TestPerformanceForensicsRoundTrip(t *testing.T) {
	snapshot := domain.RiskSnapshot{
		VaR95:          -0.05,
		VaR99:          -0.10,
		CVaR95:         -0.08,
		MaxDrawdownPct: -0.15,
	}
	input := PerformanceForensicsInput{
		Snapshot:  snapshot,
		DataClass: llm.DataClassRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed PerformanceForensicsInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}
	if parsed.Snapshot.VaR95 != snapshot.VaR95 {
		t.Errorf("Snapshot.VaR95: got %v, want %v", parsed.Snapshot.VaR95, snapshot.VaR95)
	}
	if parsed.Snapshot.VaR99 != snapshot.VaR99 {
		t.Errorf("Snapshot.VaR99: got %v, want %v", parsed.Snapshot.VaR99, snapshot.VaR99)
	}
	if parsed.Snapshot.MaxDrawdownPct != snapshot.MaxDrawdownPct {
		t.Errorf("Snapshot.MaxDrawdownPct: got %v, want %v", parsed.Snapshot.MaxDrawdownPct, snapshot.MaxDrawdownPct)
	}

	resp := PerformanceForensicsResponse{
		Commentary:  "Portfolio exhibits elevated tail risk with VaR95 at -5%",
		Calibration: "Risk model calibrated against 5-year historical window",
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp PerformanceForensicsResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.Commentary != resp.Commentary {
		t.Errorf("Commentary: got %q, want %q", parsedResp.Commentary, resp.Commentary)
	}
	if parsedResp.Calibration != resp.Calibration {
		t.Errorf("Calibration: got %q, want %q", parsedResp.Calibration, resp.Calibration)
	}
}

func TestCodeReviewAnnotationRoundTrip(t *testing.T) {
	input := CodeReviewAnnotationInput{
		DiffText:  "diff --git a/main.go b/main.go\n+func NewHandler() Handler {",
		PRURL:     "https://github.com/kaecer68/atlas-go/pull/610",
		DataClass: llm.DataClassNonRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed CodeReviewAnnotationInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DiffText != input.DiffText {
		t.Errorf("DiffText: got %q, want %q", parsed.DiffText, input.DiffText)
	}
	if parsed.PRURL != input.PRURL {
		t.Errorf("PRURL: got %q, want %q", parsed.PRURL, input.PRURL)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}

	resp := CodeReviewAnnotationResponse{
		Annotations: []CodeAnnotation{
			{File: "main.go", Line: 5, Severity: "warning", Message: "Missing error handling"},
			{File: "router.go", Line: 12, Severity: "error", Message: "Race condition possible"},
		},
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp CodeReviewAnnotationResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(parsedResp.Annotations) != 2 {
		t.Fatalf("Annotations length: got %d, want 2", len(parsedResp.Annotations))
	}
	if parsedResp.Annotations[0].File != "main.go" {
		t.Errorf("Annotations[0].File: got %q, want %q", parsedResp.Annotations[0].File, "main.go")
	}
	if parsedResp.Annotations[1].Line != 12 {
		t.Errorf("Annotations[1].Line: got %d, want 12", parsedResp.Annotations[1].Line)
	}
}

func TestSentimentExplanationRoundTrip(t *testing.T) {
	event := narrative.NarrativeEvent{
		ID:          "evt-002",
		Theme:       "earnings_surprise",
		Region:      "Global",
		Sentiment:   0.75,
		Confidence:  0.90,
		HitRate:     0.80,
		CapitalFlow: "risk_on",
		TimeWindow:  "1_week",
		Timestamp:   time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
		SourceData:  map[string]float64{"eps_surprise": 0.15, "revenue_growth": 0.08},
		Duration:    time.Hour,
		ExpiresAt:   time.Date(2024, 7, 8, 0, 0, 0, 0, time.UTC),
		Severity:    "low",
		Status:      "active",
	}
	input := SentimentExplanationInput{
		Event:     event,
		DataClass: llm.DataClassNonRegulated,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	var parsed SentimentExplanationInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if parsed.DataClass != input.DataClass {
		t.Errorf("DataClass: got %v, want %v", parsed.DataClass, input.DataClass)
	}
	if parsed.Event.ID != event.ID {
		t.Errorf("Event.ID: got %q, want %q", parsed.Event.ID, event.ID)
	}
	if parsed.Event.Sentiment != event.Sentiment {
		t.Errorf("Event.Sentiment: got %v, want %v", parsed.Event.Sentiment, event.Sentiment)
	}
	if !reflect.DeepEqual(parsed.Event.SourceData, event.SourceData) {
		t.Errorf("Event.SourceData: got %v, want %v", parsed.Event.SourceData, event.SourceData)
	}

	resp := SentimentExplanationResponse{
		Explanation: "Positive earnings surprise driving bullish sentiment across sectors",
		Factors:     []string{"EPS beat by 15%", "Revenue growth 8% YoY", "Forward guidance raised"},
	}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsedResp SentimentExplanationResponse
	if err := json.Unmarshal(data, &parsedResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if parsedResp.Explanation != resp.Explanation {
		t.Errorf("Explanation: got %q, want %q", parsedResp.Explanation, resp.Explanation)
	}
	if !reflect.DeepEqual(parsedResp.Factors, resp.Factors) {
		t.Errorf("Factors: got %v, want %v", parsedResp.Factors, resp.Factors)
	}
}
