// Package industry provides granular industry classification, seasonality,
// cycle positioning, and supply-chain linkage for Taiwan stock market analysis.
package industry

import "slices"

// DefaultRepresentativeStocks returns a map of canonical SectorID constants
// to their representative stock symbols. All symbols are WITHOUT .TW suffix.
//
// This data serves as a **fallback** for symbol-to-industry classification when
// external data sources (e.g., FinMind) are unavailable. The primary source of
// truth for industry classification is ParametersConfig.Industry.ClassificationTree.
//
// Coverage: 20 sectors, ~96 representative stocks across Taiwan's major industries.
func DefaultRepresentativeStocks() map[SectorID][]string {
	return map[SectorID][]string{
		SectorSemiconductor: {
			"2330", // 台積電
			"2303", // 聯電
			"2454", // 聯發科
			"3034", // 聯詠
			"2379", // 瑞昱
			"3443", // 創意
			"3661", // 世芯-KY
			"2337", // 旺宏
			"2344", // 華邦電
			"8299", // 群聯
			"8081", // 致新
			"6239", // 力成
		},
		SectorElectronics: {
			"2317", // 鴻海
			"2382", // 廣達
			"2324", // 仁寶
			"2356", // 英業達
			"2357", // 華碩
			"2376", // 技嘉
			"2377", // 微星
			"2395", // 研華
			"2308", // 台達電
			"3037", // 欣興
		},
		SectorOptoelectronics: {
			"3008", // 大立光
			"3406", // 玉晶光
			"5484", // 慧友
			"2393", // 億光
			"6176", // 瑞儀
		},
		SectorFinancials: {
			"2881", // 富邦金
			"2882", // 國泰金
			"2886", // 兆豐金
			"2884", // 玉山金
			"2891", // 中信金
			"2885", // 元大金
			"2892", // 第一金
			"5880", // 合庫金
			"2883", // 開發金
			"2887", // 台新金
		},
		SectorCement: {
			"1101", // 台泥
			"1102", // 亞泥
			"1103", // 嘉泥
		},
		SectorPlastics: {
			"1301", // 台塑
			"1303", // 南亞
			"1326", // 台化
			"1304", // 台聚
		},
		SectorTextiles: {
			"1476", // 儒鴻
			"1402", // 遠東新
			"1477", // 聚陽
		},
		SectorSteel: {
			"2002", // 中鋼
			"2015", // 豐興
			"2027", // 大成鋼
			"2014", // 中鴻
			"2006", // 東和鋼鐵
		},
		SectorShipping: {
			"2603", // 長榮
			"2609", // 陽明
			"2615", // 萬海
			"2618", // 長榮航
			"2605", // 新興
			"2637", // 慧洋-KY
		},
		SectorFood: {
			"1216", // 統一
			"1227", // 佳格
			"1229", // 聯華
			"1231", // 聯華食
		},
		SectorAuto: {
			"2207", // 和泰車
			"2201", // 裕隆
			"2227", // 裕日車
		},
		SectorTelecom: {
			"2412", // 中華電
			"3045", // 台灣大
			"4904", // 遠傳
		},
		SectorChemicals: {
			"1707", // 葡萄王
			"1722", // 台肥
			"1717", // 長興
		},
		SectorBiotech: {
			"4746", // 台耀
			"1795", // 美時
			"4123", // 晟德
			"4162", // 智擎
			"4142", // 國光生
		},
		SectorConstruction: {
			"2505", // 國揚
			"2545", // 皇翔
			"2548", // 華固
			"5522", // 遠雄
		},
		SectorOtherElectronics: {
			"3533", // 嘉澤
			"3653", // 健策
			"2312", // 金寶
		},
		SectorMachinery: {
			"1503", // 士電
			"1504", // 東元
			"1590", // 亞德客-KY
			"2049", // 上銀
			"4536", // 拓凱
		},
		SectorTourism: {
			"2707", // 晶華
			"2723", // 美食-KY
			"2731", // 雄獅
		},
		SectorRetail: {
			"2912", // 統一超
			"2915", // 潤泰全
			"5904", // 寶雅
		},
		SectorEnergy: {
			"6505", // 台塑化
			"9933", // 中鼎
		},
	}
}

// ClassifyBySymbol returns the canonical SectorID for a given stock symbol by
// consulting DefaultRepresentativeStocks. Returns an empty SectorID if the
// symbol is not found.
//
// This is intended as a fallback for symbol-to-industry classification
// when external data sources (e.g., FinMind) are unavailable. The primary
// classification path is through ClassificationTree.GetSegment().
func ClassifyBySymbol(symbol string) SectorID {
	for id, stocks := range DefaultRepresentativeStocks() {
		if slices.Contains(stocks, symbol) {
			return id
		}
	}
	return ""
}
