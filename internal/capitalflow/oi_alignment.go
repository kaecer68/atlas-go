package capitalflow

// OI source alignment guard — H-CF-01 data hygiene (arbitration report
// .omo/plans/2026-09-04-hcf01-arbitration.md §3, 2026-09-04).
//
// The platform has TWO futures-OI channels:
//
//  1. FinMind TaiwanFuturesInstitutionalInvestors snapshots under
//     data/state/taifex_oi/YYYY-MM-DD.json (contracts.TX.foreign.oi_net).
//  2. The TAIFEX CSV/OpenAPI channel merged into macro snapshots as
//     foreign_futures_oi_net (cmd/backfill-taifex-oi-v2).
//
// Empirical audit (2026-09-04): over 33 overlap days the two channels
// disagreed on 19, and on 18 of those 19 the macro snapshot value for day d
// exactly equals the FinMind value of the PREVIOUS trading day — i.e. the
// TAIFEX channel systematically carried the prior session's OI forward
// (weekday no-data merged unmarked). For a lag-sensitive signal such as
// H-CF-01 (ΔOI_t → spot_{t+1}) a one-session shift is a wholesale
// translation of the signal.
//
// BLOCK SCOPE (arbitration §3): until the macro channel is realigned AND
// verified with AssertOIDateAlignment, it must not be used
//   - as a direction-signal input anywhere, and
//   - mixed with / used to fill gaps in the FinMind channel by any loader.
// Offline H-CF-01 judgments are NOT blocked: they declare the FinMind
// data/state/taifex_oi snapshots as the SINGLE OI source.

import (
	"errors"
	"fmt"
	"sort"
)

// ErrMacroOISignalInputBlocked is the sentinel returned by guards that
// refuse the macro-snapshot foreign_futures_oi_net channel as an OI
// direction-signal input. Callers should errors.Is against it.
var ErrMacroOISignalInputBlocked = errors.New(
	"macro snapshot foreign_futures_oi_net is BLOCKED as OI signal input: " +
		"date-attribution bug (19/33 overlap days carried the previous session's value, macro(d)==FinMind(d-1)); " +
		"use data/state/taifex_oi (FinMind) as the single OI source until realignment passes AssertOIDateAlignment")

// MacroOISignalInputGuard is the compile-time-visible gate any wiring that
// wants to feed the macro channel's foreign_futures_oi_net into a direction
// signal MUST call. It always fails while the realignment is unverified —
// removing the block requires fixing the backfill date attribution and
// passing AssertOIDateAlignment over the repaired series.
func MacroOISignalInputGuard() error {
	return ErrMacroOISignalInputBlocked
}

// AssertOIDateAlignment verifies that the two OI channels agree on the SAME
// trading day. For every date present in both series, |macro[d]−finmind[d]|
// must be 0; any non-zero difference is a realignment failure (not noise —
// contract-level OI totals are exact integers).
//
// finMindOI is the reference series (data/state/taifex_oi). macroOI is the
// TAIFEX-CSV-channel series (macro snapshot foreign_futures_oi_net). Returns
// an error listing the first mismatching dates; nil means aligned.
func AssertOIDateAlignment(finMindOI, macroOI map[string]float64) error {
	var mismatches []string
	for d, mv := range macroOI {
		fv, ok := finMindOI[d]
		if !ok {
			continue // not an overlap day
		}
		if mv != fv {
			mismatches = append(mismatches, d)
		}
	}
	if len(mismatches) == 0 {
		return nil
	}
	sort.Strings(mismatches)
	const maxList = 10
	shown := mismatches
	if len(shown) > maxList {
		shown = shown[:maxList]
	}
	return fmt.Errorf("OI date alignment failed: %d/%d overlap days disagree (same-day |macro−FinMind|>0); first: %v%s — suspect unmarked carry-forward (macro(d)==FinMind(prev trading day)); macro channel stays blocked as signal input",
		len(mismatches), overlapDays(finMindOI, macroOI), shown, listSuffix(len(mismatches), maxList))
}

// DetectMacroOILagPattern counts overlap days where macro(d) equals the
// FinMind value of the previous FINMIND trading day — the fingerprint of the
// unmarked carry-forward bug. prevFinMindDate maps a date to the previous
// trading date present in the FinMind series (empty string when none).
// Diagnostic only; AssertOIDateAlignment is the gate.
func DetectMacroOILagPattern(finMindOI, macroOI map[string]float64, prevFinMindDate func(string) string) int {
	if prevFinMindDate == nil {
		return 0
	}
	count := 0
	for d, mv := range macroOI {
		if _, ok := finMindOI[d]; !ok {
			continue
		}
		if mv == 0 {
			continue
		}
		if pv, ok := finMindOI[prevFinMindDate(d)]; ok && mv == pv {
			count++
		}
	}
	return count
}

func overlapDays(a, b map[string]float64) int {
	n := 0
	for d := range a {
		if _, ok := b[d]; ok {
			n++
		}
	}
	return n
}

func listSuffix(total, maxShown int) string {
	if total > maxShown {
		return fmt.Sprintf(" … (%d more)", total-maxShown)
	}
	return ""
}
