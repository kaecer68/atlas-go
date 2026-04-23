package narrative

// NLG template constants for generating Chinese narrative reports.
// Each template uses Go text/template-compatible format verbs (%s, %.2f, etc.)
// and is designed to be filled with runtime data at report generation time.

const (
	// TemplateMarketOverview generates the market overview section.
	// Placeholders: %s (date), %s (regime), %s (risk level)
	TemplateMarketOverview = `【市場概況 — %s】

今日市場處於 %s 狀態，整體風險等級為 %s。

系統已偵測到多項巨觀敘事事件，並根據因果鏈模板推導對台股各板塊的潛在影響。以下為今日重點摘要與投資建議。`

	// TemplateNarrativeSummary generates a summary of detected narrative events.
	// Placeholders: %s (event theme), %.2f (confidence), %.2f (hit rate), %s (rationale excerpt)
	TemplateNarrativeSummary = `【巨觀事件摘要】

• 事件：%s
  信心度：%.2f | 歷史命中率：%.2f
  分析：%s`

	// TemplateTopPickIntro generates the introduction for top pick recommendations.
	// Placeholders: %s (symbol), %s (side), %d (conviction), %s (agent)
	TemplateTopPickIntro = `【重點推薦】

• %s（%s）— 信念值：%d | 來源：%s`

	// TemplateFactorExplanation generates a factor score explanation paragraph.
	// Placeholders: %s (symbol), %.2f (momentum), %.2f (value), %.2f (quality), %.2f (total)
	TemplateFactorExplanation = `【因子分析 — %s】

動能因子：%.2f | 價值因子：%.2f | 品質因子：%.2f
綜合因子分數：%.2f

各因子分數反映該標的在過去一段時間內的相對表現。動能因子衡量價格趨勢強度，價值因子評估估值合理性，品質因子則考量財務體質與獲利穩定性。`

	// TemplateRiskWarning generates a risk warning section.
	// Placeholders: %s (risk level), %.2f (VaR), %.2f (max drawdown)
	TemplateRiskWarning = `【風險警示】

當前風險等級：%s
在險價值（VaR 95%%）：%.2f%%
最大回撤：%.2f%%

請注意，高風險環境下應降低部位曝險，優先考慮防禦型配置。系統建議嚴格執行停損紀律，避免單一標的過度集中。`

	// TemplateControlDecision generates the control layer decision explanation.
	// Placeholders: %s (decision), %s (reason)
	TemplateControlDecision = `【控制層決策】

決策：%s
原因：%s

控制層（CRO/CIO）已根據風險預算與投資政策對推薦標的進行過濾。以上為最終執行決策。`
)
