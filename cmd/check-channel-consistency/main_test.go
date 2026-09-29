package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/monitoring"
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
	// The instant is injected (issue #2138 guardrail 2): the governance check is
	// date-driven, so a test that read the wall clock would change verdict on a
	// calendar boundary with no code change. 2026-09-29 is the day the
	// twse_oddlot retirement shipped, i.e. every declared deadline is already met.
	violations, governance, err := runChecks(
		filepath.Join(root, "cmd/atlas/data_sync_health_tasks.go"),
		filepath.Join(root, "monitoring/rules/channel_health_latent_staleness.yml"),
		time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("runChecks() error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("production configuration drifted: %+v", violations)
	}
	// The governance section must exist and must cover both retired channels
	// (canonical + dash alias) with a replacement and a shipped disposition.
	if len(governance) < 4 {
		t.Fatalf("governance report has %d rows, want ≥4 (twse_etf/twse-oddlot pairs + bdi)", len(governance))
	}
	seen := map[string]bool{}
	for _, row := range governance {
		seen[row.ChannelID] = true
		if row.Overdue {
			t.Errorf("%s is reported overdue at 2026-09-29; the deadlines were met that day", row.ChannelID)
		}
		if row.UpstreamRemovedAt == "" {
			t.Errorf("%s has no UpstreamRemovedAt in the report (the total elapsed time must always be visible)", row.ChannelID)
		}
	}
	for _, want := range []string{"twse_etf", "twse-etf", "twse_oddlot", "twse-oddlot", "bdi"} {
		if !seen[want] {
			t.Errorf("governance report is missing availability case %q", want)
		}
	}
	// The dead alias is NOT an availability case: it must not appear at all.
	if seen["taifex-daily"] {
		t.Error("taifex-daily must not be reported as an availability case (it opted out by declaration)")
	}
	// No blind spot: the governance check must JUDGE every registry entry, and the
	// report must list exactly the entries that declare a permanent removal. An
	// entry that is registered but never judged would be a governance hole that no
	// test could see.
	entries := monitoring.KnownIssueEntries()
	wantRows := 0
	for _, e := range entries {
		if e.Issue.UpstreamRemovedAt != "" {
			wantRows++
		}
	}
	if len(governance) != wantRows {
		t.Fatalf("governance rows = %d, want %d (one per registry entry declaring an upstream removal)", len(governance), wantRows)
	}
	for _, e := range entries {
		if e.Issue.UpstreamRemovedAt == "" {
			continue
		}
		if !seen[e.ChannelID] {
			t.Errorf("registry entry %q declares an upstream removal but is missing from the governance report", e.ChannelID)
		}
	}
}

// TestNoProductionPathInjectsNow pins guardrail 2 (determinism) at the source
// level: `--now` exists so a test or a reproduction can judge a deadline at a
// fixed instant. Nothing else may pass it — a production path that decided its
// own instant would make the verdict unreproducible, which is the property the
// guardrail exists to protect.
func TestNoProductionPathInjectsNow(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist", "docs", "data":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), `"--now=`) || strings.Contains(string(data), "nowFlag") {
			rel, _ := filepath.Rel(root, path)
			if rel != filepath.Join("cmd", "check-channel-consistency", "main.go") {
				offenders = append(offenders, rel)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Fatalf("the deadline instant may only be injected by the check-channel-consistency CLI flag; found %v", offenders)
	}
}

// TestCheckGovernance_DateDrivenGate pins the criterion's teeth: the SAME
// registry goes red once the injected instant is past a declared deadline, and
// green again once the disposition is declared. Two directions, both without
// touching the wall clock.
func TestCheckGovernance_DateDrivenGate(t *testing.T) {
	t.Run("before the deadline nothing is overdue", func(t *testing.T) {
		violations, report := checkGovernance(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC))
		for _, v := range violations {
			if v.Check == "governance.deadline_passed" {
				t.Errorf("no deadline had passed on 2026-08-05, got %+v", v)
			}
		}
		if len(report) == 0 {
			t.Fatal("the report must list availability cases even when none is overdue")
		}
	})
	t.Run("a shipped disposition stays green forever, an open deadline does not", func(t *testing.T) {
		// Guardrail 3: RetiredAt is a permanent disposition — judging the same
		// registry years later must NOT punish the channels that were retired.
		// The flip side is intended, not a bug: an OPEN deadline (bdi's review
		// deadline) goes red once it passes, because that is the forcing function
		// — renew it explicitly with a reason, or act on it.
		violations, _ := checkGovernance(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
		overdue := map[string]bool{}
		for _, v := range violations {
			if v.Check == "governance.deadline_passed" {
				overdue[v.ChannelID] = true
			}
		}
		for _, retired := range []string{"twse_etf", "twse-etf", "twse_oddlot", "twse-oddlot"} {
			if overdue[retired] {
				t.Errorf("%s carries RetiredAt and must never be overdue, whatever the date is", retired)
			}
		}
		if !overdue["bdi"] {
			t.Error("bdi's deadline (2026-12-31) is open and unrenewed: judging at 2030 must fail, otherwise an open deadline could sit forever")
		}
	})
	t.Run("bdi is not a retire case but stays visible with its review deadline", func(t *testing.T) {
		_, report := checkGovernance(time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC))
		for _, row := range report {
			if row.ChannelID != "bdi" {
				continue
			}
			if row.HasReplacement {
				t.Error("bdi has no usable replacement source; the report must say so")
			}
			if row.ActionBy == "" {
				t.Error("bdi must still carry a review deadline: with no replacement the action is a re-review, not a retirement")
			}
			return
		}
		t.Fatal("bdi missing from the governance report")
	})
}

func hasViolation(violations []Violation, check string) bool {
	for _, v := range violations {
		if v.Check == check {
			return true
		}
	}
	return false
}
