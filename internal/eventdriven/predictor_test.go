package eventdriven

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/industry"
)

func testPredictor() *Predictor {
	return NewPredictor(industry.NewEventCalendar())
}

type assessmentCapitalFlow struct {
	score           float64
	assessment      capitalflow.CapitalFlowAssessment
	assessmentErr   error
	qualityCalls    int
	assessmentCalls int
}

func (c *assessmentCapitalFlow) QualityScore() float64 {
	c.qualityCalls++
	return c.score
}

func (c *assessmentCapitalFlow) QualityLabel() string { return "legacy" }

func (c *assessmentCapitalFlow) LatestAssessment(context.Context) (capitalflow.CapitalFlowAssessment, error) {
	c.assessmentCalls++
	return c.assessment, c.assessmentErr
}

func TestPredict_CapitalFlowBaselineUsedWithCalibrationAwareness(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)

	t.Run("calibrating still consumes baseline with reduced weight and capped confidence", func(t *testing.T) {
		cf := &assessmentCapitalFlow{
			score: 1,
			assessment: capitalflow.CapitalFlowAssessment{
				CalibrationStatus: capitalflow.CalibrationCalibrating,
			},
		}
		p := testPredictor()
		p.SetCapitalFlow(cf)
		report := p.Predict(now)

		if cf.assessmentCalls != 1 {
			t.Errorf("LatestAssessment calls = %d, want 1", cf.assessmentCalls)
		}
		if cf.qualityCalls != 1 {
			t.Errorf("QualityScore calls = %d, want 1 so baseline is available", cf.qualityCalls)
		}
		if !strings.Contains(report.Summary, "當前資金品質偏多") {
			t.Errorf("calibrating baseline missing from prediction summary: %q", report.Summary)
		}
		if !strings.Contains(report.Summary, "校準中") {
			t.Errorf("calibrating uncertainty note missing from summary: %q", report.Summary)
		}
		for _, pred := range report.Predictions {
			if pred.Confidence > calibratingConfidenceCap {
				t.Errorf("calibrating prediction confidence %.4f exceeds cap %.4f", pred.Confidence, calibratingConfidenceCap)
			}
		}
	})

	t.Run("eligible assessment permits full baseline weight", func(t *testing.T) {
		cf := &assessmentCapitalFlow{
			score: 1,
			assessment: capitalflow.CapitalFlowAssessment{
				CalibrationStatus: capitalflow.CalibrationEligible,
			},
		}
		p := testPredictor()
		p.SetCapitalFlow(cf)
		report := p.Predict(now)

		if cf.assessmentCalls != 1 {
			t.Errorf("LatestAssessment calls = %d, want 1", cf.assessmentCalls)
		}
		if cf.qualityCalls != 1 {
			t.Errorf("QualityScore calls = %d, want 1 after assessment becomes eligible", cf.qualityCalls)
		}
		if !strings.Contains(report.Summary, "當前資金品質偏多") {
			t.Errorf("eligible baseline missing from prediction summary: %q", report.Summary)
		}
	})
}

func Test_PredictDay_BullishDrivers_ReturnsInflow(t *testing.T) {
	p := testPredictor()
	day := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	timeline := []industry.CalendarEvent{
		{
			Name: "MSCI Rebalance", EventType: string(industry.EventMSCIRebalance),
			Direction: "bullish", BaseWeight: 0.6,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			Name: "ETF Inflow", EventType: string(industry.EventTaiwan50Rebalance),
			Direction: "bullish", BaseWeight: 0.5,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	dir, conf, drivers := p.predictDay(day, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)

	if dir != "inflow" {
		t.Errorf("expected inflow, got %s", dir)
	}
	if conf <= 0.5 {
		t.Errorf("expected confidence > 0.5, got %.4f", conf)
	}
	if len(drivers) != 2 {
		t.Errorf("expected 2 drivers, got %d: %v", len(drivers), drivers)
	}
}

func Test_PredictDay_BearishDrivers_ReturnsOutflow(t *testing.T) {
	p := testPredictor()
	day := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	timeline := []industry.CalendarEvent{
		{
			Name: "ETF Outflow", EventType: string(industry.EventExDividend),
			Direction: "bearish", BaseWeight: 0.8,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			Name: "Sell Pressure", EventType: "sell_pressure",
			Direction: "bearish", BaseWeight: 0.4,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	dir, conf, drivers := p.predictDay(day, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)

	if dir != "outflow" {
		t.Errorf("expected outflow, got %s", dir)
	}
	if conf <= 0.5 {
		t.Errorf("expected confidence > 0.5, got %.4f", conf)
	}
	if len(drivers) != 2 {
		t.Errorf("expected 2 drivers, got %d: %v", len(drivers), drivers)
	}
}

func Test_PredictDay_NeutralNet_ReturnsNeutral(t *testing.T) {
	p := testPredictor()
	day := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	timeline := []industry.CalendarEvent{
		{
			Name: "Balanced Event", EventType: "balanced",
			Direction: "bullish", BaseWeight: 0.2,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			Name: "Counter Event", EventType: "counter",
			Direction: "bearish", BaseWeight: 0.15,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	dir, conf, drivers := p.predictDay(day, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)

	if dir != "neutral" {
		t.Errorf("expected neutral, got %s", dir)
	}
	if conf <= 0 || conf > 0.6 {
		t.Errorf("expected confidence in (0,0.6], got %.4f", conf)
	}
	if len(drivers) != 2 {
		t.Errorf("expected 2 drivers, got %d: %v", len(drivers), drivers)
	}
}

func Test_PredictDay_MixedDirectionEvent_ReducesWeight(t *testing.T) {
	p := testPredictor()
	day := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	timeline := []industry.CalendarEvent{
		{
			Name: "Dividend Mixed", EventType: string(industry.EventExDividend),
			Direction: "mixed", BaseWeight: 1.0,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	dir, conf, drivers := p.predictDay(day, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)

	// mixed: bullishWeight += 1.0*0.3 = 0.3, bearishWeight += 0.3
	// net = 0.3 - 0.3 = 0 → neutral
	if dir != "neutral" {
		t.Errorf("expected neutral for mixed-only event, got %s", dir)
	}
	if conf != 0.5 {
		t.Errorf("expected sigmoid(0)=0.5 for zero net, got %.4f", conf)
	}
	// mixed events don't append driver names in predictDay (see predictor.go:111-113)
	if len(drivers) != 0 {
		t.Errorf("expected 0 drivers for mixed event, got %d", len(drivers))
	}
}

func Test_PredictDay_DayOutOfRange_NoDrivers(t *testing.T) {
	p := testPredictor()
	timeline := []industry.CalendarEvent{
		{
			Name: "Future Event", EventType: "future",
			Direction: "bullish", BaseWeight: 0.8,
			StartDate: time.Date(2025, 7, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 7, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	t.Run("before_start", func(t *testing.T) {
		day := time.Date(2025, 7, 5, 0, 0, 0, 0, time.UTC)
		dir, conf, drivers := p.predictDay(day, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)
		if dir != "neutral" {
			t.Errorf("expected neutral before event, got %s", dir)
		}
		if conf != 0.5 {
			t.Errorf("expected 0.5 confidence, got %.4f", conf)
		}
		if len(drivers) != 0 {
			t.Errorf("expected 0 drivers before event, got %d", len(drivers))
		}
	})

	t.Run("after_end", func(t *testing.T) {
		day := time.Date(2025, 7, 25, 0, 0, 0, 0, time.UTC)
		dir, conf, drivers := p.predictDay(day, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)
		if dir != "neutral" {
			t.Errorf("expected neutral after event, got %s", dir)
		}
		if conf != 0.5 {
			t.Errorf("expected 0.5 confidence, got %.4f", conf)
		}
		if len(drivers) != 0 {
			t.Errorf("expected 0 drivers after event, got %d", len(drivers))
		}
	})
}

func Test_PredictDay_CFScoreTiltsDirection(t *testing.T) {
	p := testPredictor()
	day := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	barelyBullish := []industry.CalendarEvent{
		{
			Name: "Small Bullish", EventType: "small_bull",
			Direction: "bullish", BaseWeight: 0.25,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	t.Run("neutral_without_cf", func(t *testing.T) {
		dir, _, _ := p.predictDay(day, barelyBullish, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)
		if dir != "neutral" {
			t.Errorf("expected neutral without cfScore, got %s", dir)
		}
	})

	t.Run("inflow_with_positive_cf", func(t *testing.T) {
		dir, _, _ := p.predictDay(day, barelyBullish, scaleQualityScoreToBaseline(0.5), capitalflow.CalibrationEligible, 0)
		if dir != "inflow" {
			t.Errorf("expected inflow with cfScore=0.5, got %s", dir)
		}
	})

	t.Run("outflow_with_negative_cf", func(t *testing.T) {
		barelyBearish := []industry.CalendarEvent{
			{
				Name: "Small Bearish", EventType: "small_bear",
				Direction: "bearish", BaseWeight: 0.25,
				StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
			},
		}
		dir, _, _ := p.predictDay(day, barelyBearish, scaleQualityScoreToBaseline(-0.5), capitalflow.CalibrationEligible, 0)
		if dir != "outflow" {
			t.Errorf("expected outflow with cfScore=-0.5, got %s", dir)
		}
	})

	t.Run("via_SetCapitalFlow", func(t *testing.T) {
		cf := &staticCF{score: 0.6, label: "mild_inflow"}
		p.SetCapitalFlow(cf)
		dir, _, _ := p.predictDay(day, barelyBullish, scaleQualityScoreToBaseline(cf.QualityScore()), capitalflow.CalibrationEligible, 0)
		if dir != "inflow" {
			t.Errorf("expected inflow after SetCapitalFlow(0.6), got %s", dir)
		}
	})
}

func TestPredict_CalibratingBaselineConflictsWithEvents(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(now)

	cf := &assessmentCapitalFlow{
		score: -1.32,
		assessment: capitalflow.CapitalFlowAssessment{
			CalibrationStatus: capitalflow.CalibrationCalibrating,
		},
	}
	p := testPredictor()
	p.SetCapitalFlow(cf)
	report := p.Predict(now)

	// Confidence must be capped while calibrating.
	for _, pred := range report.Predictions {
		if pred.Confidence > calibratingConfidenceCap {
			t.Errorf("calibrating prediction confidence %.4f exceeds cap %.4f", pred.Confidence, calibratingConfidenceCap)
		}
	}

	// Summary must surface the current capital-flow direction and uncertainty.
	if !strings.Contains(report.Summary, "偏空") {
		t.Errorf("summary missing bearish baseline note: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "校準中") {
		t.Errorf("summary missing calibration uncertainty note: %q", report.Summary)
	}
}

func TestPredict_EligibleBaselineCanFlipWeakEvents(t *testing.T) {
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(now)

	cf := &assessmentCapitalFlow{
		score: -3.0,
		assessment: capitalflow.CapitalFlowAssessment{
			CalibrationStatus: capitalflow.CalibrationEligible,
		},
	}
	p := testPredictor()
	p.SetCapitalFlow(cf)
	report := p.Predict(now)

	outflowDays := 0
	for _, pred := range report.Predictions {
		if pred.Direction == "outflow" {
			outflowDays++
		}
	}
	// With a strong eligible bearish baseline, at least the near-term day(s)
	// should reflect selling pressure even when the calendar is bullish.
	if outflowDays == 0 {
		t.Errorf("eligible strong bearish baseline produced no outflow days; directions=%v", report.Predictions)
	}
}

func Test_ForcesForDirection_VariousKeywords(t *testing.T) {
	tests := []struct {
		name    string
		drivers []string
		want    []string
	}{
		{
			name:    "msci_and_foreign",
			drivers: []string{"MSCI Quarterly Rebalance", "外資買超"},
			want:    []string{"foreign"},
		},
		{
			name:    "etf_and_rebalance",
			drivers: []string{"ETF Rebalance", "換股"},
			want:    []string{"institutional"},
		},
		{
			name:    "revenue_report",
			drivers: []string{"月營收"},
			want:    []string{"foreign"},
		},
		{
			name:    "monthly_revenue_report_english",
			drivers: []string{"Monthly Revenue Report"},
			want:    []string{"foreign"},
		},
		{
			name:    "quarterly_revenue_english",
			drivers: []string{"Quarterly Revenue Update"},
			want:    []string{"foreign"},
		},
		{
			name:    "dealer_and_institutional",
			drivers: []string{"季底做帳行情", "window_dressing"},
			want:    []string{"dealer", "institutional"},
		},
		{
			name:    "retail_margin",
			drivers: []string{"融資餘額增加", "散戶進場"},
			want:    []string{"retail"},
		},
		{
			name:    "taiwan50_keyword",
			drivers: []string{"0050 taiwan50 rebalance"},
			want:    []string{"institutional"},
		},
		{
			name:    "fund_keyword",
			drivers: []string{"基金申購"},
			want:    []string{"institutional"},
		},
		{
			name:    "multiple_categories",
			drivers: []string{"MSCI Rebalance", "ETF Rebalance", "散戶恐慌"},
			want:    []string{"foreign", "institutional", "retail"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := forcesForDirection(tc.drivers)
			if len(got) != len(tc.want) {
				t.Fatalf("forcesForDirection(%v) = %v, want %v (len mismatch)", tc.drivers, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("forcesForDirection(%v) = %v, want %v", tc.drivers, got, tc.want)
				}
			}
		})
	}
}

func Test_Sigmoid_MidRangeValues(t *testing.T) {
	tests := []struct {
		x    float64
		want float64
	}{
		{x: 0, want: 0.5},
		{x: 1, want: 1 / (1 + math.Exp(-1))},
		{x: 2, want: 1 / (1 + math.Exp(-2))},
		{x: 5, want: 1 / (1 + math.Exp(-5))},
		{x: -1, want: 1 / (1 + math.Exp(1))},
		{x: -2, want: 1 / (1 + math.Exp(2))},
		{x: -5, want: 1 / (1 + math.Exp(5))},
		{x: 10, want: 1.0},
		{x: -10, want: 0.0},
	}
	for _, tc := range tests {
		t.Run(formatSigmoidCase(tc.x), func(t *testing.T) {
			got := sigmoid(tc.x)
			if math.Abs(got-tc.want) > 1e-4 {
				t.Errorf("sigmoid(%v) = %.6f, want %.6f", tc.x, got, tc.want)
			}
		})
	}
}

func formatSigmoidCase(x float64) string {
	switch {
	case x < 0:
		return "neg"
	case x == 0:
		return "zero"
	default:
		return "pos"
	}
}

func Test_GuessETFName_KnownNames(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "0050 臺灣50 ETF 調整", want: "0050 臺灣50"},
		{name: "臺灣50", want: "0050 臺灣50"},
		{name: "台灣50成分股調整", want: "0050 臺灣50"},
		{name: "0050", want: "0050 臺灣50"},
		{name: "0056 高股息ETF", want: "0056 高股息"},
		{name: "高股息調整", want: "0056 高股息"},
		// 00878 contains "高股息" so longest-prefix lookup must hit 00878 entry first
		{name: "00878 永續高股息", want: "00878 永續高股息"},
		{name: "00878", want: "00878 永續高股息"},
		{name: "永續ETF調整", want: "00878 永續高股息"},
		{name: "unknown market event", want: ""},
		{name: "", want: ""},
		{name: "00880", want: ""},
		{name: "00878 永續高股息 成分股調整", want: "00878 永續高股息"},
		{name: "高股息 成分股調整", want: "0056 高股息"},
		{name: "0050 臺灣50 成分股調整", want: "0050 臺灣50"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := guessETFName(tc.name)
			if got != tc.want {
				t.Errorf("guessETFName(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func Test_BuildETFEstimates_Taiwan50_ReturnsEstimates(t *testing.T) {
	p := testPredictor()
	timeline := []industry.CalendarEvent{
		{
			Name: "0050 臺灣50半年調整", EventType: string(industry.EventTaiwan50Rebalance),
			Direction: "bullish", BaseWeight: 0.8,
			StartDate: time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	estimates := p.buildETFEstimates(timeline)

	if len(estimates) != 3 {
		t.Fatalf("expected 3 ETF estimates, got %d", len(estimates))
	}
	for _, est := range estimates {
		if est.ETFName != "0050 臺灣50" {
			t.Errorf("expected ETFName=0050 臺灣50, got %s", est.ETFName)
		}
		if est.ETFAUM != 380 {
			t.Errorf("expected AUM=380, got %.0f", est.ETFAUM)
		}
		if est.EstFlow <= 0 {
			t.Errorf("expected positive EstFlow, got %.0f", est.EstFlow)
		}
	}
}

func Test_BuildETFEstimates_CustomETF_ReturnsEstimates(t *testing.T) {
	p := testPredictor()
	timeline := []industry.CalendarEvent{
		{
			Name: "0056 高股息 ETF 成分股調整", EventType: "custom_etf_rebalance",
			Direction: "bullish", BaseWeight: 0.6,
			StartDate: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 7, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			Name: "Regular Market Event", EventType: "market_update",
			Direction: "neutral", BaseWeight: 0.3,
			StartDate: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 7, 10, 0, 0, 0, 0, time.UTC),
		},
	}

	estimates := p.buildETFEstimates(timeline)

	if len(estimates) != 3 {
		t.Fatalf("expected 3 ETF estimates for 0056 event, got %d", len(estimates))
	}
	for _, est := range estimates {
		if est.ETFName != "0056 高股息" {
			t.Errorf("expected ETFName=0056 高股息, got %s", est.ETFName)
		}
		if est.ETFAUM != 320 {
			t.Errorf("expected AUM=320 for 0056, got %.0f", est.ETFAUM)
		}
	}
}

func Test_BuildETFEstimates_NonETFEvent_ReturnsEmpty(t *testing.T) {
	p := testPredictor()
	timeline := []industry.CalendarEvent{
		{
			Name: "Regular Event", EventType: string(industry.EventSpringFestival),
			Direction: "neutral", BaseWeight: 0.5,
			StartDate: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2025, 2, 5, 0, 0, 0, 0, time.UTC),
		},
	}

	estimates := p.buildETFEstimates(timeline)
	if len(estimates) != 0 {
		t.Errorf("expected 0 estimates for non-ETF event, got %d", len(estimates))
	}
}

func Test_EvaluateRevenueSurprise_ThresholdBoundaries(t *testing.T) {
	p := testPredictor()

	tests := []struct {
		name       string
		event      industry.CalendarEvent
		wantImpact string
		wantSymbol string
		wantPct    float64
	}{
		{
			name: "bullish_above_10pct",
			event: industry.CalendarEvent{
				EventType: string(industry.EventMonthlyRevenue),
				Name:      "台積電營收", BaseWeight: 0.3,
				SentimentAdjustment: 0.15,
				AffectedIndustries:  []string{"2330"},
			},
			wantImpact: "bullish", wantSymbol: "2330", wantPct: 0.15,
		},
		{
			name: "bearish_below_negative_10pct",
			event: industry.CalendarEvent{
				EventType: string(industry.EventMonthlyRevenue),
				Name:      "營收衰退", BaseWeight: 0.3,
				SentimentAdjustment: -0.2,
				AffectedIndustries:  []string{"2317"},
			},
			wantImpact: "bearish", wantSymbol: "2317", wantPct: -0.2,
		},
		{
			name: "neutral_within_10pct_positive",
			event: industry.CalendarEvent{
				EventType: string(industry.EventMonthlyRevenue),
				Name:      "小幅成長", BaseWeight: 0.3,
				SentimentAdjustment: 0.05,
				AffectedIndustries:  []string{"2454"},
			},
			wantImpact: "neutral", wantSymbol: "2454", wantPct: 0.05,
		},
		{
			name: "neutral_within_10pct_negative",
			event: industry.CalendarEvent{
				EventType: string(industry.EventMonthlyRevenue),
				Name:      "小幅衰退", BaseWeight: 0.3,
				SentimentAdjustment: -0.05,
				AffectedIndustries:  []string{"2412"},
			},
			wantImpact: "neutral", wantSymbol: "2412", wantPct: -0.05,
		},
		{
			name: "high_weight_amplifies_surprise",
			event: industry.CalendarEvent{
				EventType: string(industry.EventMonthlyRevenue),
				Name:      "權值股營收", BaseWeight: 0.6,
				SentimentAdjustment: 0.07,
				AffectedIndustries:  []string{"2330"},
			},
			// BaseWeight > 0.5: actual = 100 * (1 + 0.07*1.5) = 110.5
			// surprisePct = (110.5 - 100) / 100 = 0.105 > 0.1 → bullish
			wantImpact: "bullish", wantSymbol: "2330", wantPct: 0.105,
		},
		{
			name: "no_affected_industries_uses_empty_symbol",
			event: industry.CalendarEvent{
				EventType: string(industry.EventMonthlyRevenue),
				Name:      "未分類營收", BaseWeight: 0.3,
				SentimentAdjustment: 0.2,
			},
			wantImpact: "bullish", wantSymbol: "", wantPct: 0.2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rs := p.evaluateRevenueSurprise(tc.event)
			if rs == nil {
				t.Fatal("expected non-nil RevenueSurprise")
			}
			if rs.FlowImpact != tc.wantImpact {
				t.Errorf("expected FlowImpact=%q, got %q", tc.wantImpact, rs.FlowImpact)
			}
			if rs.StockSymbol != tc.wantSymbol {
				t.Errorf("expected StockSymbol=%q, got %q", tc.wantSymbol, rs.StockSymbol)
			}
			if math.Abs(rs.SurprisePct-tc.wantPct) > 1e-4 {
				t.Errorf("expected SurprisePct=%.6f, got %.6f", tc.wantPct, rs.SurprisePct)
			}
		})
	}
}

func Test_BuildRevenueSurprises_FiltersNonRevenueEvents(t *testing.T) {
	p := testPredictor()
	timeline := []industry.CalendarEvent{
		{
			EventType: string(industry.EventTaiwan50Rebalance),
			Name:      "ETF Rebalance", BaseWeight: 0.5,
			SentimentAdjustment: 0.2,
			StartDate:           time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:             time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			EventType: string(industry.EventMonthlyRevenue),
			Name:      "營收公告", BaseWeight: 0.3,
			SentimentAdjustment: 0.2,
			AffectedIndustries:  []string{"2330"},
			StartDate:           time.Date(2025, 6, 10, 0, 0, 0, 0, time.UTC),
			EndDate:             time.Date(2025, 6, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	surprises := p.buildRevenueSurprises(timeline)
	if len(surprises) != 1 {
		t.Fatalf("expected 1 revenue surprise, got %d", len(surprises))
	}
	if surprises[0].StockSymbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", surprises[0].StockSymbol)
	}
}

func Test_BuildPredictionSummary_AllBranches(t *testing.T) {
	tests := []struct {
		name        string
		predictions []FlowPrediction
		active      []industry.CalendarEvent
		cfScore     float64
		cfStatus    string
		wantWords   []string
	}{
		{
			name: "dominant_inflow_with_events_and_positive_cf",
			predictions: []FlowPrediction{
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "outflow"},
				{Direction: "neutral"},
			},
			active:    []industry.CalendarEvent{{Name: "MSCI調整"}},
			cfScore:   0.6,
			cfStatus:  capitalflow.CalibrationEligible,
			wantWords: []string{"偏流入", "MSCI調整", "偏多"},
		},
		{
			name: "dominant_outflow_with_events_and_negative_cf",
			predictions: []FlowPrediction{
				{Direction: "outflow"},
				{Direction: "outflow"},
				{Direction: "outflow"},
				{Direction: "inflow"},
				{Direction: "neutral"},
			},
			active:    []industry.CalendarEvent{{Name: "外資賣超"}},
			cfScore:   -0.6,
			cfStatus:  capitalflow.CalibrationEligible,
			wantWords: []string{"偏流出", "外資賣超", "偏空"},
		},
		{
			name: "divergence_no_dominant",
			predictions: []FlowPrediction{
				{Direction: "inflow"},
				{Direction: "outflow"},
				{Direction: "neutral"},
				{Direction: "inflow"},
				{Direction: "outflow"},
			},
			active:    nil,
			cfScore:   0,
			cfStatus:  capitalflow.CalibrationEligible,
			wantWords: []string{"分歧"},
		},
		{
			name: "neutral_cf_no_quality_statement",
			predictions: []FlowPrediction{
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
			},
			active:    nil,
			cfScore:   0,
			cfStatus:  capitalflow.CalibrationEligible,
			wantWords: []string{"偏流入"},
		},
		{
			name: "calibrating_shows_uncertainty",
			predictions: []FlowPrediction{
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
				{Direction: "inflow"},
			},
			active:    nil,
			cfScore:   0.6,
			cfStatus:  capitalflow.CalibrationCalibrating,
			wantWords: []string{"校準中", "不確定性"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPredictionSummary(tc.predictions, tc.active, scaleQualityScoreToBaseline(tc.cfScore), tc.cfStatus)
			for _, w := range tc.wantWords {
				if !stringsContains(got, w) {
					t.Errorf("buildPredictionSummary() = %q, want it to contain %q", got, w)
				}
			}
		})
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func Test_Predict_ReturnsFiveDayReport_BasicShape(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	report := p.Predict(now)

	if report.Window != "5-day forward" {
		t.Errorf("Window = %q, want %q", report.Window, "5-day forward")
	}
	if !report.GeneratedAt.Equal(now) {
		t.Errorf("GeneratedAt = %v, want %v", report.GeneratedAt, now)
	}
	if len(report.Predictions) != 5 {
		t.Fatalf("len(Predictions) = %d, want 5", len(report.Predictions))
	}
	for i, pred := range report.Predictions {
		wantDate := now.AddDate(0, 0, i+1)
		if !pred.Date.Equal(wantDate) {
			t.Errorf("prediction[%d].Date = %v, want %v (T+%d day)",
				i, pred.Date, wantDate, i+1)
		}
	}
}

func Test_Predict_PredictionDirectionAndConfidenceAreValid(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	report := p.Predict(now)

	validDirs := map[string]bool{"inflow": true, "outflow": true, "neutral": true}
	for i, pred := range report.Predictions {
		if !validDirs[pred.Direction] {
			t.Errorf("prediction[%d].Direction = %q, want inflow|outflow|neutral",
				i, pred.Direction)
		}
		if pred.Confidence < 0 || pred.Confidence > 1 {
			t.Errorf("prediction[%d].Confidence = %.4f, want [0, 1]",
				i, pred.Confidence)
		}
		if pred.DrivingEvents == nil {
			t.Errorf("prediction[%d].DrivingEvents is nil, want empty slice", i)
		}
		if pred.PredictedForces == nil {
			t.Errorf("prediction[%d].PredictedForces is nil, want empty slice", i)
		}
	}
}

func Test_Predict_AuxFieldsAreStableSlicesAndString(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	report := p.Predict(now)

	if len(report.Predictions) != 5 {
		t.Error("Predictions should have 5 entries")
	}
	if report.ActiveEvents == nil {
		t.Error("ActiveEvents is nil, want slice (may be empty)")
	}
	if report.ETFEstimates == nil {
		t.Error("ETFEstimates is nil, want slice (may be empty)")
	}
	if report.RevenueSurprises == nil {
		t.Error("RevenueSurprises is nil, want slice (may be empty)")
	}
	if report.Summary == "" {
		t.Error("Summary is empty, want non-empty string from buildPredictionSummary")
	}
	if _, ok := any(report.Summary).(string); !ok {
		t.Errorf("Summary type = %T, want string", report.Summary)
	}
}

func Test_Predict_JSONMarshalOutput_HasNoNullArrays(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	report := p.Predict(now)
	jsonBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	js := string(jsonBytes)
	nullPatterns := []string{
		`"driving_events":null`,
		`"predicted_forces":null`,
		`"active_events":null`,
		`"etf_estimates":null`,
		`"revenue_surprises":null`,
	}
	for _, np := range nullPatterns {
		if strings.Contains(js, np) {
			t.Errorf("Predict() JSON output contains null array: %s\nfull output: %s", np, js)
		}
	}
}

func TestEffectiveConfidence(t *testing.T) {
	cases := []struct {
		backfilled bool
		weight     float64
		want       float64
	}{
		{false, 1.0, 1.0},
		{true, 1.0, 0.7},
		{true, 0.5, 0.35},
		{false, 0.0, 0.0},
		{true, 0.0, 0.0},
	}
	for _, tc := range cases {
		e := industry.CalendarEvent{BaseWeight: tc.weight, Backfilled: tc.backfilled}
		if got := effectiveConfidence(e); got != tc.want {
			t.Errorf("effectiveConfidence(weight=%v, backfilled=%v) = %v, want %v",
				tc.weight, tc.backfilled, got, tc.want)
		}
	}
}

func TestPredictDay_BackfilledBullishDriver_NetHalfOfNonBackfilled(t *testing.T) {
	p := NewPredictor(nil)
	now := time.Now()
	end := now.AddDate(0, 0, 1)

	timeline := []industry.CalendarEvent{
		{
			Name: "non_backfilled", Direction: "bullish", BaseWeight: 1.0,
			StartDate: now, EndDate: end, Backfilled: false,
		},
		{
			Name: "backfilled", Direction: "bullish", BaseWeight: 1.0,
			StartDate: now, EndDate: end, Backfilled: true,
		},
	}

	dir, conf, drivers := p.predictDay(now, timeline, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)
	if len(drivers) != 2 {
		t.Errorf("expected 2 drivers, got %d: %v", len(drivers), drivers)
	}
	if conf <= 0 {
		t.Errorf("expected positive confidence, got %f", conf)
	}
	_ = dir

	timelineBackfilledOnly := []industry.CalendarEvent{
		{
			Name: "backfilled", Direction: "bullish", BaseWeight: 1.0,
			StartDate: now, EndDate: end, Backfilled: true,
		},
	}
	dirBF, confBF, _ := p.predictDay(now, timelineBackfilledOnly, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)
	_ = dirBF
	if confBF >= conf {
		t.Errorf("backfilled-only confidence (%f) should be lower than mixed confidence (%f)", confBF, conf)
	}

	timelineNonBackfilledOnly := []industry.CalendarEvent{
		{
			Name: "non_backfilled", Direction: "bullish", BaseWeight: 1.0,
			StartDate: now, EndDate: end, Backfilled: false,
		},
	}
	_, confNB, _ := p.predictDay(now, timelineNonBackfilledOnly, scaleQualityScoreToBaseline(0), capitalflow.CalibrationEligible, 0)
	if confBF*1.5 < confNB {
		t.Errorf("expected backfilled confidence (%f) to be roughly 0.7x non-backfilled (%f)", confBF, confNB)
	}
}

// C06: 確保 API 輸出總是包含 etf_estimates 與 revenue_surprises 欄位
// （移除 omitempty）,即使無資料也序列化為 [] 而非缺欄位或 null。
func Test_Predict_JSONMarshal_C06_ETFEstimatesAndRevenueSurprisesAlwaysPresent(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	report := p.Predict(now)
	js, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	out := string(js)

	wantKeys := []string{`"etf_estimates"`, `"revenue_surprises"`}
	for _, k := range wantKeys {
		if !strings.Contains(out, k) {
			t.Errorf("Predict() JSON missing required key %s\nfull output: %s", k, out)
		}
	}

	// 無資料時為 [] 而不是 null
	for _, nullPattern := range []string{`"etf_estimates":null`, `"revenue_surprises":null`} {
		if strings.Contains(out, nullPattern) {
			t.Errorf("Predict() JSON contains %s, want [] instead\nfull output: %s", nullPattern, out)
		}
	}
}

// C06: 當有 ETF rebalance event 時,etf_estimates 應該序列化為 array of objects,每筆
// 包含 etf_name/stock_symbol/direction/est_flow 等欄位（驗證 round-trip）。
func Test_Predict_JSONMarshal_C06_ETFEstimatesPopulatedRoundTrip(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	timeline := []industry.CalendarEvent{
		{
			Name: "元大台灣50 季配", EventType: "etf_rebalance", Direction: "neutral",
			StartDate: now.AddDate(0, 0, 1), EndDate: now.AddDate(0, 0, 1),
			BaseWeight: 1.0, AffectedIndustries: []string{"2330"},
		},
	}

	report := p.Predict(now)
	// 直接呼叫 buildETFEstimates 觀察 round-trip,不依賴 Predict 中的 timeline 注入。
	estimates := p.buildETFEstimates(timeline)
	report.ETFEstimates = estimates

	js, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	out := string(js)

	if !strings.Contains(out, `"etf_estimates":[`) {
		t.Errorf("expected etf_estimates to be a non-empty array, got: %s", out)
	}
	if !strings.Contains(out, `"etf_name"`) {
		t.Errorf("expected nested etf_name field, got: %s", out)
	}
	if !strings.Contains(out, `"stock_symbol"`) {
		t.Errorf("expected nested stock_symbol field, got: %s", out)
	}
}

func Test_Predict_SectorPredictionsAlwaysPresent(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	report := p.Predict(now)

	if report.SectorPredictions == nil {
		t.Fatal("SectorPredictions must not be nil (always present, no omitempty)")
	}
	// Without SectorPredictor wired, the field should be empty.
	if len(report.SectorPredictions) != 0 {
		t.Errorf("expected empty SectorPredictions, got %d entries", len(report.SectorPredictions))
	}
}

func Test_Predict_JSONHasSectorPredictions(t *testing.T) {
	p := testPredictor()
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	report := p.Predict(now)

	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"sector_predictions"`) {
		t.Error("JSON output must contain 'sector_predictions' key")
	}
}
