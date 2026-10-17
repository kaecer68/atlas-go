package main

// Regression tests for the replay backfill gap window (FU-20260927-09).
//
// The defect: the CSV's newest data date was parsed as 00:00 **UTC** while the
// window end was a **Asia/Taipei** midnight, so comparing the two instants was
// off by exactly 8h. The single-day case (csv_latest + 1 trading day == end)
// flipped to "gap: none" while the log printed a target= date that contradicted
// it — and that trading day was not fetched until the CSV was two days behind.
//
// The UTC 23:30 fixture below is the sharpest form: 2026-03-25T23:30Z is
// 2026-03-26T07:30 Asia/Taipei, i.e. *before* the 15:30 close, so the last
// settled day is Wednesday 2026-03-25 and the CSV (Tuesday 2026-03-24) is
// exactly one trading day behind. Pre-fix this reported "gap: none".

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func mustTaipei(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}
	return loc
}

func TestReplayBackfillWindow(t *testing.T) {
	tz := mustTaipei(t)
	parse := func(s string) time.Time {
		t.Helper()
		d, err := time.Parse(dateLayout, s) // date-only parser: UTC midnight
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return d
	}

	cases := []struct {
		name        string
		csvLatest   string
		now         time.Time
		wantStart   string
		wantEnd     string
		wantGapNone bool
	}{
		{
			// The reproduction: same instant expressed as UTC 23:30 and as the
			// next Taipei morning. CSV is exactly one trading day behind.
			name:      "UTC 23:30 = Taipei next-day 07:30, one trading day behind",
			csvLatest: "2026-03-24",
			now:       time.Date(2026, time.March, 25, 23, 30, 0, 0, time.UTC),
			wantStart: "2026-03-25",
			wantEnd:   "2026-03-25",
		},
		{
			name:      "same instant as Taipei wall clock gives the same window",
			csvLatest: "2026-03-24",
			now:       time.Date(2026, time.March, 26, 7, 30, 0, 0, tz),
			wantStart: "2026-03-25",
			wantEnd:   "2026-03-25",
		},
		{
			// Before the 15:30 cutoff the current day has not settled yet, so
			// a CSV that already has "today" is up to date.
			name:        "Taipei 07:30 with CSV already on the same date",
			csvLatest:   "2026-03-26",
			now:         time.Date(2026, time.March, 26, 7, 30, 0, 0, tz),
			wantStart:   "2026-03-27",
			wantEnd:     "2026-03-25",
			wantGapNone: true,
		},
		{
			// After the close the current trading day counts; CSV on the same
			// date is therefore also up to date.
			name:        "Taipei 16:00 with CSV on the same date",
			csvLatest:   "2026-03-25",
			now:         time.Date(2026, time.March, 25, 16, 0, 0, 0, tz),
			wantStart:   "2026-03-26",
			wantEnd:     "2026-03-25",
			wantGapNone: true,
		},
		{
			// Friday close, CSV one day behind ⇒ Friday is fetched on Friday.
			name:      "Friday 16:00 with CSV one day behind",
			csvLatest: "2026-03-19",
			now:       time.Date(2026, time.March, 20, 16, 0, 0, 0, tz),
			wantStart: "2026-03-20",
			wantEnd:   "2026-03-20",
		},
		{
			// Monday before the close: Friday was the last settled day.
			name:        "Monday 07:30 rolls end back to Friday",
			csvLatest:   "2026-03-20",
			now:         time.Date(2026, time.March, 23, 7, 30, 0, 0, tz),
			wantStart:   "2026-03-23",
			wantEnd:     "2026-03-20",
			wantGapNone: true,
		},
		{
			// Saturday: the weekend rolls back to Friday, so a CSV that already
			// holds Friday has no gap.
			name:        "Saturday rolls end back to Friday",
			csvLatest:   "2026-03-20",
			now:         time.Date(2026, time.March, 21, 12, 0, 0, 0, tz),
			wantStart:   "2026-03-23",
			wantEnd:     "2026-03-20",
			wantGapNone: true,
		},
		{
			// Monday after the close with a stale CSV: the start jumps the
			// weekend forward to Monday.
			name:      "Monday 16:00 with a two-day-stale CSV",
			csvLatest: "2026-03-19",
			now:       time.Date(2026, time.March, 23, 16, 0, 0, 0, tz),
			wantStart: "2026-03-20",
			wantEnd:   "2026-03-23",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := replayBackfillWindow(parse(tc.csvLatest), tc.now)
			if got := start.Format(dateLayout); got != tc.wantStart {
				t.Errorf("start = %s, want %s", got, tc.wantStart)
			}
			if got := end.Format(dateLayout); got != tc.wantEnd {
				t.Errorf("end = %s, want %s", got, tc.wantEnd)
			}
			if start.Location() != end.Location() {
				t.Errorf("start/end locations differ: %s vs %s", start.Location(), end.Location())
			}
			if got := start.After(end); got != tc.wantGapNone {
				t.Errorf("start.After(end) = %v, want %v (start=%s end=%s)", got, tc.wantGapNone,
					start.Format(time.RFC3339), end.Format(time.RFC3339))
			}
		})
	}
}

// TestReplayBackfillWindow_RecordDatePhase pins the 8h phase itself: a date-only
// parser result (UTC midnight) and the Taipei midnight of the same calendar date
// are different instants, so they may never be compared directly.
func TestReplayBackfillWindow_RecordDatePhase(t *testing.T) {
	mustTaipei(t)
	parsed, err := time.Parse(dateLayout, "2026-03-24")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	normalized := recordTaipeiDate(parsed)
	if got, want := normalized.Format("Z07:00"), "+08:00"; got != want {
		t.Fatalf("recordTaipeiDate offset = %s, want Asia/Taipei (+08:00)", got)
	}
	if got, want := normalized.Format(dateLayout), "2026-03-24"; got != want {
		t.Errorf("recordTaipeiDate date = %s, want %s (the calendar date must survive normalization)", got, want)
	}
	if delta := utilsAbsDuration(parsed.Sub(normalized)); delta != 8*time.Hour {
		t.Errorf("phase between the UTC-parsed record date and its Taipei midnight = %v, want 8h", delta)
	}
}

func utilsAbsDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// TestRunAutoBackfill_SingleDayGapAcrossUTCBoundary is the end-to-end
// regression: the same fixture the pre-fix code called "gap: none" must now be
// fetched, and the log line must not contradict itself.
func TestRunAutoBackfill_SingleDayGapAcrossUTCBoundary(t *testing.T) {
	dir, csvPath, jsonlPath := replayFixtureDir(t)
	buildJSONLFromDates(t, csvPath, jsonlPath, "2026-03-19")
	writeReplayCSVFixture(t, csvPath, "2026-03-24")

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	// 2026-03-25T23:30Z == 2026-03-26T07:30 Asia/Taipei.
	now := time.Date(2026, time.March, 25, 23, 30, 0, 0, time.UTC)

	var buf bytes.Buffer
	prevWriter := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prevWriter) })
	// The fetch itself runs the external converter, which is not available in
	// this fixture; the assertion is about the *window verdict*, so the fetch
	// error is ignored on purpose.
	_ = runAutoBackfill(context.Background(), cfg, now)
	logs := buf.String()

	if strings.Contains(logs, "backfill gap: none") {
		t.Fatalf("single-trading-day gap was reported as gap: none (pre-fix shape):\n%s", logs)
	}
	if !strings.Contains(logs, "backfill gap detected: 2026-03-25 to 2026-03-25") {
		t.Fatalf("expected the 2026-03-25 gap to be detected, got:\n%s", logs)
	}
	// The conversion is gap-independent and must still run.
	if !strings.Contains(logs, "backfill CSV→JSONL conversion") {
		t.Fatalf("JSONL conversion did not run:\n%s", logs)
	}
}

// TestRunAutoBackfill_NoGapLogCarriesTimeZone keeps the audit fields on the
// gap-free line: the pre-fix log printed csv_latest and target= dates that
// disagreed without saying which zone each was in.
func TestRunAutoBackfill_NoGapLogCarriesTimeZone(t *testing.T) {
	dir, csvPath, jsonlPath := replayFixtureDir(t)
	dates := []string{"2026-03-20"}
	buildJSONLFromDates(t, csvPath, jsonlPath, dates...)
	writeReplayCSVFixture(t, csvPath, dates...)

	cfg := config.Config{WorkDir: dir, ReplayDataPath: csvPath}
	now := taipeiClock(t, 2026, time.March, 20, 16, 0)

	var buf bytes.Buffer
	prevWriter := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prevWriter) })
	if err := runAutoBackfill(context.Background(), cfg, now); err != nil {
		t.Fatalf("runAutoBackfill: %v", err)
	}
	logs := buf.String()
	for _, want := range []string{
		"backfill gap: none",
		"timezone=" + replayDateZoneName,
		"now=" + now.Format(time.RFC3339),
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("gap-free log is missing %q:\n%s", want, logs)
		}
	}
}
