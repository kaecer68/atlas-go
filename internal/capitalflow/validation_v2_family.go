package capitalflow

// Batch entry point for the H-CF-01 v2 judgment family — PR-3 wiring
// (arbitration .omo/plans/2026-09-04-hcf01-arbitration.md §2.2–2.6,
// the "判決 PR" work item). This file adds NO thresholds and NO
// judgment logic: it only sequences the three pre-registered validators
// (ValidateHCF01V2A / ValidateHCF01V2APrime / ValidateHCF01V2B — all
// decision logic locked in validation_v2.go), applies the same-batch
// Holm correction (§2.6), evaluates the abandonment line (§2.5), and
// surfaces the exact sample sizes with the per-day drop list (S6) and
// the single-OI-source declaration (§3) that every -r3+ report must
// carry verbatim.
//
// Sample assembly is NEVER re-implemented here: the shared daily panel
// comes from buildHCF01V2Days (same package), and each validator
// assembles its own pre-registered sample internally. The drop audit
// below is descriptive reporting only; the authoritative per-judgment
// n is the validator's own SampleCount/Notes.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// HCF01V2SingleSourceDeclaration is the verbatim single-source
// statement required in every -r3+ judgment report (arbitration §3:
// 「判決程式聲明 FinMind `data/state/taifex_oi/` 為唯一 OI 源」).
const HCF01V2SingleSourceDeclaration = "資料源聲明：FinMind `data/state/taifex_oi/` 為唯一 OI 源；macro snapshot `foreign_futures_oi_net` 通道 BLOCKED，不作訊號輸入（見 internal/capitalflow/oi_alignment.go）。"

// HCF01V2DropEntry is one panel day excluded from the shared v2a/v2a′
// evaluable sample, with the pre-registered drop reason (arbitration
// S6: 精確 n + 逐日 drop 原因清單, written verbatim into the -r3 report).
type HCF01V2DropEntry struct {
	Date   string `json:"date"`
	Reason string `json:"reason"`
}

// HCF01V2FamilyResult carries the three same-batch v2 judgments plus
// the Holm bookkeeping, the abandonment-line outcome, the exact
// per-judgment sample sizes with the per-day drop list, and the
// single-source declaration. Judgment STATUSES always come from the
// validators themselves; the Holm fields are correction bookkeeping
// and report disclosure, never a second gate.
type HCF01V2FamilyResult struct {
	V2A      HypothesisResult
	V2APrime HypothesisResult
	V2B      HypothesisResult

	// Holm bookkeeping keyed by judgment ID. Judgments that came back
	// INSUFFICIENT_DATA are excluded from the ranking (they do not
	// consume family α); the family size stays the pre-registered
	// HCF01V2HolmFamilySize=3, so the ranked alphas remain 0.05/3,
	// 0.05/2, 0.05/1 — strictly conservative.
	HolmPValues map[string]float64 // primary pre-registered p per judgment
	HolmRanks   map[string]int     // rank 0 = smallest p
	HolmAlphas  map[string]float64 // HolmAlphaForRank(rank, family size)
	HolmPassed  map[string]bool    // Holm step-down decision on the primary p

	AbandonmentTriggered bool
	AbandonmentNote      string

	ExactN   map[string]int     // judgment ID → exact evaluable n (SampleCount)
	DropList []HCF01V2DropEntry // per-day drop reasons over the shared panel

	SingleSourceDeclaration string
}

// v2 family judgment IDs, in the fixed tie-break order used for
// deterministic Holm ranking.
var hcf01V2FamilyIDs = []string{"H-CF-01-v2a", "H-CF-01-v2a-prime", "H-CF-01-v2b"}

// RunHCF01V2Family runs the three pre-registered v2 judgments in one
// batch with the same-batch Holm correction (arbitration §2.6):
//
//  1. Rank pass: every validator runs once at the family alpha
//     (HCF01V2Alpha) purely to extract its primary pre-registered
//     p-value — v2a: min(stat_layer_p, increment_p) (the §2.6 「依各判定
//     最小 p 值排序」 reading; the two layers are a conjunctive condition
//     and do NOT further split α), v2a′: diff_p, v2b: binomial_p (its
//     pre-registered v1 gate p; bootstrap_p_block10 is disclosure).
//     Statuses from this pass are discarded and never reported.
//  2. Holm assignment: p-values are ranked ascending (ties broken by
//     the fixed family order above); rank r receives
//     HolmAlphaForRank(r, HCF01V2HolmFamilySize).
//  3. Final pass: each ranked validator reruns at its assigned Holm
//     alpha; those final statuses are the family verdicts.
//     INSUFFICIENT_DATA judgments keep their rank-pass result (they are
//     alpha-independent) and stay out of the ranking.
//  4. The abandonment line (§2.5) is evaluated from the final statuses.
//
// The bootstrap procedures are deterministic (fixed seeds), so the two
// passes reproduce identical p-values; only the gate alpha changes.
func RunHCF01V2Family(in HCF01V2Inputs) HCF01V2FamilyResult {
	// Descriptive drop audit over the SAME shared panel the validators
	// assemble (buildHCF01V2Days) — reporting only (S6).
	dropList, dropCounts, panelN, evaluated := hcf01V2PanelDropAudit(buildHCF01V2Days(in))

	// Pass 1 — rank (statuses discarded; see HCF01V2Inputs.HolmAlpha doc).
	first := in
	first.HolmAlpha = HCF01V2Alpha
	pass1 := map[string]HypothesisResult{
		"H-CF-01-v2a":       ValidateHCF01V2A(first),
		"H-CF-01-v2a-prime": ValidateHCF01V2APrime(first),
		"H-CF-01-v2b":       ValidateHCF01V2B(first),
	}
	pValues := make(map[string]float64, len(hcf01V2FamilyIDs))
	type rankedItem struct {
		id string
		p  float64
	}
	var ranked []rankedItem
	for _, id := range hcf01V2FamilyIDs {
		if p, ok := hcf01V2PrimaryP(id, pass1[id]); ok {
			pValues[id] = p
			ranked = append(ranked, rankedItem{id: id, p: p})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].p < ranked[j].p })

	ranks := make(map[string]int, len(ranked))
	alphas := make(map[string]float64, len(ranked))
	for r, it := range ranked {
		ranks[it.id] = r
		alphas[it.id] = HolmAlphaForRank(r, HCF01V2HolmFamilySize)
	}

	// Pass 2 — final statuses at the assigned Holm alphas.
	final := make(map[string]HypothesisResult, len(hcf01V2FamilyIDs))
	for _, it := range ranked {
		inFinal := in
		inFinal.HolmAlpha = alphas[it.id]
		switch it.id {
		case "H-CF-01-v2a":
			final[it.id] = ValidateHCF01V2A(inFinal)
		case "H-CF-01-v2a-prime":
			final[it.id] = ValidateHCF01V2APrime(inFinal)
		case "H-CF-01-v2b":
			final[it.id] = ValidateHCF01V2B(inFinal)
		}
	}
	for _, id := range hcf01V2FamilyIDs {
		if _, ok := final[id]; !ok {
			final[id] = pass1[id] // INSUFFICIENT_DATA: alpha-independent
		}
	}

	// Holm step-down decisions on the primary p-values (disclosure;
	// the judgment status itself is always the validator's own verdict
	// at its assigned alpha).
	passed := make(map[string]bool, len(ranked))
	if len(ranked) > 0 {
		ps := make([]float64, len(ranked))
		for i, it := range ranked {
			ps[i] = it.p
		}
		for i, ok := range HolmPass(ps, HCF01V2HolmFamilySize) {
			passed[ranked[i].id] = ok
		}
	}

	// Abandonment line (§2.5) from the final statuses.
	triggered, note := EvaluateHCF01AbandonmentLine(
		final["H-CF-01-v2a"].Status,
		final["H-CF-01-v2a-prime"].Status,
		final["H-CF-01-v2b"].Status)

	// Exact n (S6) + the drop summary/verbatim per-day list note.
	exactN := make(map[string]int, len(hcf01V2FamilyIDs))
	for _, id := range hcf01V2FamilyIDs {
		exactN[id] = final[id].SampleCount
	}
	dropNote := hcf01V2DropSummaryNote(panelN, evaluated, dropCounts, dropList)

	// Report annotations: Holm bookkeeping, single-source declaration,
	// abandonment-line outcome, and the S6 exact-n/drop audit are
	// appended to EVERY family result so the -r3 JSON/Markdown report
	// carries them verbatim.
	for _, id := range hcf01V2FamilyIDs {
		res := final[id]
		if r, ok := ranks[id]; ok {
			res.Notes = append(res.Notes, fmt.Sprintf(
				"Holm 同批校正（§2.6）：rank=%d p=%.4g α=%.4g（家族 α=%.2f, m=%d）；Holm step-down p 判定=%t（判定狀態以 validator 為準）",
				r, pValues[id], alphas[id], HCF01V2Alpha, HCF01V2HolmFamilySize, passed[id]))
		} else {
			res.Notes = append(res.Notes,
				"INSUFFICIENT_DATA：不參與 Holm 排序、不消耗家族 α（家族 m 不變，其餘判定 α 從嚴）")
		}
		res.Notes = append(res.Notes, HCF01V2SingleSourceDeclaration)
		res.Notes = append(res.Notes, fmt.Sprintf("放棄線（§2.5）：triggered=%t — %s", triggered, note))
		res.Notes = append(res.Notes, dropNote)
		final[id] = res
	}

	return HCF01V2FamilyResult{
		V2A:      final["H-CF-01-v2a"],
		V2APrime: final["H-CF-01-v2a-prime"],
		V2B:      final["H-CF-01-v2b"],

		HolmPValues: pValues,
		HolmRanks:   ranks,
		HolmAlphas:  alphas,
		HolmPassed:  passed,

		AbandonmentTriggered: triggered,
		AbandonmentNote:      note,

		ExactN:   exactN,
		DropList: dropList,

		SingleSourceDeclaration: HCF01V2SingleSourceDeclaration,
	}
}

// Taiex exposes the TAIEX-close slice of a loaded macro snapshot row
// (macroRow's fields are unexported; the manual-only v2 CLI mode needs
// this accessor to hand the raw map to RunHCF01V2Family without
// duplicating any loader logic).
func (r macroRow) Taiex() (value float64, ok bool) {
	return r.taiex, r.hasTaiex
}

// hcf01V2PrimaryP extracts the family-ranking p-value of a judgment
// (arbitration §2.6 「依各判定最小 p 值排序」). Judgments without an
// evaluable sample (INSUFFICIENT_DATA) are not ranked.
func hcf01V2PrimaryP(id string, res HypothesisResult) (float64, bool) {
	if res.Status == ValidationInsufficientData {
		return 0, false
	}
	switch id {
	case "H-CF-01-v2a":
		// Both layers are conjunctive (§2.6); the family ranks the
		// judgment by its smallest p-value.
		return math.Min(res.Metrics["stat_layer_p"], res.Metrics["increment_p"]), true
	case "H-CF-01-v2a-prime":
		return res.Metrics["diff_p"], true
	case "H-CF-01-v2b":
		return res.Metrics["binomial_p"], true
	}
	return 0, false
}

// hcf01V2PanelDropAudit classifies each shared-panel day with the SAME
// pre-registered drop reasons the v2a/v2a′ validators apply (rollover
// first, then abstain/missing ret/ΔOI, then missing/tied spot). It is
// descriptive: the authoritative n is each validator's own SampleCount.
// Returns the per-day drop list, per-reason counts, panel size, and the
// number of fully evaluable days.
func hcf01V2PanelDropAudit(days []hcf01V2Day) (dropList []HCF01V2DropEntry, counts map[string]int, panelN, evaluated int) {
	counts = map[string]int{}
	panelN = len(days)
	for _, d := range days {
		reason := ""
		switch {
		case d.rollover:
			reason = "rollover_window"
		case !d.doiOK || !d.retOK || d.ret == 0:
			reason = "abstain_ret_zero_or_missing"
		case !d.spotOK || !d.nextOK || d.spot == 0 || d.spotNext == 0:
			reason = "spot_missing_or_tie"
		}
		if reason == "" {
			evaluated++
			continue
		}
		counts[reason]++
		dropList = append(dropList, HCF01V2DropEntry{Date: d.date, Reason: reason})
	}
	return dropList, counts, panelN, evaluated
}

// hcf01V2DropSummaryNote renders the S6 disclosure: exact panel counts
// plus the verbatim per-day drop list (date=reason; …).
func hcf01V2DropSummaryNote(panelN, evaluated int, counts map[string]int, dropList []HCF01V2DropEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "精確 n 面板稽核（S6）：面板 %d 日、可評估 %d 日；drop：rollover %d、abstain/缺值 %d、spot 缺值/tie %d。",
		panelN, evaluated, counts["rollover_window"], counts["abstain_ret_zero_or_missing"], counts["spot_missing_or_tie"])
	if len(dropList) == 0 {
		b.WriteString(" 逐日 drop 清單：無。")
		return b.String()
	}
	parts := make([]string, len(dropList))
	for i, e := range dropList {
		parts[i] = e.Date + "=" + e.Reason
	}
	b.WriteString(" 逐日 drop 清單：")
	b.WriteString(strings.Join(parts, "; "))
	return b.String()
}
