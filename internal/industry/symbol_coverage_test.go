package industry

import (
	"context"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// stubStockInfoFetcher 實作 StockInfoFetcher interface,回傳固定 symbol set。
// 用純 stub 而非真實 *marketdata.FinMindClient 的原因:
//  1. production token 在 docker secrets,本機測試拿不到
//  2. 不想在 test 跑時打真實 FinMind API(會消耗 daily quota、依賴外部網路)
//  3. 改 FinMindClient 為 interface 風險過大 — 有 30+ caller
type stubStockInfoFetcher struct {
	symbols map[string]bool
	err     error // 注入時可以設此欄位模擬 fetcher 失敗
}

func (s *stubStockInfoFetcher) GetStockInfo(ctx context.Context) ([]marketdata.StockInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]marketdata.StockInfo, 0, len(s.symbols))
	for sym := range s.symbols {
		out = append(out, marketdata.StockInfo{StockID: sym})
	}
	return out, nil
}

// TestValidateFinMindSymbolCoverage_AllCovered 用全 symbol 都被 FinMind 收錄的 happy path
// 驗證 CoveredStocks == 所有 stocks,MissingStocks == 空,CoverageRatio == 1.0。
func TestValidateFinMindSymbolCoverage_AllCovered(t *testing.T) {
	tree := buildSimpleCoverageTree([]*IndustrySegment{
		{ID: "semiconductor", Level: Level1, RepresentativeStocks: []string{"2330.TW", "2303.TW"}},
		{ID: "shipping", Level: Level1, RepresentativeStocks: []string{"2603.TW"}},
	})

	fetcher := &stubStockInfoFetcher{symbols: map[string]bool{
		"2330": true, "2303": true, "2603": true, "9999": true, // 9999 在清單但沒人用
	}}

	report, err := ValidateFinMindSymbolCoverage(context.Background(), fetcher, tree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.FinMindSize != 4 {
		t.Errorf("FinMindSize: got %d, want 4", report.FinMindSize)
	}
	for _, ind := range []string{"semiconductor", "shipping"} {
		c, ok := report.Industries[ind]
		if !ok {
			t.Errorf("missing industry %q in report", ind)
			continue
		}
		if c.CoverageRatio != 1.0 {
			t.Errorf("%s: CoverageRatio got %v, want 1.0", ind, c.CoverageRatio)
		}
		if len(c.MissingStocks) != 0 {
			t.Errorf("%s: MissingStocks should be empty, got %v", ind, c.MissingStocks)
		}
	}
}

// TestValidateFinMindSymbolCoverage_PartialMissing 模擬 production 觀察到的場景:
// 某些 industry 全部 missing,有些全部 covered。驗證 CoverageReport 正確分類。
func TestValidateFinMindSymbolCoverage_PartialMissing(t *testing.T) {
	tree := buildSimpleCoverageTree([]*IndustrySegment{
		{ID: "semiconductor", Level: Level1, RepresentativeStocks: []string{"2330.TW", "2303.TW"}},
		// 模擬 3426.TW 不在 FinMind 收錄(對應 2026-08-05 觀察到的 leo_satellite 失敗)
		{ID: "leo_satellite", Level: Level1, RepresentativeStocks: []string{"6271.TW", "3426.TW"}},
	})

	fetcher := &stubStockInfoFetcher{symbols: map[string]bool{
		"2330": true, "2303": true, // semiconductor 全部 covered
		"6271": true, // 6271 在,3426 不在
	}}

	report, err := ValidateFinMindSymbolCoverage(context.Background(), fetcher, tree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	semi := report.Industries["semiconductor"]
	if semi.CoverageRatio != 1.0 || len(semi.MissingStocks) != 0 {
		t.Errorf("semiconductor should be fully covered, got ratio=%v missing=%v",
			semi.CoverageRatio, semi.MissingStocks)
	}

	leo := report.Industries["leo_satellite"]
	if leo.CoverageRatio != 0.5 {
		t.Errorf("leo_satellite CoverageRatio: got %v, want 0.5", leo.CoverageRatio)
	}
	if len(leo.MissingStocks) != 1 || leo.MissingStocks[0] != "3426.TW" {
		t.Errorf("leo_satellite MissingStocks: got %v, want [3426.TW]", leo.MissingStocks)
	}
	if !report.MissingAnyIndustry() {
		t.Error("MissingAnyIndustry should return true when at least one industry has missing stocks")
	}
}

// TestValidateFinMindSymbolCoverage_EmptyStocks 驗證 industry 沒有 representative stocks
// (例如 etf_rotation) 不會被誤判為「missing」,CoverageRatio 為 1.0。
func TestValidateFinMindSymbolCoverage_EmptyStocks(t *testing.T) {
	tree := buildSimpleCoverageTree([]*IndustrySegment{
		{ID: "etf_rotation", Level: Level1, RepresentativeStocks: []string{}},
	})
	fetcher := &stubStockInfoFetcher{symbols: map[string]bool{"2330": true}}

	report, err := ValidateFinMindSymbolCoverage(context.Background(), fetcher, tree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cover := report.Industries["etf_rotation"]
	if cover.CoverageRatio != 1.0 {
		t.Errorf("empty stocks CoverageRatio should be 1.0, got %v", cover.CoverageRatio)
	}
	if cover.TotalStocks != 0 || len(cover.MissingStocks) != 0 {
		t.Errorf("empty stocks should have 0 total / 0 missing, got total=%d missing=%v",
			cover.TotalStocks, cover.MissingStocks)
	}
}

// TestValidateFinMindSymbolCoverage_NormalizesSymbolSuffix 驗證 normalizeSymbol 路徑:
// tree 內 "2317.TW" 對應 fetcher 內 "2317" (純 4-digit)。沒有這層處理會全部 missing。
func TestValidateFinMindSymbolCoverage_NormalizesSymbolSuffix(t *testing.T) {
	tree := buildSimpleCoverageTree([]*IndustrySegment{
		{ID: "semiconductor", Level: Level1, RepresentativeStocks: []string{"2317.TW"}},
	})
	fetcher := &stubStockInfoFetcher{symbols: map[string]bool{"2317": true}}

	report, err := ValidateFinMindSymbolCoverage(context.Background(), fetcher, tree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := report.Industries["semiconductor"]
	if c.CoverageRatio != 1.0 {
		t.Errorf("expected 1.0 after .TW suffix normalization, got %v missing=%v",
			c.CoverageRatio, c.MissingStocks)
	}
}

// TestValidateFinMindSymbolCoverage_FetcherErrorPropagates 驗證 fetcher error 走 wrap。
// 當 FinMind API 失敗時,CLI 工具要能看到「symbol_coverage: fetch TaiwanStockInfo: ...」
// 才能跟其他上游錯誤區分開。
func TestValidateFinMindSymbolCoverage_FetcherErrorPropagates(t *testing.T) {
	tree := buildSimpleCoverageTree([]*IndustrySegment{{ID: "semiconductor", Level: Level1}})
	fetcher := &stubStockInfoFetcher{err: errors.New("finmind: status 500")}

	_, err := ValidateFinMindSymbolCoverage(context.Background(), fetcher, tree)
	if err == nil {
		t.Fatal("expected error from fetcher, got nil")
	}
	// 確認 wrap chain 有「symbol_coverage:」前綴,方便 log 過濾
	const want = "symbol_coverage: fetch TaiwanStockInfo"
	if got := err.Error(); len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("error message missing wrap prefix: got %q, want prefix %q", got, want)
	}
}

// TestValidateFinMindSymbolCoverage_NilInputsReturnError 驗證 nil fetcher / nil tree 走 error path。
func TestValidateFinMindSymbolCoverage_NilInputsReturnError(t *testing.T) {
	fetcher := &stubStockInfoFetcher{symbols: map[string]bool{"x": true}}
	tree := buildSimpleCoverageTree([]*IndustrySegment{{ID: "x", Level: Level1}})

	if _, err := ValidateFinMindSymbolCoverage(context.Background(), nil, tree); err == nil {
		t.Error("expected error for nil fetcher, got nil")
	}
	if _, err := ValidateFinMindSymbolCoverage(context.Background(), fetcher, nil); err == nil {
		t.Error("expected error for nil tree, got nil")
	}
}

// TestMissingAnyIndustry_NilSafe 驗證 nil report 走 false(不 panic)。
// 這對 CLI 工具在尚未跑出 report 時直接呼叫是合理的行為。
func TestMissingAnyIndustry_NilSafe(t *testing.T) {
	var r *CoverageReport
	if r.MissingAnyIndustry() {
		t.Error("nil report should return false, got true")
	}
}

// buildSimpleCoverageTree 從 segments slice 建一個 ClassificationTree。
// 跳過 DefaultClassification(),因為 production 那份帶 14 industries 對單元測試太雜。
// 我們只測 validator 邏輯,不需要完整的預設資料。
func buildSimpleCoverageTree(segs []*IndustrySegment) *ClassificationTree {
	tree := NewClassificationTree()
	for _, s := range segs {
		tree.AddSegment(s)
	}
	return tree
}
