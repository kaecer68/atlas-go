package paramcheck

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestCountSections(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   int
	}{
		{
			name:   "nil config",
			config: nil,
			want:   0,
		},
		{
			name:   "empty config",
			config: map[string]any{},
			want:   0,
		},
		{
			name: "top-level metadata",
			config: map[string]any{
				"darwinian_weight_min": map[string]any{
					"value":              0.3,
					"calibration_method": "historical_backtest",
					"last_calibrated":    "2026-06-01",
				},
			},
			want: 1,
		},
		{
			name: "nested metadata sections",
			config: map[string]any{
				"garch": map[string]any{
					"alpha": map[string]any{
						"value":              0.1,
						"calibration_method": "mle",
						"last_calibrated":    "2026-06-02",
					},
					"beta": map[string]any{
						"value":                 0.2,
						"calibration_method":    "mle",
						"calibration_timestamp": "2026-06-03",
					},
				},
			},
			want: 2,
		},
		{
			name: "array of metadata sections",
			config: map[string]any{
				"seeds": []any{
					map[string]any{
						"value":              1.0,
						"calibration_method": "manual",
						"last_calibrated":    "2026-06-04",
					},
					map[string]any{
						"value":              2.0,
						"calibration_method": "manual",
						"last_calibrated":    "2026-06-05",
					},
				},
			},
			want: 2,
		},
		{
			name: "citation sub-object without value is not a section",
			config: map[string]any{
				"darwinian_weight_min": map[string]any{
					"value":              0.3,
					"calibration_method": "historical_backtest",
					"last_calibrated":    "2026-06-01",
					"citation": map[string]any{
						"evidence_quality":   "high",
						"calibration_method": "historical_backtest",
					},
				},
			},
			want: 1,
		},
		{
			name: "plain value object is not a section",
			config: map[string]any{
				"max_position_pct": map[string]any{
					"value": 0.25,
				},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountSections(tt.config)
			if got != tt.want {
				t.Errorf("CountSections() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWalkTree(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		strict     bool
		wantWarns  []string
		wantErrors []string
	}{
		{
			name:       "empty config",
			input:      map[string]any{},
			wantWarns:  nil,
			wantErrors: nil,
		},
		{
			name: "valid high-quality calibrated section",
			input: map[string]any{
				"alpha": map[string]any{
					"value": 0.1,
					"citation": map[string]any{
						"evidence_quality": "high",
					},
					"calibration_method": "mle",
					"last_calibrated":    "2026-06-01",
				},
			},
			wantWarns:  nil,
			wantErrors: nil,
		},
		{
			name: "high quality missing timestamp",
			input: map[string]any{
				"alpha": map[string]any{
					"value":              0.1,
					"calibration_method": "mle",
					"citation": map[string]any{
						"evidence_quality": "high",
					},
				},
			},
			wantErrors: []string{
				"alpha: citation.evidence_quality=\"high\" but no calibration timestamp (need last_calibrated or calibration_timestamp)",
				"alpha: calibration_method=\"mle\" but no calibration timestamp",
			},
		},
		{
			name: "calibration_method missing timestamp",
			input: map[string]any{
				"alpha": map[string]any{
					"value":              0.1,
					"calibration_method": "mle",
				},
			},
			wantErrors: []string{
				"alpha: calibration_method=\"mle\" but no calibration timestamp",
			},
		},
		{
			name: "timestamp with low quality produces warning",
			input: map[string]any{
				"alpha": map[string]any{
					"value": 0.1,
					"citation": map[string]any{
						"evidence_quality": "low",
					},
					"last_calibrated": "2026-06-01",
				},
			},
			wantWarns: []string{
				"alpha: calibration timestamp exists but citation.evidence_quality=\"low\" (expected 'high' or 'medium' after calibration)",
			},
		},
		{
			name: "timestamp with low quality is an error in strict mode",
			input: map[string]any{
				"alpha": map[string]any{
					"value": 0.1,
					"citation": map[string]any{
						"evidence_quality": "low",
					},
					"last_calibrated": "2026-06-01",
				},
			},
			strict: true,
			wantErrors: []string{
				"alpha: calibration timestamp exists but citation.evidence_quality=\"low\" (expected 'high' or 'medium' after calibration)",
			},
		},
		{
			name: "strict synthetic with real reference warning",
			input: map[string]any{
				"alpha": map[string]any{
					"value": 0.1,
					"citation": map[string]any{
						"source_reference": "real_data.csv",
					},
					"calibration_method":      "mle",
					"last_calibrated":         "2026-06-01",
					"calibration_data_source": "synthetic",
				},
			},
			strict: true,
			wantWarns: []string{
				"alpha: calibration_data_source='synthetic' but citation.source_reference=\"real_data.csv\" — re-run calibrator with --replay for real data",
			},
		},
		{
			name: "evidence_quality alone does not identify a section",
			input: map[string]any{
				"alpha": map[string]any{
					"value": 0.1,
					"citation": map[string]any{
						"evidence_quality": "high",
					},
				},
			},
			wantWarns:  nil,
			wantErrors: nil,
		},
		{
			name: "nested sections accumulate errors",
			input: map[string]any{
				"garch": map[string]any{
					"alpha": map[string]any{
						"value":              0.1,
						"calibration_method": "mle",
						"citation": map[string]any{
							"evidence_quality": "high",
						},
					},
				},
			},
			wantErrors: []string{
				"garch.alpha: citation.evidence_quality=\"high\" but no calibration timestamp (need last_calibrated or calibration_timestamp)",
				"garch.alpha: calibration_method=\"mle\" but no calibration timestamp",
			},
		},
		{
			name: "array elements get indexed paths",
			input: map[string]any{
				"seeds": []any{
					map[string]any{
						"value":              1.0,
						"calibration_method": "manual",
						"citation": map[string]any{
							"evidence_quality": "medium",
						},
					},
				},
			},
			wantErrors: []string{
				"seeds[0]: citation.evidence_quality=\"medium\" but no calibration timestamp (need last_calibrated or calibration_timestamp)",
				"seeds[0]: calibration_method=\"manual\" but no calibration timestamp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWarns, gotErrors := WalkTree(tt.input, "", tt.strict)
			if !slices.Equal(gotWarns, tt.wantWarns) {
				t.Errorf("warnings = %v, want %v", gotWarns, tt.wantWarns)
			}
			if !slices.Equal(gotErrors, tt.wantErrors) {
				t.Errorf("errors = %v, want %v", gotErrors, tt.wantErrors)
			}
		})
	}
}

func TestCheckSection(t *testing.T) {
	tests := []struct {
		name       string
		obj        map[string]any
		strict     bool
		wantWarns  []string
		wantErrors []string
	}{
		{
			name: "high quality with timestamp is clean",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "high",
				},
				"calibration_method": "mle",
				"last_calibrated":    "2026-06-01",
			},
		},
		{
			name: "empty calibration_method is treated as absent",
			obj: map[string]any{
				"value":              0.1,
				"calibration_method": "",
				"citation": map[string]any{
					"evidence_quality": "high",
				},
			},
			wantErrors: []string{
				"param: citation.evidence_quality=\"high\" but no calibration timestamp (need last_calibrated or calibration_timestamp)",
			},
		},
		{
			name: "calibration_timestamp satisfies timestamp requirement",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "medium",
				},
				"calibration_timestamp": "2026-06-01",
			},
		},
		{
			name: "heuristic quality with timestamp warns",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "heuristic",
				},
				"last_calibrated": "2026-06-01",
			},
			wantWarns: []string{
				"param: calibration timestamp exists but citation.evidence_quality=\"heuristic\" (expected 'high' or 'medium' after calibration)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWarns, gotErrors := CheckSection(tt.obj, "param", tt.strict)
			if !slices.Equal(gotWarns, tt.wantWarns) {
				t.Errorf("warnings = %v, want %v", gotWarns, tt.wantWarns)
			}
			if !slices.Equal(gotErrors, tt.wantErrors) {
				t.Errorf("errors = %v, want %v", gotErrors, tt.wantErrors)
			}
		})
	}
}

func TestValidateAndReport(t *testing.T) {
	tests := []struct {
		name         string
		config       map[string]any
		strict       bool
		wantExit     int
		wantStdout   string
		wantStderrIn []string
	}{
		{
			name:       "empty config is valid",
			config:     map[string]any{},
			wantExit:   0,
			wantStdout: "OK: params.json is valid (0 sections checked)\n",
		},
		{
			name: "error exits 1",
			config: map[string]any{
				"alpha": map[string]any{
					"value":              0.1,
					"calibration_method": "mle",
				},
			},
			wantExit:     1,
			wantStdout:   "\n1 error(s), 0 warning(s)\n",
			wantStderrIn: []string{"FAIL: alpha: calibration_method=\"mle\" but no calibration timestamp"},
		},
		{
			name: "strict warning exits 1",
			config: map[string]any{
				"alpha": map[string]any{
					"value": 0.1,
					"citation": map[string]any{
						"evidence_quality": "low",
					},
					"last_calibrated": "2026-06-01",
				},
			},
			strict:       true,
			wantExit:     1,
			wantStdout:   "\n1 error(s), 0 warning(s)\n",
			wantStderrIn: []string{"FAIL: alpha: calibration timestamp exists but citation.evidence_quality=\"low\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := ValidateAndReport(tt.config, "params.json", tt.strict, &stdout, &stderr)
			if got != tt.wantExit {
				t.Errorf("exit code = %d, want %d", got, tt.wantExit)
			}
			if gotStdout := stdout.String(); gotStdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", gotStdout, tt.wantStdout)
			}
			stderrStr := stderr.String()
			for _, want := range tt.wantStderrIn {
				if !strings.Contains(stderrStr, want) {
					t.Errorf("stderr missing %q, got %q", want, stderrStr)
				}
			}
		})
	}
}
