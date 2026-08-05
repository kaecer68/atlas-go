package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lookup returns the right entry for channels that have a known issue
// declared (currently twse_etf and twse_oddlot per the v3.0 dispatch).
func TestLookupKnownIssue_ReturnsRegisteredEntry(t *testing.T) {
	issue := LookupKnownIssue("twse_etf")
	if issue == nil {
		t.Fatal("twse_etf should have a known issue, got nil")
	}
	if issue.Key == "" {
		t.Errorf("KnownIssue.Key must be non-empty")
	}
	if !strings.Contains(strings.ToLower(issue.Title), "etf") {
		t.Errorf("title should mention ETF, got %q", issue.Title)
	}
	if issue.DocumentedAt == "" {
		t.Errorf("KnownIssue.DocumentedAt must be non-empty (UI shows 'known for X days')")
	}
}

// TestLookupKnownIssue_ReturnsCopyNotReference ensures callers can't
// mutate the registry by holding a reference to the returned struct.
// (Returning a pointer to the map value would let external code change
// the description; the implementation returns a copy.)
func TestLookupKnownIssue_ReturnsCopyNotReference(t *testing.T) {
	first := LookupKnownIssue("twse_oddlot")
	if first == nil {
		t.Fatal("twse_oddlot should have a known issue, got nil")
	}
	origTitle := first.Title
	first.Title = "MUTATED"

	second := LookupKnownIssue("twse_oddlot")
	if second == nil {
		t.Fatal("second lookup returned nil")
	}
	if second.Title != origTitle {
		t.Errorf("registry was mutated through returned pointer: orig=%q, second=%q",
			origTitle, second.Title)
	}
}

// TestLookupKnownIssue_UnknownChannelReturnsNil covers the happy path
// for healthy channels: lookup returns nil, so the dashboard JSON
// omits the `known_issue` field (it's tagged `omitempty`).
func TestLookupKnownIssue_UnknownChannelReturnsNil(t *testing.T) {
	if LookupKnownIssue("finmind") != nil {
		t.Errorf("finmind is a healthy channel, must NOT have a known issue declared")
	}
	if LookupKnownIssue("") != nil {
		t.Errorf("empty channel ID must return nil, not a known issue")
	}
}

// TestKnownIssueChannelIDs enumerates channels in the registry. If a
// new known issue is added the test must be updated — this is a guard
// against silent registry growth.
func TestKnownIssueChannelIDs(t *testing.T) {
	ids := KnownIssueChannelIDs()
	if len(ids) < 2 {
		t.Errorf("expected at least 2 known-issue channels (twse_etf, twse_oddlot), got %d", len(ids))
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, required := range []string{"twse_etf", "twse_oddlot"} {
		if !got[required] {
			t.Errorf("required known-issue channel %q not in registry", required)
		}
	}
}

// TestDashboardAPI_ChannelHealthEndpoint_KnownIssueField is the
// integration test: when the channel-health JSON file contains a
// channel that's in the known-issue registry, the endpoint surfaces
// the KnownIssue metadata so the dashboard can render the badge.
func TestDashboardAPI_ChannelHealthEndpoint_KnownIssueField(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "data/state"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	payload := map[string]any{
		"channels": map[string]any{
			"twse_etf": map[string]any{
				"status":        "error",
				"last_fetch_at": now.Format(time.RFC3339),
				"last_error":    "circuit breaker open for channel twse_etf",
			},
			"fugle": map[string]any{
				"status":        "ok",
				"last_fetch_at": now.Format(time.RFC3339),
			},
		},
		"updated_at": now.Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "data/state", "channel_health.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := NewDashboardAPIWithGateway(tmpDir, tmpDir, nil, NoopFetcher())
	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Channels []map[string]any `json:"channels"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var twse, fugle map[string]any
	for _, ch := range resp.Channels {
		switch ch["channel_id"] {
		case "twse_etf":
			twse = ch
		case "fugle":
			fugle = ch
		}
	}

	if twse == nil {
		t.Fatal("twse_etf channel missing from response")
	}
	ki, ok := twse["known_issue"]
	if !ok {
		t.Fatal("twse_etf must include known_issue field (it's in the registry)")
	}
	kiMap, ok := ki.(map[string]any)
	if !ok {
		t.Fatalf("known_issue should be an object, got %T", ki)
	}
	if _, ok := kiMap["title"]; !ok {
		t.Errorf("known_issue.title missing")
	}
	if _, ok := kiMap["documented_at"]; !ok {
		t.Errorf("known_issue.documented_at missing")
	}

	// fugle is healthy — must NOT include the field.
	if fugle == nil {
		t.Fatal("fugle channel missing from response")
	}
	if _, ok := fugle["known_issue"]; ok {
		t.Errorf("fugle is healthy, must NOT include known_issue field")
	}
}
