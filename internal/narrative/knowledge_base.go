package narrative

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// WeightAuditHook is called when model weights are updated (Stage 2.3b).
// modelID identifies the model; weightHash is a SHA-256 hex digest of the
// canonical JSON representation of all model weights.
type WeightAuditHook func(modelID, weightHash string)

// NarrativeEngine orchestrates event detection and causal chain matching.
type NarrativeEngine struct {
	kb            *KnowledgeBase
	models        []InvestmentModel
	stressCalc    *TaiwanStressCalculator
	stressHistory []TaiwanStressIndex
	stressMu      sync.Mutex
	lastMacro     marketdata.MacroDataSnapshot
	prevMacro     marketdata.MacroDataSnapshot
	lastGeo       geopolitical.GeopoliticalRiskScore
	weightHook    WeightAuditHook

	evalMu      sync.Mutex
	evalPath    string
	evalModTime time.Time
	evalDone    bool
}

// KnowledgeBase returns the underlying template knowledge base.
func (ne *NarrativeEngine) KnowledgeBase() *KnowledgeBase {
	return ne.kb
}

// SetWeightAuditHook wires a mutation audit callback for model weight changes.
func (ne *NarrativeEngine) SetWeightAuditHook(hook WeightAuditHook) {
	ne.stressMu.Lock()
	ne.weightHook = hook
	ne.stressMu.Unlock()
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
	"tourism":         {"2707.TW", "2731.TW", "2739.TW", "2748.TW", "5706.TW"},
	"tech":            {"2330.TW", "2317.TW", "2382.TW", "3231.TW"},
	"defensive":       {"2881.TW", "0056.TW", "1301.TW"},
	"technology":      {"2330.TW", "2317.TW", "2382.TW", "2454.TW", "2303.TW", "3034.TW"},
	"traditional":     {"1301.TW", "1303.TW", "1326.TW", "1216.TW", "1101.TW", "2002.TW"},
}

// sectorAliasMap translates Chinese sector names used in causal templates
// to the English keys expected by sectorSymbolMap.
var sectorAliasMap = map[string]string{
	"金融":    "financials",
	"高股息":   "high_dividend",
	"ETF輪動": "etf_rotation",
	"AI供應鏈": "ai_supply_chain",
	"半導體":   "semiconductor",
	"晶圓代工":  "semiconductor",
	"PCB":   "pcb",
	"散熱":    "thermal",
	"航運":    "shipping",
	"貨櫃航運":  "shipping",
	"散裝航運":  "shipping",
	"小型股":   "small_cap",
	"中小型股":  "small_cap",
	"消費":    "consumer",
	"內需":    "consumer",
	"觀光":    "tourism",
	"科技板塊":  "technology",
	"防禦性板塊": "defensive",
	"傳產":    "traditional",
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
	ne := &NarrativeEngine{
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
				RecentError:    0.28,
				HitRate:        0.72,
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
				RecentError:    0.19,
				HitRate:        0.81,
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
				RecentError:    0.35,
				HitRate:        0.65,
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
				RecentError:    0.35,
				HitRate:        0.65,
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
				RecentError:    0.40,
				HitRate:        0.60,
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
				RecentError:    0.30,
				HitRate:        0.70,
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
				RecentError:    0.35,
				HitRate:        0.65,
				Weight:         1.0,
			},
			{
				ID:             "retail_divergence_model",
				Name:           "散戶與法人背離",
				Description:    "當散戶情緒與法人籌碼出現明顯背離時，通常預示市場轉折點",
				Rationale:      "散戶傾向追高殺低，法人具有資訊優勢。散戶增加槓桿但法人持續賣出時，暗示市場即將修正",
				ActiveThemes:   []string{"retail_institutional_divergence"},
				FavoredSectors: []string{"defensive"},
				AvoidedSectors: []string{"technology", "semiconductor"},
				RecentError:    0.40,
				HitRate:        0.60,
				Weight:         0.60,
			},
			{
				ID:             "earnings_surprise_model",
				Name:           "財報驚喜驅動",
				Description:    "台灣科技股財報超出預期時，資金重分配至半導體與AI供應鏈",
				Rationale:      "台灣股市以科技股為核心，財報驚喜直接影響法人評等調整與資金流向",
				ActiveThemes:   []string{"earnings_surprise"},
				FavoredSectors: []string{"semiconductor", "ai_supply_chain"},
				AvoidedSectors: []string{"traditional"},
				RecentError:    0.25,
				HitRate:        0.75,
				Weight:         0.75,
			},
		},
	}
	ne.UpdateModelWeights()
	return ne
}

// DetectEvents accepts raw market/narrative data and returns events.
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
// Uses inverse-error weighting with a 40% single-model cap to prevent one
// lucky model from dominating the ensemble.
func (ne *NarrativeEngine) UpdateModelWeights() {
	const maxWeight = 0.40

	// Compute raw inverse-error weights.
	invErrs := make([]float64, len(ne.models))
	var totalInvErr float64
	for i := range ne.models {
		err := ne.models[i].RecentError
		if err <= 0 {
			err = 0.001
		}
		invErrs[i] = 1.0 / err
		totalInvErr += invErrs[i]
	}

	weights := make([]float64, len(ne.models))
	for i := range ne.models {
		weights[i] = invErrs[i] / totalInvErr
	}

	// Iteratively cap and redistribute excess.
	for {
		excess := 0.0
		uncappedSum := 0.0
		cappedCount := 0
		for i, w := range weights {
			if w > maxWeight {
				excess += w - maxWeight
				weights[i] = maxWeight
				cappedCount++
			} else {
				uncappedSum += w
			}
		}
		if excess == 0 || uncappedSum == 0 {
			break
		}
		// Redistribute excess proportionally to uncapped models.
		for i, w := range weights {
			if w < maxWeight {
				weights[i] = w + (w/uncappedSum)*excess
			}
		}
		if cappedCount == len(weights) {
			break
		}
	}

	for i := range ne.models {
		ne.models[i].Weight = weights[i]
	}
	// Stage 2.3b: fire weight change audit hook.
	if ne.weightHook != nil {
		for i := range ne.models {
			raw, _ := json.Marshal(ne.models[i])
			hash := sha256Hex(raw)
			ne.weightHook(ne.models[i].ID, hash)
		}
	}
}

// EvaluateModels computes RecentError for each model by comparing favored vs
// avoided sector forward returns over the last lookback days in the replay dataset.
// Results are memoized: if the replay CSV has not changed (same path and
// modification time) since the last call, the computation is skipped.
func (ne *NarrativeEngine) EvaluateModels(replayPath string) error {
	// Check memoization: skip when the CSV has not changed.
	ne.evalMu.Lock()
	if stat, err := os.Stat(replayPath); err == nil && ne.evalDone && ne.evalPath == replayPath && stat.ModTime().Equal(ne.evalModTime) {
		ne.evalMu.Unlock()
		return nil
	}
	if stat, err := os.Stat(replayPath); err == nil {
		ne.evalModTime = stat.ModTime()
	}
	ne.evalPath = replayPath
	ne.evalMu.Unlock()

	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		return fmt.Errorf("load replay: %w", err)
	}

	params := config.GetParametersConfig().Narrative
	lookback := params.ModelLookbackDays.Value
	if lookback <= 0 {
		lookback = 30
	}
	holdWindow := params.ModelHoldWindowDays.Value
	if holdWindow <= 0 {
		holdWindow = 5
	}
	if len(ds.Dates) < lookback+holdWindow {
		return fmt.Errorf("insufficient replay data: %d dates", len(ds.Dates))
	}

	startIdx := max(len(ds.Dates)-lookback-holdWindow, 0)

	for i := range ne.models {
		m := &ne.models[i]
		correct := 0
		total := 0
		nanSectorSet := make(map[string]bool)
		var lastFavored, lastAvoided float64

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
			lastFavored = favored
			lastAvoided = avoided
			total++
			if favored > avoided {
				correct++
			}
		}

		if total > 0 {
			m.RecentError = 1.0 - (float64(correct) / float64(total))
			if !math.IsNaN(lastFavored) && !math.IsNaN(lastAvoided) {
				m.RecentPrediction = lastFavored - lastAvoided
			}
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
		m.HitRate = hitRate
	}

	ne.UpdateModelWeights()
	ne.updateTemplateHitRates()
	ne.evalMu.Lock()
	ne.evalDone = true
	ne.evalMu.Unlock()
	return nil
}

func (ne *NarrativeEngine) updateTemplateHitRates() {
	const alpha = 0.2
	for i := range ne.models {
		m := &ne.models[i]
		if m.RecentError > 0.5 {
			continue
		}
		for _, theme := range m.ActiveThemes {
			if tmpl, ok := ne.kb.GetTemplateByTheme(theme); ok {
				updated := (1-alpha)*tmpl.HistoricalHitRate + alpha*m.HitRate
				tmpl.HistoricalHitRate = updated
				ne.kb.RegisterTemplate(tmpl)
			}
		}
	}
}

// RecalculateTemplateHitRates exposes the EMA-blended template recalculation
// so external schedulers (Stage 3 BTM) can drive it without exposing
// updateTemplateHitRates on the engine's private surface.
func (ne *NarrativeEngine) RecalculateTemplateHitRates() {
	ne.updateTemplateHitRates()
}

// RecalculateAllTemplateHitRates — Stage 4 PR#4.
//
// Fills the templates that existing RecalculateTemplateHitRates leaves
// at default 0.5 because no active model touches them. Chained light-EMA
// pull toward a backtest-derived globalHitRate (typically AVG(hit) from
// Stage 4 prediction_backtest). Returns templates updated past epsilon.
func (ne *NarrativeEngine) RecalculateAllTemplateHitRates(globalHitRate float64) int {
	ne.updateTemplateHitRates()

	const lightAlpha = 0.1
	const epsilon = 0.001
	updated := 0
	for _, tmpl := range ne.kb.ListTemplates() {
		if math.Abs(tmpl.HistoricalHitRate-globalHitRate) < epsilon {
			continue
		}
		newRate := (1-lightAlpha)*tmpl.HistoricalHitRate + lightAlpha*globalHitRate
		if math.Abs(newRate-tmpl.HistoricalHitRate) < epsilon {
			continue
		}
		tmpl.HistoricalHitRate = newRate
		ne.kb.RegisterTemplate(tmpl)
		updated++
	}
	return updated
}

func (ne *NarrativeEngine) avgSectorReturn(ds *replay.Dataset, date time.Time, window int, sectors []string) float64 {
	var totalReturn float64
	var count int
	for _, sector := range sectors {
		// Resolve Chinese aliases to English keys.
		if alias, ok := sectorAliasMap[sector]; ok {
			sector = alias
		}
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

// UpdateMacro updates the engine's macro state after each successful ingestion.
// Must be called to populate lastMacro/prevMacro/lastGeo used by GetCurrentStressIndex.
func (ne *NarrativeEngine) UpdateMacro(macro marketdata.MacroDataSnapshot, geo geopolitical.GeopoliticalRiskScore) {
	ne.stressMu.Lock()
	defer ne.stressMu.Unlock()
	ne.prevMacro = ne.lastMacro
	ne.lastMacro = macro
	ne.lastGeo = geo
}

// MarketNarrativeData re-derives MarketNarrativeData from the
// latest macro snapshot + lastGeo. Overlays GeopoliticalGPR (which
// the snapshot converter leaves at 0).
func (ne *NarrativeEngine) MarketNarrativeData() MarketNarrativeData {
	ne.stressMu.Lock()
	snap := ne.lastMacro
	geo := ne.lastGeo
	ne.stressMu.Unlock()

	data := MarketNarrativeDataFromSnapshot(snap)
	data.GeopoliticalGPR = geo.Intensity
	return data
}

func (ne *NarrativeEngine) GetCurrentStressIndex() TaiwanStressIndex {
	if ne.stressCalc == nil {
		return TaiwanStressIndex{}
	}
	return ne.stressCalc.Calculate(ne.lastMacro, ne.prevMacro, ne.lastGeo)
}

// RecordStressIndex appends an index value to the in-memory ring buffer.
// Callers that want to grow stressHistory explicitly (e.g. tests or a future
// persistence adapter) use this after computing the index with GetCurrentStressIndex.
func (ne *NarrativeEngine) RecordStressIndex(idx TaiwanStressIndex) {
	ne.stressMu.Lock()
	defer ne.stressMu.Unlock()
	ne.stressHistory = append(ne.stressHistory, idx)
	if len(ne.stressHistory) > 365 {
		ne.stressHistory = ne.stressHistory[len(ne.stressHistory)-365:]
	}
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
