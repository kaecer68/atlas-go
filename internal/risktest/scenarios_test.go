package risktest

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestRunMarketCrash(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantScenario string
		wantPassed   bool
		wantAction   string
	}{
		{
			name:         "default config passes with severe action",
			cfg:          Config{},
			wantScenario: "market_crash",
			wantPassed:   true,
			wantAction:   "severe",
		},
		{
			name: "green macro risk fails because no severe drawdown",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"market_crash": {MacroRiskLevel: "green"},
				},
			},
			wantScenario: "market_crash",
			wantPassed:   false,
			wantAction:   "none",
		},
		{
			name: "foreign outflow probability override still passes",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"market_crash": {ForeignOutflowProb: 60.0},
				},
			},
			wantScenario: "market_crash",
			wantPassed:   true,
			wantAction:   "severe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunMarketCrash(tt.cfg)

			if got.Scenario != tt.wantScenario {
				t.Errorf("Scenario = %q, want %q", got.Scenario, tt.wantScenario)
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", got.Passed, tt.wantPassed)
			}
			if !strings.EqualFold(got.Results.DrawdownAction, tt.wantAction) {
				t.Errorf("DrawdownAction = %q, want %q", got.Results.DrawdownAction, tt.wantAction)
			}
			if got.Results.RiskSnapshot == nil {
				t.Error("RiskSnapshot is nil")
			}
			if !got.Results.VaRBreach {
				t.Error("expected VaRBreach to be true")
			}
			if !got.Results.RiskGateHalt {
				t.Error("expected RiskGateHalt to be true")
			}
			if len(got.Results.Breakdown) == 0 {
				t.Error("expected non-empty breakdown")
			}
		})
	}
}

func TestRunSectorRotation(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantScenario string
		wantPassed   bool
		wantAction   string
	}{
		{
			name:         "default config passes with escalation",
			cfg:          Config{},
			wantScenario: "sector_rotation",
			wantPassed:   true,
			wantAction:   "severe",
		},
		{
			name: "custom portfolio weights still pass",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"sector_rotation": {
						PortfolioWeights: map[string]float64{
							"2330.TW": 0.30,
							"2454.TW": 0.20,
							"2412.TW": 0.30,
							"4904.TW": 0.20,
						},
					},
				},
			},
			wantScenario: "sector_rotation",
			wantPassed:   true,
			wantAction:   "severe",
		},
		{
			name: "empty portfolio weights falls back to defaults and passes",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"sector_rotation": {PortfolioWeights: map[string]float64{}},
				},
			},
			wantScenario: "sector_rotation",
			wantPassed:   true,
			wantAction:   "severe",
		},
		{
			name: "low recession percentage yields moderate action but may fail defensive check",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"sector_rotation": {RecessionIndustryPct: 0.10},
				},
			},
			wantScenario: "sector_rotation",
			wantPassed:   false,
			wantAction:   "moderate",
		},
		{
			name: "custom cyclical and defensive symbols",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"sector_rotation": {
						CyclicalSymbols:  []string{"2330.TW"},
						DefensiveSymbols: []string{"2412.TW"},
					},
				},
			},
			wantScenario: "sector_rotation",
			wantPassed:   true,
			wantAction:   "severe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunSectorRotation(tt.cfg)

			if got.Scenario != tt.wantScenario {
				t.Errorf("Scenario = %q, want %q", got.Scenario, tt.wantScenario)
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", got.Passed, tt.wantPassed)
			}
			if !strings.EqualFold(got.Results.DrawdownAction, tt.wantAction) {
				t.Errorf("DrawdownAction = %q, want %q", got.Results.DrawdownAction, tt.wantAction)
			}
			if len(got.Results.Breakdown) == 0 {
				t.Error("expected non-empty breakdown")
			}
		})
	}
}

func TestRunSectorRotation_CyclicalWeightsDoNotIncreaseInSevere(t *testing.T) {
	cfg := Config{
		Scenarios: map[string]ScenarioConfig{
			"sector_rotation": {
				PortfolioWeights: map[string]float64{
					"2330.TW": 0.25,
					"2454.TW": 0.15,
					"3008.TW": 0.10,
					"2308.TW": 0.10,
					"2412.TW": 0.15,
					"4904.TW": 0.10,
					"2881.TW": 0.10,
					"2891.TW": 0.05,
				},
			},
		},
	}

	result := RunSectorRotation(cfg)

	if !result.Passed {
		t.Fatalf("expected scenario to pass, got %v", result.Passed)
	}

	adjustments := make(map[string]WeightAdjustment)
	for _, adj := range result.Results.WeightAdjustments {
		if adj.Symbol != "" {
			adjustments[adj.Symbol] = adj
		}
	}

	cyclical := []string{"2330.TW", "2454.TW", "3008.TW", "2308.TW"}
	for _, sym := range cyclical {
		adj, ok := adjustments[sym]
		if !ok {
			continue
		}
		if adj.After > adj.Before {
			t.Errorf("cyclical symbol %s increased from %.4f to %.4f", sym, adj.Before, adj.After)
		}
	}
}

func TestRunLiquidityCrisis(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		wantScenario string
		wantPassed   bool
		wantAction   string
	}{
		{
			name:         "default config passes with severe action",
			cfg:          Config{},
			wantScenario: "liquidity_crisis",
			wantPassed:   true,
			wantAction:   "severe",
		},
		{
			name: "high volatility boost still passes",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"liquidity_crisis": {VolatilityBoost: 5.0},
				},
			},
			wantScenario: "liquidity_crisis",
			wantPassed:   true,
			wantAction:   "severe",
		},
		{
			name: "low portfolio risk score yields moderate action and fails severe threshold",
			cfg: Config{
				Scenarios: map[string]ScenarioConfig{
					"liquidity_crisis": {PortfolioRiskScore: 0.50},
				},
			},
			wantScenario: "liquidity_crisis",
			wantPassed:   false,
			wantAction:   "moderate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunLiquidityCrisis(tt.cfg)

			if got.Scenario != tt.wantScenario {
				t.Errorf("Scenario = %q, want %q", got.Scenario, tt.wantScenario)
			}
			if got.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", got.Passed, tt.wantPassed)
			}
			if !strings.EqualFold(got.Results.DrawdownAction, tt.wantAction) {
				t.Errorf("DrawdownAction = %q, want %q", got.Results.DrawdownAction, tt.wantAction)
			}
			if got.Results.RiskSnapshot == nil {
				t.Error("RiskSnapshot is nil")
			}
			if !got.Results.VaRBreach {
				t.Error("expected VaRBreach to be true")
			}
			if !got.Results.RiskGateHalt {
				t.Error("expected RiskGateHalt to be true")
			}
			if len(got.Results.Breakdown) == 0 {
				t.Error("expected non-empty breakdown")
			}
		})
	}
}

func TestRunScenario(t *testing.T) {
	tests := []struct {
		name      string
		scenario  string
		wantFound bool
	}{
		{name: "market_crash", scenario: "market_crash", wantFound: true},
		{name: "sector_rotation", scenario: "sector_rotation", wantFound: true},
		{name: "liquidity_crisis", scenario: "liquidity_crisis", wantFound: true},
		{name: "unknown", scenario: "unknown", wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := RunScenario(tt.scenario, Config{})
			if found != tt.wantFound {
				t.Errorf("RunScenario(%q) found = %v, want %v", tt.scenario, found, tt.wantFound)
			}
			if found && got.Scenario != tt.scenario {
				t.Errorf("RunScenario(%q) result.Scenario = %q, want %q", tt.scenario, got.Scenario, tt.scenario)
			}
		})
	}
}

func TestRunAll(t *testing.T) {
	results := RunAll(Config{})
	if len(results) != 3 {
		t.Errorf("RunAll returned %d results, want 3", len(results))
	}
	for _, r := range results {
		if r.Scenario == "" {
			t.Error("expected non-empty scenario name")
		}
	}
}

func TestSummarizeResults(t *testing.T) {
	results := []Result{
		{Scenario: "a", Passed: true},
		{Scenario: "b", Passed: false},
		{Scenario: "c", Passed: true},
	}
	all := SummarizeResults(results)
	if all.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", all.Summary.Total)
	}
	if all.Summary.Passed != 2 {
		t.Errorf("Passed = %d, want 2", all.Summary.Passed)
	}
	if all.Summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", all.Summary.Failed)
	}
	if all.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestGetScenarioCfg(t *testing.T) {
	cfg := Config{
		Scenarios: map[string]ScenarioConfig{
			"market_crash": {MacroRiskLevel: "red"},
		},
	}

	got := getScenarioCfg(cfg, "market_crash")
	if got.MacroRiskLevel != "red" {
		t.Errorf("MacroRiskLevel = %q, want %q", got.MacroRiskLevel, "red")
	}

	got = getScenarioCfg(cfg, "unknown")
	if got.MacroRiskLevel != "" {
		t.Errorf("expected empty ScenarioConfig for unknown scenario, got %+v", got)
	}
}

func TestMockPortfolioRiskProvider(t *testing.T) {
	assessment := &risk.PortfolioRiskAssessment{TotalRiskScore: 0.9}
	provider := &mockPortfolioRiskProvider{assessment: assessment}

	if got := provider.Assess(); got != assessment {
		t.Error("Assess() did not return the configured assessment")
	}
}
