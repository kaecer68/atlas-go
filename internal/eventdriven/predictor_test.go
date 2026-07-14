package eventdriven

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

func testPredictor() *Predictor {
	return NewPredictor(industry.NewEventCalendar())
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

	dir, conf, drivers := p.predictDay(day, timeline, 0)

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

	dir, conf, drivers := p.predictDay(day, timeline, 0)

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

	dir, conf, drivers := p.predictDay(day, timeline, 0)

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

	dir, conf, drivers := p.predictDay(day, timeline, 0)

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
		dir, conf, drivers := p.predictDay(day, timeline, 0)
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
		dir, conf, drivers := p.predictDay(day, timeline, 0)
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
		dir, _, _ := p.predictDay(day, barelyBullish, 0)
		if dir != "neutral" {
			t.Errorf("expected neutral without cfScore, got %s", dir)
		}
	})

	t.Run("inflow_with_positive_cf", func(t *testing.T) {
		dir, _, _ := p.predictDay(day, barelyBullish, 0.5)
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
		dir, _, _ := p.predictDay(day, barelyBearish, -0.5)
		if dir != "outflow" {
			t.Errorf("expected outflow with cfScore=-0.5, got %s", dir)
		}
	})

	t.Run("via_SetCapitalFlow", func(t *testing.T) {
		cf := &staticCF{score: 0.6, label: "mild_inflow"}
		p.SetCapitalFlow(cf)
		dir, _, _ := p.predictDay(day, barelyBullish, cf.QualityScore())
		if dir != "inflow" {
			t.Errorf("expected inflow after SetCapitalFlow(0.6), got %s", dir)
		}
	})
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
		// 00878 contains "高股息" so prefix-order matches 0056 branch first
		{name: "00878 永續高股息", want: "0056 高股息"},
		{name: "00878", want: "00878 永續高股息"},
		{name: "永續ETF調整", want: "00878 永續高股息"},
		{name: "unknown market event", want: ""},
		{name: "", want: ""},
		{name: "00880", want: ""},
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
		wantWords   []string
	}{
		{
			name: "dominant_inflow_with_events_and_positive_cf",
			predictions: []FlowPrediction{
				{Direction: "inflow"}, {Direction: "inflow"}, {Direction: "inflow"},
				{Direction: "outflow"}, {Direction: "neutral"},
			},
			active:    []industry.CalendarEvent{{Name: "MSCI調整"}},
			cfScore:   0.6,
			wantWords: []string{"偏流入", "MSCI調整", "偏多"},
		},
		{
			name: "dominant_outflow_with_events_and_negative_cf",
			predictions: []FlowPrediction{
				{Direction: "outflow"}, {Direction: "outflow"}, {Direction: "outflow"},
				{Direction: "inflow"}, {Direction: "neutral"},
			},
			active:    []industry.CalendarEvent{{Name: "外資賣超"}},
			cfScore:   -0.6,
			wantWords: []string{"偏流出", "外資賣超", "偏空"},
		},
		{
			name: "divergence_no_dominant",
			predictions: []FlowPrediction{
				{Direction: "inflow"}, {Direction: "outflow"},
				{Direction: "neutral"}, {Direction: "inflow"}, {Direction: "outflow"},
			},
			active:    nil,
			cfScore:   0,
			wantWords: []string{"分歧"},
		},
		{
			name: "neutral_cf_no_quality_statement",
			predictions: []FlowPrediction{
				{Direction: "inflow"}, {Direction: "inflow"}, {Direction: "inflow"},
				{Direction: "inflow"}, {Direction: "inflow"},
			},
			active:    nil,
			cfScore:   0,
			wantWords: []string{"偏流入"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPredictionSummary(tc.predictions, tc.active, tc.cfScore)
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
