package capitalflow

// Tests for the H-CF-01 v2 family batch runner (RunHCF01V2Family).
// Synthetic data only (same generator style as validation_v2_test.go);
// the judgment logic itself is locked in validation_v2.go and is NOT
// re-tested here — these tests pin the WIRING: same-batch Holm
// assignment, abandonment-line evaluation, exact n / drop list, and
// the single-source declaration.

import (
	"math"
	"strings"
	"testing"
)

// TestRunHCF01V2Family_ShapeAndHolm runs the family on the v2a
// PASS-case generator and pins the wiring contract: three results with
// the pre-registered IDs, a full Holm ranking, alphas exactly
// HolmAlphaForRank(rank, 3), primary p-values recomputed from the
// final metrics, per-judgment status fidelity against a direct
// validator call at the assigned alpha, exact n, the declaration, and
// the abandonment-line consistency.
func TestRunHCF01V2Family_ShapeAndHolm(t *testing.T) {
	// Strict downtrend (same style as the v2a′ PASS test) keeps the
	// sell-side quadrant populated so NONE of the three judgments is
	// INSUFFICIENT_DATA and the Holm bookkeeping is fully exercised;
	// 440 days keep v2b above the 252-day evaluation threshold (the
	// strict-downtrend generator yields ~248 evaluable v2b days per 400). The
	// statuses themselves are irrelevant here — the wiring is what is
	// pinned (see the assertions below).
	in := genHCF01V2Inputs(440,
		func(i int) float64 {
			if i%2 == 0 {
				return 0.56
			}
			return 0.24
		},
		func(i int) int { return -1 },
	)
	fam := RunHCF01V2Family(in)

	results := map[string]HypothesisResult{
		"H-CF-01-v2a":       fam.V2A,
		"H-CF-01-v2a-prime": fam.V2APrime,
		"H-CF-01-v2b":       fam.V2B,
	}
	if len(fam.ExactN) != 3 {
		t.Fatalf("family must carry exactly the three pre-registered judgments, got %d", len(fam.ExactN))
	}
	for id, res := range results {
		if res.ID != id {
			t.Fatalf("result ID = %s, want %s", res.ID, id)
		}
		if fam.ExactN[id] != res.SampleCount {
			t.Fatalf("%s ExactN = %d, want SampleCount %d", id, fam.ExactN[id], res.SampleCount)
		}
		if res.Status == ValidationInsufficientData {
			t.Fatalf("%s unexpectedly INSUFFICIENT_DATA on the 400-day generator", id)
		}
	}

	// Full ranking: ranks are a permutation of {0,1,2}, alphas match
	// HolmAlphaForRank exactly, and primary p-values recompute from the
	// final metrics.
	if len(fam.HolmRanks) != 3 {
		t.Fatalf("all three judgments must be ranked, got %d", len(fam.HolmRanks))
	}
	seen := map[int]string{}
	for id, r := range fam.HolmRanks {
		if r < 0 || r > 2 {
			t.Fatalf("%s rank = %d", id, r)
		}
		if other, dup := seen[r]; dup {
			t.Fatalf("rank %d assigned twice (%s, %s)", r, id, other)
		}
		seen[r] = id
		if got := fam.HolmAlphas[id]; got != HolmAlphaForRank(r, HCF01V2HolmFamilySize) {
			t.Fatalf("%s alpha = %.6g, want %.6g", id, got, HolmAlphaForRank(r, HCF01V2HolmFamilySize))
		}
	}
	recomputed := map[string]float64{
		"H-CF-01-v2a":       math.Min(results["H-CF-01-v2a"].Metrics["stat_layer_p"], results["H-CF-01-v2a"].Metrics["increment_p"]),
		"H-CF-01-v2a-prime": results["H-CF-01-v2a-prime"].Metrics["diff_p"],
		"H-CF-01-v2b":       results["H-CF-01-v2b"].Metrics["binomial_p"],
	}
	for id, want := range recomputed {
		if fam.HolmPValues[id] != want {
			t.Fatalf("%s HolmPValues = %.6g, want %.6g recomputed from final metrics", id, fam.HolmPValues[id], want)
		}
	}

	// Status fidelity: each final status must equal a direct validator
	// call at its assigned Holm alpha (pure wiring — no logic drift).
	for id, res := range results {
		inAtAlpha := in
		inAtAlpha.HolmAlpha = fam.HolmAlphas[id]
		var direct HypothesisResult
		switch id {
		case "H-CF-01-v2a":
			direct = ValidateHCF01V2A(inAtAlpha)
		case "H-CF-01-v2a-prime":
			direct = ValidateHCF01V2APrime(inAtAlpha)
		case "H-CF-01-v2b":
			direct = ValidateHCF01V2B(inAtAlpha)
		}
		if direct.Status != res.Status {
			t.Fatalf("%s status = %s, direct call at assigned alpha = %s", id, res.Status, direct.Status)
		}
	}

	// Panel audit consistency: fully evaluable panel days must equal the
	// v2a regression sample (same filters over the same buildHCF01V2Days
	// panel).
	panel := buildHCF01V2Days(in)
	_, _, panelN, evaluated := hcf01V2PanelDropAudit(panel)
	if evaluated != fam.V2A.SampleCount {
		t.Fatalf("panel evaluated %d != v2a SampleCount %d (drop-audit drift)", evaluated, fam.V2A.SampleCount)
	}
	if panelN != len(panel) {
		t.Fatalf("panelN = %d, want %d", panelN, len(panel))
	}

	// Single-source declaration verbatim on every family result.
	for id, res := range results {
		found := false
		for _, n := range res.Notes {
			if n == HCF01V2SingleSourceDeclaration {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s notes missing the single-source declaration", id)
		}
	}

	// Abandonment line consistency with the final statuses.
	wantTriggered, wantNote := EvaluateHCF01AbandonmentLine(fam.V2A.Status, fam.V2APrime.Status, fam.V2B.Status)
	if fam.AbandonmentTriggered != wantTriggered || fam.AbandonmentNote != wantNote {
		t.Fatalf("abandonment line drifted: got (%t, %q), want (%t, %q)",
			fam.AbandonmentTriggered, fam.AbandonmentNote, wantTriggered, wantNote)
	}
}

// TestRunHCF01V2Family_InsufficientData: tiny samples → all three
// INSUFFICIENT_DATA, no Holm ranking (no α consumed), abandonment line
// NOT triggered with the pre-registered INSUFFICIENT_DATA note.
func TestRunHCF01V2Family_InsufficientData(t *testing.T) {
	in := genHCF01V2Inputs(20,
		func(i int) float64 { return 0.5 },
		func(i int) int {
			if i%2 == 0 {
				return 1
			}
			return -1
		},
	)
	fam := RunHCF01V2Family(in)
	for id, res := range map[string]HypothesisResult{
		"H-CF-01-v2a": fam.V2A, "H-CF-01-v2a-prime": fam.V2APrime, "H-CF-01-v2b": fam.V2B,
	} {
		if res.Status != ValidationInsufficientData {
			t.Fatalf("%s status = %s, want INSUFFICIENT_DATA", id, res.Status)
		}
	}
	if len(fam.HolmRanks) != 0 || len(fam.HolmAlphas) != 0 {
		t.Fatalf("INSUFFICIENT_DATA judgments must not be ranked (got %d ranks)", len(fam.HolmRanks))
	}
	if fam.AbandonmentTriggered {
		t.Fatal("INSUFFICIENT_DATA must never trigger the abandonment line")
	}
	joined := strings.Join(fam.V2A.Notes, "\n")
	for _, want := range []string{"INSUFFICIENT_DATA", "不參與 Holm 排序"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("v2a notes missing %q", want)
		}
	}
}

// TestRunHCF01V2Family_DropList: inputs with deliberate holes — flat
// TAIEX (ret==0 abstain), a zero spot day, and a missing OI day — must
// surface as per-day drop entries with the pre-registered reasons, and
// the summary note must carry the verbatim date=reason list.
func TestRunHCF01V2Family_DropList(t *testing.T) {
	in := genHCF01V2Inputs(60,
		func(i int) float64 { return 0.5 },
		func(i int) int {
			if i%2 == 0 {
				return 1
			}
			return -1
		},
	)
	dates := in.Dates
	// Indices chosen away from the monthly rollover windows (the 3rd
	// Wednesday + next trading day: idx 9/10, 34/35, 54/55 on this
	// calendar) so rollover never masks the drop reason under test.
	// ret==0 abstain: repeat the previous close on one day.
	in.TAIEX[dates[25]] = in.TAIEX[dates[24]]
	// spot tie (0) on one day.
	in.SpotNet[dates[30]] = 0
	// missing OI on one day → ΔOI unknown on that day.
	delete(in.FuturesOI, dates[40])

	fam := RunHCF01V2Family(in)
	reasons := map[string]string{}
	for _, e := range fam.DropList {
		reasons[e.Date] = e.Reason
	}
	wantDrops := map[string]string{
		dates[25]: "abstain_ret_zero_or_missing",
		dates[30]: "spot_missing_or_tie",
		dates[40]: "abstain_ret_zero_or_missing",
	}
	for date, want := range wantDrops {
		if reasons[date] != want {
			t.Fatalf("drop[%s] = %q, want %q", date, reasons[date], want)
		}
	}
	// The summary note lists every drop verbatim.
	summary := ""
	for _, n := range fam.V2A.Notes {
		if strings.Contains(n, "逐日 drop 清單") {
			summary = n
			break
		}
	}
	for date, want := range wantDrops {
		if !strings.Contains(summary, date+"="+want) {
			t.Fatalf("drop summary note missing %s=%s: %s", date, want, summary)
		}
	}
	if strings.Contains(summary, "逐日 drop 清單：無") {
		t.Fatalf("drop summary claims no drops but %d were recorded", len(fam.DropList))
	}
}
