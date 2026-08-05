//go:build livefinmind

package industry

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// liveStockInfoFetcher 包 *marketdata.FinMindClient 暴露真實 TaiwanStockInfo 資料。
// 只在 `//go:build livefinmind` build tag 下編譯 — 平常 go test ./internal/industry/
// 不會跑這個檔,避免消耗 daily quota、依賴外部網路、或需要外部 token。
//
// 跑法:
//
//	FINMIND_API_KEY=<key> go test -tags livefinmind -run TestSymbolCoverage_LiveProduction \
//	  -v ./internal/industry/
//
// 輸出會印 JSON CoverageReport 到 stdout,方便 ./output/ > snapshots/ 路徑留底。
type liveStockInfoFetcher struct {
	client *marketdata.FinMindClient
}

func (f *liveStockInfoFetcher) GetStockInfo(ctx context.Context) ([]marketdata.StockInfo, error) {
	return f.client.GetStockInfo(ctx)
}

// TestSymbolCoverage_LiveProduction 對 production FinMind TaiwanStockInfo 收錄清單
// 跟 DefaultClassification() 的 representative stocks 做差集,吐完 JSON CoverageReport。
//
// 預期用途: 跑完看哪些 industry 的哪些 stock 在 FinMind 沒收到資料 → 從 defaults_narrative.go
// 拿掉,避免 auto_cycle_update 每次都 fail 該 industry。
//
// 注意: 這 test 會消耗一個 FinMind API call (TaiwanStockInfo dataset, 通常 1 request 內回傳)。
// 不算昂貴(整個市場的 stock info 一次回傳),但 daily quota 600/hr 仍要注意。
func TestSymbolCoverage_LiveProduction(t *testing.T) {
	apiKey := os.Getenv("FINMIND_API_KEY")
	if apiKey == "" {
		t.Skip("FINMIND_API_KEY not set; skipping live production coverage test")
	}

	// stateDir 指向 ./data/state,跟 GetSharedFinMindClient 預設一致。
	// 跑完會寫 DailyQuotaTracker 進度,跟 atlas-go container 共享(或衝突,如果同一機跑)。
	// 但 quota tracker 是 process-isolated state,host 跟 docker 各跑各的。
	client := marketdata.NewFinMindClient(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tree := DefaultClassification()
	if tree == nil {
		t.Fatal("DefaultClassification() returned nil; cannot run")
	}

	report, err := ValidateFinMindSymbolCoverage(ctx, &liveStockInfoFetcher{client: client}, tree)
	if err != nil {
		t.Fatalf("coverage validation failed: %v", err)
	}

	// 印 JSON 到 stdout,方便 pipe 到檔案留底。
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		t.Fatalf("encode report: %v", err)
	}

	// 額外印一份「missing symbols 依 industry 排序」清單,方便給 defaults_narrative.go 修。
	// 這部分是給 kaecer 直接對照「哪些 stock 該刪掉」的 actionable 輸出,JSON 報告是給 CI 留底。
	t.Log("=== missing stocks by industry (sorted) ===")
	industryIDs := make([]string, 0, len(report.Industries))
	for id := range report.Industries {
		industryIDs = append(industryIDs, id)
	}
	sort.Strings(industryIDs)
	for _, id := range industryIDs {
		cover := report.Industries[id]
		if len(cover.MissingStocks) == 0 {
			continue
		}
		// sort 缺的 stocks 讓輸出 deterministic
		sortedMissing := append([]string{}, cover.MissingStocks...)
		sort.Strings(sortedMissing)
		t.Logf("industry=%s total=%d covered=%d missing=%d ratio=%.2f missing_symbols=%v",
			id, cover.TotalStocks, len(cover.CoveredStocks), len(cover.MissingStocks),
			cover.CoverageRatio, sortedMissing)
	}
}
