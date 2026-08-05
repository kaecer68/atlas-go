package industry

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// CoverageReport 描述 representative stocks 對 FinMind 收錄清單的覆蓋狀態。
//
// 用途：取代「auto_cycle_update 失敗時只看到 last_error 字串」的被動診斷,
// 主動告訴我們「symbol X 不在 FinMind 收錄清單裡」這類 upstream 限制。
// 詳見 docs/investigations/2026-08-05-auto-cycle-update-quota-misconception.md。
type CoverageReport struct {
	UpdatedAt   time.Time                `json:"updated_at"`
	FinMindSize int                      `json:"finmind_size"` // FinMind 收錄的 symbol 總數
	Industries  map[string]IndustryCover `json:"industries"`   // industryID → 該 industry 的覆蓋狀態
}

// IndustryCover 單一 industry 的 representative stock 覆蓋狀態。
type IndustryCover struct {
	IndustryID    string   `json:"industry_id"`
	TotalStocks   int      `json:"total_stocks"`   // representative stock 總數
	CoveredStocks []string `json:"covered_stocks"` // 在 FinMind 收錄清單內
	MissingStocks []string `json:"missing_stocks"` // 不在 FinMind 收錄清單內 (auto_cycle_update 會永遠失敗)
	CoverageRatio float64  `json:"coverage_ratio"` // CoveredStocks / TotalStocks
}

// MissingAnyIndustry 報告中是否任一 industry 有 missing stock。
// 用於在 CLI 輸出時快速判斷要不要印「需要修 default stocks」警告。
func (r *CoverageReport) MissingAnyIndustry() bool {
	if r == nil {
		return false
	}
	for _, c := range r.Industries {
		if len(c.MissingStocks) > 0 {
			return true
		}
	}
	return false
}

// StockInfoFetcher 是 *marketdata.FinMindClient.GetStockInfo 的最小介面。
// 拆成 interface 的目的：讓 unit test 可以注入 stub 而不需要真的打 FinMind API
// (也避免在 production code 上加 test-only hook)。
// production caller 傳 *marketdata.FinMindClient,test caller 傳 stub implementation。
type StockInfoFetcher interface {
	GetStockInfo(ctx context.Context) ([]marketdata.StockInfo, error)
}

// ValidateFinMindSymbolCoverage 對 FinMind TaiwanStockInfo 收錄清單跟 ClassificationTree
// representative stocks 做差集，回傳每個 industry 的覆蓋狀態。
//
// 實作細節：
//   - FinMind TaiwanStockInfo API 一次回傳全市場 symbols (~2k+)，用 map[string]bool 建索引 O(1) lookup
//   - symbol 比對時走既有 normalizeSymbol(symbol_l1_mapper.go)：剝掉 .TW 後綴 + TrimSpace +
//     case-insensitive，跟現有 L1 mapper 共用邏輯避免分叉
//   - fetcher 為 nil 時回 error；呼叫端負責 graceful skip
func ValidateFinMindSymbolCoverage(ctx context.Context, fetcher StockInfoFetcher, tree *ClassificationTree) (*CoverageReport, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("symbol_coverage: nil fetcher")
	}
	if tree == nil {
		return nil, fmt.Errorf("symbol_coverage: nil classification tree")
	}

	stockInfos, err := fetcher.GetStockInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("symbol_coverage: fetch TaiwanStockInfo: %w", err)
	}

	finmindSet := make(map[string]bool, len(stockInfos))
	for _, info := range stockInfos {
		if info.StockID != "" {
			finmindSet[info.StockID] = true
		}
	}

	report := &CoverageReport{
		UpdatedAt:   time.Now(),
		FinMindSize: len(finmindSet),
		Industries:  make(map[string]IndustryCover, 16),
	}

	for _, seg := range tree.GetLevel1() {
		cover := IndustryCover{IndustryID: seg.ID}
		if len(seg.RepresentativeStocks) == 0 {
			cover.CoverageRatio = 1.0 // 沒有 stocks 就不算 missing
			report.Industries[seg.ID] = cover
			continue
		}

		cover.TotalStocks = len(seg.RepresentativeStocks)
		for _, raw := range seg.RepresentativeStocks {
			sym := normalizeSymbol(raw)
			if finmindSet[sym] {
				cover.CoveredStocks = append(cover.CoveredStocks, raw)
			} else {
				cover.MissingStocks = append(cover.MissingStocks, raw)
			}
		}
		// 排序讓輸出穩定（diff-friendly、便於人眼比對）
		sort.Strings(cover.CoveredStocks)
		sort.Strings(cover.MissingStocks)
		cover.CoverageRatio = float64(len(cover.CoveredStocks)) / float64(cover.TotalStocks)
		report.Industries[seg.ID] = cover
	}

	return report, nil
}
