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
			ID:             "春節行情",
			Name:           "春節行情",
			TriggerTheme:   "spring_festival_season",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "農曆年前資金回籠，市場出現紅包行情", Affected: []string{"台股大盤", "高股息"}, Impact: 0.5},
				{Description: "年後資金回流，補漲行情延續", Affected: []string{"台股大盤", "中小型股"}, Impact: 0.4},
				{Description: "除權除息旺季，高股息股受追捧", Affected: []string{"高股息", "金融"}, Impact: 0.6},
				{Description: "電子股進入淡季，傳產內需相對強勢", Affected: []string{"內需", "傳產"}, Impact: 0.3},
			},
			HistoricalHitRate: 0.70,
			SourceReferences:  []string{"Taiwan Stock Exchange Seasonal Analysis"},
			Rationale: `台股有明顯的春節季節性規律。歷史數據顯示，年前2周上漲概率超過70%，年後1個月外資回流推升大盤。這是因為：1) 台灣企業年終獎金發放後資金流入股市；2) 農曆年後法人重新布局；3) Q2進入除權除息旺季，高股息股提前受追捧。

投資配置：
• 【押注】高股息、金融：除權除息旺季的確定性收益
• 【押注】中小型股：年後資金回流時彈性較大
• 【迴避】電子股：進入傳統淡季，營收動能較弱`,
		},
		{
			ID:             "選舉週期",
			Name:           "選舉週期",
			TriggerTheme:   "election_cycle",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "選前3個月政策不確定性升溫", Affected: []string{"台股大盤", "外資流向"}, Impact: -0.5},
				{Description: "外資因不確定性減少台股曝險", Affected: []string{"外資流向_台股"}, Impact: -0.6},
				{Description: "選後政策明朗化，資金回流", Affected: []string{"台股大盤"}, Impact: 0.5},
				{Description: "特定產業（綠能/基建/國防）受政策青睞", Affected: []string{"綠能", "基建", "國防"}, Impact: 0.4},
			},
			HistoricalHitRate: 0.65,
			SourceReferences:  []string{"Taiwan Election and Stock Market Historical Analysis"},
			Rationale: `台灣選舉對台股有顯著的週期性影響。選前3個月，政策不確定性導致外資觀望，波動率通常上升30%。選後1個月，政策明朗化帶動資金回流，上漲概率約65%。

投資配置：
• 【選前】高股息、金融：防禦型配置，降低不確定性曝險
• 【選後】綠能、基建、國防：受惠於新政府政策方向
• 【迴避】高Beta科技股：選前波動大，外資減持首當其衝`,
		},
		{
			ID:             "台灣地緣政治風險",
			Name:           "台灣地緣政治風險",
			TriggerTheme:   "taiwan_political_risk",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "兩岸關係緊張或軍事演習升級", Affected: []string{"台海風險", "軍事不確定性"}, Impact: -0.9},
				{Description: "外資擔憂地緣風險而減持台股", Affected: []string{"外資流向_台股", "台股大盤"}, Impact: -0.8},
				{Description: "台幣貶壓加劇，資金外流", Affected: []string{"USD/TWD", "台灣流動性"}, Impact: -0.6},
				{Description: "防禦型板塊（金融、高股息、內需）相對抗跌", Affected: []string{"金融", "高股息", "內需"}, Impact: 0.4},
				{Description: "高Beta科技股與出口導向個股面臨本益比下修", Affected: []string{"AI供應鏈", "半導體", "中小型股"}, Impact: -0.7},
			},
			HistoricalHitRate: 0.65,
			SourceReferences:  []string{"Taiwan Relations Act", "CSIS First Battle Report 2023"},
			Rationale: `台灣地緣政治風險是台股特有的「系統性風險」。當兩岸關係緊張、軍事演習頻繁或國際制裁升級時，外資會因擔憂「尾部風險」而主動減持台股，導致資金面緊縮與估值下修。

對台股的傳導路徑：
1. 兩岸緊張 → 外資風險溢酬要求提高 → 台股估值壓縮
2. 避險情緒 → 台幣貶值壓力 → 資金外流
3. 終端客戶擔憂供應鏈中斷 → 訂單延後或轉單

投資配置：
• 【押注】金融、高股息、內需：這些板塊較不受地緣風險直接影響，且能提供穩定現金流
• 【迴避】AI供應鏈、半導體、小盤股：外資持股比重高，地緣風險升高時首當其衝`,
		},
		{
			ID:             "USD/TWD 劇烈波動",
			Name:           "USD/TWD 劇烈波動",
			TriggerTheme:   "USD_TWD_volatility",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "美元對台幣匯率劇烈變動（>1%）", Affected: []string{"USD/TWD", "出口競爭力"}, Impact: -0.6},
				{Description: "美元走強 → 台灣出口商競爭力下降，進口成本上升", Affected: []string{"出口導向", "進口成本"}, Impact: -0.5},
				{Description: "外公換匯成本增加，減少台股投資意願", Affected: []string{"外公流向_台股"}, Impact: -0.4},
				{Description: "內需與金融板塊相對抗跌", Affected: []string{"內需", "金融"}, Impact: 0.3},
			},
			HistoricalHitRate: 0.62,
			SourceReferences:  []string{"Central Bank of Taiwan Quarterly Report"},
			Rationale: `USD/TWD 匯率是台灣出口競爭力的核心指標。台灣出口佔 GDP 65%，電子產品為主要出口項目，對匯率極度敏感。當美元對台急需貶（>1%）時，雖然短期對出口商有匯兌收益，但長期會侵蝕競爭力；且外公會因換匯成本增加而減少投資意願。

投資配置：
• 【押注】內需、金融：較不受匯率波動直接影響
• 【迴避】出口導向科技股：雖有匯兌收益，但競爭力長期受損`,
		},
		{
			ID:             "半導體週期下行",
			Name:           "半導體週期下行",
			TriggerTheme:   "semiconductor_downturn",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "台灣電子零組件出口連續下滑（<-5%）", Affected: []string{"台灣出口", "電子零組件"}, Impact: -0.7},
				{Description: "半導體庫存去化，產能利用率下降", Affected: []string{"半導體", "晶圓代工"}, Impact: -0.6},
				{Description: "AI 供應鏈訂單能見度下降", Affected: []string{"AI供應鏈", "伺服器"}, Impact: -0.5},
				{Description: "防禦型板塊（高股息、內需）相對抗跌", Affected: []string{"高股息", "內需"}, Impact: 0.3},
			},
			HistoricalHitRate: 0.60,
			SourceReferences:  []string{"WSTS Semiconductor Market Forecast", "Taiwan Ministry of Economic Affairs Export Statistics"},
			Rationale: `半導體是台灣經濟的核心引擎，佔出口比重超過 35%。當電子零組件出口連續下滑時，意味著全球科技需求正在放緩，半導體庫存開始堆積，產能利用率下降。

這對台股的影響：
1. 營收與獲利下修：科技股本益比是建立在成長預期上，一旦成長放緩，估值會被大幅下調
2. 庫存減記：半導體庫存價值快速貶值，影響獲利
3. 資本支出縮減：設備與材料訂單減少，影響上游供應鏈

投資配置：
• 【押注】高股息、內需、金融：在週期下行時提供防禦
• 【迴避】半導體、AI供應鏈、PCB：庫存去化期通常持續 2-4 個季度，期間股價承壓`,
		},
		{
			ID:             "散戶機構分歧",
			Name:           "散戶機構分歧",
			TriggerTheme:   "retail_institutional_divergence",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "融資餘額 Z-Score > 1.5（散戶過度樂觀）", Affected: []string{"散戶情緒", "融資餘額"}, Impact: 0.7},
				{Description: "外公與投信同步賣超（機構偏空）", Affected: []string{"外公流向", "投信流向"}, Impact: -0.6},
				{Description: "市場流動性邊際惡化 → 軋空風險升高", Affected: []string{"台股大盤", "流動性"}, Impact: -0.5},
				{Description: "高Beta股和主題型中小股面臨短期回調壓力", Affected: []string{"中小型股", "主題股"}, Impact: -0.4},
			},
			HistoricalHitRate: 0.60,
			SourceReferences:  []string{"TWSE Margin Statistics", "Taiwan Financial Supervisory Commission"},
			Rationale: `當散戶（透過融資餘額衡量）和機構投資人（外資+投信）的方向出現明顯分歧時，市場往往處於過度杠桿或過度樂觀的狀態。融資餘額創新高代表散戶信心飽滿，但若同期機構投資人卻在賣超，這種「群眾 vs 專業」的分歧是經典的逆向訊號。

歷史規律：
• 散戶融資餘額 Z-Score > 1.5 且外资持續賣超 → 未來 1 個月高Beta股回調概率超過 60%
• 軋空行情往往在散戶過度樂觀後突然反轉

投資配置：
• 【迴避】高Beta、主題型中小股：杠桿化程度最高，回調時殺最慘
• 【押注】高股息、金融：較不受散戶情緒邊際影響`,
		},
		{
			ID:             "黃金避險行情",
			Name:           "黃金避險行情",
			TriggerTheme:   "gold_rally",
			RequiredRegion: "Global",
			Steps: []CausalStep{
				{Description: "黃金價格突破關鍵閾值 → 避險需求急升", Affected: []string{"黃金", "VIX", "避險資產"}, Impact: 0.8},
				{Description: "風險資產資金流向避險資產 → 貴金屬相對強勢", Affected: []string{"貴金屬", "黃金ETF"}, Impact: 0.7},
				{Description: "新興市場貨幣承壓 → 台幣貶值，外資流出台股", Affected: []string{"台股大盤", "新興市場"}, Impact: -0.5},
				{Description: "貴金屬板塊受惠於避險需求與通膨預期", Affected: []string{"貴金屬ETF", "原物料"}, Impact: 0.6},
			},
			HistoricalHitRate: 0.65,
			SourceReferences:  []string{"World Gold Council", "BIS Gold and FX Reserves"},
			Rationale: `黃金作為全球最老牌的避險資產，在市場恐慌、地緣政治緊張或通膨預期升溫時，往往出現急升行情。這種「flight to safety」的資金流動對台股有兩層影響：
1. 外資為降低新興市場曝險，會從台股抽離資金
2. 貴金屬相關標的（如黃金ETF 00635U）反而受惠

配置建議：
• 【押注】貴金屬ETF、原物料板塊：避險資金直接受益
• 【迴避】高Beta科技股：外資撤離首當其衝`,
		},
		{
			ID:             "美元強勢",
			Name:           "美元強勢",
			TriggerTheme:   "dollar_surge",
			RequiredRegion: "US",
			Steps: []CausalStep{
				{Description: "DXY 美元指數急升突破門檻 → 美元走強", Affected: []string{"DXY", "美元"}, Impact: 0.7},
				{Description: "美元走強壓抑新興市場貨幣 → 台幣貶值壓力", Affected: []string{"台幣", "USD/TWD"}, Impact: -0.6},
				{Description: "台幣貶值引發外資回流美元資產 → 台股資金流出", Affected: []string{"外資流向", "台股大盤"}, Impact: -0.6},
				{Description: "出口導向企業受惠台幣貶值 → 但資金流出抵消利多", Affected: []string{"出口股", "電子股"}, Impact: 0.2},
			},
			HistoricalHitRate: 0.70,
			SourceReferences:  []string{"Federal Reserve DXY Index", "BIS Triennial Survey"},
			Rationale: `美元走強（DXY飆升）是全球資金流動最重要的訊號之一。當美元急升時：
1. 新興市場貨幣全面承壓，台幣跟貶
2. 外資為鎖定美元匯兌收益，會加速從台股撤離
3. 貴金屬因美元定價關係，短期承壓（美元強 = 黃金弱）

配置建議：
• 【迴避】貴金屬ETF：美元強勢直接打壓金價
• 【押注】現金/美元部位：提高現金比例等待回調`,
		},
		{
			ID:             "通膨升溫",
			Name:           "通膨升溫",
			TriggerTheme:   "inflation_spike",
			RequiredRegion: "US",
			Steps: []CausalStep{
				{Description: "VIX 急升 + DXY 走強 → 通膨預期重新定價", Affected: []string{"通膨預期", "實質利率"}, Impact: 0.7},
				{Description: "通膨升溫侵蝕企業利潤率 → 本益比壓縮", Affected: []string{"台股大盤", "高估值股"}, Impact: -0.6},
				{Description: "實質資產（貴金屬）受惠通膨避險需求", Affected: []string{"貴金屬", "黃金"}, Impact: 0.6},
				{Description: "央行可能被迫升息 → 成長股估值進一步承壓", Affected: []string{"AI供應鏈", "中小型股"}, Impact: -0.5},
			},
			HistoricalHitRate: 0.62,
			SourceReferences:  []string{"Caldara-Iacoviello GPR Index", "Federal Reserve Economic Data (FRED)"},
			Rationale: `通膨預期升溫是資產配置最重要的宏觀訊號之一。當市場開始重新定價通膨風險時：
1. 實質利率下降 → 貴金屬（黃金）吸引力上升
2. 企業成本端壓力 → 利潤率受壓縮，尤其對低毛利產業
3. 央行可能進入升息循環 → 成長股估值面臨下修

配置建議：
• 【押注】貴金屬ETF：通膨避險 + 實質利率下降雙重利多
• 【迴避】高估值成長股：對利率最敏感，首當其衝`,
		},
		{
			ID:             "財報驚喜",
			Name:           "財報驚喜",
			TriggerTheme:   "earnings_surprise",
			RequiredRegion: "TW",
			Steps: []CausalStep{
				{Description: "台積電或重點權值股財報大幅優於預期", Affected: []string{"半導體", "AI供應鏈"}, Impact: 0.7},
				{Description: "外資重新上調台股目標價，資金流入", Affected: []string{"外資流向_台股", "台股大盤"}, Impact: 0.6},
				{Description: "科技股與高Beta板塊短期受惠", Affected: []string{"AI供應鏈", "半導體", "中小型股"}, Impact: 0.5},
			},
			HistoricalHitRate: 0.60,
			SourceReferences:  []string{"Ball & Brown (1968) JAR", "Taiwan Stock Exchange Earnings Announcement Studies"},
			Rationale: `財報驚喜是短線最強的事件驅動因子之一。當台積電或其他重點權值股公布的營收或獲利大幅超越市場預期時，會引發兩層連鎖反應：第一，分析師集體上調目標價與獲利預估，形成「預期修正循環」；第二，外資因看到基本面支撐而重新流入台股，推升大盤與高Beta個股。

配置建議：
• 【押注】AI供應鏈、半導體、中小型股：財報優於預期時，這些板塊的Beta最高、彈性最大
• 【迴避】防禦型板塊（金融、高股息）：在樂觀情緒主導的市場中，防禦型資產的相對報酬落後`,
		},
	}
}
