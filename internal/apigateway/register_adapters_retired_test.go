package apigateway

// Regression locks for the twse_oddlot retirement (issue #2134).
//
// Production kept ChannelHealthStatusError{channel="twse_oddlot"} firing
// permanently (measured 2026-09-29) because the record left behind by the last
// fetch attempt was status="degraded" and DeriveChannelStatus escalates a
// degraded record to ERROR once the data behind it is older than the contract
// freshness window (E29-3 rule 2b). Removing the adapter registration alone
// therefore does NOT silence the alert — the leftover record keeps exporting
// atlas_channel_health_status=2. The retirement must (a) drop the fetch path and
// (b) leave the record in a verdict that never escalates.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// retiredOddLotIDs are the two spellings of the retired channel. The dash form
// is the historical runtime ID: no alert rule matches either spelling once the
// record is inactive, but an environment that still carries the frozen
// "circuit breaker open for channel twse-oddlot" record would keep firing on
// the alias, so both are written.
var retiredOddLotIDs = []string{"twse_oddlot", "twse-oddlot"}

func newRetirementGateway(t *testing.T) *Gateway {
	t.Helper()
	g, err := NewGateway(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	return g
}

// TestRegisterChannelAdapters_TwseOddlotRetired locks the retirement shape:
// the fetch path is gone, and the record the metrics export reads can no longer
// produce the paging verdict.
func TestRegisterChannelAdapters_TwseOddlotRetired(t *testing.T) {
	g := newRetirementGateway(t)
	if err := RegisterChannelAdapters(g, t.TempDir(), config.Config{}, nil, nil); err != nil {
		t.Fatalf("RegisterChannelAdapters: %v", err)
	}

	// 1. Fetch path retired: no adapter behind the channel ID. A registered
	// adapter is what a background task or fetcher would probe, and a Fetch on
	// an unregistered channel is counted as a circuit-breaker failure.
	if provider, err := g.registry.Get("twse_oddlot"); err == nil {
		t.Errorf("twse_oddlot adapter must not be registered (fetch path retired), got %T", provider)
	}

	// 2. The record is "inactive" for BOTH IDs (this is the actual fix: the
	// trigger of the firing alert was the degraded record, not the adapter).
	for _, id := range retiredOddLotIDs {
		rec := g.Health().Get(id)
		if rec == nil {
			t.Fatalf("no health record for %q: the retirement must write one, otherwise a leftover degraded record (or none at all) is what the metrics export reads", id)
		}
		if rec.Status != StatusInactive {
			t.Errorf("%s record status = %q, want %q", id, rec.Status, StatusInactive)
		}
		if !strings.Contains(rec.LastError, "twse_capital_flow") {
			t.Errorf("%s record reason = %q, want it to name the replacement input (twse_capital_flow)", id, rec.LastError)
		}
		if derived := DeriveChannelStatusForID(rec, id, time.Now()); derived != StatusInactive {
			t.Errorf("%s derived status = %q, want %q (an inactive verdict never escalates)", id, derived, StatusInactive)
		}
	}

	// 3. Identity kept: retirement is not deletion. channelIDs() is the
	// contract/limiter/alert bookkeeping list; keeping the ID there is what
	// keeps ValidateContracts, the alias resolution and the dashboard badge
	// working (same choice as twse_etf).
	found := false
	for _, id := range channelIDs() {
		if id == "twse_oddlot" {
			found = true
		}
	}
	if !found {
		t.Error("twse_oddlot must stay in channelIDs(): retirement removes the fetch path, not the channel identity")
	}
}

// TestDeriveChannelStatus_TwseOddlotRetirementClosesTheAlertLoop pins the exact
// production shapes: the pre-fix record escalated to error (which is the gauge
// value 2 the alert rule matches), and the retired record does not.
func TestDeriveChannelStatus_TwseOddlotRetirementClosesTheAlertLoop(t *testing.T) {
	contract := ChannelContracts().Contract("twse_oddlot")

	// Production 2026-09-29: degraded record, last real data 2026-09-07, contract
	// window 48h. DeriveChannelStatus escalates it to error — this is what made
	// ChannelHealthStatusError fire forever, and why deregistering the adapter
	// alone was not a fix.
	preFix := &ChannelHealthRecord{
		Status:        StatusDegraded,
		LastFetchAt:   "2026-09-27T12:59:28Z",
		LastSuccessAt: "2026-09-07T00:18:11Z",
		LastError:     "twse_oddlot: 上游回傳空/停用資料（stale payload）",
	}
	if got := DeriveChannelStatus(preFix, contract, time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)); got != StatusError {
		t.Fatalf("pre-fix production record derived status = %q, want %q (this is the firing condition the retirement removes)", got, StatusError)
	}

	// Post-fix: the retirement writes "inactive" and keeps the same historical
	// timestamps, so the escalation branch (degraded only) is unreachable.
	retired := &ChannelHealthRecord{
		Status:        StatusInactive,
		LastFetchAt:   "2026-09-29T07:00:00Z",
		LastSuccessAt: "2026-09-07T00:18:11Z",
		LastError:     "BFI84U 上游已由 TWSE 移除（2026-08，改服務停券預告表）⇒ 本 channel 永久退役，不再抓取；零售商零股失衡輸入改由 twse_capital_flow 代理（monitoring.NewOddLotFetcher）",
	}
	for _, now := range []time.Time{
		time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 31, 7, 0, 0, 0, time.UTC),
	} {
		if got := DeriveChannelStatus(retired, contract, now); got != StatusInactive {
			t.Errorf("retired record derived status at %s = %q, want %q (must never age into error)", now.Format(time.RFC3339), got, StatusInactive)
		}
	}
}

// TestNoProductionPathProbesRetiredOddLotChannel is the source-level lock for
// "the fetch path is retired": any new non-test call site that passes the
// retired ID to a function — a registry registration, a gateway Fetch, a health
// probe — would reopen the trap the twse_etf probe hit in 2026-08-18: an
// unregistered channel's Fetch is booked as a circuit-breaker failure and
// overwrites the intentional inactive record with a permanent error.
//
// The rule is deliberately about CALL ARGUMENTS, not about the ID appearing at
// all: the retirement write in register_adapters.go legitimately mentions both
// spellings (composite literal, not a call), and comments are exempt.
func TestNoProductionPathProbesRetiredOddLotChannel(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	// "probe(... "twse_oddlot")" on one line = the retired ID handed to a probe or
	// registry call. Go call arguments do not span lines in this codebase.
	// Contract metadata is exempt: the retired channel keeps its contract (like
	// twse_etf), so live("twse_oddlot", ...) / Contract("twse_oddlot") stay legal.
	callWithID := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_.]*)\s*\([^)]*"(twse_oddlot|twse-oddlot)"`)
	// The retired ID has exactly two legitimate call sites: the contract
	// declaration (live) and the contract lookup that reads it back. Everything
	// else — a registry registration, a gateway Fetch, a health probe, or any
	// helper that forwards the ID to one of those — is a re-opened fetch path.
	legitFuncs := map[string]bool{"live": true, "Contract": true}

	var offenders []string
	scanned := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "data", "sessions", ".omo":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			match := callWithID.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			fn := match[1]
			if idx := strings.LastIndex(fn, "."); idx >= 0 {
				fn = fn[idx+1:]
			}
			if !legitFuncs[fn] {
				offenders = append(offenders, rel+":"+itoa(i+1)+"  "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk repo: %v", walkErr)
	}
	if scanned < 100 {
		t.Fatalf("only %d production .go files scanned under %s — the guard is not looking at the repo", scanned, root)
	}
	if len(offenders) > 0 {
		t.Errorf("production code must not probe the retired twse_oddlot channel (twse_capital_flow proxy is the input now); found:\n%s",
			strings.Join(offenders, "\n"))
	}
	t.Logf("scanned %d production .go files: no probe of the retired twse_oddlot channel", scanned)
}

// itoa avoids pulling strconv in for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
