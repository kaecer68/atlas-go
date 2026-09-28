package futures

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// 影子預測（階段 1）：把特徵組合成一個**透明可解釋**的分數，量測它對
// 「隔一交易日報酬符號」的預測力。結果**只寫入獨立的影子儲存**，不進任何
// 引擎路徑（R3 未修前不得接線；見 spec §2）。

// ShadowParams 是影子規則的參數（**全部由呼叫端注入，程式內零字面係數**）。
type ShadowParams struct {
	Features FeatureParams

	// 各項假設的方向權重。正權重 = 「該項偏多」時分數上升。
	// 這些權重的**數值本身是待校準的假設**，不是已驗證的事實；
	// spec §6 對每一項給出可對散戶解釋的經濟讀法。
	WTermStructure float64
	WOIChange      float64
	WForeignNet    float64
	WPCR           float64

	// DirectionThreshold：|score| > 門檻才給方向，否則 neutral。
	DirectionThreshold float64
}

// Validate 檢查影子參數。
func (p ShadowParams) Validate() error {
	if err := p.Features.Validate(); err != nil {
		return err
	}
	if math.IsNaN(p.DirectionThreshold) || math.IsInf(p.DirectionThreshold, 0) || p.DirectionThreshold < 0 {
		return fmt.Errorf("futures shadow: DirectionThreshold must be finite and >= 0 (got %v)", p.DirectionThreshold)
	}
	for name, w := range map[string]float64{
		"WTermStructure": p.WTermStructure,
		"WOIChange":      p.WOIChange,
		"WForeignNet":    p.WForeignNet,
		"WPCR":           p.WPCR,
	} {
		if math.IsNaN(w) || math.IsInf(w, 0) {
			return fmt.Errorf("futures shadow: %s must be finite (got %v)", name, w)
		}
	}
	return nil
}

// Direction 是影子預測與實際標籤的方向。
type Direction string

const (
	DirectionUp      Direction = "up"
	DirectionDown    Direction = "down"
	DirectionNeutral Direction = "neutral"
)

// ShadowInputs 是某一個交易日的所有可用輸入。
//
// 除了期貨 bars 之外的外部輸入（法人、PCR）以指標提供：**缺就當作該項不可用**，
// 不得以 0 假造（0 是合法值，例如外資淨部位為 0 口）。
type ShadowInputs struct {
	Institutional     *InstitutionalInput
	InstitutionalPrev *InstitutionalInput
	PCR               *PCRInput
	PCRPrev           *PCRInput
}

// ShadowSample 是一筆影子樣本（預測 ＋ 已實現標籤 ＋ 命中）。
type ShadowSample struct {
	TradeDate time.Time `json:"trade_date"`
	Contract  string    `json:"contract"`
	NearMonth string    `json:"near_month"`

	TermStructure   string  `json:"term_structure"`
	SpreadPoints    float64 `json:"spread_points"`
	SpreadBP        float64 `json:"spread_bp"`
	OINearTrend     string  `json:"oi_near_trend"`
	OINearChangePct float64 `json:"oi_near_change_pct"`
	NearReturnPct   float64 `json:"near_return_pct"`

	ForeignOIChange *int64   `json:"foreign_oi_change,omitempty"`
	PCROIRatio      *float64 `json:"pcr_oi_ratio,omitempty"`
	PCRDelta        *float64 `json:"pcr_oi_ratio_change,omitempty"`

	// Score 是各項假設的加權和（已按可用權重絕對值和正規化 ⇒ 值域 [-1, 1]）。
	Score float64 `json:"score"`
	// TermsUsed 是本筆實際採用的假設項數（未提供的輸入不計入，也不補 0）。
	TermsUsed  int       `json:"terms_used"`
	Predicted  Direction `json:"predicted_direction"`
	Confidence float64   `json:"confidence"`

	// 標籤（T+1，同一契約月）：無法計算時為 nil（不插值）。
	LabelReturnPct  *float64  `json:"label_return_pct,omitempty"`
	ActualDirection Direction `json:"actual_direction,omitempty"`
	// Hit = 預測與實際同向；neutral 任一側 ⇒ nil（與 repo 既有雙邊語意一致，下游略過）。
	Hit *bool `json:"hit,omitempty"`
}

// BuildShadowSeries 由**單一契約**的期貨 bars 產生影子樣本序列。
//
// 規則（透明、可對散戶解釋；spec §6）：
//
//	s1 跨月價差結構：backwardation ⇒ +1（遠月較便宜 ⇒ 現貨偏緊的假設），
//	                  contango ⇒ −1，flat/unavailable ⇒ 0（不計入）
//	s2 OI 變化：     sign(OI 變化) × sign(近月報酬) ⇒ 「新倉順勢」假設（+1/−1/0）
//	s3 外資淨部位變化：sign(變化) ⇒ +1（淨多增加）
//	s4 PCR OI ratio 變化：−sign(變化) ⇒ 買權賣權避險比上升視為偏空假設
//
// score = Σ w_i·s_i / Σ |w_i|（僅計入可用項）；confidence = |score|；
// |score| > DirectionThreshold ⇒ up/down，否則 neutral。
//
// 標籤：**同一契約月**的 T+1 報酬符號。刻意不用「換倉後的新契約」，
// 以免把換倉價差誤當成報酬；因此換倉日（舊契約隔日無報價）的樣本**沒有標籤**，
// 會被計入但 Hit 為 nil（不插值、不合成）。
func BuildShadowSeries(bars []domain.FuturesBar, contract string, params ShadowParams, inputs map[string]ShadowInputs) ([]ShadowSample, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	days := tradingDays(bars, contract)
	if len(days) == 0 {
		return nil, nil
	}

	out := make([]ShadowSample, 0, len(days))
	for _, day := range days {
		dayKey := day.Format("2006-01-02")
		spread, err := CalendarSpreadAt(bars, day, params.Features)
		if err != nil {
			return nil, err
		}
		oi, err := OIChangeAt(bars, day, params.Features)
		if err != nil {
			return nil, err
		}
		nearRet, hasNearRet := nearMonthReturn(bars, day, spread.NearMonth)

		sample := ShadowSample{
			TradeDate:       day,
			Contract:        contract,
			NearMonth:       spread.NearMonth,
			TermStructure:   string(spread.Structure),
			SpreadPoints:    spread.SpreadPoints,
			SpreadBP:        spread.SpreadBP,
			OINearTrend:     oi.OINearTrend,
			OINearChangePct: oi.NearOIChangePct,
			NearReturnPct:   nearRet,
		}

		var (
			weighted float64
			absSum   float64
		)
		addTerm := func(weight, sign float64) {
			if weight == 0 || sign == 0 {
				return
			}
			weighted += weight * sign
			absSum += math.Abs(weight)
			sample.TermsUsed++
		}

		switch spread.Structure {
		case StructureBackwardation:
			addTerm(params.WTermStructure, 1)
		case StructureContango:
			addTerm(params.WTermStructure, -1)
		case StructureFlat, StructureUnavailable:
			// 不計入（平坦/無價差不是一個方向假設）
		}

		if hasNearRet && oi.OINearTrend != "unavailable" && oi.NearOIChange != 0 {
			addTerm(params.WOIChange, signOf(float64(oi.NearOIChange))*signOf(nearRet))
		}

		if in, ok := inputs[dayKey]; ok {
			if in.Institutional != nil && in.InstitutionalPrev != nil {
				pos := InstitutionalPositionFrom(*in.Institutional, in.InstitutionalPrev)
				sample.ForeignOIChange = &pos.ForeignOIChange
				addTerm(params.WForeignNet, signOf(float64(pos.ForeignOIChange)))
			}
			if in.PCR != nil {
				pcr := PCRFeatureFrom(*in.PCR, in.PCRPrev)
				sample.PCROIRatio = &pcr.PutCallOIRatio
				if pcr.HasPrev {
					sample.PCRDelta = &pcr.OIRatioChange
					addTerm(params.WPCR, -signOf(pcr.OIRatioChange))
				}
			}
		}

		if absSum > 0 {
			sample.Score = weighted / absSum
		}
		sample.Confidence = math.Abs(sample.Score)
		sample.Predicted = directionFromScore(sample.Score, params.DirectionThreshold)

		// 標籤：同一契約月的 T+1 報酬。
		if ret, ok := nextDaySameMonthReturn(bars, day, spread.NearMonth); ok {
			sample.LabelReturnPct = &ret
			sample.ActualDirection = directionFromSignValue(ret)
			if sample.Predicted != DirectionNeutral && sample.ActualDirection != DirectionNeutral {
				hit := sample.Predicted == sample.ActualDirection
				sample.Hit = &hit
			}
		}
		out = append(out, sample)
	}
	return out, nil
}

// HitRate 匯總命中率（neutral 樣本略過，與 repo 既有校準器語意一致）。
type HitRate struct {
	Total   int     `json:"total"` // 有標籤且雙邊非 neutral 的樣本數
	Hits    int     `json:"hits"`
	Rate    float64 `json:"rate"`
	Skipped int     `json:"skipped"` // 無標籤或任一方 neutral
}

// SummarizeHitRate 計算命中率。
func SummarizeHitRate(samples []ShadowSample) HitRate {
	var hr HitRate
	for _, s := range samples {
		if s.Hit == nil {
			hr.Skipped++
			continue
		}
		hr.Total++
		if *s.Hit {
			hr.Hits++
		}
	}
	if hr.Total > 0 {
		hr.Rate = float64(hr.Hits) / float64(hr.Total)
	}
	return hr
}

func directionFromScore(score, threshold float64) Direction {
	switch {
	case score > threshold:
		return DirectionUp
	case score < -threshold:
		return DirectionDown
	default:
		return DirectionNeutral
	}
}

func directionFromSignValue(v float64) Direction {
	switch {
	case v > 0:
		return DirectionUp
	case v < 0:
		return DirectionDown
	default:
		return DirectionNeutral
	}
}

func signOf(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// tradingDays 回傳某契約有 canonical 月契約資料的交易日（升冪、去重）。
func tradingDays(bars []domain.FuturesBar, contract string) []time.Time {
	seen := make(map[string]time.Time)
	for _, b := range bars {
		if contract != "" && b.Contract != contract {
			continue
		}
		if b.Session != domain.SessionRegular || !domain.IsMonthlyContractMonth(b.ContractMonth) {
			continue
		}
		if b.Close == nil {
			continue
		}
		seen[b.TradeDate.Format("2006-01-02")] = b.TradeDate
	}
	out := make([]time.Time, 0, len(seen))
	for _, d := range seen {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// nearMonthReturn 回傳當日近月契約的日報酬（相對前一個有該月報價的交易日）。
func nearMonthReturn(bars []domain.FuturesBar, day time.Time, month string) (float64, bool) {
	if month == "" {
		return 0, false
	}
	dayKey := day.Format("2006-01-02")
	now, ok := closeFor(bars, month, dayKey)
	if !ok {
		return 0, false
	}
	prevKey := ""
	for _, b := range bars {
		if b.ContractMonth != month || b.Session != domain.SessionRegular || b.Close == nil {
			continue
		}
		k := b.TradeDate.Format("2006-01-02")
		if k >= dayKey {
			continue
		}
		if prevKey == "" || k > prevKey {
			prevKey = k
		}
	}
	if prevKey == "" {
		return 0, false
	}
	prev, ok := closeFor(bars, month, prevKey)
	if !ok || prev == 0 {
		return 0, false
	}
	return (now - prev) / prev * 100, true
}

// nextDaySameMonthReturn 回傳「同一契約月」在下一個有報價交易日的報酬。
func nextDaySameMonthReturn(bars []domain.FuturesBar, day time.Time, month string) (float64, bool) {
	if month == "" {
		return 0, false
	}
	dayKey := day.Format("2006-01-02")
	now, ok := closeFor(bars, month, dayKey)
	if !ok || now == 0 {
		return 0, false
	}
	nextKey := ""
	for _, b := range bars {
		if b.ContractMonth != month || b.Session != domain.SessionRegular || b.Close == nil {
			continue
		}
		k := b.TradeDate.Format("2006-01-02")
		if k <= dayKey {
			continue
		}
		if nextKey == "" || k < nextKey {
			nextKey = k
		}
	}
	if nextKey == "" {
		return 0, false
	}
	next, ok := closeFor(bars, month, nextKey)
	if !ok {
		return 0, false
	}
	return (next - now) / now * 100, true
}

func closeFor(bars []domain.FuturesBar, month, dayKey string) (float64, bool) {
	for _, b := range bars {
		if b.ContractMonth != month || b.Session != domain.SessionRegular {
			continue
		}
		if b.TradeDate.Format("2006-01-02") != dayKey || b.Close == nil {
			continue
		}
		return *b.Close, true
	}
	return 0, false
}
