package sectormap

import "slices"

// TWSESIndustryCode is one key of the TWSE/TPEx listed-company industry-code
// vocabulary: the `產業別` field of TWSE OpenAPI `opendata/t187ap03_L` (上市) and
// the `SecuritiesIndustryCode` field of TPEx OpenAPI `mopsfin_t187ap03_O` (上櫃).
//
// Upstream publishes the 2-digit code only. The Chinese name comes from the TWSE
// ISIN classifier legend (https://isin.twse.com.tw/isin/class_i.jsp?kind=1),
// which is the same coding table those fields use — cross-checked on 2026-09-24
// against known listings (2330 台積電 → 24 半導體業, 2603 長榮 → 15 航運業,
// 2912 統一超 → 18 貿易百貨業, 2059 川湖 → 28 電子零組件業).
//
// Why it lives here: this is the per-symbol industry vocabulary that lets an
// ETF's published holdings become a canonical L1 exposure vector without anyone
// guessing an industry. It is declared and reported like every other vocabulary
// in this package (issue #1943 rule: no fuzzy matching, no implicit aliases).
type TWSESIndustryCode struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Target string `json:"target,omitempty"`
	// Candidates are canonical L1 IDs an operator may choose from when the code
	// has no defensible single target. They are documentation, never applied
	// automatically.
	Candidates []string `json:"candidates,omitempty"`
	Reason     string   `json:"reason"`
}

// twseIndustryCodes is the authoritative list, ordered by upstream code.
var twseIndustryCodes = []TWSESIndustryCode{
	{Code: "01", Name: "水泥工業", Target: "cement", Reason: "TWSE/TPEx 產業別 01（水泥工業）；canonical L1 cement 為唯一對應"},
	{Code: "02", Name: "食品工業", Target: "food", Reason: "TWSE/TPEx 產業別 02（食品工業）；canonical L1 food 為唯一對應"},
	{Code: "03", Name: "塑膠工業", Target: "plastics", Reason: "TWSE/TPEx 產業別 03（塑膠工業）；canonical L1 plastics 為唯一對應"},
	{Code: "04", Name: "紡織纖維", Target: "textiles", Reason: "TWSE/TPEx 產業別 04（紡織纖維）；canonical L1 textiles 為唯一對應"},
	{Code: "05", Name: "電機機械", Target: "machinery", Reason: "TWSE/TPEx 產業別 05（電機機械）；canonical L1 machinery 為唯一對應"},
	{Code: "06", Name: "電器電纜", Target: "machinery", Reason: "TWSE/TPEx 產業別 06（電器電纜）；與 TWSE 指數名表同規則，電線電纜併入 canonical L1 machinery"},
	{Code: "08", Name: "玻璃陶瓷", Candidates: []string{"cement", "chemicals"}, Reason: "TWSE/TPEx 產業別 08（玻璃陶瓷）；canonical taxonomy 無玻璃／陶瓷節點"},
	{Code: "09", Name: "造紙工業", Candidates: []string{"plastics", "chemicals"}, Reason: "TWSE/TPEx 產業別 09（造紙工業）；canonical taxonomy 無造紙節點"},
	{Code: "10", Name: "鋼鐵工業", Target: "steel", Reason: "TWSE/TPEx 產業別 10（鋼鐵工業）；canonical L1 steel 為唯一對應"},
	{Code: "11", Name: "橡膠工業", Candidates: []string{"plastics"}, Reason: "TWSE/TPEx 產業別 11（橡膠工業）；canonical taxonomy 無橡膠節點，最近者為 plastics"},
	{Code: "12", Name: "汽車工業", Target: "auto", Reason: "TWSE/TPEx 產業別 12（汽車工業）；canonical L1 auto 為唯一對應"},
	{Code: "13", Name: "電子工業", Candidates: []string{"electronics", "semiconductor", "other_electronics", "optoelectronics"}, Reason: "TWSE/TPEx 產業別 13（電子工業）；legacy 聚合碼，已被 24-31 細分，對映任一者都會重複計算"},
	{Code: "14", Name: "建材營造業", Target: "construction", Reason: "TWSE/TPEx 產業別 14（建材營造業）；canonical L1 construction 為唯一對應"},
	{Code: "15", Name: "航運業", Target: "shipping", Reason: "TWSE/TPEx 產業別 15（航運業）；canonical L1 shipping 為唯一對應"},
	{Code: "16", Name: "觀光餐旅", Target: "tourism", Reason: "TWSE/TPEx 產業別 16（觀光餐旅）；canonical L1 tourism 為唯一對應"},
	{Code: "17", Name: "金融保險業", Target: "financials", Reason: "TWSE/TPEx 產業別 17（金融保險業）；canonical L1 financials 為唯一對應"},
	{Code: "18", Name: "貿易百貨業", Target: "retail", Reason: "TWSE/TPEx 產業別 18（貿易百貨業）；canonical L1 retail 為唯一對應"},
	{Code: "19", Name: "綜合", Reason: "TWSE/TPEx 產業別 19（綜合）；殘差桶（綜合企業），canonical taxonomy 無對應節點"},
	{Code: "20", Name: "其他業", Reason: "TWSE/TPEx 產業別 20（其他業）；殘差桶，依建構即無 canonical L1 對應"},
	{Code: "21", Name: "化學工業", Target: "chemicals", Reason: "TWSE/TPEx 產業別 21（化學工業）；canonical L1 chemicals 為唯一對應"},
	{Code: "22", Name: "生技醫療業", Target: "biotech", Reason: "TWSE/TPEx 產業別 22（生技醫療業）；canonical L1 biotech 為唯一對應"},
	{Code: "23", Name: "油電燃氣業", Target: "energy", Reason: "TWSE/TPEx 產業別 23（油電燃氣業）；canonical L1 energy 為唯一對應"},
	{Code: "24", Name: "半導體業", Target: "semiconductor", Reason: "TWSE/TPEx 產業別 24（半導體業）；canonical L1 semiconductor 為唯一對應"},
	{Code: "25", Name: "電腦及週邊設備業", Target: "electronics", Reason: "TWSE/TPEx 產業別 25（電腦及週邊設備業）；與 TWSE 指數名表同規則，TWSE 無獨立電腦週邊 L1，canonical electronics 為唯一歸屬"},
	{Code: "26", Name: "光電業", Target: "optoelectronics", Reason: "TWSE/TPEx 產業別 26（光電業）；canonical L1 optoelectronics 為唯一對應"},
	{Code: "27", Name: "通信網路業", Target: "telecom", Reason: "TWSE/TPEx 產業別 27（通信網路業）；canonical L1 telecom 為唯一對應"},
	{Code: "28", Name: "電子零組件業", Target: "electronics", Reason: "TWSE/TPEx 產業別 28（電子零組件業）；與 TWSE 指數名表同規則，canonical electronics 為唯一歸屬"},
	{Code: "29", Name: "電子通路業", Candidates: []string{"electronics", "retail"}, Reason: "TWSE/TPEx 產業別 29（電子通路業）；通路商非製造業，canonical taxonomy 無對應節點"},
	{Code: "30", Name: "資訊服務業", Candidates: []string{"other_electronics", "telecom"}, Reason: "TWSE/TPEx 產業別 30（資訊服務業）；canonical taxonomy 無 IT 服務節點"},
	{Code: "31", Name: "其他電子業", Target: "other_electronics", Reason: "TWSE/TPEx 產業別 31（其他電子業）；canonical L1 other_electronics 為唯一對應"},
	{Code: "32", Name: "文化創意業", Candidates: []string{"retail"}, Reason: "TWSE/TPEx 產業別 32（文化創意業）；canonical taxonomy 無文創節點"},
	{Code: "33", Name: "農業科技業", Candidates: []string{"food"}, Reason: "TWSE/TPEx 產業別 33（農業科技業）；canonical taxonomy 無農技節點，最近者為 food"},
	{Code: "35", Name: "綠能環保", Candidates: []string{"energy"}, Reason: "TWSE/TPEx 產業別 35（綠能環保）；canonical taxonomy 無綠能節點，最近者為 energy"},
	{Code: "36", Name: "數位雲端", Candidates: []string{"other_electronics"}, Reason: "TWSE/TPEx 產業別 36（數位雲端）；canonical taxonomy 無數位雲端節點"},
	{Code: "37", Name: "運動休閒", Candidates: []string{"retail", "textiles", "auto"}, Reason: "TWSE/TPEx 產業別 37（運動休閒）；消費休閒桶，非單一 equity 產業"},
	{Code: "38", Name: "居家生活", Candidates: []string{"retail"}, Reason: "TWSE/TPEx 產業別 38（居家生活）；消費桶，非單一 equity 產業"},
}

// twseIndustryCodeKeys adapts the list above to the namespace table format. A
// code with no canonical target is declared unmapped *with* its candidates, so
// callers must report it instead of picking a sector on their own.
var twseIndustryCodeKeys = func() map[string]decl {
	out := make(map[string]decl, len(twseIndustryCodes))
	for _, c := range twseIndustryCodes {
		if c.Target != "" {
			out[c.Code] = one(c.Target, c.Reason)
			continue
		}
		out[c.Code] = unmapped(c.Reason, c.Candidates...)
	}
	return out
}()

// TWSESIndustryCodes returns the declared vocabulary as a deep copy, ordered by
// upstream code. Callers may mutate the result.
func TWSESIndustryCodes() []TWSESIndustryCode {
	out := make([]TWSESIndustryCode, len(twseIndustryCodes))
	for i, c := range twseIndustryCodes {
		out[i] = c
		out[i].Candidates = slices.Clone(c.Candidates)
	}
	return out
}

// TWSESIndustryCodeName returns the Chinese industry name for a 2-digit TWSE
// industry code, and whether the code is declared at all.
func TWSESIndustryCodeName(code string) (string, bool) {
	for _, c := range twseIndustryCodes {
		if c.Code == code {
			return c.Name, true
		}
	}
	return "", false
}

// TWSESIndustryCodeL1 resolves a 2-digit TWSE industry code to its canonical L1
// sector. ok=false means "no defensible single L1 target" (residual buckets,
// compound/legacy codes, or a code outside the declared vocabulary) — callers
// must report that rather than defaulting to a sector.
func TWSESIndustryCodeL1(code string) (string, bool) {
	return ResolveL1(NamespaceTWSESIndustryCode, code)
}
