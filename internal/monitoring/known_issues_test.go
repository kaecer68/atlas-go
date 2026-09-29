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
// TestLookupKnownIssue_ReturnsRegisteredEntry covers both the canonical
// underscore-separated channel IDs (twse_etf, twse_oddlot) AND the
// dash-separated runtime aliases (twse-etf, twse-oddlot). The dash
// variants are the same upstream issue observed at runtime via a
// different naming convention; PR-D (2026-08-05) registered both so
// the dashboard renders the known-issue badge regardless of which form
// the runtime channel_health record carries.
func TestLookupKnownIssue_ReturnsRegisteredEntry(t *testing.T) {
	for _, id := range []string{"twse_etf", "twse-etf", "twse_oddlot", "twse-oddlot", "taifex-daily", "bdi"} {
		issue := LookupKnownIssue(id)
		if issue == nil {
			t.Errorf("%q should have a known issue (PR-C + PR-D), got nil", id)
			continue
		}
		if issue.Key == "" {
			t.Errorf("%q: KnownIssue.Key must be non-empty", id)
		}
		if issue.DocumentedAt == "" {
			t.Errorf("%q: KnownIssue.DocumentedAt must be non-empty (UI shows 'known for X days')", id)
		}
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
	// PR-D (2026-08-05) added the dash-separated aliases twse-etf and
	// twse-oddlot so the dashboard renders known-issue badges on both
	// the canonical (underscore) and runtime-observed (dash) forms.
	// 2026-09-23: "bdi" was added for the CNBC `.BADI` empty-quote outage
	// (bdi_cnbc_empty_quote_2026_09) — also asserted explicitly in
	// TestLookupKnownIssue_BDIEmptyQuote.
	for _, required := range []string{"twse_etf", "twse-etf", "twse_oddlot", "twse-oddlot", "bdi"} {
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

// TestLookupKnownIssue_BDIEmptyQuote pins the 2026-09-23 known-issue entry for
// the CNBC `.BADI` empty-quote outage: the dashboard resolves the badge through
// LookupKnownIssue, so the key, the date and the two facts an on-call reader
// needs (this is externally caused; alternative sources are dead) must all be
// present in the registry.
func TestLookupKnownIssue_BDIEmptyQuote(t *testing.T) {
	issue := LookupKnownIssue("bdi")
	if issue == nil {
		t.Fatal("bdi must have a known issue declared (CNBC .BADI empty quote, 2026-09-20T08:35Z)")
	}
	if issue.Key != "bdi_cnbc_empty_quote_2026_09" {
		t.Errorf("Key = %q, want bdi_cnbc_empty_quote_2026_09", issue.Key)
	}
	if issue.DocumentedAt == "" {
		t.Error("DocumentedAt must be set (the dashboard shows how long the issue has been known)")
	}
	if issue.TrackingURL == "" {
		t.Error("TrackingURL must be set")
	}
	for _, want := range []string{"2026-09-20T08:35Z", "ErrEmptyQuote", "delisted", "mergeWithPrev", "circuit breaker", ".BADI"} {
		if !strings.Contains(issue.Description, want) {
			t.Errorf("Description must mention %q so the badge is self-explanatory; got: %s", want, issue.Description)
		}
	}
}

// TestKnownIssue_RetiredChannelsSaySo — issue #2134. The known-issue registry
// used to describe twse_etf / twse_oddlot as long-standing failures that are
// "consciously deferred". They are neither deferred nor failing any more: the
// upstreams are gone for good, both fetch paths are retired, and replacement
// inputs are wired. The badge text is the only place an operator can learn
// that, so it must say "RETIRED" — otherwise the next reader re-investigates a
// closed issue (that is exactly how the 60+ day entry kept its misleading
// "waiting for a fix" framing).
func TestKnownIssue_RetiredChannelsSaySo(t *testing.T) {
	retired := map[string]string{
		"twse_oddlot": "twse_capital_flow", // replacement input
		"twse-oddlot": "twse_capital_flow",
		"twse_etf":    "Fubon",
		"twse-etf":    "Fubon",
	}
	for id, replacement := range retired {
		issue := LookupKnownIssue(id)
		if issue == nil {
			t.Errorf("%q missing from the known-issue registry", id)
			continue
		}
		if !strings.Contains(issue.Title, "RETIRED") {
			t.Errorf("%s title = %q, want it to say RETIRED (the channel is off, not deferred)", id, issue.Title)
		}
		if !strings.Contains(issue.Description, "RETIRED") {
			t.Errorf("%s description must start from the retirement fact, got %q", id, issue.Description)
		}
		if !strings.Contains(issue.Description, replacement) {
			t.Errorf("%s description must name the replacement input %q so the next reader knows where the data comes from", id, replacement)
		}
	}
	// The dead-alias entry (canonical channel healthy) is deliberately NOT a
	// retirement: taifex-daily must keep its "dead alias" framing.
	if issue := LookupKnownIssue("taifex-daily"); issue == nil || strings.Contains(issue.Title, "RETIRED") {
		t.Errorf("taifex-daily is a dead alias of a healthy channel, not a retired channel")
	}
}
