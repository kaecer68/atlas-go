package marketexplain

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// makeSnapshot returns a MacroDataSnapshot with the given per-field
// values, leaving everything else zero. Tests use this to drive the
// pure-function section builders without standing up a real provider.
func makeSnapshot(modify func(s *marketdata.MacroDataSnapshot)) marketdata.MacroDataSnapshot {
	s := marketdata.MacroDataSnapshot{}
	if modify != nil {
		modify(&s)
	}
	return s
}

func makeForce(name capitalflow.ForceName, role, trend string, z, raw float64) capitalflow.ForceScore {
	return capitalflow.ForceScore{
		Force:    name,
		Role:     role,
		Trend:    trend,
		ZScore:   z,
		RawValue: raw,
	}
}

func TestSectionBodies_returnsAllBodies(t *testing.T) {
	in := []Section{
		{Title: "a", Body: "1"},
		{Title: "b", Body: "2"},
	}
	got := sectionBodies(in)
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("sectionBodies = %v, want [1 2]", got)
	}
}

func TestDirectionText(t *testing.T) {
	cases := []struct {
		z    float64
		want string
	}{
		{1.0, "買超"},
		{-1.0, "賣超"},
		{0.1, "買賣超"},
	}
	for _, c := range cases {
		if got := directionText(c.z); got != c.want {
			t.Errorf("directionText(%v) = %q, want %q", c.z, got, c.want)
		}
	}
}

func TestForceLabel_allNamedForces(t *testing.T) {
	// Every force the seven-dimension spec recognises as subject
	// must have a Traditional-Chinese label; otherwise retail users
	// see raw enum strings on the home page.
	cases := map[capitalflow.ForceName]string{
		capitalflow.ForceForeign:       "外資",
		capitalflow.ForceInstitutional: "投信",
		capitalflow.ForceDealer:        "自營商",
		capitalflow.ForceGovernment:    "公股行庫",
		capitalflow.ForceRetail:        "散戶",
	}
	for force, want := range cases {
		if got := forceLabel(force); got != want {
			t.Errorf("forceLabel(%v) = %q, want %q", force, got, want)
		}
	}
	// Unknown force must fall back to its raw string, never crash.
	if got := forceLabel("nonsense"); got != "nonsense" {
		t.Errorf("forceLabel unknown = %q, want raw string", got)
	}
}

func TestBuildTAIEXSection_quietDay(t *testing.T) {
	// 0% change → 持平 + 觀望氣氛 (no semis context).
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{Value: 17000, ChangePct: 0}
	})
	got := buildTAIEXSection(s)
	if got.Title != "大盤表現" {
		t.Errorf("title = %q, want 大盤表現", got.Title)
	}
	if !strings.Contains(got.Body, "幾乎持平") {
		t.Errorf("quiet day body missing 持平 marker: %q", got.Body)
	}
	if !strings.Contains(got.Body, "📈") && !strings.Contains(got.Body, "📉") {
		t.Errorf("expected direction emoji, got: %q", got.Body)
	}
}

func TestBuildTAIEXSection_bigMove(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{Value: 17000, ChangePct: 2.5}
	})
	got := buildTAIEXSection(s)
	if !strings.Contains(got.Body, "漲跌幅較大") {
		t.Errorf("big-move body should flag 漲跌幅較大, got: %q", got.Body)
	}
	if !strings.Contains(got.Body, "上漲") {
		t.Errorf("up day should say 上漲, got: %q", got.Body)
	}
}

func TestBuildTAIEXSection_downDay(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{Value: 16500, ChangePct: -1.2}
	})
	got := buildTAIEXSection(s)
	if !strings.Contains(got.Body, "下跌") {
		t.Errorf("down day should say 下跌, got: %q", got.Body)
	}
	if !strings.Contains(got.Body, "📉") {
		t.Errorf("down day should show bear emoji, got: %q", got.Body)
	}
}

func TestBuildTAIEXSection_semiAlignment(t *testing.T) {
	// TAIEX up + semis up → "方向一致".
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{Value: 17000, ChangePct: 0.8}
		s.TaiwanSemiIndex = marketdata.MacroDataPoint{Value: 500, ChangePct: 1.2}
	})
	got := buildTAIEXSection(s)
	if !strings.Contains(got.Body, "方向一致") {
		t.Errorf("aligned semis should say 方向一致, got: %q", got.Body)
	}

	// TAIEX up + semis down → divergence.
	s = makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{Value: 17000, ChangePct: 0.8}
		s.TaiwanSemiIndex = marketdata.MacroDataPoint{Value: 500, ChangePct: -0.5}
	})
	got = buildTAIEXSection(s)
	if !strings.Contains(got.Body, "分歧") {
		t.Errorf("divergent semis should say 分歧, got: %q", got.Body)
	}
}

func TestBuildCapitalSection_emptySummary(t *testing.T) {
	// Empty Summary → zero-value Section (compose() will skip it).
	got := buildCapitalSection(capitalflow.SummaryReport{})
	if got.Body != "" || got.Title != "" {
		t.Errorf("empty summary should produce zero Section, got %+v", got)
	}
}

func TestBuildCapitalSection_subjectForces(t *testing.T) {
	cf := capitalflow.SummaryReport{
		Summary: "三大法人合計買超 120 億。",
		Forces: []capitalflow.ForceScore{
			// Subject force with z=+2.0 → 顯著偏多, raw=80 (億).
			makeForce(capitalflow.ForceForeign, "subject", "bullish", 2.0, 80),
			// Subject force with z=0.2 → no suffix.
			makeForce(capitalflow.ForceInstitutional, "subject", "bullish", 0.2, 15),
			// Subject force with z=-1.8 → 顯著偏空.
			makeForce(capitalflow.ForceDealer, "subject", "bearish", -1.8, -25),
			// Non-subject force must be ignored.
			makeForce(capitalflow.ForceRetail, "leading_indicator", "bullish", 1.0, 5),
		},
	}
	got := buildCapitalSection(cf)
	if got.Title != "資金面" {
		t.Errorf("title = %q, want 資金面", got.Title)
	}
	for _, want := range []string{"外資", "投信", "自營商", "顯著偏多", "顯著偏空"} {
		if !strings.Contains(got.Body, want) {
			t.Errorf("body missing %q: %q", want, got.Body)
		}
	}
	// Retail is a leading_indicator, must NOT appear.
	if strings.Contains(got.Body, "散戶") {
		t.Errorf("non-subject force leaked into subject list: %q", got.Body)
	}
}

func TestBuildGlobalSection_omitsEmpty(t *testing.T) {
	// All zero ChangePct → buildGlobalSection returns empty Section
	// (compose() then drops it from the response).
	got := buildGlobalSection(marketdata.MacroDataSnapshot{})
	if got.Body != "" {
		t.Errorf("empty global should drop section, got %+v", got)
	}
}

func TestBuildGlobalSection_panicVIX(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.VIX = marketdata.MacroDataPoint{Value: 30, ChangePct: 5.0}
	})
	got := buildGlobalSection(s)
	if !strings.Contains(got.Body, "VIX 恐慌指數30.00（上升）") {
		t.Errorf("VIX panic phrase missing: %q", got.Body)
	}
	if !strings.Contains(got.Body, "市場恐慌情緒偏高") {
		t.Errorf("VIX>25 phrase missing: %q", got.Body)
	}
}

func TestBuildGlobalSection_allComponents(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.VIX = marketdata.MacroDataPoint{Value: 18, ChangePct: -1.0}
		s.USD_TWD = marketdata.MacroDataPoint{Value: 31.5, ChangePct: 0.3}
		s.US10Y = marketdata.MacroDataPoint{Value: 4.25, ChangePct: 0.1}
		s.DXY = marketdata.MacroDataPoint{Value: 104.5, ChangePct: 0.2}
	})
	got := buildGlobalSection(s)
	for _, want := range []string{"VIX", "新台幣", "美債 10 年期殖利率", "美元指數"} {
		if !strings.Contains(got.Body, want) {
			t.Errorf("global section missing %q: %q", want, got.Body)
		}
	}
}

func TestBuildRiskSection_emptyWhenCalm(t *testing.T) {
	// No VIX spike, no divergence, no TAIEX crash → no warnings.
	got := buildRiskSection(marketdata.MacroDataSnapshot{}, capitalflow.SummaryReport{})
	if got.Body != "" {
		t.Errorf("calm day should produce no risk section, got %q", got.Body)
	}
}

func TestBuildRiskSection_highVIX(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.VIX = marketdata.MacroDataPoint{Value: 30}
	})
	got := buildRiskSection(s, capitalflow.SummaryReport{})
	if !strings.Contains(got.Body, "VIX 高於 25") {
		t.Errorf("high VIX warning missing: %q", got.Body)
	}
}

func TestBuildRiskSection_capFlowDivergence(t *testing.T) {
	cf := capitalflow.SummaryReport{
		Forces: []capitalflow.ForceScore{
			makeForce(capitalflow.ForceForeign, "subject", "bullish", 1.0, 50),
			makeForce(capitalflow.ForceInstitutional, "subject", "bearish", -1.0, -20),
		},
	}
	got := buildRiskSection(marketdata.MacroDataSnapshot{}, cf)
	if !strings.Contains(got.Body, "資金勢力分歧") {
		t.Errorf("divergence warning missing: %q", got.Body)
	}
}

func TestBuildRiskSection_taiexCrash(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{ChangePct: -3.5}
	})
	got := buildRiskSection(s, capitalflow.SummaryReport{})
	if !strings.Contains(got.Body, "波動超過 3%") {
		t.Errorf("crash warning missing: %q", got.Body)
	}
}

func TestCountDivergent(t *testing.T) {
	cases := []struct {
		name   string
		forces []capitalflow.ForceScore
		want   int
	}{
		{
			name:   "no forces",
			forces: nil,
			want:   0,
		},
		{
			name: "all aligned bullish",
			forces: []capitalflow.ForceScore{
				makeForce(capitalflow.ForceForeign, "subject", "bullish", 1, 0),
				makeForce(capitalflow.ForceInstitutional, "subject", "bullish", 1, 0),
				makeForce(capitalflow.ForceDealer, "subject", "bullish", 1, 0),
			},
			want: 0,
		},
		{
			name: "two bullish + one bearish = 3 divergent",
			forces: []capitalflow.ForceScore{
				makeForce(capitalflow.ForceForeign, "subject", "bullish", 1, 0),
				makeForce(capitalflow.ForceInstitutional, "subject", "bullish", 1, 0),
				makeForce(capitalflow.ForceDealer, "subject", "bearish", -1, 0),
			},
			want: 3,
		},
		{
			name: "non-subject forces are ignored",
			forces: []capitalflow.ForceScore{
				makeForce(capitalflow.ForceRetail, "leading_indicator", "bullish", 1, 0),
				makeForce(capitalflow.ForceForeign, "subject", "bullish", 1, 0),
			},
			want: 0,
		},
		{
			name: "non-official subject forces are ignored",
			forces: []capitalflow.ForceScore{
				makeForce(capitalflow.ForceGovernment, "subject", "bullish", 1, 0),
				makeForce(capitalflow.ForceRetail, "subject", "bearish", -1, 0),
			},
			want: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := countDivergent(c.forces); got != c.want {
				t.Errorf("countDivergent = %d, want %d", got, c.want)
			}
		})
	}
}

func TestBuildHeadline_basic(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{ChangePct: 0.8}
	})
	cf := capitalflow.SummaryReport{Summary: "三大法人合計買超 50 億。"}
	got := buildHeadline(s, cf)
	if !strings.Contains(got, "上漲 0.80%") {
		t.Errorf("headline missing TAIEX direction: %q", got)
	}
	if !strings.Contains(got, "三大法人合計買超 50 億") {
		t.Errorf("headline missing capital sentence: %q", got)
	}
}

func TestBuildHeadline_downWithVIX(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{ChangePct: -1.5}
		s.VIX = marketdata.MacroDataPoint{Value: 28}
	})
	got := buildHeadline(s, capitalflow.SummaryReport{})
	if !strings.Contains(got, "下跌") {
		t.Errorf("down headline missing 下跌: %q", got)
	}
	if !strings.Contains(got, "市場波動偏高") {
		t.Errorf("high VIX phrase missing: %q", got)
	}
}

func TestBuildHeadline_truncatesCapitalAtFirstSentence(t *testing.T) {
	// buildHeadline should only embed the FIRST sentence of cfSummary
	// to keep the headline one line; otherwise it overflows the UI.
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{ChangePct: 0.5}
	})
	cf := capitalflow.SummaryReport{
		Summary: "三大法人合計買超 50 億。投信加碼電子股。",
	}
	got := buildHeadline(s, cf)
	if strings.Contains(got, "投信加碼") {
		t.Errorf("headline should stop at first sentence, got: %q", got)
	}
}

// TestCompose_happyPath exercises the full compose() pipeline: every
// section builder fires, sections are concatenated, headline is
// produced, GeneratedAt is stamped. This is the integration test
// for the rule-based fallback (the LLM-degraded path).
func TestCompose_happyPath(t *testing.T) {
	s := makeSnapshot(func(s *marketdata.MacroDataSnapshot) {
		s.TAIEX = marketdata.MacroDataPoint{Value: 17000, ChangePct: 0.8}
		s.TaiwanSemiIndex = marketdata.MacroDataPoint{Value: 500, ChangePct: 1.0}
		s.VIX = marketdata.MacroDataPoint{Value: 16, ChangePct: -1.0}
		s.USD_TWD = marketdata.MacroDataPoint{Value: 31.5, ChangePct: 0.2}
		s.US10Y = marketdata.MacroDataPoint{Value: 4.0, ChangePct: 0.05}
		s.DXY = marketdata.MacroDataPoint{Value: 104, ChangePct: 0.1}
	})
	cf := capitalflow.SummaryReport{
		Date:    time.Now(),
		Summary: "三大法人合計買超 80 億。",
		Forces: []capitalflow.ForceScore{
			makeForce(capitalflow.ForceForeign, "subject", "bullish", 1.2, 60),
		},
	}
	got := compose(s, cf)

	if got.Source != "rule_based" {
		t.Errorf("source = %q, want rule_based", got.Source)
	}
	if got.GeneratedAt.IsZero() {
		t.Error("GeneratedAt must be stamped")
	}
	// TAIEX is always present; global + capital appear when their
	// data is non-zero. Risk section only fires on VIX spike /
	// divergence / crash — the happy path is the absence of risk.
	if len(got.Sections) < 3 {
		t.Errorf("expected >=3 sections (taiex/capital/global), got %d (%+v)", len(got.Sections), got.Sections)
	}
	// Detail must join all section bodies.
	if !strings.Contains(got.Detail, "今日加權指數上漲") {
		t.Errorf("Detail missing TAIEX body: %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "資金面") && !strings.Contains(got.Detail, "三大法人") {
		t.Errorf("Detail missing capital body: %q", got.Detail)
	}
	// Headline must reference today's move + capital summary.
	if !strings.Contains(got.Headline, "上漲") {
		t.Errorf("Headline missing TAIEX direction: %q", got.Headline)
	}
}

// TestCompose_emptyData still produces a usable explanation — the
// design principle says "always return a usable explanation, even
// when channels are degraded" (marketexplain/doc.go).
func TestCompose_emptyData(t *testing.T) {
	got := compose(marketdata.MacroDataSnapshot{}, capitalflow.SummaryReport{})
	if got.Source != "rule_based" {
		t.Errorf("source = %q, want rule_based", got.Source)
	}
	// TAIEX is always present (zero-value), so we get at least the
	// 大盤表現 section even with no data.
	if len(got.Sections) == 0 {
		t.Error("compose with no data must still produce >=1 section")
	}
	// Headline must be non-empty even with zero data.
	if got.Headline == "" {
		t.Error("Headline must not be empty even with zero data")
	}
	// Capital/global/risk should be DROPPED (not rendered with empty
	// bodies) when their underlying data is missing.
	for _, s := range got.Sections {
		if s.Title == "資金面" || s.Title == "國際環境" || s.Title == "風險提示" {
			t.Errorf("section %q should be dropped when no data, got %+v", s.Title, s)
		}
	}
}

func TestNewHandler_storesDependencies(t *testing.T) {
	// NewHandler must not validate or fetch; it just stores pointers.
	// We pass nil deliberately; calling HandleExplain with a nil
	// provider would panic, but that is exercised in server-side
	// integration tests, not here.
	h := NewHandler(nil, nil)
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if h.provider != nil || h.cf != nil {
		t.Error("NewHandler must store the same pointers it was given")
	}
}
