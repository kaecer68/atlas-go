package futures

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// testShadowParams 只啟用「內部自足」的兩項（價差結構、OI 變化），
// 外部輸入項權重為 0 ⇒ golden 期望值可手算，且不依賴外部資料。
func testShadowParams() ShadowParams {
	return ShadowParams{
		Features:           testFeatureParams(),
		WTermStructure:     1,
		WOIChange:          1,
		WForeignNet:        0,
		WPCR:               0,
		DirectionThreshold: 0,
	}
}

// TestBuildShadowSeries_Golden 是影子序列的 golden test（手算期望值）。
//
// 用 twoMonthSeries()：三日皆為 contango（遠月 − 近月 = +10/+9/+11）。
//
//	d1 2026-01-05：s1=−1（contango），無近月前值 ⇒ 不計 s2
//	               score = −1 ⇒ down；標籤 = 202601 下一日 (102−100)/100 = +2% ⇒ up ⇒ **hit=false**
//	d2 2026-01-06：s1=−1；s2 = sign(+100 口)·sign(+2%) = +1
//	               score = (−1+1)/2 = 0 ⇒ neutral；標籤 (101−102)/102 < 0 ⇒ down ⇒ **hit=nil**（neutral 略過）
//	d3 2026-01-07：s1=−1；s2 = sign(+50 口)·sign(−0.98%) = −1
//	               score = −1 ⇒ down；**無下一日 ⇒ 無標籤**
func TestBuildShadowSeries_Golden(t *testing.T) {
	samples, err := BuildShadowSeries(twoMonthSeries(), "TX", testShadowParams(), nil)
	if err != nil {
		t.Fatalf("BuildShadowSeries: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("want 3 samples, got %d", len(samples))
	}

	d1 := samples[0]
	if d1.TradeDate.Format("2006-01-02") != "2026-01-05" || d1.Predicted != DirectionDown {
		t.Fatalf("d1 = %s/%s, want 2026-01-05/down", d1.TradeDate.Format("2006-01-02"), d1.Predicted)
	}
	if math.Abs(d1.Score+1) > 1e-9 {
		t.Fatalf("d1 score = %v, want -1", d1.Score)
	}
	if d1.TermsUsed != 1 {
		t.Fatalf("d1 terms = %d, want 1 (OI term unusable without near-month previous close)", d1.TermsUsed)
	}
	if d1.LabelReturnPct == nil || math.Abs(*d1.LabelReturnPct-2) > 1e-9 {
		t.Fatalf("d1 label = %v, want +2%%", d1.LabelReturnPct)
	}
	if d1.ActualDirection != DirectionUp {
		t.Fatalf("d1 actual = %s, want up", d1.ActualDirection)
	}
	if d1.Hit == nil || *d1.Hit {
		t.Fatalf("d1 hit = %v, want false (predicted down, actual up)", d1.Hit)
	}

	d2 := samples[1]
	if d2.Predicted != DirectionNeutral {
		t.Fatalf("d2 predicted = %s, want neutral (score 0)", d2.Predicted)
	}
	if math.Abs(d2.Score) > 1e-9 {
		t.Fatalf("d2 score = %v, want 0", d2.Score)
	}
	if d2.TermsUsed != 2 {
		t.Fatalf("d2 terms = %d, want 2", d2.TermsUsed)
	}
	if d2.Hit != nil {
		t.Fatalf("d2 hit = %v, want nil (neutral predicted is skipped, mirroring the repo calibrator)", d2.Hit)
	}

	d3 := samples[2]
	if d3.Predicted != DirectionDown || math.Abs(d3.Score+1) > 1e-9 {
		t.Fatalf("d3 = %s/%v, want down/-1", d3.Predicted, d3.Score)
	}
	if d3.LabelReturnPct != nil || d3.Hit != nil {
		t.Fatalf("d3 must have no label (no next day): label=%v hit=%v", d3.LabelReturnPct, d3.Hit)
	}

	hr := SummarizeHitRate(samples)
	if hr.Total != 1 || hr.Hits != 0 || hr.Rate != 0 || hr.Skipped != 2 {
		t.Fatalf("hit rate = %+v, want {Total:1 Hits:0 Rate:0 Skipped:2}", hr)
	}
}

// TestBuildShadowSeries_RollDayHasNoLabel 換倉日（舊契約隔日無報價）必須**沒有標籤**。
//
// 這是刻意的：若用「換倉後的新契約」算報酬，換倉價差會被誤當成報酬。
func TestBuildShadowSeries_RollDayHasNoLabel(t *testing.T) {
	bars := []domain.FuturesBar{
		bar("202601", "2026-02-10", 100, iPtr(1000)),
		bar("202602", "2026-02-10", 110, iPtr(500)),
		// 202601 在 2026-02-10 之後不再有報價（到期）；202602 續存。
		bar("202602", "2026-02-11", 112, iPtr(520)),
		bar("202603", "2026-02-11", 120, iPtr(30)),
	}
	samples, err := BuildShadowSeries(bars, "TX", testShadowParams(), nil)
	if err != nil {
		t.Fatalf("BuildShadowSeries: %v", err)
	}
	var rollDay *ShadowSample
	for i := range samples {
		if samples[i].TradeDate.Format("2006-01-02") == "2026-02-10" {
			rollDay = &samples[i]
		}
	}
	if rollDay == nil {
		t.Fatal("roll-day sample missing")
	}
	if rollDay.NearMonth != "202601" {
		t.Fatalf("roll day near month = %s, want 202601", rollDay.NearMonth)
	}
	if rollDay.LabelReturnPct != nil || rollDay.Hit != nil {
		t.Fatalf("roll day must have no label (would misread the roll gap as a return): label=%v", rollDay.LabelReturnPct)
	}
}

// TestNextDaySameMonthReturn_DoesNotCrossContracts 直接在函式層釘住「標籤不得跨契約」。
//
// 這一條刻意做成**函式級**測試：序列級的 roll-day 測試只能證明「最終標籤為 nil」，
// 而該性質在實作裡由**兩道獨立的 month 過濾**共同保證
// （nextDaySameMonthReturn 的迴圈 + closeFor 的 month 參數）。
// 因此單點突變會成為等價突變（被另一道守住），必須用函式級測試把契約寫明。
func TestNextDaySameMonthReturn_DoesNotCrossContracts(t *testing.T) {
	bars := []domain.FuturesBar{
		bar("202601", "2026-02-10", 100, iPtr(1000)), // 到期月最後一日
		bar("202602", "2026-02-10", 110, iPtr(500)),
		bar("202602", "2026-02-11", 112, iPtr(520)),
	}
	if _, ok := nextDaySameMonthReturn(bars, day("2026-02-10"), "202601"); ok {
		t.Fatal("202601 has no next day: the label must be unavailable (cross-contract label = roll gap misread as return)")
	}
	// 反向對照：同契約有下一日 ⇒ 必須算得出來（否則上面的 nil 可能是「永遠 nil」的假綠）。
	if ret, ok := nextDaySameMonthReturn(bars, day("2026-02-10"), "202602"); !ok {
		t.Fatal("202602 has a next day and must produce a label")
	} else if math.Abs(ret-(112.0-110.0)/110.0*100) > 1e-9 {
		t.Fatalf("label = %v, want +1.8181...%%", ret)
	}
	// nearMonthReturn 同樣不得跨契約。
	if _, ok := nearMonthReturn(bars, day("2026-02-10"), "202603"); ok {
		t.Fatal("202603 has no same-month prior day: near return must be unavailable")
	}
	if ret, ok := nearMonthReturn(bars, day("2026-02-11"), "202602"); !ok {
		t.Fatal("202602 has a prior day and must produce a return")
	} else if math.Abs(ret-(112.0-110.0)/110.0*100) > 1e-9 {
		t.Fatalf("near return = %v, want +1.8181...%%", ret)
	}
}

// TestBuildShadowSeries_WeightsAreInjected 證明分數**由注入權重決定**（無隱藏係數）。
func TestBuildShadowSeries_WeightsAreInjected(t *testing.T) {
	base := testShadowParams()
	flipped := base
	flipped.WTermStructure = -1 // 反轉「逆價差偏多」的假設方向

	a, err := BuildShadowSeries(twoMonthSeries(), "TX", base, nil)
	if err != nil {
		t.Fatalf("build base: %v", err)
	}
	b, err := BuildShadowSeries(twoMonthSeries(), "TX", flipped, nil)
	if err != nil {
		t.Fatalf("build flipped: %v", err)
	}
	if a[0].Predicted != DirectionDown || b[0].Predicted != DirectionUp {
		t.Fatalf("injected weights must control direction: base=%s flipped=%s", a[0].Predicted, b[0].Predicted)
	}
	// 門檻也是注入的：把門檻拉到 1.0 以上 ⇒ 全部 neutral。
	strict := base
	strict.DirectionThreshold = 1.5
	c, err := BuildShadowSeries(twoMonthSeries(), "TX", strict, nil)
	if err != nil {
		t.Fatalf("build strict: %v", err)
	}
	for _, s := range c {
		if s.Predicted != DirectionNeutral {
			t.Fatalf("threshold 1.5 must neutralize every sample, got %s", s.Predicted)
		}
	}
}

// TestBuildShadowSeries_MissingExternalInputsAreNotFabricated 缺外部輸入 ⇒ 該項不計入、欄位 nil。
func TestBuildShadowSeries_MissingExternalInputsAreNotFabricated(t *testing.T) {
	samples, err := BuildShadowSeries(twoMonthSeries(), "TX", testShadowParams(), nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, s := range samples {
		if s.ForeignOIChange != nil {
			t.Fatalf("ForeignOIChange must be nil without institutional input, got %d", *s.ForeignOIChange)
		}
		if s.PCROIRatio != nil || s.PCRDelta != nil {
			t.Fatal("PCR fields must be nil without PCR input")
		}
	}
}

// TestBuildShadowSeries_ExternalInputsAddTerms 提供外部輸入時，項數與值必須如實反映。
func TestBuildShadowSeries_ExternalInputsAddTerms(t *testing.T) {
	params := testShadowParams()
	params.WForeignNet = 1
	params.WPCR = 1

	inputs := map[string]ShadowInputs{
		"2026-01-06": {
			Institutional:     &InstitutionalInput{TradeDate: day("2026-01-06"), ForeignOINet: 2000},
			InstitutionalPrev: &InstitutionalInput{TradeDate: day("2026-01-05"), ForeignOINet: 1000},
			PCR:               &PCRInput{TradeDate: day("2026-01-06"), PutVolume: 120, CallVolume: 100, PutOI: 60, CallOI: 80},
			PCRPrev:           &PCRInput{TradeDate: day("2026-01-05"), PutVolume: 100, CallVolume: 100, PutOI: 50, CallOI: 80},
		},
	}
	samples, err := BuildShadowSeries(twoMonthSeries(), "TX", params, inputs)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	d2 := samples[1]
	if d2.TermsUsed != 4 {
		t.Fatalf("terms = %d, want 4 (structure + OI + foreign + PCR)", d2.TermsUsed)
	}
	if d2.ForeignOIChange == nil || *d2.ForeignOIChange != 1000 {
		t.Fatalf("foreign change = %v, want 1000", d2.ForeignOIChange)
	}
	if d2.PCROIRatio == nil || math.Abs(*d2.PCROIRatio-0.75) > 1e-9 {
		t.Fatalf("PCR OI ratio = %v, want 0.75", d2.PCROIRatio)
	}
	if d2.PCRDelta == nil || math.Abs(*d2.PCRDelta-0.125) > 1e-9 {
		t.Fatalf("PCR delta = %v, want 0.125", d2.PCRDelta)
	}
	// s1=−1（contango）、s2=+1、s3=+1（外資淨多增加）、s4=−sign(+0.125)=−1
	// ⇒ score = (−1+1+1−1)/4 = 0
	if math.Abs(d2.Score) > 1e-9 {
		t.Fatalf("score = %v, want 0", d2.Score)
	}
}

func TestBuildShadowSeries_ValidatesParams(t *testing.T) {
	bad := testShadowParams()
	bad.DirectionThreshold = -1
	if _, err := BuildShadowSeries(twoMonthSeries(), "TX", bad, nil); err == nil {
		t.Fatal("negative threshold must be rejected")
	}
	bad2 := testShadowParams()
	bad2.WPCR = math.NaN()
	if _, err := BuildShadowSeries(twoMonthSeries(), "TX", bad2, nil); err == nil {
		t.Fatal("NaN weight must be rejected")
	}
}

func TestSummarizeHitRate(t *testing.T) {
	hit := true
	miss := false
	samples := []ShadowSample{
		{Hit: &hit}, {Hit: &miss}, {Hit: nil},
	}
	hr := SummarizeHitRate(samples)
	if hr.Total != 2 || hr.Hits != 1 || math.Abs(hr.Rate-0.5) > 1e-9 || hr.Skipped != 1 {
		t.Fatalf("hit rate = %+v, want {2 1 0.5 1}", hr)
	}
	if empty := SummarizeHitRate(nil); empty.Total != 0 || empty.Rate != 0 {
		t.Fatalf("empty hit rate = %+v, want zeros", empty)
	}
}
