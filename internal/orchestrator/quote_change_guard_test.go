package orchestrator

// quote_change_guard_test.go — k3 decision 3-3 / FU-20260930-04 follow-up.
//
// Both quote projections computed (Last-Open)/Open*100 with NO divisor guard.
// A quote whose Open is 0 (or NaN/Inf) therefore produced +-Inf / NaN:
//
//   PROBE narrative DXY=+Inf US10Ybps=4.2 VIX=20
//   PROBE macro US10Y={Symbol: Value:4.2 ChangePct:+Inf Timestamp:0}
//
// and nothing was logged, so the malformed session move looked like a real one.
// The rule under test: an unusable Open makes the change a MISSING observation
// (field keeps its zero value) and the skip is logged for audit. Normal quotes
// keep the exact same arithmetic.

import (
	"math"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// hasSkippedQuoteWarn reports whether an audit WARN for symbol appears in logs.
// slog quotes a value that itself contains '=' (e.g. symbol="GC=F"), so both
// spellings are accepted; the assertion stays exact about the event and symbol.
func hasSkippedQuoteWarn(logs, symbol string) bool {
	return strings.Contains(logs, "quote_change_skipped") &&
		(strings.Contains(logs, "symbol="+symbol) || strings.Contains(logs, "symbol=\""+symbol+"\""))
}

func TestQuotesToNarrativeData_UnusableOpenIsSkippedAndLogged(t *testing.T) {
	buf := captureOrchestratorLogsAt(t, -4) // WARN and above

	quotes := []domain.Quote{
		{Symbol: "DXY", Open: 0, Last: 110},
		{Symbol: "^GSPC", Open: 0, Last: 7616},
		{Symbol: "BDI", Open: math.NaN(), Last: 1950},
		{Symbol: "HG=F", Open: -1, Last: 4.2},
		{Symbol: "TSM", Open: math.Inf(1), Last: 210},
		// One healthy quote in the same call: the guard must not affect it.
		{Symbol: "^IXIC", Open: 25892, Last: 26186},
	}
	data := QuotesToNarrativeData(quotes)

	if data.DXYChangePct != 0 {
		t.Errorf("DXYChangePct = %v, want 0 (unusable open = missing observation, not +Inf)", data.DXYChangePct)
	}
	if data.SPXIndexChangePct != 0 {
		t.Errorf("SPXIndexChangePct = %v, want 0", data.SPXIndexChangePct)
	}
	if data.BDIChangePct != 0 || math.IsNaN(data.BDIChangePct) {
		t.Errorf("BDIChangePct = %v, want 0 for a NaN open", data.BDIChangePct)
	}
	if data.CopperChangePct != 0 {
		t.Errorf("CopperChangePct = %v, want 0 for a negative open", data.CopperChangePct)
	}
	if data.TSMADRChangePct != 0 {
		t.Errorf("TSMADRChangePct = %v, want 0 for an Inf open", data.TSMADRChangePct)
	}
	if want := (26186.0 - 25892.0) / 25892.0 * 100; math.Abs(data.NDXIndexChangePct-want) > 1e-12 {
		t.Errorf("NDXIndexChangePct = %v, want %v (normal quote must not change)", data.NDXIndexChangePct, want)
	}

	logs := buf.String()
	for _, symbol := range []string{"DXY", "^GSPC", "BDI", "HG=F", "TSM"} {
		if !hasSkippedQuoteWarn(logs, symbol) {
			t.Errorf("missing audit WARN for skipped symbol %s; logs:\n%s", symbol, logs)
		}
	}
	if strings.Contains(logs, "symbol=^IXIC") {
		t.Errorf("normal quote ^IXIC must not be logged as skipped; logs:\n%s", logs)
	}
}

func TestQuotesToMacroDataSnapshot_UnusableOpenIsSkippedAndLogged(t *testing.T) {
	buf := captureOrchestratorLogsAt(t, -4)

	quotes := []domain.Quote{
		{Symbol: "^TNX", Open: 0, Last: 4.2},
		{Symbol: "^VIX", Open: math.NaN(), Last: 25},
		{Symbol: "GC=F", Open: 0, Last: 2350},
		// Control: healthy quotes keep their exact change.
		{Symbol: "CL=F", Open: 80, Last: 84},
	}
	snapshot := QuotesToMacroDataSnapshot(quotes)

	if snapshot.US10Y != (narrative.MacroDataPoint{}) {
		t.Errorf("US10Y = %+v, want the zero MacroDataPoint (unusable open = missing observation)", snapshot.US10Y)
	}
	if snapshot.VIX.Value != 0 || snapshot.VIX.ChangePct != 0 {
		t.Errorf("VIX = %+v, want zero for a NaN open", snapshot.VIX)
	}
	if snapshot.Gold.Value != 0 || snapshot.Gold.ChangePct != 0 {
		t.Errorf("Gold = %+v, want zero for a zero open", snapshot.Gold)
	}
	if want := 5.0; math.Abs(snapshot.Oil.ChangePct-want) > 1e-12 {
		t.Errorf("Oil.ChangePct = %v, want %v (normal quote must not change)", snapshot.Oil.ChangePct, want)
	}

	logs := buf.String()
	for _, symbol := range []string{"^TNX", "^VIX", "GC=F"} {
		if !hasSkippedQuoteWarn(logs, symbol) || !strings.Contains(logs, "component=orchestrator.macro_snapshot") {
			t.Errorf("missing audit WARN for skipped symbol %s; logs:\n%s", symbol, logs)
		}
	}
	if strings.Contains(logs, "symbol=CL=F") {
		t.Errorf("normal quote CL=F must not be logged as skipped; logs:\n%s", logs)
	}
}
