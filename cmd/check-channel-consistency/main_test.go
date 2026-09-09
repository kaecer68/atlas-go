package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

func writeFixture(t *testing.T, rel, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseScheduledTasks(t *testing.T) {
	source := writeFixture(t, "tasks.go", `package atlas

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

var (
	fast = &apigateway.ScheduledTask{
		Name:      "us_market_refresh_us_yahoo",
		ChannelID: "us_yahoo",
		Interval:  5 * time.Minute,
	}
	internal = &apigateway.ScheduledTask{
		Name:     "channel_health_sync",
		Interval: 5 * time.Minute,
	}
)
`)
	tasks, err := parseScheduledTasks(source)
	if err != nil {
		t.Fatalf("parseScheduledTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("parsed %d tasks, want 1 (ChannelID-less internal tasks are excluded): %+v", len(tasks), tasks)
	}
	got := tasks[0]
	if got.Name != "us_market_refresh_us_yahoo" || got.ChannelID != "us_yahoo" || got.Interval != 5*time.Minute {
		t.Fatalf("parsed task = %+v", got)
	}
}

func TestRuleAppliesToChannel(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want bool
	}{
		{"no matcher", `atlas_channel_health_status == 2`, true},
		{"equal", `atlas_channel_health_status{channel="fugle"} == 2`, true},
		{"not equal", `atlas_channel_health_status{channel!="fugle"} == 2`, false},
		{"regex match", `atlas_channel_health_status{channel=~"^(fugle|fubon)$"} == 2`, true},
		{"regex not match", `atlas_channel_health_status{channel!~"^(fugle|fubon)$"} == 2`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ruleAppliesToChannel(tc.expr, "fugle"); got != tc.want {
				t.Fatalf("ruleAppliesToChannel() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckSchedules_Fixtures(t *testing.T) {
	t.Run("known channel without explicit contract fails", func(t *testing.T) {
		violations := checkSchedules([]TaskSpec{{
			Name:      "fixture",
			ChannelID: "us_yahoo",
			Interval:  5 * time.Minute,
		}}, apigateway.NewChannelContractRegistry())
		if !hasViolation(violations, "schedule.missing_contract") {
			t.Fatalf("missing_contract violation not found: %+v", violations)
		}
	})

	t.Run("unregistered derived channel cannot be scheduled", func(t *testing.T) {
		violations := checkSchedules([]TaskSpec{{
			Name:      "fixture",
			ChannelID: "vix",
			Interval:  5 * time.Minute,
		}}, apigateway.ChannelContracts())
		if !hasViolation(violations, "schedule.unknown_channel") {
			t.Fatalf("unknown_channel violation not found: %+v", violations)
		}
	})

	t.Run("interval above freshness window fails", func(t *testing.T) {
		violations := checkSchedules([]TaskSpec{{
			Name:      "fixture",
			ChannelID: "us_yahoo",
			Interval:  49 * time.Hour,
		}}, apigateway.ChannelContracts())
		if !hasViolation(violations, "schedule.interval_exceeds_freshness") {
			t.Fatalf("interval_exceeds_freshness violation not found: %+v", violations)
		}
	})
}

func TestCheckAlertFor_Fixtures(t *testing.T) {
	alerts := []alertRule{{Alert: statusErrorAlert, Expr: `atlas_channel_health_status == 2`, For: "1m"}}
	tasks := []TaskSpec{{Name: "fast", ChannelID: "us_yahoo", Interval: 5 * time.Minute}}
	violations := checkAlertFor(tasks, alerts)
	if !hasViolation(violations, "alert.for_below_interval") {
		t.Fatalf("for_below_interval violation not found: %+v", violations)
	}

	okAlerts := []alertRule{{Alert: statusErrorAlert, Expr: `atlas_channel_health_status == 2`, For: "5m"}}
	if got := checkAlertFor(tasks, okAlerts); len(got) != 0 {
		t.Fatalf("expected no violations, got %+v", got)
	}
}

func TestRunChecks_CurrentConfiguration(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	violations, err := runChecks(
		filepath.Join(root, "cmd/atlas/data_sync_health_tasks.go"),
		filepath.Join(root, "monitoring/rules/channel_health_latent_staleness.yml"),
	)
	if err != nil {
		t.Fatalf("runChecks() error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("production configuration drifted: %+v", violations)
	}
}

func hasViolation(violations []Violation, check string) bool {
	for _, v := range violations {
		if v.Check == check {
			return true
		}
	}
	return false
}
