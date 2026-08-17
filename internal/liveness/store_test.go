package liveness

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeExecer records Exec calls and returns a configurable error.
type fakeExecer struct {
	err   error
	calls int
	args  [][]any
}

func (f *fakeExecer) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.calls++
	f.args = append(f.args, args)
	return pgconn.CommandTag{}, f.err
}

// Query satisfies the dbConn read half; unused by these tests.
func (f *fakeExecer) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func TestRecord_SuccessMarksWriteAndResetsFailuresInSQL(t *testing.T) {
	ex := &fakeExecer{}
	s := newStoreWithExec(ex)
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	s.setNow(func() time.Time { return now })

	if err := s.Record(context.Background(), RecordInput{TaskName: "t1", Err: nil, Duration: 3 * time.Second}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if s.lastWrite() != now {
		t.Fatalf("lastWrite = %v, want %v", s.lastWrite(), now)
	}
	// Upsert SQL must set consecutive_failures=0 and last_success_at on success:
	args := ex.args[0]
	if args[3] != "" {
		t.Errorf("last_error arg = %q, want empty", args[3])
	}
	if args[2] == nil {
		t.Error("last_success_at arg must be set on success")
	}
	if args[4] != 0 {
		t.Errorf("seed failures arg = %v, want 0 (SQL increments from existing row)", args[4])
	}
	if args[5] != int64(3000) {
		t.Errorf("duration_ms arg = %v, want 3000", args[5])
	}
}

func TestRecord_FailureKeepsSuccessAtNilAndSetsError(t *testing.T) {
	ex := &fakeExecer{}
	s := newStoreWithExec(ex)
	s.setNow(func() time.Time { return time.Now() })

	err := &PingError{ExitCode: 1, Msg: "boom"}
	if err2 := s.Record(context.Background(), RecordInput{TaskName: "t1", Err: err}); err2 != nil {
		t.Fatalf("Record: %v", err2)
	}
	args := ex.args[0]
	if args[3] != "boom" {
		t.Errorf("last_error arg = %q, want boom", args[3])
	}
	if args[2] != nil {
		t.Errorf("last_success_at arg = %v, want nil on failure", args[2])
	}
	if args[4] != 1 {
		t.Errorf("seed failures arg = %v, want 1 on failure (fresh-row insert must start at 1)", args[4])
	}
}

func TestRecord_ErrorDoesNotMarkWrite(t *testing.T) {
	ex := &fakeExecer{err: context.DeadlineExceeded}
	s := newStoreWithExec(ex)
	s.setNow(func() time.Time { return time.Now() })

	if err := s.Record(context.Background(), RecordInput{TaskName: "t1"}); err == nil {
		t.Fatal("expected error from store")
	}
	if !s.lastWrite().IsZero() {
		t.Error("failed write must not advance meta-heartbeat")
	}
}

func TestMetaHeartbeat_WarnsAfterSilenceOncePerWindow(t *testing.T) {
	ex := &fakeExecer{}
	s := newStoreWithExec(ex)
	base := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	cur := base
	s.setNow(func() time.Time { return cur })

	// First write at t0.
	if err := s.Record(context.Background(), RecordInput{TaskName: "t1"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// 1 minute later: still inside window -> no warn.
	cur = base.Add(time.Minute)
	s.checkMetaHeartbeat(DefaultMetaHeartbeatWarnAfter)
	if !s.lastWarnAt.IsZero() {
		t.Fatal("must not warn inside the silent window")
	}

	// 16 minutes after the write: warn.
	cur = base.Add(16 * time.Minute)
	s.checkMetaHeartbeat(DefaultMetaHeartbeatWarnAfter)
	if s.lastWarnAt != cur {
		t.Fatalf("expected warn at %v, got %v", cur, s.lastWarnAt)
	}

	// 17 minutes: still same window (warned < warnAfter ago) -> no new warn.
	cur = base.Add(17 * time.Minute)
	s.checkMetaHeartbeat(DefaultMetaHeartbeatWarnAfter)
	if s.lastWarnAt != base.Add(16*time.Minute) {
		t.Fatalf("expected no re-warn within window, lastWarnAt = %v", s.lastWarnAt)
	}

	// 35 minutes: new window -> warn again.
	cur = base.Add(35 * time.Minute)
	s.checkMetaHeartbeat(DefaultMetaHeartbeatWarnAfter)
	if s.lastWarnAt != cur {
		t.Fatalf("expected re-warn at %v, got %v", cur, s.lastWarnAt)
	}

	// A new successful write resets the silence: the same instant no longer
	// warns (silentFor=0), and lastWarnAt stays at its previous value.
	cur = base.Add(36 * time.Minute)
	if err := s.Record(context.Background(), RecordInput{TaskName: "t2"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	prevWarn := s.lastWarnAt
	s.checkMetaHeartbeat(DefaultMetaHeartbeatWarnAfter)
	if s.lastWarnAt != prevWarn {
		t.Fatalf("write must stop the silent window without moving lastWarnAt; lastWarnAt = %v", s.lastWarnAt)
	}

	// One minute later with no further writes: still inside window -> no warn.
	cur = base.Add(37 * time.Minute)
	s.checkMetaHeartbeat(DefaultMetaHeartbeatWarnAfter)
	if s.lastWarnAt != prevWarn {
		t.Fatalf("expected no warn after fresh write, lastWarnAt = %v", s.lastWarnAt)
	}
}
