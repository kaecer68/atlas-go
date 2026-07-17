package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type stubParams struct {
	autoEnabled bool
}

func (s *stubParams) GetAutoEnabled() bool { return s.autoEnabled }

// mondayAt returns a Monday at the given HH:MM (UTC) for stable cron window tests.
func mondayAt(hour, minute int) time.Time {
	// 2026-07-13 is a Monday.
	return time.Date(2026, 7, 13, hour, minute, 0, 0, time.UTC)
}

// weekdayAt returns the date for the given weekday at the given HH:MM,
// anchored to 2026-07-13 (Monday). Days offset: Mon=0, Tue=1, ..., Sun=6.
func weekdayAt(weekday time.Weekday, hour, minute int) time.Time {
	base := mondayAt(hour, minute)
	offset := int(weekday) - int(time.Monday)
	return base.AddDate(0, 0, offset)
}

// tempObservationLog creates a temp file with the given content.
// Returns the file path. Caller should defer os.RemoveAll on the parent dir.
func tempObservationLog(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "l2-4-observation-log.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp log: %v", err)
	}
	return path
}

func TestShouldL24AutoCronFire_DefaultDisabled_NoEnvVar(t *testing.T) {
	os.Unsetenv(L24AutoCronEnvVar)

	state := ShouldL24AutoCronFire(
		&stubParams{autoEnabled: true},
		tempObservationLog(t, "# Observation Log\nDay 7 entry here\n"),
		"09:00", "09:30", "Mon-Fri",
		mondayAt(9, 15),
	)

	if state.Fired {
		t.Errorf("Fired = true with no env var set, want false (default-off)")
	}
	if state.EnvVarSet {
		t.Errorf("EnvVarSet = true, want false")
	}
	if state.SkippedReason == "" {
		t.Errorf("SkippedReason empty, want explanation")
	}
}

func TestShouldL24AutoCronFire_EnvVarSetButAutoEnabledFalse(t *testing.T) {
	os.Setenv(L24AutoCronEnvVar, "true")
	defer os.Unsetenv(L24AutoCronEnvVar)

	state := ShouldL24AutoCronFire(
		&stubParams{autoEnabled: false},
		tempObservationLog(t, "Day 7"),
		"09:00", "09:30", "Mon-Fri",
		mondayAt(9, 15),
	)

	if state.Fired {
		t.Errorf("Fired = true with auto_enabled=false, want false")
	}
	if !state.EnvVarSet {
		t.Errorf("EnvVarSet = false, want true")
	}
	if state.AutoEnabledParam {
		t.Errorf("AutoEnabledParam = true, want false")
	}
}

func TestShouldL24AutoCronFire_AutoEnabledTrueButNoLog(t *testing.T) {
	os.Setenv(L24AutoCronEnvVar, "true")
	defer os.Unsetenv(L24AutoCronEnvVar)

	state := ShouldL24AutoCronFire(
		&stubParams{autoEnabled: true},
		"/nonexistent/path/observation-log.md",
		"09:00", "09:30", "Mon-Fri",
		mondayAt(9, 15),
	)

	if state.Fired {
		t.Errorf("Fired = true with no log file, want false")
	}
	if !state.EnvVarSet || !state.AutoEnabledParam {
		t.Errorf("gate should pass conditions 1+2 before failing on condition 3")
	}
	if state.ObservationLogOK {
		t.Errorf("ObservationLogOK = true, want false")
	}
}

func TestShouldL24AutoCronFire_LogExistsButNoDay7Entry(t *testing.T) {
	os.Setenv(L24AutoCronEnvVar, "true")
	defer os.Unsetenv(L24AutoCronEnvVar)

	state := ShouldL24AutoCronFire(
		&stubParams{autoEnabled: true},
		tempObservationLog(t, "# Week 0\n## Day 1\n## Day 2\n## Day 3\n"),
		"09:00", "09:30", "Mon-Fri",
		mondayAt(9, 15),
	)

	if state.Fired {
		t.Errorf("Fired = true with only Day 1-3 entries, want false (Day 7+ required)")
	}
	if !state.ObservationLogOK {
		t.Errorf("ObservationLogOK = false, want true (file exists)")
	}
	if state.HasDay7Entry {
		t.Errorf("HasDay7Entry = true, want false")
	}
}

func TestShouldL24AutoCronFire_Day7EntryButOutsideCronWindow(t *testing.T) {
	os.Setenv(L24AutoCronEnvVar, "true")
	defer os.Unsetenv(L24AutoCronEnvVar)

	// Monday 14:00 — outside 09:00-09:30 window
	state := ShouldL24AutoCronFire(
		&stubParams{autoEnabled: true},
		tempObservationLog(t, "Day 7 entry exists"),
		"09:00", "09:30", "Mon-Fri",
		mondayAt(14, 0),
	)

	if state.Fired {
		t.Errorf("Fired = true outside cron window, want false")
	}
	if !state.HasDay7Entry {
		t.Errorf("HasDay7Entry = false, want true")
	}
	if state.InCronWindow {
		t.Errorf("InCronWindow = true, want false")
	}
}

func TestShouldL24AutoCronFire_AllConditionsMet(t *testing.T) {
	os.Setenv(L24AutoCronEnvVar, "true")
	defer os.Unsetenv(L24AutoCronEnvVar)

	state := ShouldL24AutoCronFire(
		&stubParams{autoEnabled: true},
		tempObservationLog(t, "Day 7 entry exists\nDay 14 entry exists\n"),
		"09:00", "09:30", "Mon-Fri",
		mondayAt(9, 15),
	)

	if !state.Fired {
		t.Errorf("Fired = false with all conditions met, want true. SkippedReason: %s", state.SkippedReason)
	}
	if !state.EnvVarSet || !state.AutoEnabledParam || !state.ObservationLogOK ||
		!state.HasDay7Entry || !state.InCronWindow {
		t.Errorf("not all conditions marked true: %+v", state)
	}
	if state.SkippedReason != "" {
		t.Errorf("SkippedReason = %q, want empty when Fired=true", state.SkippedReason)
	}
}

func TestShouldL24AutoCronFire_NilParams(t *testing.T) {
	os.Setenv(L24AutoCronEnvVar, "true")
	defer os.Unsetenv(L24AutoCronEnvVar)

	state := ShouldL24AutoCronFire(
		nil,
		tempObservationLog(t, "Day 7"),
		"09:00", "09:30", "Mon-Fri",
		mondayAt(9, 15),
	)

	if state.Fired {
		t.Errorf("Fired = true with nil params, want false")
	}
}

func TestHasDay7PlusEntry(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"only Day 1-6", "Day 1\nDay 2\nDay 6", false},
		{"Day 7", "Day 7 entry", true},
		{"Day 14", "Day 14 acceptance", true},
		{"Day 30", "Day 30 monthly window", true},
		{"Day 99", "Day 99 future", false}, // out of scan range
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasDay7PlusEntry(tc.content)
			if got != tc.want {
				t.Errorf("hasDay7PlusEntry(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}

func TestInCronWindow(t *testing.T) {
	cases := []struct {
		name      string
		weekday   time.Weekday
		hour, min int
		start     string
		end       string
		days      string
		want      bool
		wantErr   bool
	}{
		{"Mon in window", time.Monday, 9, 15, "09:00", "09:30", "Mon-Fri", true, false},
		{"Mon before window", time.Monday, 8, 59, "09:00", "09:30", "Mon-Fri", false, false},
		{"Mon after window", time.Monday, 9, 31, "09:00", "09:30", "Mon-Fri", false, false},
		{"Mon at start", time.Monday, 9, 0, "09:00", "09:30", "Mon-Fri", true, false},
		{"Mon at end", time.Monday, 9, 30, "09:00", "09:30", "Mon-Fri", true, false},
		{"Sat not in Mon-Fri", time.Saturday, 9, 15, "09:00", "09:30", "Mon-Fri", false, false},
		{"Sat in Sat-Sun", time.Saturday, 9, 15, "09:00", "09:30", "Sat-Sun", true, false},
		{"Wed in Mon,Wed,Fri", time.Wednesday, 9, 15, "09:00", "09:30", "Mon,Wed,Fri", true, false},
		{"Thu not in Mon,Wed,Fri", time.Thursday, 9, 15, "09:00", "09:30", "Mon,Wed,Fri", false, false},
		{"invalid start", time.Monday, 9, 15, "9am", "09:30", "Mon-Fri", false, true},
		{"invalid end", time.Monday, 9, 15, "09:00", "9:30pm", "Mon-Fri", false, true},
		{"start after end", time.Monday, 9, 15, "10:00", "09:00", "Mon-Fri", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := inCronWindow(weekdayAt(tc.weekday, tc.hour, tc.min), tc.start, tc.end, tc.days)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDayMatches(t *testing.T) {
	cases := []struct {
		weekday string
		spec    string
		want    bool
	}{
		{"Mon", "Mon-Fri", true},
		{"Fri", "Mon-Fri", true},
		{"Sat", "Mon-Fri", false},
		{"Mon", "Sat-Sun", false},
		{"Sat", "Sat-Sun", true},
		{"Mon", "Mon,Wed,Fri", true},
		{"Wed", "Mon,Wed,Fri", true},
		{"Thu", "Mon,Wed,Fri", false},
		{"XXX", "Mon-Fri", false}, // unknown weekday
	}
	for _, tc := range cases {
		t.Run(tc.weekday+"-"+tc.spec, func(t *testing.T) {
			if got := dayMatches(tc.weekday, tc.spec); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
