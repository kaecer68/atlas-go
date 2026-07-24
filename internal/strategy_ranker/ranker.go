// Package strategy_ranker 提供策略排名的可呼叫介面，供策略推薦器使用。
//
// Ranker 封裝 Rank() 與 AssignTiers()，
// 對外提供統一的排名 + 分層入口。
//
// 預設分層規則：
//   - premium（付費）：排名 1~2（績效最佳的深度策略）
//   - registered（註冊用戶）：排名 3~4
//   - free（免費公開）：排名 5 以後（適合對外展示的策略）
//
// Maturity: experimental
package strategy_ranker

import ()

// Ranker 為策略排名的統一入口。
type Ranker struct{}

// New 建立一個 Ranker 實例。
func New() *Ranker {
	return &Ranker{}
}

// RankAndTier 對一組策略報告進行排名並賦予付費分層標籤。
//
// 回傳已排序的報告（第 1 名在前），各報告 .Tier 為 "premium" /
// "registered" / "free" 三者之一。
func (r *Ranker) RankAndTier(reports []*StrategyReport) []RankedReport {
	ranked := Rank(reports)
	AssignTiers(ranked)
	return ranked
}

// FreeReports 過濾並回傳免費層的策略報告。
func (r *Ranker) FreeReports(ranked []RankedReport) []RankedReport {
	return filterByTier(ranked, "free")
}

// PremiumReports 過濾並回傳付費層的策略報告。
func (r *Ranker) PremiumReports(ranked []RankedReport) []RankedReport {
	return filterByTier(ranked, "premium")
}

func filterByTier(ranked []RankedReport, tier string) []RankedReport {
	result := make([]RankedReport, 0)
	for _, r := range ranked {
		if r.Tier == tier {
			result = append(result, r)
		}
	}
	return result
}
