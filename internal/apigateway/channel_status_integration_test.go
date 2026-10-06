//go:build integration

package apigateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

// TestChannelHealthStore_SyncAllToDB_DerivedVerdictAndHonestTimestamps is the
// end-to-end DB assertion for the 2026-09-24 channel-status-truth fix:
// the row a human queries must show the same verdict as the UI and must not
// claim a fetch that never happened. Gated behind the integration build tag
// because it requires a live PostgreSQL (testdb.Pool fatal-fails in CI
// without DATABASE_URL — see testdb.go).
//
// Run locally:  DATABASE_URL=postgres://atlas:.../atlas go test -tags=integration -run TestSyncAllToDB -count=1 ./internal/apigateway/
func TestChannelHealthStore_SyncAllToDB_DerivedVerdictAndHonestTimestamps(t *testing.T) {
	pool := testdb.Pool(t, "../../sql/migrations")
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	store := NewChannelHealthStoreWithPool(dir, pool)
	wrapper := struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}{Channels: map[string]*ChannelHealthRecord{
		"twse_oddlot": {
			Status:        StatusOK,
			LastFetchAt:   now.Add(-17 * 24 * time.Hour).Format(time.RFC3339),
			LastSuccessAt: now.Add(-17 * 24 * time.Hour).Format(time.RFC3339),
		},
		"twse_capital_flow": {
			Status:        StatusOK,
			LastFetchAt:   now.Add(-2 * time.Minute).Format(time.RFC3339),
			LastSuccessAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
	}}
	b, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "channel_health.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := store.SyncAllToDB(); err != nil {
		t.Fatalf("SyncAllToDB: %v", err)
	}

	var status string
	var lastFetch, lastSuccess *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT status, last_fetch_at, last_success_at FROM channel_health WHERE channel_id = 'twse_oddlot'`,
	).Scan(&status, &lastFetch, &lastSuccess); err != nil {
		t.Fatalf("query twse_oddlot: %v", err)
	}
	// Rule 0 of DeriveChannelStatus (internal/apigateway/channel_status.go):
	// retirement is a CONTRACT-level fact and outranks the record's freshness,
	// so a retired channel reports StatusRetired even with a 17-day-old
	// LastFetchAt. twse_oddlot was retired by design when TWSE removed BFI84U,
	// so "stale" is no longer the verdict the channel page and the DB agree on.
	//
	// The stale-on-expiry rule itself stays covered by TestDeriveChannelStatus,
	// which runs this exact shape against a copy of the same contract with its
	// retirement stripped (internal/apigateway/channel_status_test.go).
	if status != StatusRetired {
		t.Errorf("DB status = %q, want retired (Rule 0: retirement outranks freshness — the DB and the channel page must agree)", status)
	}
	if lastFetch == nil || !lastFetch.UTC().Equal(now.Add(-17*24*time.Hour)) {
		t.Errorf("DB last_fetch_at = %v, want the record's real fetch time (%s) — not the sync clock",
			lastFetch, now.Add(-17*24*time.Hour))
	}
	if lastSuccess == nil || !lastSuccess.UTC().Equal(now.Add(-17*24*time.Hour)) {
		t.Errorf("DB last_success_at = %v, want the record's real success time", lastSuccess)
	}

	var freshStatus string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM channel_health WHERE channel_id = 'twse_capital_flow'`,
	).Scan(&freshStatus); err != nil {
		t.Fatalf("query twse_capital_flow: %v", err)
	}
	if freshStatus != StatusOK {
		t.Errorf("fresh channel DB status = %q, want ok (no false staleness)", freshStatus)
	}
}
