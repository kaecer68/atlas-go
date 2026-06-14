package data

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func newTestStore(t *testing.T) ChannelHealthRecorder {
	t.Helper()
	return NewChannelHealthStoreWithPool(t.TempDir(), nil)
}

func TestNewChannelHealthStoreWithPool_ReturnsStore(t *testing.T) {
	store := newTestStore(t)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestRecord_SetsStatus(t *testing.T) {
	store := newTestStore(t)
	if err := store.Record("twse", "ok", ""); err != nil {
		t.Fatalf("Record: %v", err)
	}
	rec := store.Get("twse")
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Status != "ok" {
		t.Errorf("status = %q, want %q", rec.Status, "ok")
	}
	if rec.LastFetchAt == "" {
		t.Error("LastFetchAt empty")
	}
	if rec.LastSuccessAt == "" {
		t.Error("LastSuccessAt empty")
	}
	if rec.LastError != "" {
		t.Errorf("LastError = %q, want empty", rec.LastError)
	}
}

func TestRecord_ErrorStatus(t *testing.T) {
	store := newTestStore(t)
	if err := store.Record("finmind", "error", "connection refused"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	rec := store.Get("finmind")
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Status != "error" {
		t.Errorf("status = %q, want %q", rec.Status, "error")
	}
	if rec.LastError != "connection refused" {
		t.Errorf("LastError = %q, want %q", rec.LastError, "connection refused")
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("LastSuccessAt = %q, want empty for error status", rec.LastSuccessAt)
	}
}

func TestRecord_UpdatesExisting(t *testing.T) {
	store := newTestStore(t)
	_ = store.Record("twse", "ok", "")
	_ = store.Record("twse", "error", "timeout")

	rec := store.Get("twse")
	if rec.Status != "error" {
		t.Errorf("status = %q, want %q", rec.Status, "error")
	}
	if rec.LastError != "timeout" {
		t.Errorf("LastError = %q, want %q", rec.LastError, "timeout")
	}
}

func TestGet_NonexistentChannel(t *testing.T) {
	store := newTestStore(t)
	rec := store.Get("nonexistent")
	if rec != nil {
		t.Errorf("expected nil for nonexistent channel, got %+v", rec)
	}
}

func TestAlerts_OnlyNonOkStatus(t *testing.T) {
	store := newTestStore(t)
	_ = store.Record("twse", "ok", "")
	_ = store.Record("finmind", "error", "down")
	_ = store.Record("fugle", "degraded", "slow")
	_ = store.Record("fubon", "inactive", "")

	alerts := store.Alerts()
	// Only "error" and "degraded" should appear; "ok" and "inactive" should not
	if len(alerts) != 2 {
		t.Errorf("Alerts count = %d, want 2", len(alerts))
	}
	for _, a := range alerts {
		if a.Status == "ok" || a.Status == "inactive" {
			t.Errorf("unexpected alert with status=%q for channel %q", a.Status, a.ChannelID)
		}
	}
}

func TestAlerts_Empty(t *testing.T) {
	store := newTestStore(t)
	_ = store.Record("twse", "ok", "")
	alerts := store.Alerts()
	if len(alerts) != 0 {
		t.Errorf("Alerts count = %d, want 0", len(alerts))
	}
}

func TestRecord_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	store := NewChannelHealthStoreWithPool(dir, nil)
	_ = store.Record("twse", "ok", "")

	// Re-open same file path to verify persistence
	store2 := NewChannelHealthStoreWithPool(dir, nil)
	rec := store2.Get("twse")
	if rec == nil {
		t.Fatal("expected record from persisted file")
	}
	if rec.Status != "ok" {
		t.Errorf("status = %q, want %q", rec.Status, "ok")
	}
}

func TestRecord_MultipleChannels(t *testing.T) {
	store := newTestStore(t)
	_ = store.Record("twse", "ok", "")
	_ = store.Record("finmind", "ok", "")
	_ = store.Record("fugle", "ok", "")

	for _, id := range []string{"twse", "finmind", "fugle"} {
		rec := store.Get(id)
		if rec == nil {
			t.Errorf("Get(%q) returned nil", id)
		}
	}
}

func TestSyncAllToDB_NilPool(t *testing.T) {
	store := newTestStore(t)
	_ = store.Record("twse", "ok", "")
	err := store.SyncAllToDB()
	if err == nil {
		t.Error("expected error when pool is nil")
	}
}

func TestNewChannelHealthStoreWithPool_FileDoesNotExist(t *testing.T) {
	// Ensure no channel_health.json exists yet - store should handle gracefully
	dir := t.TempDir()
	store := NewChannelHealthStoreWithPool(dir, nil)
	rec := store.Get("any-channel")
	if rec != nil {
		t.Errorf("expected nil for nonexistent channel on a fresh store")
	}
	alerts := store.Alerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts on fresh store, got %d", len(alerts))
	}
}

func TestRecord_CorruptHealthFile(t *testing.T) {
	dir := t.TempDir()
	// Write corrupt JSON to the channel_health.json
	if err := os.WriteFile(filepath.Join(dir, "channel_health.json"), []byte("{not valid"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	store := NewChannelHealthStoreWithPool(dir, nil)
	if err := store.Record("twse", "ok", ""); err != nil {
		t.Fatalf("Record after corrupt file: %v", err)
	}
	rec := store.Get("twse")
	if rec == nil {
		t.Fatal("expected record after fixing corrupt file")
	}
}

func TestLoad_ReadErrorNotIsNotExist(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "channel_health.json")
	if err := os.MkdirAll(jsonPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	store := &channelHealthStore{
		path: jsonPath,
		data: make(map[string]*ChannelHealthRecord),
	}
	err := store.load()
	if err == nil {
		t.Error("expected error when reading a directory as regular file")
	}
}

func TestGet_ExistingRecordWithoutPool(t *testing.T) {
	store := newTestStore(t)
	_ = store.Record("twse", "ok", "")
	rec := store.Get("twse")
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Status != "ok" {
		t.Errorf("status = %q, want ok", rec.Status)
	}
	if rec.LastFetchAt == "" {
		t.Error("LastFetchAt should be populated")
	}
}

func TestSyncAllToDB_NilPoolError(t *testing.T) {
	store := &channelHealthStore{
		path: filepath.Join(t.TempDir(), "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
		pool: nil,
	}
	err := store.SyncAllToDB()
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

type mockExecer struct {
	row    pgx.Row
	calls  []mockExecCall
	err    error
	errFor map[string]error
}

type mockExecCall struct {
	channelID string
	status    string
	errMsg    *string
}

func (m *mockExecer) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	channelID := args[0].(string)
	var errMsg *string
	if len(args) > 3 && args[3] != nil {
		errMsg = args[3].(*string)
	}
	m.calls = append(m.calls, mockExecCall{
		channelID: channelID,
		status:    args[1].(string),
		errMsg:    errMsg,
	})
	if e, ok := m.errFor[channelID]; ok {
		return pgconn.CommandTag{}, e
	}
	return pgconn.CommandTag{}, m.err
}

func (m *mockExecer) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return m.row
}

type mockRow struct {
	status        string
	lastFetchAt   *time.Time
	lastError     string
	lastSuccessAt *time.Time
	err           error
}

func (r *mockRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*string) = r.status
	*dest[1].(**time.Time) = r.lastFetchAt
	*dest[2].(*string) = r.lastError
	*dest[3].(**time.Time) = r.lastSuccessAt
	return nil
}

func TestRecordToDB(t *testing.T) {
	cases := []struct {
		name      string
		pool      dbExecer
		wantErr   bool
		wantCalls int
	}{
		{"success", &mockExecer{}, false, 1},
		{"exec error", &mockExecer{err: errors.New("db down")}, true, 1},
		{"nil pool", nil, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &channelHealthStore{
				path: filepath.Join(t.TempDir(), "channel_health.json"),
				data: make(map[string]*ChannelHealthRecord),
				pool: tc.pool,
			}
			err := store.recordToDB("twse", "ok", "")
			if (err != nil) != tc.wantErr {
				t.Fatalf("recordToDB error = %v, wantErr %v", err, tc.wantErr)
			}
			if mock, ok := tc.pool.(*mockExecer); ok && len(mock.calls) != tc.wantCalls {
				t.Errorf("exec calls = %d, want %d", len(mock.calls), tc.wantCalls)
			}
		})
	}
}

func TestGetFromDB(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		pool dbExecer
		want *ChannelHealthRecord
	}{
		{
			name: "record",
			pool: &mockExecer{row: &mockRow{status: "ok", lastFetchAt: &now, lastError: "", lastSuccessAt: &now}},
			want: &ChannelHealthRecord{Status: "ok", LastFetchAt: now.Format(time.RFC3339), LastSuccessAt: now.Format(time.RFC3339)},
		},
		{
			name: "error",
			pool: &mockExecer{row: &mockRow{err: errors.New("not found")}},
			want: nil,
		},
		{
			name: "nil pool",
			pool: nil,
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &channelHealthStore{
				path: filepath.Join(t.TempDir(), "channel_health.json"),
				data: make(map[string]*ChannelHealthRecord),
				pool: tc.pool,
			}
			got := store.getFromDB("twse")
			if tc.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected record, got nil")
			}
			if got.Status != tc.want.Status {
				t.Errorf("status = %q, want %q", got.Status, tc.want.Status)
			}
			if got.LastFetchAt != tc.want.LastFetchAt {
				t.Errorf("LastFetchAt = %q, want %q", got.LastFetchAt, tc.want.LastFetchAt)
			}
			if got.LastSuccessAt != tc.want.LastSuccessAt {
				t.Errorf("LastSuccessAt = %q, want %q", got.LastSuccessAt, tc.want.LastSuccessAt)
			}
		})
	}
}

func TestSyncAllToDB(t *testing.T) {
	cases := []struct {
		name    string
		errFor  map[string]error
		wantErr bool
	}{
		{"success", nil, false},
		{"partial failure", map[string]error{"finmind": errors.New("db down")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockExecer{errFor: tc.errFor}
			store := &channelHealthStore{
				path: filepath.Join(t.TempDir(), "channel_health.json"),
				data: map[string]*ChannelHealthRecord{
					"twse":    {Status: "ok"},
					"finmind": {Status: "error", LastError: "down"},
				},
				pool: mock,
			}
			err := store.SyncAllToDB()
			if (err != nil) != tc.wantErr {
				t.Fatalf("SyncAllToDB error = %v, wantErr %v", err, tc.wantErr)
			}
			if len(mock.calls) != 2 {
				t.Errorf("exec calls = %d, want 2", len(mock.calls))
			}
		})
	}
}

func TestRecord_WithPool_WritesToDB(t *testing.T) {
	mock := &mockExecer{}
	store := &channelHealthStore{
		path: filepath.Join(t.TempDir(), "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
		pool: mock,
	}
	if err := store.Record("twse", "ok", ""); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(mock.calls))
	}
	rec := store.Get("twse")
	if rec == nil {
		t.Fatal("expected in-memory record")
	}
	if rec.Status != "ok" {
		t.Errorf("status = %q, want ok", rec.Status)
	}
}

func TestGet_FallsBackToDB(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	row := &mockRow{
		status:        "ok",
		lastFetchAt:   &now,
		lastError:     "",
		lastSuccessAt: &now,
	}
	store := &channelHealthStore{
		path: filepath.Join(t.TempDir(), "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
		pool: &mockExecer{row: row},
	}
	rec := store.Get("twse")
	if rec == nil {
		t.Fatal("expected record from DB fallback")
	}
	if rec.Status != "ok" {
		t.Errorf("status = %q, want ok", rec.Status)
	}
	if rec.LastFetchAt != now.Format(time.RFC3339) {
		t.Errorf("LastFetchAt = %q, want %q", rec.LastFetchAt, now.Format(time.RFC3339))
	}
}
