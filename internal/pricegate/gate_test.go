package pricegate

import (
	"bytes"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/logging"
)

func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return buf
}

// TestCheck_GapMultipleRejected pins the corrupted-multiple case that the price
// guard alone cannot see: 5904.TW was stored at 720 for a session whose
// official close was 72 (FU-20260930-03). Both closes are usable prices, so
// before this gate the +900% return was consumed as real.
func TestCheck_GapMultipleRejected(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)

	if Check("test.gap", "5904.TW", 72, 720, 40, 1) {
		t.Fatal("Check(72 -> 720) = true, want false: a +900% daily move is a corrupted stored price")
	}
	logs := buf.String()
	if !strings.Contains(logs, "gap_multiple_rejected") {
		t.Errorf("rejected row must be auditable (WARN expected); logs:\n%s", logs)
	}
	for _, want := range []string{"symbol=5904.TW", "prev=72", "curr=720", "horizon_days=1"} {
		if !strings.Contains(logs, want) {
			t.Errorf("audit log missing %q; logs:\n%s", want, logs)
		}
	}

	// The reverse direction (a 10x drop) is just as implausible.
	if Check("test.gap", "5904.TW", 720, 72, 40, 1) {
		t.Error("Check(720 -> 72) = true, want false")
	}
}

// TestCheck_NormalDataUnchanged locks that the gate is a gross-error gate and
// not a volatility filter: borderline-but-real sessions pass untouched.
func TestCheck_NormalDataUnchanged(t *testing.T) {
	cases := [][2]float64{
		{100, 110},    // TWSE limit-up
		{100, 90},     // TWSE limit-down
		{100, 120},    // 20% instrument class
		{100, 100.01}, // flat
		{100, 150},    // exactly at the bound
		{100, 50},     // exactly at the bound, down
	}
	for _, tc := range cases {
		if !Check("test.normal", "T.TW", tc[0], tc[1], 40, 1) {
			t.Errorf("Check(%v -> %v) = false, want true", tc[0], tc[1])
		}
	}
}

// TestCheck_MissingPriceSkippedSilently locks the FU-20260930-04 half: an
// unusable close is an expected missing observation, so it is skipped WITHOUT a
// WARN (otherwise the log would fill with the documented close<=0 rows).
func TestCheck_MissingPriceSkippedSilently(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)
	for _, bad := range []float64{0, -5, math.NaN(), math.Inf(1)} {
		if Check("test.missing", "T.TW", bad, 100, 40, 1) {
			t.Errorf("Check with prev=%v = true, want false", bad)
		}
		if Check("test.missing", "T.TW", 100, bad, 40, 1) {
			t.Errorf("Check with curr=%v = true, want false", bad)
		}
	}
	if logs := buf.String(); logs != "" {
		t.Errorf("missing observations must not be logged as gap multiples; logs:\n%s", logs)
	}
}

// TestCheck_IPOExemptWindow pins the first mandatory condition of k3 decision
// 3-1: a newly listed symbol has no daily price limit, so a >50% move inside
// its first 5 trading days is real data and must pass (and be auditable).
func TestCheck_IPOExemptWindow(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)

	// pos 5 = the 5th daily return of the symbol, still inside the window.
	if !Check("test.ipo", "9999.TW", 50, 90, 5, 1) {
		t.Fatal("first-5-day IPO move rejected, want accepted (no price limit applies)")
	}
	if logs := buf.String(); !strings.Contains(logs, "gap_multiple_ipo_exempt") {
		t.Errorf("IPO exemption must be auditable; logs:\n%s", logs)
	}

	// pos 6 is the 6th return: the price limit applies again.
	if Check("test.ipo", "9999.TW", 50, 90, 6, 1) {
		t.Error("move outside the IPO window accepted, want rejected")
	}
}

// TestCheck_HorizonScalesTheBound pins the horizon rule: a 5-day return may
// legitimately exceed 50% under a 10% daily limit (1.1^5-1 = 61%), so the bound
// compounds instead of staying at 50%.
func TestCheck_HorizonScalesTheBound(t *testing.T) {
	if !Check("test.horizon", "T.TW", 100, 161, 40, 5) {
		t.Error("+61% over 5 days rejected, want accepted (10% limit compounds to 61%)")
	}
	if Check("test.horizon", "T.TW", 72, 720, 40, 5) {
		t.Error("+900% over 5 days accepted, want rejected")
	}
}

// TestWarnIsBoundedPerComponent keeps a corrupted series from turning the gate
// into a log flood, while still telling an operator that more rows were
// suppressed.
func TestWarnIsBoundedPerComponent(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)
	const component = "test.warn_cap"
	for range warnCapPerComponent + 5 {
		Check(component, "T.TW", 72, 720, 40, 1)
	}
	logs := buf.String()
	if n := strings.Count(logs, "msg=gap_multiple_rejected "); n != warnCapPerComponent-1 {
		t.Errorf("individual WARN lines = %d, want %d", n, warnCapPerComponent-1)
	}
	if n := strings.Count(logs, "gap_multiple_rejected_suppressed"); n != 1 {
		t.Errorf("suppression notice = %d, want 1; logs tail:\n%s", n, lastLines(logs, 3))
	}
	if n := strings.Count(logs, "level=WARN"); n != warnCapPerComponent {
		t.Errorf("total WARN lines = %d, want %d", n, warnCapPerComponent)
	}
}

// TestSharedBoundariesAreWiredIntoCheck guards the coupling between this gate
// and the shared predicates without restating them.
func TestSharedBoundariesAreWiredIntoCheck(t *testing.T) {
	if got := shared.MaxPlausibleReturn(1); got != shared.MaxPlausibleDailyReturn {
		t.Errorf("MaxPlausibleReturn(1) = %v, want %v", got, shared.MaxPlausibleDailyReturn)
	}
	if got, want := shared.MaxPlausibleReturn(5), math.Pow(1.5, 5)-1; math.Abs(got-want) > 1e-12 {
		t.Errorf("MaxPlausibleReturn(5) = %v, want %v", got, want)
	}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) <= n {
		return s
	}
	return fmt.Sprintf("...\n%s", strings.Join(lines[len(lines)-n:], "\n"))
}
