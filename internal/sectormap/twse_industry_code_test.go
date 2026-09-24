package sectormap

import (
	"slices"
	"testing"
)

// The TWSE/TPEx industry-code vocabulary is upstream data, so the shape of the
// table (which codes exist, how many are mapable) is pinned here. A legend
// change upstream must show up as a red test, not as a silent fall-through to
// StatusUnknown while an ETF silently loses an industry.

// twseIndustryCodeLegend is the TWSE ISIN 產業別 legend as published at
// https://isin.twse.com.tw/isin/class_i.jsp?kind=1 (read 2026-09-24). The codes
// are exactly the set the two OpenAPI datasets emit.
var twseIndustryCodeLegend = []string{
	"01", "02", "03", "04", "05", "06", "08", "09", "10", "11", "12", "13",
	"14", "15", "16", "17", "18", "19", "20", "21", "22", "23", "24", "25",
	"26", "27", "28", "29", "30", "31", "32", "33", "35", "36", "37", "38",
}

// noCandidateCodes are the codes that legitimately offer no canonical L1
// candidate: 19 綜合 and 20 其他業 are residual buckets (a "pick one" list would
// be a guess), and 13 電子工業 is the superseded aggregate the 24..31 codes
// replaced.
var noCandidateCodes = []string{"13", "19", "20"}

func TestTWSESIndustryCodes_MatchThePublishedLegend(t *testing.T) {
	got := make([]string, 0, len(TWSESIndustryCodes()))
	seen := map[string]bool{}
	for _, c := range TWSESIndustryCodes() {
		got = append(got, c.Code)
		if seen[c.Code] {
			t.Errorf("duplicate industry code %q", c.Code)
		}
		seen[c.Code] = true
		if c.Name == "" {
			t.Errorf("code %q has no Chinese name; the legend lookup was skipped", c.Code)
		}
		if c.Target != "" && len(c.Candidates) > 0 {
			t.Errorf("code %q declares both a target and candidates", c.Code)
		}
		if c.Target == "" && len(c.Candidates) == 0 && !slices.Contains(noCandidateCodes, c.Code) {
			t.Errorf("code %q is unmapped without candidates and is not a documented residual bucket %v",
				c.Code, noCandidateCodes)
		}
		if c.Reason == "" {
			t.Errorf("code %q has no reason; every disposition must be explained", c.Code)
		}
	}
	slices.Sort(got)
	want := slices.Clone(twseIndustryCodeLegend)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("industry code vocabulary drifted:\n declared-only: %v\n legend-only:   %v",
			missing(got, want), missing(want, got))
	}
	if !slices.Equal(Keys(NamespaceTWSESIndustryCode), want) {
		t.Errorf("namespace keys are not the legend codes: %v", Keys(NamespaceTWSESIndustryCode))
	}
}

func TestTWSESIndustryCodes_CoverTheWholeCanonicalTaxonomy(t *testing.T) {
	covered := CoveredL1(NamespaceTWSESIndustryCode, false)
	if !slices.Equal(covered, CanonicalL1IDs()) {
		t.Errorf("industry codes reach %d/%d canonical L1 sectors: %v",
			len(covered), len(CanonicalL1IDs()), covered)
	}
}

func TestTWSESIndustryCodes_ResidualBucketsAreDeclaredUnmapped(t *testing.T) {
	// 19 綜合 and 20 其他業 are residual buckets by construction; mapping them to
	// any sector would be the guess #1943 forbids.
	for _, code := range []string{"19", "20", "13"} {
		if _, ok := TWSESIndustryCodeL1(code); ok {
			t.Errorf("residual/legacy code %q must not resolve to an L1 sector", code)
		}
		if m := Resolve(NamespaceTWSESIndustryCode, code); m.Status != StatusUnmapped {
			t.Errorf("code %q status = %q, want %q", code, m.Status, StatusUnmapped)
		}
	}
	if _, ok := TWSESIndustryCodeL1("99"); ok {
		t.Error("undeclared code 99 must not resolve")
	}
	name, ok := TWSESIndustryCodeName("24")
	if !ok || name != "半導體業" {
		t.Errorf("TWSESIndustryCodeName(24) = (%q, %v), want (半導體業, true)", name, ok)
	}
}

// TestDrift_TWSECodeAndIndexNameAgree is the same guarantee #1943 gave the two
// TWSE index-name maps: the industry-code vocabulary and the index-name
// vocabulary describe the same listed companies, so they must not disagree about
// which canonical L1 a TWSE industry is.
func TestDrift_TWSECodeAndIndexNameAgree(t *testing.T) {
	pairs := []struct{ indexName, code string }{
		{"半導體類指數", "24"},
		{"電腦及週邊設備類指數", "25"},
		{"電子零組件類指數", "28"},
		{"其他電子類指數", "31"},
		{"光電類指數", "26"},
		{"通信網路類指數", "27"},
		{"航運類指數", "15"},
		{"金融保險類指數", "17"},
		{"油電燃氣類指數", "23"},
		{"電機機械類指數", "05"},
		{"電器電纜類指數", "06"},
		{"水泥類指數", "01"},
		{"食品類指數", "02"},
		{"塑膠類指數", "03"},
		{"紡織纖維類指數", "04"},
		{"鋼鐵類指數", "10"},
		{"汽車類指數", "12"},
		{"化學類指數", "21"},
		{"生技醫療類指數", "22"},
		{"建材營造類指數", "14"},
		{"觀光餐旅類指數", "16"},
		{"貿易百貨類指數", "18"},
	}
	if len(pairs) != 22 {
		t.Fatalf("expected the 22 mapped TWSE industries, table lists %d", len(pairs))
	}
	for _, p := range pairs {
		fromIndex, ok := ResolveL1(NamespaceTWSESectorIndex, p.indexName)
		if !ok {
			t.Errorf("index name %q does not resolve", p.indexName)
			continue
		}
		fromCode, ok := TWSESIndustryCodeL1(p.code)
		if !ok {
			t.Errorf("industry code %q does not resolve", p.code)
			continue
		}
		if fromIndex != fromCode {
			t.Errorf("TWSE industry %s: index name %q says %s but code %q says %s — two TWSE maps disagree",
				p.indexName, p.indexName, fromIndex, p.code, fromCode)
		}
	}
}
