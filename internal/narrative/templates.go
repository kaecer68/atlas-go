package narrative

// DefaultTemplates returns the built-in causal narrative templates.
func DefaultTemplates() []CausalTemplate {
	return []CausalTemplate{
		{
			ID:             "美國升息 / 鷹派聯準會",
			Name:           "美國升息 / 鷹派聯準會",
			TriggerTheme:   "US_rates_up",
			RequiredRegion: "US",
			Steps: []CausalStep{
				{Description: "美債殖利率上升 → 美元走強", Affected: []string{"DXY", "新興市場貨幣"}, Impact: 0.6},
				{Description: "美元走強 + 利率上升引發新興亞洲外資流出", Affected: []string{"外資流向_台股"}, Impact: -0.7},
				{Description: "台股大盤承壓；防禦型板塊相對抗跌", Affected: []string{"金融", "高股息", "ETF輪動"}, Impact: 0.4},
				{Description: "高Beta板塊（AI供應鏈、中小型股）面臨估值壓縮", Affected: []string{"AI供應鏈", "中小型股"}, Impact: -0.6},
			},
			HistoricalHitRate: 0.72,
			SourceReferences:  []string{"IMF Working Paper WP/19/128", "BIS Quarterly Review Dec 2022"},
			Rationale: `當美國進入鷹派升息週期，無風險利率（以美債10年期為代表）快速攀升，這會直接抬高全球資產的折現率。對台股而言，這意味著兩件事：第一，外資會因美元資產收益率上升而回流美國，導致台股資金面緊縮；第二，高估值、高成長性的科技股對折現率最敏感，本益比會被大幅壓縮。

因此，此時應該【押注】對利率上升「免疫」甚至「受惠」的板塊：
• 金融股（如富邦金、國泰金）：淨息差（NIM）隨利率上升而擴大，獲利直接改善。
• 高股息ETF（如0056、00878）：在市場動盪時提供穩定現金流，且成分股多為成熟企業，估值相對穩健。
• ETF輪動策略：透過降低個股集中風險，捕捉防禦型資產的相對強勢。

同時必須【迴避】對資金成本最敏感的板塊：
• AI供應鏈（台積電、廣達等）：雖然長期趨勢不變，但在升息環境下，遠期現金流的估值折讓會讓股價劇烈波動。
• 小盤股：流動性較差，在外資撤離時往往出現「流動性折價」，跌幅會大於大盤。`,
		},
		{
			ID:             "日圓套利平倉",
			Name:           "日圓套利平倉",
			TriggerTheme:   "JPY_carry_unwind",
			RequiredRegion: "JP",
			Steps: []CausalStep{
				{Description: "日本央行升息或縮表推升日圓", Affected: []string{"日圓", "全球流動性"}, Impact: 0.5},
				{Description: "日圓融資槓桿部位平倉，降低全球風險偏好", Affected: []string{"全球股市", "VIX"}, Impact: -0.6},
				{Description: "台灣高槓桿 / 高估值個股面臨短期拋壓", Affected: []string{"AI供應鏈", "中小型股"}, Impact: -0.5},
				{Description: "避險資金流入日圓與優質資產", Affected: []string{"日圓", "黃金", "高股息"}, Impact: 0.4},
			},
			HistoricalHitRate: 0.68,
			SourceReferences:  []string{"BIS Annual Economic Report 2024", "Goldman Sachs Global Macro Note Aug 2024"},
			Rationale: `日圓長期是全球最大的「融資貨幣」之一，許多對沖基金與機構投資人以極低利率借入日圓，再投入高風險資產（包括台灣科技股）。當日本央行轉鷹或日圓急升時，這些「套利交易」會被迫快速平倉——賣掉台股、買回日圓還債，形成全球流動性的瞬間收縮。

這種去槓桿過程對台股的衝擊具有明顯的「結構性偏好」：
• 應該【押注】高股息與優質資產：因為套利平倉引發的是「質量逃離」（flight to quality），資金會湧向現金流穩定、財務體質健全的標的。高股息ETF和大型金融股在此階段相對抗跌。
• 必須【迴避】AI供應鏈與小盤股：這些是外資與槓桿資金最愛的板塊，也是平倉時第一個被賣出的標的。歷史上日圓急升（如2007年8月、2024年8月）時，台灣電子股往往出現「無差別殺盤」，跌幅遠超加權指數。`,
		},
		{
			ID:             "AI 資本支出激增",
			Name:           "AI 資本支出激增",
			TriggerTheme:   "AI_capex_surge",
			RequiredRegion: "US",
			Steps: []CausalStep{
				{Description: "輝達 / 微軟 / 超大規模雲端業者上修資本支出展望", Affected: []string{"美國科技資本支出"}, Impact: 0.8},
				{Description: "CoWoS 與先進封裝需求吃緊", Affected: []string{"半導體", "晶圓代工"}, Impact: 0.7},
				{Description: "台灣 AI 供應鏈（晶圓代工、封裝、PCB、散熱）受惠", Affected: []string{"AI供應鏈", "半導體", "PCB", "散熱"}, Impact: 0.8},
				{Description: "上游設備與材料訂單能見度提升", Affected: []string{"半導體設備", "材料"}, Impact: 0.5},
			},
			HistoricalHitRate: 0.81,
			SourceReferences:  []string{"Morgan Stanley Taiwan Tech Outlook 2024", "Goldman Sachs Asia Pacific Strategy 2024"},
			Rationale: `AI 被視為繼智慧型手機之後最大的科技硬體週期。當輝達、微軟、Google、亞馬遜等超大規模雲端業者上修 AI 資本支出時，這些訂單幾乎100%會落到台灣供應鏈——因為全球只有台積電能大量生產最先進的 AI 晶片，只有台灣擁有完整的 CoWoS 先進封裝、高階 PCB（如欣興）、以及伺服器組裝（廣達、緯穎）產能。

這不是一般的景氣循環，而是「結構性產能短缺」帶來的定價權轉移：
• 應該【押注】AI供應鏈、半導體、PCB、散熱：這些板塊的訂單能見度已經排到數個季度之後，營收與獲利上修的確定性最高。
• 應該【迴避】消費、觀光：在總資金有限的情況下，市場會持續「汰弱留強」，把資金從缺乏成長動能的傳產消費板塊撤出，轉而追逐 AI 硬體的稀缺產能。這種「擠出效應」會讓消費股相對大盤持續跑輸。`,
		},
		{
			ID:             "地緣政治風險飆升",
			Name:           "地緣政治風險飆升",
			TriggerTheme:   "geopolitical_risk_spike",
			RequiredRegion: "Global",
			Steps: []CausalStep{
				{Description: "中東或台海緊張情勢升級，不確定性上升", Affected: []string{"地緣政治風險指數", "原油"}, Impact: -0.8},
				{Description: "避險需求升溫：美元與黃金同步上漲", Affected: []string{"DXY", "黃金"}, Impact: 0.7},
				{Description: "台股風險溢酬擴大；資金轉向防禦配置", Affected: []string{"台股大盤"}, Impact: -0.5},
				{Description: "防禦型板塊（金融、高股息、航運避險）表現優於週期股", Affected: []string{"金融", "高股息", "航運"}, Impact: 0.4},
				{Description: "高Beta科技股與出口導向個股面臨本益比下修", Affected: []string{"AI供應鏈", "中小型股"}, Impact: -0.6},
			},
			HistoricalHitRate: 0.65,
			SourceReferences:  []string{"Caldara-Iacoviello GPR Dataset", "Fed Finance and Economics Discussion Series 2023"},
			Rationale: `地緣政治風險飆升（無論是中東衝突、台海緊張或東歐戰事）會引發市場的「風險規避」本能。投資人會要求更高的風險溢酬（risk premium），這直接體現在股票估值上：不確定性越高，投資人願意支付的本益比就越低。

對台股這個高度出口導向、外資持股比重高的市場，影響尤其劇烈：
• 應該【押注】金融、高股息、航運：金融股估值低、現金流穩定，是傳統避風港；高股息提供「下跌保護」；航運股則常在地緣衝突初期因運價飆升而逆勢上漲。
• 必須【迴避】AI供應鏈與小盤股：科技股本益比高、對風險溢酬最敏感；小盤股流動性差，在地緣風險升高時會被外資優先減持。更重要的是，台灣 AI 供應鏈的終端需求高度依賴中美貿易與全球資本支出，地緣政治緊張會讓客戶「延後拉貨」或「分散供應鏈」，直接衝擊訂單能見度。`,
		},
		{
			ID:             "油價衝擊",
			Name:           "油價衝擊",
			TriggerTheme:   "oil_price_shock",
			RequiredRegion: "Global",
			Steps: []CausalStep{
				{Description: "油價劇烈變動改變通膨預期", Affected: []string{"通膨預期", "原油"}, Impact: -0.6},
				{Description: "聯準會政策路徑重新定價（供給衝擊則偏鷹、需求崩潰則偏鴿）", Affected: []string{"聯準會基金期貨", "美國利率"}, Impact: -0.5},
				{Description: "利率敏感資產重估；台灣運輸與石化板塊直接受影響", Affected: []string{"航運", "石化"}, Impact: -0.4},
				{Description: "消費性產業與利潤敏感板塊面臨成本壓力", Affected: []string{"消費", "觀光"}, Impact: -0.3},
			},
			HistoricalHitRate: 0.58,
			SourceReferences:  []string{"Hamilton (1983) JPE - Oil and Macroeconomy", "IMF World Economic Outlook 2022"},
			Rationale: `油價是現代經濟最重要的「隱性稅收」。當油價劇烈波動時，它會同時影響通膨預期、企業成本結構、以及聯準會的利率路徑——這三個因素幾乎決定了所有風險資產的定價。

判斷油價衝擊的投資策略，關鍵在於區分「供給衝擊」還是「需求崩潰」：
• 如果是供給衝擊（如中東產能中斷），這是「壞的通膨」——聯準會會被迫維持鷹派，對利率敏感的高估值股不利。此時應減碼成長股，轉向現金流穩健的防禦型板塊。
• 如果是需求崩潰（如全球衰退），油價下跌雖然降低通膨，但反映的是企業獲利萎縮，這時週期股（如航運、石化）和消費股都會受創。

對台股而言，無論哪種情境，【航運股】都首當其衝：油價是航運最大的變動成本，油價飆升會侵蝕獲利；但如果是地緣因素導致的運價上漲，又可能抵銷部分成本壓力。整體而言，油價劇烈波動期應降低對週期股的曝險，提高現金或防禦型資產的比重。`,
		},
		{
			ID:             "散戶狂熱預警",
			Name:           "散戶狂熱預警",
			TriggerTheme:   "retail_frenzy",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "融資餘額創高，散戶槓桿部位過度集中", Affected: []string{"台股大盤", "高Beta股"}, Impact: -0.5},
				{Description: "融資維持率下降，追繳壓力增", Affected: []string{"中小型股"}, Impact: -0.4},
				{Description: "籌碼轉為脆弱，回調風險升溫", Affected: []string{"台股大盤"}, Impact: -0.3},
			},
			HistoricalHitRate: 0.55,
			SourceReferences:  []string{"TWSE Margin Trading Statistics"},
			Rationale:         `當融資餘額達歷史90th percentile以上，代表散戶情緒極度亢奮、槓桿部位過度集中。歷史統計顯示，此時後1-2周回調機率約55%。應減碼高Beta股與中小型股，轉向防禦型配置。`,
		},
		{
			ID:             "散戶恐慌反向信號",
			Name:           "散戶恐慌反向信號",
			TriggerTheme:   "retail_fear",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "融資餘額創低，散戶極度悲觀", Affected: []string{"台股大盤"}, Impact: 0.4},
				{Description: "融資維持率回升，追繳壓力減", Affected: []string{"中小型股"}, Impact: 0.3},
				{Description: "籌碼清洗完成，反彈機率升", Affected: []string{"台股大盤"}, Impact: 0.3},
			},
			HistoricalHitRate: 0.60,
			SourceReferences:  []string{"TWSE Margin Trading Statistics"},
			Rationale:         `當融資餘額降至歷史10th percentile以下，代表散戶極度恐慌、槓桿已大幅去化。歷史統計顯示，此時後1-2周反彈機率約60%。應逢低布局優質股，尤其是被錯殺的中小型股。`,
		},
		{
			ID:             "中東衝突升級",
			Name:           "中東衝突升級",
			TriggerTheme:   "middle_east_escalation",
			RequiredRegion: "Middle East",
			Steps: []CausalStep{
				{Description: "衝突升級威脅石油供給與紅海航運通道", Affected: []string{"原油", "航運"}, Impact: -0.8},
				{Description: "油價飆升推升全球通膨預期", Affected: []string{"通膨預期", "美國利率"}, Impact: -0.6},
				{Description: "聯準會被迫維持鷹派立場以對抗供給型通膨", Affected: []string{"聯準會基金期貨", "DXY"}, Impact: 0.5},
				{Description: "避險資金流入黃金與美元，推升美元走強", Affected: []string{"黃金", "DXY"}, Impact: 0.7},
				{Description: "新興市場因風險偏好下降面臨資金外流", Affected: []string{"新興市場貨幣", "外資流向_台股"}, Impact: -0.7},
				{Description: "台灣高Beta科技與出口導向個股面臨本益比下修", Affected: []string{"AI供應鏈", "半導體", "中小型股"}, Impact: -0.6},
				{Description: "防禦型板塊（金融、高股息、航運避險）相對抗跌", Affected: []string{"金融", "高股息", "航運"}, Impact: 0.3},
			},
			HistoricalHitRate: 0.68,
			SourceReferences:  []string{"Caldara-Iacoviello GPR Dataset", "BIS Quarterly Review Dec 2023 - Geopolitical risk and commodity prices", "Goldman Sachs Global Macro Research 2024"},
			Rationale: `中東衝突升級是一種「複合式供給衝擊」：它同時打擊能源供給（推升油價）、打擊全球貿易通道（紅海航運），並引發避險情緒（資金流向美元與黃金）。這三股力量會形成惡性循環，對新興市場股市造成「三重打擊」。

對台股的傳導路徑非常清晰：
1. 油價飆升 → 通膨預期上升 → 聯準會維持高利率 → 科技股估值受壓
2. 避險情緒升溫 → 美元走強 → 外資從台股撤出 → 流動性緊縮
3. 紅海航運受阻 → 運價飆升 + 供應鏈不確定性 → 出口導向企業成本上升

因此，此時的投資配置必須極度防禦：
• 【押注】金融、高股息：在低成長、高不確定性環境中，「現金為王」——能穩定配息、財務槓桿低的企業會獲得資金青睞。
• 【迴避】AI供應鏈、半導體、小盤股：這些板塊同時承受「利率估值壓縮」、「外資賣壓」、「終端需求延後」三重風險。歷史數據顯示，當地緣政治風險指數（GPR）突破150時，台灣電子股的相對報酬在隨後一個月平均落後大盤 3~5 個百分點。`,
		},
	}
}
