// Flag parsing tests for the one-shot flow backfill CLI. The backfill run
// logic (and its stub-provider tests) lives in internal/stockflows so the CLI
// and the scheduled refresh share one implementation (issue #1945).
package main

import (
	"strings"
	"testing"
)

func TestParseFlags_Defaults(t *testing.T) {
	cfg, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.workDir != "." {
		t.Errorf("workDir = %q, want .", cfg.workDir)
	}
	if cfg.start.Format("2006-01-02") != defaultStart {
		t.Errorf("start = %s, want %s", cfg.start.Format("2006-01-02"), defaultStart)
	}
	if cfg.dryRun || cfg.symbols != nil {
		t.Errorf("dryRun/symbols = %v/%v, want false/nil", cfg.dryRun, cfg.symbols)
	}
	if cfg.minRows != 500 || cfg.sleep.Seconds() != 3 {
		t.Errorf("minRows/sleep = %d/%v, want 500/3s", cfg.minRows, cfg.sleep)
	}
}

func TestParseFlags_SymbolsAndWindow(t *testing.T) {
	cfg, err := parseFlags([]string{
		"-workdir", "/tmp/atlas", "-start", "2026-01-05", "-end", "2026-01-07",
		"-symbols", "2330, 2317 ,", "-min-rows", "10", "-sleep", "1s", "-dry-run",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.workDir != "/tmp/atlas" || !cfg.dryRun {
		t.Errorf("workDir/dryRun = %q/%v", cfg.workDir, cfg.dryRun)
	}
	if cfg.end.Format("2006-01-02") != "2026-01-07" {
		t.Errorf("end = %s, want 2026-01-07", cfg.end.Format("2006-01-02"))
	}
	if len(cfg.symbols) != 2 || !cfg.symbols["2330"] || !cfg.symbols["2317"] {
		t.Errorf("symbols = %v, want {2330, 2317}", cfg.symbols)
	}
	if cfg.minRows != 10 || cfg.sleep.Seconds() != 1 {
		t.Errorf("minRows/sleep = %d/%v", cfg.minRows, cfg.sleep)
	}
}

func TestParseFlags_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bad start", []string{"-start", "05/01/2026"}, "parse -start"},
		{"bad end", []string{"-end", "nope"}, "parse -end"},
		{"end before start", []string{"-start", "2026-02-01", "-end", "2026-01-01"}, "is before -start"},
		{"min rows", []string{"-min-rows", "0"}, "min-rows must be > 0"},
		{"negative sleep", []string{"-sleep", "-1s"}, "sleep must be >= 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFlags(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseFlags(%v) err = %v, want containing %q", tc.args, err, tc.want)
			}
		})
	}
}
