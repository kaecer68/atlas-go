// Package futures 提供期貨市場的**跨市場訊號特徵**（階段 1）。
//
// 設計邊界（業主 2026-09-28 定案；詳見 docs/specs/futures-crossmarket-signal-spec.md）：
//   - 本套件是**純函式**：只做數學，不碰 DB、不打網路、不讀環境變數。
//   - **零字面係數**：所有門檻／權重都必須由呼叫端以參數注入（門檻不得藏在產品邏輯裡）。
//     這是 product-positioning.md §8「heuristic 一律經過驗證管道」的落地方式：
//     未經校準的門檻不得寫死，只能作為「待校準的假設」以參數形式流入。
//   - 本階段**不做基差（futures − spot）**：現貨側在本 repo 沒有可回溯多年的 first-party
//     日序列（taiwan_index_history.go 只是給 tw_vol 用的 rolling JSON），列為後續票。
//   - 本套件**不得**被接進部位規模／風控／歸因（WeightFor / ApplySignal）。R3 未修前，
//     接線由階段 2（#2110）處理。本套件只產生「影子預測」供量測。
package futures

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FeatureParams 是所有特徵計算所需的門檻。
//
// **沒有任何預設值**：呼叫端必須明確提供。刻意不提供 DefaultFeatureParams()，
// 以免「預設值」在不知不覺間變成產品邏輯（本專案 2026-09-27 #2107 的同型教訓）。
type FeatureParams struct {
	// FlatSpreadPoints 是「價差視為平坦」的門檻（指數點）。|spread| <= 此值 ⇒ flat。
	FlatSpreadPoints float64
	// OIFlatChangePct 是「OI 變化視為持平」的百分比門檻（例：0.5 代表 ±0.5%）。
	OIFlatChangePct float64
}

// Validate 檢查參數可用性（不得為負、不得 NaN/Inf）。
func (p FeatureParams) Validate() error {
	if math.IsNaN(p.FlatSpreadPoints) || math.IsInf(p.FlatSpreadPoints, 0) || p.FlatSpreadPoints < 0 {
		return fmt.Errorf("futures features: FlatSpreadPoints must be a finite value >= 0 (got %v)", p.FlatSpreadPoints)
	}
	if math.IsNaN(p.OIFlatChangePct) || math.IsInf(p.OIFlatChangePct, 0) || p.OIFlatChangePct < 0 {
		return fmt.Errorf("futures features: OIFlatChangePct must be a finite value >= 0 (got %v)", p.OIFlatChangePct)
	}
	return nil
}

// TermStructure 描述跨月（近月 vs 遠月）價差結構。
type TermStructure string

const (
	// StructureContango：遠月 > 近月（正價差；期貨市場對未來的定價高於近月）。
	StructureContango TermStructure = "contango"
	// StructureBackwardation：遠月 < 近月（逆價差）。
	StructureBackwardation TermStructure = "backwardation"
	// StructureFlat：兩者差距在門檻內。
	StructureFlat TermStructure = "flat"
	// StructureUnavailable：當日沒有同時具備近月與遠月報價（無價差可算）。
	StructureUnavailable TermStructure = "unavailable"
)

// CalendarSpread 是某交易日的跨月價差結構（自足特徵：只需期貨 bars）。
type CalendarSpread struct {
	TradeDate time.Time `json:"trade_date"`
	Contract  string    `json:"contract"`
	NearMonth string    `json:"near_month"`
	FarMonth  string    `json:"far_month"`
	NearClose float64   `json:"near_close"`
	FarClose  float64   `json:"far_close"`
	// SpreadPoints = FarClose − NearClose（指數點；散戶可直接理解為「遠月貴/便宜幾點」）。
	SpreadPoints float64 `json:"spread_points"`
	// SpreadBP = SpreadPoints / NearClose × 10000（萬分點；消除絕對指數位階的影響）。
	SpreadBP  float64       `json:"spread_bp"`
	Structure TermStructure `json:"structure"`
}

// OIChange 是未平倉量（OI）的日變化（自足特徵：只需期貨 bars）。
type OIChange struct {
	TradeDate time.Time `json:"trade_date"`
	// Near 是前月契約的 OI 變化（口）。
	NearOI          int64   `json:"near_oi"`
	NearOIPrev      int64   `json:"near_oi_prev"`
	NearOIChange    int64   `json:"near_oi_change"`
	NearOIChangePct float64 `json:"near_oi_change_pct"`
	// All 是所有**月契約**（一般時段）的 OI 合計變化（口）。
	AllOI          int64   `json:"all_oi"`
	AllOIPrev      int64   `json:"all_oi_prev"`
	AllOIChange    int64   `json:"all_oi_change"`
	AllOIChangePct float64 `json:"all_oi_change_pct"`
	// OINearTrend 是近月 OI 變化的方向標籤（up/down/flat），門檻由參數注入。
	OINearTrend string `json:"oi_near_trend"`
}

// InstitutionalInput 是三大法人期貨部位的**輸入**（由呼叫端提供，非本套件取得）。
//
// 資料來源（呼叫端責任）：internal/marketdata.FetchInstitutionalFuturesDaily
// （TAIFEX OpenAPI，僅最新交易日）。歷史深度見 spec §5 F3。
type InstitutionalInput struct {
	TradeDate            time.Time `json:"trade_date"`
	ForeignOINet         int64     `json:"foreign_oi_net"`
	InvestmentTrustOINet int64     `json:"investment_trust_oi_net"`
	DealerOINet          int64     `json:"dealer_oi_net"`
}

// InstitutionalPosition 是三大法人期貨淨部位特徵。
type InstitutionalPosition struct {
	TradeDate            time.Time `json:"trade_date"`
	ForeignOINet         int64     `json:"foreign_oi_net"`
	ForeignOINetPrev     int64     `json:"foreign_oi_net_prev"`
	ForeignOIChange      int64     `json:"foreign_oi_change"`
	InvestmentTrustOINet int64     `json:"investment_trust_oi_net"`
	DealerOINet          int64     `json:"dealer_oi_net"`
	// ThreePartyNet 是三方合計淨部位（口）。
	ThreePartyNet int64 `json:"three_party_net"`
}

// PCRInput 是選擇權 put/call 資料的**輸入**（由呼叫端提供，非本套件取得）。
//
// 資料來源（呼叫端責任）：internal/marketdata.FetchPCR（TAIFEX OpenAPI PutCallRatio）。
type PCRInput struct {
	TradeDate  time.Time `json:"trade_date"`
	PutVolume  int64     `json:"put_volume"`
	CallVolume int64     `json:"call_volume"`
	PutOI      int64     `json:"put_oi"`
	CallOI     int64     `json:"call_oi"`
}

// PCRFeature 是 put/call ratio 特徵。
type PCRFeature struct {
	TradeDate time.Time `json:"trade_date"`
	// PutCallVolumeRatio = PutVolume / CallVolume（>1 表 put 成交量大於 call）。
	PutCallVolumeRatio float64 `json:"put_call_volume_ratio"`
	// PutCallOIRatio = PutOI / CallOI。
	PutCallOIRatio float64 `json:"put_call_oi_ratio"`
	// OIRatioChange = 當日 PutCallOIRatio − 前一日（需前一日輸入；無則 0 且 HasPrev=false）。
	OIRatioChange     float64 `json:"oi_ratio_change"`
	VolumeRatioChange float64 `json:"volume_ratio_change"`
	HasPrev           bool    `json:"has_prev"`
}

// ─── F1 跨月價差結構 ─────────────────────────────────────────────────────────

// CalendarSpreadAt 計算某交易日的跨月價差結構（F1）。
//
// 前月（near）＝該日 canonical（一般時段）月契約中月份最小者。
// 遠月（far）＝該日 canonical 月契約中，月份**大於**前月且最小者。
//
// 只使用 domain.SessionRegular 與標準月契約（^\d{6}$）；週契約與盤後列不參與。
// 缺少近月或遠月任一 ⇒ 回傳 StructureUnavailable（**不猜測、不插值**）。
func CalendarSpreadAt(bars []domain.FuturesBar, day time.Time, params FeatureParams) (CalendarSpread, error) {
	if err := params.Validate(); err != nil {
		return CalendarSpread{}, err
	}
	dayKey := day.Format("2006-01-02")
	closes := make(map[string]float64)
	contract := ""
	for _, b := range bars {
		if b.Session != domain.SessionRegular || !domain.IsMonthlyContractMonth(b.ContractMonth) {
			continue
		}
		if b.Contract == "" || b.Close == nil {
			continue
		}
		if b.Contract != "" && contract == "" {
			contract = b.Contract
		}
		if b.TradeDate.Format("2006-01-02") != dayKey {
			continue
		}
		closes[b.ContractMonth] = *b.Close
	}
	out := CalendarSpread{TradeDate: day, Contract: contract, Structure: StructureUnavailable}
	if len(closes) < 2 {
		return out, nil
	}
	months := make([]string, 0, len(closes))
	for m := range closes {
		months = append(months, m)
	}
	sort.Strings(months)
	near, far := months[0], months[1]
	out.NearMonth, out.FarMonth = near, far
	out.NearClose, out.FarClose = closes[near], closes[far]
	out.SpreadPoints = out.FarClose - out.NearClose
	if out.NearClose != 0 {
		out.SpreadBP = out.SpreadPoints / out.NearClose * 10000
	}
	switch {
	case math.Abs(out.SpreadPoints) <= params.FlatSpreadPoints:
		out.Structure = StructureFlat
	case out.SpreadPoints > 0:
		out.Structure = StructureContango
	default:
		out.Structure = StructureBackwardation
	}
	return out, nil
}

// ─── F2 OI 日變化 ────────────────────────────────────────────────────────────

// OIChangeAt 計算某交易日的 OI 日變化（F2）。
//
// 近月 OI ＝ 該日 canonical 月契約中月份最小者的 OpenInterest。
// 全市場月契約 OI ＝ 該日所有 canonical **月契約** OpenInterest 之和。
// 前一日以「上一個有資料的交易日」為基準（呼叫端必須提供已排序的 bars；本函式自行找前一日）。
//
// 任一側缺值（nil）⇒ 該欄位以 0 計並在回傳中反映（不插值）。若當日或前一日完全無資料，
// 回傳零值結構並帶 OINearTrend="unavailable"。
func OIChangeAt(bars []domain.FuturesBar, day time.Time, params FeatureParams) (OIChange, error) {
	if err := params.Validate(); err != nil {
		return OIChange{}, err
	}
	dayKey := day.Format("2006-01-02")
	prevKey := previousTradingDayKey(bars, dayKey)
	if prevKey == "" {
		return OIChange{TradeDate: day, OINearTrend: "unavailable"}, nil
	}

	nearNow, allNow, okNow := oiSnapshot(bars, dayKey)
	nearPrev, allPrev, okPrev := oiSnapshot(bars, prevKey)
	if !okNow || !okPrev {
		return OIChange{TradeDate: day, OINearTrend: "unavailable"}, nil
	}

	out := OIChange{
		TradeDate:    day,
		NearOI:       nearNow,
		NearOIPrev:   nearPrev,
		NearOIChange: nearNow - nearPrev,
		AllOI:        allNow,
		AllOIPrev:    allPrev,
		AllOIChange:  allNow - allPrev,
		OINearTrend:  "flat",
	}
	out.NearOIChangePct = pctChange(float64(nearPrev), float64(nearNow))
	out.AllOIChangePct = pctChange(float64(allPrev), float64(allNow))
	switch {
	case math.Abs(out.NearOIChangePct) <= params.OIFlatChangePct:
		out.OINearTrend = "flat"
	case out.NearOIChangePct > 0:
		out.OINearTrend = "up"
	default:
		out.OINearTrend = "down"
	}
	return out, nil
}

// oiSnapshot 回傳某交易日的（近月 OI, 所有月契約 OI 合計, 是否有任何 canonical 月契約列）。
//
// 近月 = 該日月份最小的 canonical 月契約。缺值（OpenInterest == nil）不計入合計、
// 且不會被當成 0 混進「近月 OI」（近月缺值 ⇒ 近月回 0 但 seen 仍為 true）。
func oiSnapshot(bars []domain.FuturesBar, dayKey string) (int64, int64, bool) {
	nearMonth := nearestMonthKey(bars, dayKey)
	if nearMonth == "" {
		return 0, 0, false
	}
	var (
		near int64
		all  int64
	)
	for _, b := range bars {
		if b.Session != domain.SessionRegular || !domain.IsMonthlyContractMonth(b.ContractMonth) {
			continue
		}
		if b.TradeDate.Format("2006-01-02") != dayKey || b.OpenInterest == nil {
			continue
		}
		all += *b.OpenInterest
		if b.ContractMonth == nearMonth {
			near = *b.OpenInterest
		}
	}
	return near, all, true
}

// nearestMonthKey 回傳某交易日 canonical 月契約中月份最小者（無則空字串）。
func nearestMonthKey(bars []domain.FuturesBar, dayKey string) string {
	best := ""
	for _, b := range bars {
		if b.Session != domain.SessionRegular || !domain.IsMonthlyContractMonth(b.ContractMonth) {
			continue
		}
		if b.TradeDate.Format("2006-01-02") != dayKey {
			continue
		}
		if best == "" || b.ContractMonth < best {
			best = b.ContractMonth
		}
	}
	return best
}

// previousTradingDayKey 回傳 bars 中小於 dayKey 的最大日期（YYYY-MM-DD）。
func previousTradingDayKey(bars []domain.FuturesBar, dayKey string) string {
	best := ""
	for _, b := range bars {
		k := b.TradeDate.Format("2006-01-02")
		if k >= dayKey {
			continue
		}
		if best == "" || k > best {
			best = k
		}
	}
	return best
}

func pctChange(prev, now float64) float64 {
	if prev == 0 {
		return 0
	}
	return (now - prev) / math.Abs(prev) * 100
}

// ─── F3 三大法人期貨部位 ─────────────────────────────────────────────────────

// InstitutionalPositionFrom 由當日與前一日的法人輸入計算 F3。
// prev 為 nil ⇒ ForeignOIChange 以 0 計，HasPrev 概念由呼叫端判讀（本函式不假造前值）。
func InstitutionalPositionFrom(now InstitutionalInput, prev *InstitutionalInput) InstitutionalPosition {
	out := InstitutionalPosition{
		TradeDate:            now.TradeDate,
		ForeignOINet:         now.ForeignOINet,
		InvestmentTrustOINet: now.InvestmentTrustOINet,
		DealerOINet:          now.DealerOINet,
		ThreePartyNet:        now.ForeignOINet + now.InvestmentTrustOINet + now.DealerOINet,
	}
	if prev != nil {
		out.ForeignOINetPrev = prev.ForeignOINet
		out.ForeignOIChange = now.ForeignOINet - prev.ForeignOINet
	}
	return out
}

// ─── F4 PCR ──────────────────────────────────────────────────────────────────

// PCRFeatureFrom 由當日（可選前一日）PCR 輸入計算 F4。
// 缺前一日 ⇒ HasPrev=false 且兩個 change 欄位為 0（不得假造）。
func PCRFeatureFrom(now PCRInput, prev *PCRInput) PCRFeature {
	out := PCRFeature{TradeDate: now.TradeDate}
	if now.CallVolume != 0 {
		out.PutCallVolumeRatio = float64(now.PutVolume) / float64(now.CallVolume)
	}
	if now.CallOI != 0 {
		out.PutCallOIRatio = float64(now.PutOI) / float64(now.CallOI)
	}
	if prev == nil {
		return out
	}
	out.HasPrev = true
	if prev.CallVolume != 0 {
		out.VolumeRatioChange = out.PutCallVolumeRatio - float64(prev.PutVolume)/float64(prev.CallVolume)
	}
	if prev.CallOI != 0 {
		out.OIRatioChange = out.PutCallOIRatio - float64(prev.PutOI)/float64(prev.CallOI)
	}
	return out
}
