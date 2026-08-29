package calibration

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
)

func makeReturns(n int, scale float64) []float64 {
	returns := make([]float64, n)
	for i := range returns {
		returns[i] = scale * math.Sin(float64(i)*0.1)
	}
	return returns
}

func TestCalibrateModule(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)
	returns := makeReturns(150, 0.02)

	tests := []struct {
		name    string
		module  string
		wantErr bool
	}{
		{"garch", "garch", false},
		{"var", "var", false},
		{"factor", "factor", false},
		{"darwinian", "darwinian", false},
		{"all", "all", false},
		{"unknown", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := CalibrateModule(tt.module, ie, returns, len(returns), cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(results) == 0 {
				t.Fatal("expected results")
			}
		})
	}
}

func TestComputeAutocorrelation(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		lag     int
		want    float64
	}{
		{
			name:    "constant series",
			returns: []float64{1, 1, 1, 1, 1},
			lag:     1,
			want:    0,
		},
		{
			name:    "linear trend",
			returns: []float64{1, 2, 3, 4, 5},
			lag:     1,
			want:    0.4,
		},
		{
			name:    "nil input",
			returns: nil,
			lag:     1,
			want:    0,
		},
		{
			name:    "insufficient length",
			returns: []float64{1, 2},
			lag:     5,
			want:    0,
		},
		{
			name:    "lag zero identity",
			returns: []float64{1, 2, 3, 4, 5},
			lag:     0,
			want:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeAutocorrelation(tt.returns, tt.lag)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalibrateGARCH(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)
	returns := makeReturns(150, 0.02)

	res := CalibrateGARCH(ie, returns, len(returns), cfg)
	if res.Module != "garch" {
		t.Errorf("module = %s, want garch", res.Module)
	}
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}
	if len(res.Parameters) != 3 {
		t.Fatalf("got %d parameters, want 3", len(res.Parameters))
	}
	paths := []string{"garch.omega", "garch.alpha", "garch.beta"}
	for i, p := range res.Parameters {
		if p.Path != paths[i] {
			t.Errorf("param[%d].Path = %s, want %s", i, p.Path, paths[i])
		}
		if p.SampleSize != len(returns) {
			t.Errorf("param[%d].SampleSize = %d, want %d", i, p.SampleSize, len(returns))
		}
	}

	short := makeReturns(10, 0.02)
	resErr := CalibrateGARCH(ie, short, len(short), cfg)
	if len(resErr.Errors) == 0 {
		t.Fatal("expected error for insufficient returns")
	}
}

func TestCalibrateVaR(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)
	returns := makeReturns(100, 0.02)

	res := CalibrateVaR(ie, returns, len(returns), cfg)
	if res.Module != "var" {
		t.Errorf("module = %s, want var", res.Module)
	}
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}
	if len(res.Parameters) != 2 {
		t.Fatalf("got %d parameters, want 2", len(res.Parameters))
	}

	short := makeReturns(10, 0.02)
	resErr := CalibrateVaR(ie, short, len(short), cfg)
	if len(resErr.Errors) == 0 {
		t.Fatal("expected error for insufficient returns")
	}
}

func TestCalibrateFactor(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)
	returns := makeReturns(50, 0.05)

	res := CalibrateFactor(ie, returns, len(returns), cfg)
	if res.Module != "factor" {
		t.Errorf("module = %s, want factor", res.Module)
	}
	if len(res.Parameters) != 2 {
		t.Fatalf("got %d parameters, want 2", len(res.Parameters))
	}
	paths := []string{"factor.momentum_stddev_divisor", "factor.momentum_lookback_days"}
	for i, p := range res.Parameters {
		if p.Path != paths[i] {
			t.Errorf("param[%d].Path = %s, want %s", i, p.Path, paths[i])
		}
	}

	resNil := CalibrateFactor(ie, nil, 0, cfg)
	if len(resNil.Parameters) != 2 {
		t.Fatalf("got %d parameters for nil returns, want 2", len(resNil.Parameters))
	}
}

func TestCalibrateDarwinian(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)

	t.Run("insufficient outcome data", func(t *testing.T) {
		res := CalibrateDarwinian(ie, 100, cfg)
		if res.Module != "darwinian" {
			t.Errorf("module = %s, want darwinian", res.Module)
		}
		if len(res.Errors) == 0 {
			t.Fatal("expected error for insufficient outcome data")
		}
		if len(res.Parameters) != 2 {
			t.Fatalf("got %d parameters, want 2", len(res.Parameters))
		}
	})

	t.Run("with session outcomes", func(t *testing.T) {
		tmp := t.TempDir()
		sessionsDir := filepath.Join(tmp, "data", "state", "sessions")
		if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		sessionDir := filepath.Join(sessionsDir, "session-20240101-daily")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir session: %v", err)
		}

		cfg := config.DefaultParametersConfig()
		cfg.Darwinian.SharpeMinSampleSize.Value = 2
		ie := config.NewInferenceEngine(cfg)

		agents := []string{"agent-a", "agent-b", "agent-c", "agent-d", "agent-e", "agent-f"}
		outcomes := make([]recommendation.RecommendationOutcome, 0)
		for i, agent := range agents {
			hit := i%2 == 0
			for j := range 5 {
				outcomes = append(outcomes, recommendation.RecommendationOutcome{
					AgentID:       agent,
					Symbol:        "2330",
					Side:          "buy",
					Conviction:    5,
					Window:        "1d",
					Hit:           hit,
					ForwardReturn: 0.01*float64(i+1) + 0.001*float64(j),
				})
			}
		}

		f, err := os.Create(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"))
		if err != nil {
			t.Fatalf("create outcomes: %v", err)
		}
		enc := json.NewEncoder(f)
		for _, o := range outcomes {
			if err := enc.Encode(o); err != nil {
				t.Fatalf("encode outcome: %v", err)
			}
		}
		_ = f.Close()

		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		defer func() {
			_ = os.Chdir(wd)
		}()
		if err := os.Chdir(tmp); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		res := CalibrateDarwinian(ie, 100, cfg)
		if len(res.Errors) != 0 {
			t.Fatalf("unexpected errors: %v", res.Errors)
		}
		if len(res.Parameters) == 0 {
			t.Fatal("expected parameters")
		}
	})
}

func TestDarwinianHeuristicDefaults(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	params := darwinianHeuristicDefaults(50, cfg)
	if len(params) != 2 {
		t.Fatalf("got %d defaults, want 2", len(params))
	}
	if params[0].Path != "darwinian.hit_rate_high_threshold" {
		t.Errorf("path = %s, want hit_rate_high_threshold", params[0].Path)
	}
	if params[1].Path != "darwinian.hit_rate_low_threshold" {
		t.Errorf("path = %s, want hit_rate_low_threshold", params[1].Path)
	}
	for _, p := range params {
		if p.Before != p.After {
			t.Errorf("%s before != after: %v vs %v", p.Path, p.Before, p.After)
		}
		if p.SampleSize != 50 {
			t.Errorf("%s sample size = %d, want 50", p.Path, p.SampleSize)
		}
	}
}
