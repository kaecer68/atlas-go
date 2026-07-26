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
// 資金流共鳴偏置：當 FlowResonanceProvider 被設定時，
// 策略方向與主流資金方向一致的策略獲得小幅加分（+2 分），
// 方向相反則不扣分（避免過度懲罰逆勢策略）。
//
// Maturity: experimental
package strategy_ranker

import "sort"

// FlowResonanceProvider 提供當前資金流方向與信心度。
// 當策略排名器有此 provider 時，策略方向與資金流共鳴的會獲得偏置加分。
type FlowResonanceProvider interface {
	// GetResonanceDirection 回傳當前主流資金方向（"up"/"down"/""）
	// 與信心度 [0,1]。空字串表示無明確方向。
	GetResonanceDirection() (direction string, confidence float64)
}

// Ranker 為策略排名的統一入口。
type Ranker struct {
	flowProvider FlowResonanceProvider
}

// New 建立一個 Ranker 實例。
func New() *Ranker {
	return &Ranker{}
}

// WithFlowProvider 注入資金流方向提供者（選填）。
// 當設定時，Rank 會對方向與資金流共鳴一致的策略給予偏置加分。
func (r *Ranker) WithFlowProvider(fp FlowResonanceProvider) *Ranker {
	r.flowProvider = fp
	return r
}

// RankAndTier 對一組策略報告進行排名並賦予付費分層標籤。
func (r *Ranker) RankAndTier(reports []*StrategyReport) []RankedReport {
	ranked := r.RankWithFlow(reports)
	AssignTiers(ranked)
	return ranked
}

// RankWithFlow 對策略報告排名，若設有 flowProvider 則應用資金流偏置。
func (r *Ranker) RankWithFlow(reports []*StrategyReport) []RankedReport {
	// 獲取資金流方向（若有 provider）
	var flowDir string
	var flowConf float64
	if r.flowProvider != nil {
		flowDir, flowConf = r.flowProvider.GetResonanceDirection()
	}

	ranked := Rank(reports)

	// 應用資金流共鳴偏置：方向一致的策略 +2 分（僅在信心度 >0.3 時）
	if flowDir != "" && flowConf > 0.3 {
		for i := range ranked {
			if ranked[i].Direction == flowDir {
				ranked[i].Score += 2.0
			}
		}
		// 偏置後重新排序
		sortByScore(ranked)
		for i := range ranked {
			ranked[i].Rank = i + 1
		}
	}

	return ranked
}

// FreeReports 過濾並回傳免費層的策略報告。
func (r *Ranker) FreeReports(ranked []RankedReport) []RankedReport {
	return filterByTier(ranked, "free")
}

// sortByScore 按 Score 降冪排序（並列時保持原順序）。
func sortByScore(ranked []RankedReport) {
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
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
