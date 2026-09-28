package futures

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func day(s string) time.Time {
	d, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return d
}

func fPtr(v float64) *float64 { return &v }
func iPtr(v int64) *int64     { return &v }

// bar 建立 canonical（一般時段）月契約 bar。
func bar(month, date string, close float64, oi *int64) domain.FuturesBar {
	return domain.FuturesBar{
		Contract: "TX", ContractMonth: month, TradeDate: day(date),
		Session: domain.SessionRegular, Close: fPtr(close), OpenInterest: oi,
		Volume: iPtr(100), Source: "test",
	}
}

// twoMonthSeries 是三個交易日的近月/遠月序列（供 F1/F2 golden 使用）。
//
//	2026-01-05：近月(202601) 100 / OI 1000；遠月(202602) 110 / OI 500
//	2026-01-06：近月 102 / OI 1100；遠月 111 / OI 520
//	2026-01-07：近月 101 / OI 1150；遠月 112 / OI 540
func twoMonthSeries() []domain.FuturesBar {
	return []domain.FuturesBar{
		bar("202601", "2026-01-05", 100, iPtr(1000)),
		bar("202602", "2026-01-05", 110, iPtr(500)),
		bar("202601", "2026-01-06", 102, iPtr(1100)),
		bar("202602", "2026-01-06", 111, iPtr(520)),
		bar("202601", "2026-01-07", 101, iPtr(1150)),
		bar("202602", "2026-01-07", 112, iPtr(540)),
	}
}

func testFeatureParams() FeatureParams {
	// 門檻由測試注入（產品碼內零字面係數）。
	return FeatureParams{FlatSpreadPoints: 0, OIFlatChangePct: 0.1}
}

// TestCalendarSpreadAt_ContangoGolden 釘住 F1 的數值（突變「改壞價差方向」必紅）。
func TestCalendarSpreadAt_ContangoGolden(t *testing.T) {
	spread, err := CalendarSpreadAt(twoMonthSeries(), day("2026-01-05"), testFeatureParams())
	if err != nil {
		t.Fatalf("CalendarSpreadAt: %v", err)
	}
	if spread.NearMonth != "202601" || spread.FarMonth != "202602" {
		t.Fatalf("months = %s/%s, want 202601/202602", spread.NearMonth, spread.FarMonth)
	}
	if spread.NearClose != 100 || spread.FarClose != 110 {
		t.Fatalf("closes = %v/%v, want 100/110", spread.NearClose, spread.FarClose)
	}
	if spread.SpreadPoints != 10 {
		t.Fatalf("spread points = %v, want 10 (遠月 − 近月)", spread.SpreadPoints)
	}
	if spread.SpreadBP != 1000 {
		t.Fatalf("spread bp = %v, want 1000 (10/100 × 10000)", spread.SpreadBP)
	}
	if spread.Structure != StructureContango {
		t.Fatalf("structure = %s, want contango", spread.Structure)
	}
}

func TestCalendarSpreadAt_BackwardationAndFlat(t *testing.T) {
	bars := []domain.FuturesBar{
		bar("202601", "2026-01-05", 110, iPtr(1)),
		bar("202602", "2026-01-05", 100, iPtr(1)),
	}
	spread, err := CalendarSpreadAt(bars, day("2026-01-05"), testFeatureParams())
	if err != nil {
		t.Fatalf("CalendarSpreadAt: %v", err)
	}
	if spread.Structure != StructureBackwardation || spread.SpreadPoints != -10 {
		t.Fatalf("got %s/%v, want backwardation/-10", spread.Structure, spread.SpreadPoints)
	}

	// 平坦門檻：|spread| <= FlatSpreadPoints ⇒ flat（門檻是參數，不是常數）。
	params := FeatureParams{FlatSpreadPoints: 12, OIFlatChangePct: 0.1}
	spread, err = CalendarSpreadAt(bars, day("2026-01-05"), params)
	if err != nil {
		t.Fatalf("CalendarSpreadAt: %v", err)
	}
	if spread.Structure != StructureFlat {
		t.Fatalf("structure = %s, want flat under threshold 12", spread.Structure)
	}
}

func TestCalendarSpreadAt_UnavailableWithoutFarMonth(t *testing.T) {
	bars := []domain.FuturesBar{bar("202601", "2026-01-05", 100, iPtr(1))}
	spread, err := CalendarSpreadAt(bars, day("2026-01-05"), testFeatureParams())
	if err != nil {
		t.Fatalf("CalendarSpreadAt: %v", err)
	}
	if spread.Structure != StructureUnavailable {
		t.Fatalf("structure = %s, want unavailable (must not synthesize a far month)", spread.Structure)
	}
}

// TestCalendarSpreadAt_IgnoresWeeklyAndAfterHours 週契約與盤後列不得參與價差。
func TestCalendarSpreadAt_IgnoresWeeklyAndAfterHours(t *testing.T) {
	bars := twoMonthSeries()
	weekly := bar("202601W2", "2026-01-05", 999, iPtr(9)) // 週契約：月份最小 ⇒ 若未排除會變成「近月」
	afterHours := bar("202512", "2026-01-05", 888, iPtr(8))
	afterHours.Session = domain.SessionAfterHours // 盤後：若未排除會變成「更小的近月」
	bars = append(bars, weekly, afterHours)

	spread, err := CalendarSpreadAt(bars, day("2026-01-05"), testFeatureParams())
	if err != nil {
		t.Fatalf("CalendarSpreadAt: %v", err)
	}
	if spread.NearMonth != "202601" || spread.FarMonth != "202602" {
		t.Fatalf("months = %s/%s, want 202601/202602 (weekly/after-hours leaked)", spread.NearMonth, spread.FarMonth)
	}
	if spread.NearClose != 100 {
		t.Fatalf("near close = %v, want 100", spread.NearClose)
	}
}

// TestOIChangeAt_Golden 釘住 F2 的數值與趨勢標籤。
func TestOIChangeAt_Golden(t *testing.T) {
	oi, err := OIChangeAt(twoMonthSeries(), day("2026-01-06"), testFeatureParams())
	if err != nil {
		t.Fatalf("OIChangeAt: %v", err)
	}
	if oi.NearOI != 1100 || oi.NearOIPrev != 1000 || oi.NearOIChange != 100 {
		t.Fatalf("near OI = %d/%d change %d, want 1100/1000/100", oi.NearOI, oi.NearOIPrev, oi.NearOIChange)
	}
	if math.Abs(oi.NearOIChangePct-10) > 1e-9 {
		t.Fatalf("near OI change pct = %v, want 10", oi.NearOIChangePct)
	}
	// 全市場月契約 OI = 1100 + 520 = 1620；前一日 1000 + 500 = 1500。
	if oi.AllOI != 1620 || oi.AllOIPrev != 1500 || oi.AllOIChange != 120 {
		t.Fatalf("all OI = %d/%d change %d, want 1620/1500/120", oi.AllOI, oi.AllOIPrev, oi.AllOIChange)
	}
	if oi.OINearTrend != "up" {
		t.Fatalf("trend = %s, want up", oi.OINearTrend)
	}

	// 第一個交易日沒有前一日 ⇒ unavailable（不得以 0 當前值）。
	first, err := OIChangeAt(twoMonthSeries(), day("2026-01-05"), testFeatureParams())
	if err != nil {
		t.Fatalf("OIChangeAt: %v", err)
	}
	if first.OINearTrend != "unavailable" {
		t.Fatalf("first day trend = %s, want unavailable", first.OINearTrend)
	}
}

// TestOIChangeAt_MissingOINotZero 缺值（nil）不得被當成 0 混入合計。
func TestOIChangeAt_MissingOINotZero(t *testing.T) {
	bars := []domain.FuturesBar{
		bar("202601", "2026-01-05", 100, iPtr(1000)),
		bar("202602", "2026-01-05", 110, nil), // 遠月 OI 缺值
		bar("202601", "2026-01-06", 102, iPtr(1100)),
		bar("202602", "2026-01-06", 111, nil),
	}
	oi, err := OIChangeAt(bars, day("2026-01-06"), testFeatureParams())
	if err != nil {
		t.Fatalf("OIChangeAt: %v", err)
	}
	if oi.AllOI != 1100 || oi.AllOIPrev != 1000 {
		t.Fatalf("all OI = %d/%d, want 1100/1000 (nil OI must not count as 0 but must also not be summed)", oi.AllOI, oi.AllOIPrev)
	}
}

func TestFeatureParams_Validate(t *testing.T) {
	if err := (FeatureParams{}).Validate(); err != nil {
		t.Fatalf("zero params must be valid: %v", err)
	}
	if err := (FeatureParams{FlatSpreadPoints: -1}).Validate(); err == nil {
		t.Fatal("negative threshold must be rejected")
	}
	if err := (FeatureParams{OIFlatChangePct: math.NaN()}).Validate(); err == nil {
		t.Fatal("NaN threshold must be rejected")
	}
	if _, err := CalendarSpreadAt(twoMonthSeries(), day("2026-01-05"), FeatureParams{FlatSpreadPoints: -1}); err == nil {
		t.Fatal("feature functions must validate params")
	}
}

func TestInstitutionalPositionFrom_NoFabricatedPrev(t *testing.T) {
	now := InstitutionalInput{TradeDate: day("2026-01-06"), ForeignOINet: -5000, InvestmentTrustOINet: 1200, DealerOINet: 300}
	pos := InstitutionalPositionFrom(now, nil)
	if pos.ForeignOIChange != 0 || pos.ForeignOINetPrev != 0 {
		t.Fatalf("without prev, change must stay 0 (no fabrication): %+v", pos)
	}
	if pos.ThreePartyNet != -3500 {
		t.Fatalf("three party net = %d, want -3500", pos.ThreePartyNet)
	}

	prev := InstitutionalInput{TradeDate: day("2026-01-05"), ForeignOINet: -7000}
	pos = InstitutionalPositionFrom(now, &prev)
	if pos.ForeignOIChange != 2000 {
		t.Fatalf("foreign change = %d, want 2000", pos.ForeignOIChange)
	}
}

func TestPCRFeatureFrom_NoFabricatedPrev(t *testing.T) {
	now := PCRInput{TradeDate: day("2026-01-06"), PutVolume: 120, CallVolume: 100, PutOI: 60, CallOI: 80}
	f := PCRFeatureFrom(now, nil)
	if f.HasPrev {
		t.Fatal("no prev ⇒ HasPrev must be false")
	}
	if f.OIRatioChange != 0 || f.VolumeRatioChange != 0 {
		t.Fatalf("changes must stay 0 without prev: %+v", f)
	}
	if math.Abs(f.PutCallVolumeRatio-1.2) > 1e-9 || math.Abs(f.PutCallOIRatio-0.75) > 1e-9 {
		t.Fatalf("ratios = %v/%v, want 1.2/0.75", f.PutCallVolumeRatio, f.PutCallOIRatio)
	}

	prev := PCRInput{TradeDate: day("2026-01-05"), PutVolume: 100, CallVolume: 100, PutOI: 50, CallOI: 80}
	f = PCRFeatureFrom(now, &prev)
	if !f.HasPrev {
		t.Fatal("HasPrev must be true with prev")
	}
	if math.Abs(f.OIRatioChange-0.125) > 1e-9 {
		t.Fatalf("OI ratio change = %v, want 0.125", f.OIRatioChange)
	}
}
