package narrative

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// KnowledgeBase holds causal templates and produces instantiated chains.
type KnowledgeBase struct {
	mu        sync.RWMutex
	templates map[string]CausalTemplate
}

// NewKnowledgeBase creates a knowledge base preloaded with default templates.
func NewKnowledgeBase() *KnowledgeBase {
	kb := &KnowledgeBase{
		templates: make(map[string]CausalTemplate),
	}
	for _, t := range DefaultTemplates() {
		kb.templates[t.ID] = t
	}
	return kb
}

// RegisterTemplate adds or replaces a template.
func (kb *KnowledgeBase) RegisterTemplate(t CausalTemplate) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.templates[t.ID] = t
}

// GetTemplate returns a template by ID.
func (kb *KnowledgeBase) GetTemplate(id string) (CausalTemplate, bool) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	t, ok := kb.templates[id]
	return t, ok
}

// ListTemplates returns all registered templates.
func (kb *KnowledgeBase) ListTemplates() []CausalTemplate {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	out := make([]CausalTemplate, 0, len(kb.templates))
	for _, t := range kb.templates {
		out = append(out, t)
	}
	return out
}

// MatchChains finds all causal templates that match a given event and instantiates chains.
func (kb *KnowledgeBase) MatchChains(event NarrativeEvent) []CausalChain {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var chains []CausalChain
	for _, tmpl := range kb.templates {
		if !strings.EqualFold(tmpl.TriggerTheme, event.Theme) {
			continue
		}
		if tmpl.RequiredRegion != "" && !strings.EqualFold(tmpl.RequiredRegion, event.Region) {
			continue
		}

		score := event.Confidence * tmpl.HistoricalHitRate
		steps := make([]CausalStep, len(tmpl.Steps))
		copy(steps, tmpl.Steps)

		chains = append(chains, CausalChain{
			EventID:    event.ID,
			TemplateID: tmpl.ID,
			Steps:      steps,
			Score:      score,
		})
	}
	return chains
}

// NarrativeEngine orchestrates event detection and causal chain matching.
type NarrativeEngine struct {
	kb            *KnowledgeBase
	models        []InvestmentModel
	stressCalc    *TaiwanStressCalculator
	stressHistory []TaiwanStressIndex
	stressMu      sync.Mutex
	lastMacro     marketdata.MacroDataSnapshot
	prevMacro     marketdata.MacroDataSnapshot
	lastGeo       GeopoliticalRiskScore
}

var defaultSectorSymbolMap = map[string][]string{
	"financials":      {"2881.TW", "2882.TW", "2884.TW", "2885.TW", "2886.TW", "2891.TW", "2892.TW"},
	"high_dividend":   {"0056.TW", "00878.TW"},
	"etf_rotation":    {"0050.TW", "0056.TW", "00878.TW"},
	"ai_supply_chain": {"2330.TW", "2382.TW", "2317.TW", "2345.TW", "3231.TW", "3037.TW", "6669.TW"},
	"semiconductor":   {"2330.TW", "2303.TW", "2308.TW", "2454.TW", "3034.TW"},
	"pcb":             {"3037.TW", "2357.TW"},
	"thermal":         {"2382.TW", "2317.TW", "2357.TW"},
	"shipping":        {"2603.TW", "2609.TW", "2615.TW"},
	"small_cap":       {"3008.TW", "3034.TW", "6669.TW", "3711.TW"},
	"consumer":        {"1301.TW", "1303.TW", "1326.TW", "1216.TW"},
	"tourism":         {},
	"tech":            {"2330.TW", "2317.TW", "2382.TW", "3231.TW"},
	"defensive":       {"2881.TW", "0056.TW", "1301.TW"},
}

var sectorSymbolMap map[string][]string

func init() {
	sectorSymbolMap = loadSectorSymbols("configs/sector_symbols.json")
}

func loadSectorSymbols(path string) map[string][]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return copySectorMap(defaultSectorSymbolMap)
	}

	var loaded map[string][]string
	if err := json.Unmarshal(data, &loaded); err != nil {
		return copySectorMap(defaultSectorSymbolMap)
	}

	return loaded
}

func copySectorMap(src map[string][]string) map[string][]string {
	dst := make(map[string][]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// NewNarrativeEngine creates a narrative engine with default templates and models.
func NewNarrativeEngine() *NarrativeEngine {
	return &NarrativeEngine{
		kb:         NewKnowledgeBase(),
		stressCalc: NewTaiwanStressCalculator(nil, ""),
		models: []InvestmentModel{
			{
				ID:             "hawkish_fed_model",
				Name:           "鷹派聯準會模型",
				Description:    "假設美國利率維持高位且美元走強；偏好台股防禦型板塊（金融、高股息、ETF輪動）。",
				Rationale:      "當美國進入鷹派升息週期，無風險利率攀升會直接抬高全球資產折現率。外資因美元資產收益率上升而回流美國，導致台股資金面緊縮；高估值科技股對折現率最敏感，本益比會被大幅壓縮。此時應押注金融股（淨息差擴大）、高股息ETF（提供穩定現金流與下跌保護）與ETF輪動策略（降低個股風險）；同時迴避AI供應鏈（遠期現金流估值折讓劇烈）與小盤股（外資撤離時流動性折價嚴重）。",
				ActiveThemes:   []string{"US_rates_up", "JPY_carry_unwind"},
				FavoredSectors: []string{"financials", "high_dividend", "etf_rotation"},
				AvoidedSectors: []string{"ai_supply_chain", "small_cap"},
				Weight:         1.0,
			},
			{
				ID:             "ai_supercycle_model",
				Name:           "AI 超級週期模型",
				Description:    "假設 AI 資本支出週期凌駕總經逆風；偏好台灣科技供應鏈（AI供應鏈、半導體、PCB、散熱）。",
				Rationale:      "AI 被視為繼智慧型手機之後最大的科技硬體週期。當輝達、微軟、Google、亞馬遜上修 AI 資本支出時，訂單幾乎100%落到台灣供應鏈——全球只有台積電能量產最先進 AI 晶片，只有台灣擁有完整 CoWoS 先進封裝、高階 PCB 與伺服器組裝產能。這是『結構性產能短缺』帶來的定價權轉移，應押注 AI 供應鏈、半導體、PCB、散熱；同時迴避消費、觀光——市場會持續汰弱留強，把資金從缺乏成長動能的傳產板塊撤出。",
				ActiveThemes:   []string{"AI_capex_surge"},
				FavoredSectors: []string{"ai_supply_chain", "semiconductor", "pcb", "thermal"},
				AvoidedSectors: []string{"consumer", "tourism"},
				Weight:         1.0,
			},
			{
				ID:             "geopolitical_hedge_model",
				Name:           "地緣政治避險模型",
				Description:    "假設地緣政治風險升溫且資金流向避險；偏好黃金、美元避險資產與台股防禦型板塊（金融、高股息、航運）。",
				Rationale:      "地緣政治風險飆升會引發市場風險規避本能，投資人要求更高風險溢酬，直接壓縮股票估值。對外資持股比重高的台股影響尤其劇烈。此時應押注金融（估值低、現金流穩定）、高股息（下跌保護）、航運（地緣衝突初期運價常飆升）；同時迴避 AI 供應鏈與小盤股——科技股本益比高、對風險溢酬最敏感，小盤股流動性差會被外資優先減持。歷史上地緣風險指數突破150時，台灣電子股相對報酬常落後大盤 3~5 個百分點。",
				ActiveThemes:   []string{"geopolitical_risk_spike", "oil_price_shock", "taiwan_political_risk"},
				FavoredSectors: []string{"financials", "high_dividend", "shipping"},
				AvoidedSectors: []string{"ai_supply_chain", "small_cap"},
				Weight:         1.0,
			},
			{
				ID:             "taiwan_political_risk_model",
				Name:           "台灣地緣風險模型",
				Description:    "假設台灣地緣政治風險升溫；偏好內需、金融、高股息等防禦型板塊。",
				Rationale:      "台灣地緣政治風險是台股特有的系統性風險。當兩岸關係緊張、軍事演習頻繁或國際制裁升級時，外資會因擔憂尾部風險而主動減持台股，導致資金面緊縮與估值下修。此時應押注內需（較不受地緣風險直接影響）、金融（估值低、現金流穩定）、高股息（提供下跌保護）；同時迴避 AI 供應鏈、半導體、小盤股——外資持股比重高，地緣風險升高時首當其衝。",
				ActiveThemes:   []string{"taiwan_political_risk", "USD_TWD_volatility"},
				FavoredSectors: []string{"financials", "high_dividend", "consumer"},
				AvoidedSectors: []string{"ai_supply_chain", "semiconductor", "small_cap"},
				Weight:         1.0,
			},
			{
				ID:             "semiconductor_cycle_model",
				Name:           "半導體週期模型",
				Description:    "假設半導體週期下行；偏好防禦型板塊，迴避科技股。",
				Rationale:      "半導體是台灣經濟的核心引擎，佔出口比重超過 35%。當電子零組件出口連續下滑時，意味著全球科技需求正在放緩，半導體庫存開始堆積，產能利用率下降。此時應押注高股息、內需、金融（在週期下行時提供防禦）；同時迴避半導體、AI 供應鏈、PCB——庫存去化期通常持續 2-4 個季度，期間股價承壓。",
				ActiveThemes:   []string{"semiconductor_downturn", "USD_TWD_volatility"},
				FavoredSectors: []string{"high_dividend", "consumer", "financials"},
				AvoidedSectors: []string{"ai_supply_chain", "semiconductor", "pcb"},
				Weight:         1.0,
			},
			{
				ID:             "seasonal_model",
				Name:           "季節性輪動模型",
				Description:    "假設春節季節性行情；偏好高股息、金融、中小型股。",
				Rationale:      "台股有明顯的春節季節性規律。年前2周上漲概率超過70%，年後1個月外資回流推升大盤。Q2進入除權除息旺季，高股息股提前受追捧。此時應押注高股息（除權除息確定性收益）、金融（淨息差擴大）、中小型股（年後資金回流彈性大）；同時迴避電子股（進入傳統淡季）。",
				ActiveThemes:   []string{"spring_festival_season"},
				FavoredSectors: []string{"high_dividend", "financials", "small_cap"},
				AvoidedSectors: []string{"ai_supply_chain", "semiconductor"},
				Weight:         1.0,
			},
			{
				ID:             "election_model",
				Name:           "選舉週期模型",
				Description:    "假設選舉週期影響；選前防禦，選後押注政策受惠股。",
				Rationale:      "台灣選舉對台股有顯著週期性影響。選前3個月政策不確定性導致外資觀望，波動率上升30%。選後1個月政策明朗化帶動資金回流，上漲概率約65%。選前應押注高股息、金融（防禦型配置）；選後押注綠能、基建、國防（受惠於新政府政策方向）；迴避高Beta科技股（選前波動大，外資減持首當其衝）。",
				ActiveThemes:   []string{"election_cycle"},
				FavoredSectors: []string{"high_dividend", "financials", "consumer"},
				AvoidedSectors: []string{"ai_supply_chain", "small_cap"},
				Weight:         1.0,
			},
		},
	}
}

// DetectEvents accepts raw market/narrative data and returns events.
func (ne *NarrativeEngine) DetectEvents(data MarketNarrativeData) []NarrativeEvent {
	var events []NarrativeEvent
	if evt := detectUSRatesEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectAICapexEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectGeopoliticalRiskEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectOilShockEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectJPYCarryUnwindEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectUSDTWDEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectTaiwanPoliticalRiskEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectSemiconductorDownturnEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectSeasonalEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectRetailDivergenceEvent(data); evt != nil {
		events = append(events, *evt)
	}
	return events
}

// MatchChains returns all causal chains matching the given events.
func (ne *NarrativeEngine) MatchChains(events []NarrativeEvent) []CausalChain {
	var all []CausalChain
	for _, evt := range events {
		chains := ne.kb.MatchChains(evt)
		all = append(all, chains...)
	}
	return all
}

// ActiveModels returns investment models whose active themes intersect with the given event themes.
func (ne *NarrativeEngine) ActiveModels(eventThemes []string) []InvestmentModel {
	themeSet := make(map[string]struct{})
	for _, t := range eventThemes {
		themeSet[strings.ToLower(t)] = struct{}{}
	}

	var active []InvestmentModel
	for _, m := range ne.models {
		for _, t := range m.ActiveThemes {
			if _, ok := themeSet[strings.ToLower(t)]; ok {
				active = append(active, m)
				break
			}
		}
	}
	return active
}

// UpdateModelWeights adjusts model weights based on recent prediction errors.
func (ne *NarrativeEngine) UpdateModelWeights() {
	// Simple inverse-error weighting.
	var totalInvErr float64
	for i := range ne.models {
		err := ne.models[i].RecentError
		if err <= 0 {
			err = 0.001
		}
		totalInvErr += 1.0 / err
	}
	for i := range ne.models {
		err := ne.models[i].RecentError
		if err <= 0 {
			err = 0.001
		}
		ne.models[i].Weight = (1.0 / err) / totalInvErr
	}
}

// EvaluateModels computes RecentError for each model by comparing favored vs
// avoided sector forward returns over the last lookback days in the replay dataset.
func (ne *NarrativeEngine) EvaluateModels(replayPath string) error {
	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		return fmt.Errorf("load replay: %w", err)
	}

	const lookback = 30
	const holdWindow = 5
	if len(ds.Dates) < lookback+holdWindow {
		return fmt.Errorf("insufficient replay data: %d dates", len(ds.Dates))
	}

	startIdx := max(len(ds.Dates)-lookback-holdWindow, 0)

	for i := range ne.models {
		m := &ne.models[i]
		correct := 0
		total := 0
		nanSectorSet := make(map[string]bool)

		for d := startIdx; d < startIdx+lookback && d < len(ds.Dates)-holdWindow; d++ {
			date := ds.Dates[d]
			favored := ne.avgSectorReturn(ds, date, holdWindow, m.FavoredSectors)
			avoided := ne.avgSectorReturn(ds, date, holdWindow, m.AvoidedSectors)

			if math.IsNaN(favored) || math.IsNaN(avoided) {
				if math.IsNaN(favored) {
					for _, sector := range m.FavoredSectors {
						nanSectorSet[sector] = true
					}
				}
				if math.IsNaN(avoided) {
					for _, sector := range m.AvoidedSectors {
						nanSectorSet[sector] = true
					}
				}
				continue
			}
			total++
			if favored > avoided {
				correct++
			}
		}

		if total > 0 {
			m.RecentError = 1.0 - (float64(correct) / float64(total))
		} else {
			logging.Warn("narrative", "model_eval_no_data",
				logging.FStr("model", m.Name),
				logging.FStr("reason", "no_valid_sector_comparisons"))
			m.RecentError = 0.5
		}

		if total < 5 && total > 0 {
			logging.Warn("narrative", "model_eval_low_confidence",
				logging.FStr("model", m.Name),
				logging.FInt("observations", total),
				logging.FStr("reason", "few_observations"))
		}

		if len(nanSectorSet) > 0 {
			sectors := make([]string, 0, len(nanSectorSet))
			for s := range nanSectorSet {
				sectors = append(sectors, s)
			}
			logging.Warn("narrative", "model_eval_nan_sectors",
				logging.FStr("model", m.Name),
				logging.FStr("sectors", strings.Join(sectors, ",")))
		}

		hitRate := 1.0 - m.RecentError
		if hitRate < 0 {
			hitRate = 0
		} else if hitRate > 1 {
			hitRate = 1
		}
	}

	ne.UpdateModelWeights()
	return nil
}

func (ne *NarrativeEngine) avgSectorReturn(ds *replay.Dataset, date time.Time, window int, sectors []string) float64 {
	var totalReturn float64
	var count int
	for _, sector := range sectors {
		symbols, ok := sectorSymbolMap[sector]
		if !ok {
			continue
		}
		for _, sym := range symbols {
			ret, ok := ds.ForwardReturn(sym, date, window)
			if !ok {
				continue
			}
			totalReturn += ret
			count++
		}
	}
	if count == 0 {
		return math.NaN()
	}
	return totalReturn / float64(count)
}

// ListModels returns all investment models.
func (ne *NarrativeEngine) ListModels() []InvestmentModel {
	out := make([]InvestmentModel, len(ne.models))
	copy(out, ne.models)
	return out
}

func (ne *NarrativeEngine) GetCurrentStressIndex() TaiwanStressIndex {
	if ne.stressCalc == nil {
		return TaiwanStressIndex{}
	}
	idx := ne.stressCalc.Calculate(ne.lastMacro, ne.prevMacro, ne.lastGeo)
	ne.stressMu.Lock()
	ne.stressHistory = append(ne.stressHistory, idx)
	if len(ne.stressHistory) > 365 {
		ne.stressHistory = ne.stressHistory[len(ne.stressHistory)-365:]
	}
	ne.stressMu.Unlock()
	return idx
}

func (ne *NarrativeEngine) GetStressIndexHistory(days int) []TaiwanStressIndex {
	if days <= 0 {
		days = 30
	}
	ne.stressMu.Lock()
	defer ne.stressMu.Unlock()
	if len(ne.stressHistory) == 0 {
		return []TaiwanStressIndex{}
	}
	if days >= len(ne.stressHistory) {
		return append([]TaiwanStressIndex(nil), ne.stressHistory...)
	}
	return append([]TaiwanStressIndex(nil), ne.stressHistory[len(ne.stressHistory)-days:]...)
}

func (ne *NarrativeEngine) GetStressIndexThresholds() StressIndexThresholds {
	if ne.stressCalc == nil {
		return StressIndexThresholds{}
	}
	tCrisis, tHigh, tAlert := ne.stressCalc.getThresholds()
	return StressIndexThresholds{
		Crisis: tCrisis,
		High:   tHigh,
		Alert:  tAlert,
	}
}

// MarketNarrativeData carries raw inputs for narrative detection.
type MarketNarrativeData struct {
	US10YChangeBps                float64
	DXYChangePct                  float64
	VIXLevel                      float64
	USD_TWD_ChangePct             float64
	OilChangePct                  float64
	GoldChangePct                 float64
	JPY_ChangePct                 float64
	AICapexSentiment              float64 // +1 bullish, -1 bearish
	GeopoliticalGPR               float64 // Geopolitical risk index level
	RetailInstitutionalDivergence float64 // + retail bullish, - retail bearish
	MarginZScore                  float64 // how extreme current margin balance is (reverse indicator)
}

func detectUSRatesEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.US10YChangeBps > 10 || data.DXYChangePct > 1.5 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-us-rates-%d", nowUnix()),
			Theme:            "US_rates_up",
			Region:           "US",
			Sentiment:        -0.6,
			Confidence:       0.75,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("US_rates_up"),
			CapitalFlow:      "flight_to_USD",
			TimeWindow:       "1_week",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"us10y_change_bps": data.US10YChangeBps,
				"dxy_change_pct":   data.DXYChangePct,
			},
		}
	}
	return nil
}

func detectAICapexEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.AICapexSentiment > 0.5 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-ai-capex-%d", nowUnix()),
			Theme:            "AI_capex_surge",
			Region:           "US",
			Sentiment:        0.8,
			Confidence:       0.70,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("AI_capex_surge"),
			CapitalFlow:      "tech_capex_inflow",
			TimeWindow:       "1_month",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"ai_capex_sentiment": data.AICapexSentiment,
			},
		}
	}
	return nil
}

func detectGeopoliticalRiskEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.GeopoliticalGPR > 150 || data.GoldChangePct > 2.0 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-geo-%d", nowUnix()),
			Theme:            "geopolitical_risk_spike",
			Region:           "Global",
			Sentiment:        -0.8,
			Confidence:       0.65,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("geopolitical_risk_spike"),
			CapitalFlow:      "risk_off",
			TimeWindow:       "immediate",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"geopolitical_gpr": data.GeopoliticalGPR,
				"gold_change_pct":  data.GoldChangePct,
			},
		}
	}
	return nil
}

func detectOilShockEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.OilChangePct > 5.0 || data.OilChangePct < -5.0 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-oil-%d", nowUnix()),
			Theme:            "oil_price_shock",
			Region:           "Global",
			Sentiment:        -0.5,
			Confidence:       0.60,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("oil_price_shock"),
			CapitalFlow:      "inflation_reprice",
			TimeWindow:       "1_week",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"oil_change_pct": data.OilChangePct,
			},
		}
	}
	return nil
}

func detectJPYCarryUnwindEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.JPY_ChangePct > 2.0 || data.VIXLevel > 25 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-jpy-%d", nowUnix()),
			Theme:            "JPY_carry_unwind",
			Region:           "JP",
			Sentiment:        -0.6,
			Confidence:       0.65,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("JPY_carry_unwind"),
			CapitalFlow:      "global_liquidity_drain",
			TimeWindow:       "immediate",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"jpy_change_pct": data.JPY_ChangePct,
				"vix_level":      data.VIXLevel,
			},
		}
	}
	return nil
}

func detectUSDTWDEvent(data MarketNarrativeData) *NarrativeEvent {
	if math.Abs(data.USD_TWD_ChangePct) > 1.0 {
		sentiment := -0.5
		if data.USD_TWD_ChangePct > 0 {
			sentiment = -0.7
		}
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-usd-twd-%d", nowUnix()),
			Theme:            "USD_TWD_volatility",
			Region:           "TW",
			Sentiment:        sentiment,
			Confidence:       0.60,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("USD_TWD_volatility"),
			CapitalFlow:      "fx_driven_outflow",
			TimeWindow:       "1_week",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"usd_twd_change_pct": data.USD_TWD_ChangePct,
			},
		}
	}
	return nil
}

func detectTaiwanPoliticalRiskEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.GeopoliticalGPR > 150 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-tw-pol-%d", nowUnix()),
			Theme:            "taiwan_political_risk",
			Region:           "TW",
			Sentiment:        -0.8,
			Confidence:       0.70,
			ConfidenceSource: "heuristic_fixed_v1",
			HitRate:          hitRateForTheme("taiwan_political_risk"),
			CapitalFlow:      "risk_off",
			TimeWindow:       "immediate",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"geopolitical_gpr": data.GeopoliticalGPR,
			},
		}
	}
	return nil
}

func detectSemiconductorDownturnEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.VIXLevel > 25 && data.DXYChangePct > 1.0 && data.AICapexSentiment < -0.3 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-semi-dt-%d", nowUnix()),
			Theme:            "semiconductor_downturn",
			Region:           "TW",
			Sentiment:        -0.6,
			Confidence:       0.55,
			ConfidenceSource: "heuristic_composite_v1",
			HitRate:          hitRateForTheme("semiconductor_downturn"),
			CapitalFlow:      "tech_capex_slowdown",
			TimeWindow:       "1_month",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"vix_level":          data.VIXLevel,
				"dxy_change_pct":     data.DXYChangePct,
				"ai_capex_sentiment": data.AICapexSentiment,
			},
		}
	}
	return nil
}

func detectSeasonalEvent(data MarketNarrativeData) *NarrativeEvent {
	now := time.Now().UTC()
	month := now.Month()
	day := now.Day()

	if (month == 1 && day >= 15) || (month == 2 && day <= 15) {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-spring-%d", nowUnix()),
			Theme:            "spring_festival_season",
			Region:           "TW",
			Sentiment:        0.3,
			Confidence:       0.65,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          hitRateForTheme("spring_festival_season"),
			CapitalFlow:      "seasonal_rotation",
			TimeWindow:       "1_month",
			Timestamp:        now,
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if (month == 1 && day <= 15) || (month == 12 && day >= 20) {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-election-%d", nowUnix()),
			Theme:            "election_cycle",
			Region:           "TW",
			Sentiment:        -0.2,
			Confidence:       0.60,
			ConfidenceSource: "calendar_political",
			HitRate:          hitRateForTheme("election_cycle"),
			CapitalFlow:      "policy_uncertainty",
			TimeWindow:       "1_month",
			Timestamp:        now,
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if (month == 3 && day >= 1) || (month == 4 && day <= 15) {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-blackout-%d", nowUnix()),
			Theme:            "earnings_blackout",
			Region:           "TW",
			Sentiment:        0.1,
			Confidence:       0.55,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          0.55,
			CapitalFlow:      "pre_earnings_positioning",
			TimeWindow:       "1_month",
			Timestamp:        now,
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if (month == 7 && day >= 1) || (month == 8) || (month == 9 && day <= 15) {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-tech-peak-%d", nowUnix()),
			Theme:            "tech_peak_season",
			Region:           "TW",
			Sentiment:        0.5,
			Confidence:       0.75,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          0.75,
			CapitalFlow:      "tech_capex_inflow",
			TimeWindow:       "2_months",
			Timestamp:        now,
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if month == 11 || month == 12 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-yearend-%d", nowUnix()),
			Theme:            "year_end_window_dressing",
			Region:           "TW",
			Sentiment:        0.2,
			Confidence:       0.58,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          0.58,
			CapitalFlow:      "institutional_rebalancing",
			TimeWindow:       "2_months",
			Timestamp:        now,
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	return nil
}

func detectRetailDivergenceEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.MarginZScore > 1.5 && data.RetailInstitutionalDivergence > 0 {
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-retail-div-%d", nowUnix()),
			Theme:            "retail_institutional_divergence",
			Region:           "TW",
			Sentiment:        -0.5,
			Confidence:       0.60,
			ConfidenceSource: "divergence_zscore_v1",
			HitRate:          hitRateForTheme("retail_institutional_divergence"),
			CapitalFlow:      "crowding_risk",
			TimeWindow:       "immediate",
			Timestamp:        time.Now().UTC(),
			SourceData: map[string]float64{
				"margin_zscore":                   data.MarginZScore,
				"retail_institutional_divergence": data.RetailInstitutionalDivergence,
			},
		}
	}
	return nil
}

var nowUnix = func() int64 {
	// Overridden in tests.
	return time.Now().UnixNano()
}
