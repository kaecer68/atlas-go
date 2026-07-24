// SPDX-License-Identifier: AGPL-3.0

package config

import (
	"bytes"
	"slices"
	"testing"
)

func TestCountParameterMetadataSections(t *testing.T) {
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
			got := CountParameterMetadataSections(tt.config)
			if got != tt.want {
				t.Errorf("CountParameterMetadataSections() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWalkParameterMetadataTree(t *testing.T) {
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
			gotWarns, gotErrors := WalkParameterMetadataTree(tt.input, "", tt.strict)
			if !slices.Equal(gotWarns, tt.wantWarns) {
				t.Errorf("warnings = %v, want %v", gotWarns, tt.wantWarns)
			}
			if !slices.Equal(gotErrors, tt.wantErrors) {
				t.Errorf("errors = %v, want %v", gotErrors, tt.wantErrors)
			}
		})
	}
}

func TestCheckParameterMetadataSection(t *testing.T) {
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
				"last_calibrated":    "2026-06-01",
			},
			wantWarns:  nil,
			wantErrors: nil,
		},
		{
			name: "high quality without timestamp is error",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "high",
				},
				"calibration_method": "mle",
			},
			wantErrors: []string{
				"section: citation.evidence_quality=\"high\" but no calibration timestamp (need last_calibrated or calibration_timestamp)",
				"section: calibration_method=\"mle\" but no calibration timestamp",
			},
		},
		{
			name: "low quality with timestamp is warning",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "low",
				},
				"calibration_method": "heuristic",
				"last_calibrated":    "2026-06-01",
			},
			wantWarns: []string{
				"section: calibration timestamp exists but citation.evidence_quality=\"low\" (expected 'high' or 'medium' after calibration)",
			},
		},
		{
			name: "low quality with timestamp is error in strict mode",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "low",
				},
				"calibration_method": "heuristic",
				"last_calibrated":    "2026-06-01",
			},
			strict: true,
			wantErrors: []string{
				"section: calibration timestamp exists but citation.evidence_quality=\"low\" (expected 'high' or 'medium' after calibration)",
			},
		},
		{
			name: "synthetic with real reference warning in strict",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"source_reference": "real.csv",
				},
				"calibration_method":      "mle",
				"last_calibrated":         "2026-06-01",
				"calibration_data_source": "synthetic",
			},
			strict: true,
			wantWarns: []string{
				"section: calibration_data_source='synthetic' but citation.source_reference=\"real.csv\" — re-run calibrator with --replay for real data",
			},
		},
		{
			name: "synthetic with synthetic reference is clean",
			obj: map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"source_reference": "synthetic_2026.csv",
				},
				"calibration_method":      "mle",
				"last_calibrated":         "2026-06-01",
				"calibration_data_source": "synthetic",
			},
			strict:     true,
			wantWarns:  nil,
			wantErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWarns, gotErrors := CheckParameterMetadataSection(tt.obj, "section", tt.strict)
			if !slices.Equal(gotWarns, tt.wantWarns) {
				t.Errorf("warnings = %v, want %v", gotWarns, tt.wantWarns)
			}
			if !slices.Equal(gotErrors, tt.wantErrors) {
				t.Errorf("errors = %v, want %v", gotErrors, tt.wantErrors)
			}
		})
	}
}

func TestValidateAndReportParameterMetadata(t *testing.T) {
	t.Run("valid config returns 0", func(t *testing.T) {
		config := map[string]any{
			"alpha": map[string]any{
				"value": 0.1,
				"citation": map[string]any{
					"evidence_quality": "high",
				},
				"calibration_method": "mle",
				"last_calibrated":    "2026-06-01",
			},
		}
		var stdout, stderr bytes.Buffer
		got := ValidateAndReportParameterMetadata(config, "params.json", false, &stdout, &stderr)
		if got != 0 {
			t.Errorf("exit code = %d, want 0", got)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		want := "OK: params.json is valid (1 sections checked)\n"
		if stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("invalid config returns 1", func(t *testing.T) {
		config := map[string]any{
			"alpha": map[string]any{
				"value":              0.1,
				"calibration_method": "mle",
			},
		}
		var stdout, stderr bytes.Buffer
		got := ValidateAndReportParameterMetadata(config, "params.json", false, &stdout, &stderr)
		if got != 1 {
			t.Errorf("exit code = %d, want 1", got)
		}
		if stderr.Len() == 0 {
			t.Error("expected stderr errors, got empty")
		}
	})
}
