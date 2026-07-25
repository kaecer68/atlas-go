// internal/industry/sector_test.go
//
// Contract enforcement for the canonical sector resource (FU-7 Phase A):
//   - Every SectorID constant has a DisplayZHTw entry (no orphan IDs)
//   - Constants are unique
//   - DisplayZHAliases round-trips legacy Chinese aliases (including
//     the well-known demo-data.js truncated "金融" → "financials")
//   - SectorIDFromString handles canonical, full Chinese, and legacy
//   - AllSectors returns a deterministic sorted slice with the canonical
//     count of 20

package industry

import (
	"encoding/json"
	"sort"
	"testing"
)

func Test_SectorID_AllConstantsUnique(t *testing.T) {
	ids := AllSectors()
	seen := make(map[SectorID]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			t.Errorf("duplicate SectorID constant detected: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func Test_SectorID_DisplayZHNeverEmpty(t *testing.T) {
	for _, id := range AllSectors() {
		label := DisplayZH(id)
		if label == "" {
			t.Errorf("DisplayZH(%q) returned empty string for known canonical id", id)
		}
	}
}

func Test_SectorID_DisplayZHTwOnlyContainsCanonical(t *testing.T) {
	for id := range DisplayZHTw {
		if id == "" {
			t.Errorf("DisplayZHTw contains empty SectorID key")
			continue
		}
		if !id.IsValid() {
			t.Errorf("DisplayZHTw contains non-canonical SectorID key %q", id)
		}
	}
}

func Test_SectorID_IsValid(t *testing.T) {
	for _, id := range AllSectors() {
		if !id.IsValid() {
			t.Errorf("AllSectors entry %q should be valid", id)
		}
	}
	if SectorID("not_a_real_sector").IsValid() {
		t.Error("fabricated SectorID should not be valid")
	}
	if SectorID("").IsValid() {
		t.Error("empty SectorID should not be valid")
	}
}

func Test_SectorID_String(t *testing.T) {
	for _, id := range AllSectors() {
		if id.String() != string(id) {
			t.Errorf("SectorID(%q).String() = %q", id, id.String())
		}
	}
}

func Test_SectorIDFromString_CanonicalIDs(t *testing.T) {
	for _, id := range AllSectors() {
		got, ok := SectorIDFromString(string(id))
		if !ok {
			t.Errorf("SectorIDFromString(%q) returned !ok for canonical id", id)
		}
		if got != id {
			t.Errorf("SectorIDFromString(%q) = %q, want %q", id, got, id)
		}
	}
}

func Test_SectorIDFromString_DisplayZHTwFullLabels(t *testing.T) {
	for id, label := range DisplayZHTw {
		got, ok := SectorIDFromString(label)
		if !ok {
			t.Errorf("SectorIDFromString(%q) returned !ok for full display label", label)
		}
		if got != id {
			t.Errorf("SectorIDFromString(%q) = %q, want %q", label, got, id)
		}
	}
}

func Test_SectorIDFromString_LegacyAliases(t *testing.T) {
	cases := map[string]SectorID{
		"半導體":  SectorSemiconductor,
		"電子":   SectorElectronics,
		"金融":   SectorFinancials,
		"金融保險": SectorFinancials,
		"金融類":  SectorFinancials,
		"半導體類": SectorSemiconductor,
		"航運":   SectorShipping,
		"塑化":   SectorPlastics,
		"通信":   SectorTelecom,
		"生技":   SectorBiotech,
		"機械":   SectorMachinery,
		"觀光":   SectorTourism,
		"能源":   SectorEnergy,
	}
	for alias, want := range cases {
		got, ok := SectorIDFromString(alias)
		if !ok {
			t.Errorf("SectorIDFromString(%q) returned !ok (known legacy alias)", alias)
		}
		if got != want {
			t.Errorf("SectorIDFromString(%q) = %q, want %q", alias, got, want)
		}
	}
}

func Test_SectorIDFromString_EmptyAndUnknown(t *testing.T) {
	cases := []string{"", "  ", "不存在", "ZZZ", "unknown_sector"}
	for _, s := range cases {
		got, ok := SectorIDFromString(s)
		if ok {
			t.Errorf("SectorIDFromString(%q) returned ok=true with %q (expected miss)", s, got)
		}
		if got != "" {
			t.Errorf("SectorIDFromString(%q) returned non-empty id %q on miss", s, got)
		}
	}
}

func Test_AllSectors_Sorted(t *testing.T) {
	got := AllSectors()
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Errorf("AllSectors not sorted: %v", got)
	}
}

func Test_AllSectors_CanonicalCount(t *testing.T) {
	const want = 38
	if got := len(AllSectors()); got != want {
		t.Errorf("AllSectors length = %d, want %d (20 L1 + 18 L2)", got, want)
	}
}

func Test_AllSectors_L1L2Breakdown(t *testing.T) {
	l1, l2 := 0, 0
	for _, id := range AllSectors() {
		switch Layer(id) {
		case "L1":
			l1++
		case "L2":
			l2++
		}
	}
	if l1 != 20 {
		t.Errorf("L1 sector count = %d, want 20", l1)
	}
	if l2 != 18 {
		t.Errorf("L2 sub-industry count = %d, want 18", l2)
	}
}

func Test_DisplayZH_ReturnsEmptyForInvalid(t *testing.T) {
	if got := DisplayZH(SectorID("nonexistent")); got != "" {
		t.Errorf("DisplayZH(nonexistent) = %q, want empty string", got)
	}
	if got := DisplayZH(""); got != "" {
		t.Errorf("DisplayZH(empty) = %q, want empty string", got)
	}
}

func Test_DisplayZH_MatchesLabeltWap(t *testing.T) {
	for id := range DisplayZHTw {
		if DisplayZH(id) != DisplayZHTw[id] {
			t.Errorf("DisplayZH(%q) inconsistent with DisplayZHTw table", id)
		}
	}
}

func Test_SectorID_JSONMarshal(t *testing.T) {
	for _, id := range AllSectors() {
		b, err := json.Marshal(string(id))
		if err != nil {
			t.Errorf("json.Marshal(%q) error: %v", id, err)
			continue
		}
		if string(b) != `"`+string(id)+`"` {
			t.Errorf("json.Marshal(%q) = %s, want %q", id, b, `"`+string(id)+`"`)
		}

		var got string
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("json.Unmarshal(%s) error: %v", b, err)
			continue
		}
		if got != string(id) {
			t.Errorf("json.Unmarshal(%s) = %q, want %q", b, got, string(id))
		}
	}
}

// Lock zero-value behavior: uninitialized `var s SectorID` must not be
// mistakenly treated as a real sector.
func Test_ZeroValue_NotRegistered(t *testing.T) {
	var zero SectorID
	if zero.IsValid() {
		t.Error("zero SectorID should not be valid")
	}
	if DisplayZH(zero) != "" {
		t.Error("DisplayZH(zero) should return empty string")
	}
	if got, ok := SectorIDFromString(""); ok || got != "" {
		t.Errorf("SectorIDFromString(\"\") should return miss; got id=%q ok=%v", got, ok)
	}
}

func Test_Layer_L1(t *testing.T) {
	for _, id := range []SectorID{SectorSemiconductor, SectorFinancials, SectorShipping, SectorEnergy} {
		if got := id.Layer(); got != "L1" {
			t.Errorf("SectorID(%q).Layer() = %q, want L1", id, got)
		}
	}
}

func Test_Layer_L2(t *testing.T) {
	for _, id := range []SectorID{SubIndustryAISupplyChain, SubIndustryFoundry, SubIndustryRobotics, SubIndustryETFRotation} {
		if got := id.Layer(); got != "L2" {
			t.Errorf("SubIndustry(%q).Layer() = %q, want L2", id, got)
		}
	}
}

func Test_Layer_Unknown_ReturnsUnknown(t *testing.T) {
	if got := SectorID("nonexistent").Layer(); got != "unknown" {
		t.Errorf("unknown SectorID.Layer() = %q, want unknown", got)
	}
}

func Test_IsL1_L1ReturnsTrue(t *testing.T) {
	if !SectorSemiconductor.IsL1() {
		t.Error("SectorSemiconductor.IsL1() should be true")
	}
	if !SectorFinancials.IsL1() {
		t.Error("SectorFinancials.IsL1() should be true")
	}
	if !SectorShipping.IsL1() {
		t.Error("SectorShipping.IsL1() should be true")
	}
}

func Test_IsL1_L2ReturnsFalse(t *testing.T) {
	if SubIndustryAISupplyChain.IsL1() {
		t.Error("SubIndustryAISupplyChain.IsL1() should be false")
	}
}

func Test_IsL2_L2ReturnsTrue(t *testing.T) {
	if !SubIndustryAISupplyChain.IsL2() {
		t.Error("SubIndustryAISupplyChain.IsL2() should be true")
	}
}

func Test_IsL2_L1ReturnsFalse(t *testing.T) {
	if SectorSemiconductor.IsL2() {
		t.Error("SectorSemiconductor.IsL2() should be false")
	}
}

func Test_DisplayZH_L2HasChineseLabel(t *testing.T) {
	if got := DisplayZH(SubIndustryAISupplyChain); got != "AI供應鏈" {
		t.Errorf("DisplayZH(ai_supply_chain) = %q, want AI供應鏈", got)
	}
	if got := DisplayZH(SubIndustryFoundry); got != "晶圓代工" {
		t.Errorf("DisplayZH(foundry) = %q, want 晶圓代工", got)
	}
}

// TWSE mapping validation — guards against drift between mapIndustryName string literals
// and canonical SectorID constants. If a SectorID is renamed, this test catches the gap.
func Test_TWSEMapping_AllValuesAreValidSectorIDs(t *testing.T) {
	vals := []string{
		"semiconductor",
		"ai_supply_chain",
		"electronics",
		"other_electronics",
		"shipping",
		"financials",
		"energy",
		"robotics",
	}
	for _, v := range vals {
		if !SectorID(v).IsValid() {
			t.Errorf("TWSE mapping value %q is not a valid SectorID (rename or register it)", v)
		}
	}
}
