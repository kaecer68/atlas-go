//go:build integration

package main

// 本檔是 #2107 形狀在 quotes range 回補上的 PG 端到端證據，也是 FU-20260929-01
// （quotes 在 2026-06-25 之前只有 114 檔）修復路徑的落地驗證。
//
// 走**生產形狀**：環境宣告 ATLAS_STORE_BACKEND=postgres ＋ $DATABASE_URL、**不帶**
// -backend，經由真的 flag 解析（run(...)）與真的 wiring（defaultStoreDeps =
// atlasdb.Init ＋ ledger.SetPostgresPool）。任一處斷線都會紅：
//   - DSN 沒讀到 ⇒ resolveStore 回錯
//   - 池沒開／沒注入 ⇒ 建不出 PostgresQuoteStore（生產會說 "requires SetPostgresPool"）
//   - migrations 路徑錯（-workdir 沒接）⇒ atlasdb.Init 失敗
//   - 寫入沒真的進 PG ⇒ 用**另一條連線**查不到列
//   - 只補缺的交易日失效 ⇒ 第二次執行會再打上游、或列數改變
//
// 唯一的替身是 provider（newFetcher seam）：本測試不打 FinMind 網路，也不消耗配額。
//
// 資料紀律：只用明顯假造的 1996 交易日與假代碼（PGQR1996），並在 t.Cleanup 內用
// 自己的連線刪掉自己插入的列。日期刻意與其他 lane 的 integration fixture
// （1990／1994）互斥，避免平行執行時互踩。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	// itQuotesSymbol 是 bare 代碼：本測試同時驗證「裸代碼 → <code>.TW」的鍵正規化。
	itQuotesSymbol = "PGQR1996"
	// itQuotesKey 是 CLI 應該寫入的 quotes 鍵（= 生產 FinMind 路徑的形式）。
	itQuotesKey = itQuotesSymbol + ".TW"
	// repoRootFromCmd 是 repo 根（cmd/backfill-quotes-range → ../..）。
	repoRootFromCmd = "../.."
	// 1996-01-04(四) / 01-05(五) / 01-08(一) 是交易日；01-06 是週六（必須被丟棄）。
	itQuotesStart = "1996-01-04"
	itQuotesEnd   = "1996-01-08"
)

// cleanupPGQuotesRange 刪掉本檔自己插入的列（可重跑）。
func cleanupPGQuotesRange(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	del := func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM quotes WHERE symbol LIKE $1", itQuotesSymbol+"%")
	}
	del()
	t.Cleanup(del)
}

func TestRun_PostgresDeclaredWritesQuotesRangeAndResumes(t *testing.T) {
	dsn := testdb.URL(t)
	// 用既有 testdb seam 套 migrations（CI 缺 PG 硬失敗、本機缺 PG skip 的政策在 testdb）。
	testdb.Pool(t, filepath.Join(repoRootFromCmd, "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	cleanupPGQuotesRange(t, raw)

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("FINMIND_API_KEY", "integration-test-key")

	// 上游回 4 列，其中一列是週六（休市）——必須被丟棄。
	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{
		barsFor(itQuotesKey, "1996-01-04", "1996-01-05", "1996-01-06", "1996-01-08"),
	}}
	restore := swapFetcher(t, fetcher)

	var out bytes.Buffer
	err := run([]string{
		"-start", itQuotesStart, "-end", itQuotesEnd,
		"-symbols", itQuotesSymbol, "-workdir", repoRootFromCmd,
	}, &out)
	restore()
	if err != nil {
		t.Fatalf("run(...): %v\nstdout:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "backend=postgres") {
		t.Fatalf("stdout must prove the postgres backend was used (declared, no -backend):\n%s", out.String())
	}
	if strings.Contains(out.String(), "sqlite") {
		t.Fatalf("declared postgres must not touch sqlite:\n%s", out.String())
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("upstream calls = %v, want exactly 1 (range form: 每檔一次)", fetcher.calls)
	}
	if fetcher.calls[0] != fmt.Sprintf("%s %s..%s", itQuotesKey, itQuotesStart, itQuotesEnd) {
		t.Fatalf("call = %q, want the whole window in one request", fetcher.calls[0])
	}

	ctx := context.Background()
	var rows int
	if err := raw.QueryRow(ctx, "SELECT count(*) FROM quotes WHERE symbol = $1", itQuotesKey).Scan(&rows); err != nil {
		t.Fatalf("count quotes: %v", err)
	}
	if rows != 3 {
		t.Fatalf("quotes rows = %d, want 3 (週六的列不得寫入)", rows)
	}

	var closePrice float64
	var source, name string
	if err := raw.QueryRow(ctx,
		"SELECT close, source, COALESCE(name, '') FROM quotes WHERE symbol = $1 AND date = $2",
		itQuotesKey, "1996-01-04").Scan(&closePrice, &source, &name); err != nil {
		t.Fatalf("select 1996-01-04: %v", err)
	}
	if closePrice != 10.5 {
		t.Fatalf("close = %v, want 10.5 (fixture 第一列)", closePrice)
	}
	if source != quotesRangeSource {
		t.Fatalf("source = %q, want %q（稽核要能分辨 range 補的列）", source, quotesRangeSource)
	}

	// 第二次執行：已完整 ⇒ **不得**再呼叫上游，也不得改動列數。
	fetcher2 := &fakeRangeFetcher{errFor: map[string]error{
		itQuotesKey: errors.New("must not be called: symbol is already complete"),
	}}
	restore2 := swapFetcher(t, fetcher2)
	out.Reset()
	err = run([]string{
		"-start", itQuotesStart, "-end", itQuotesEnd,
		"-symbols", itQuotesSymbol, "-workdir", repoRootFromCmd,
	}, &out)
	restore2()
	if err != nil {
		t.Fatalf("second run: %v\nstdout:\n%s", err, out.String())
	}
	if len(fetcher2.calls) != 0 {
		t.Fatalf("second run called upstream %v, want none (續傳機制：已完整就跳過)", fetcher2.calls)
	}
	for _, want := range []string{"requests=0", "skipped_complete=1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("second run summary missing %q:\n%s", want, out.String())
		}
	}
	var after int
	if err := raw.QueryRow(ctx, "SELECT count(*) FROM quotes WHERE symbol = $1", itQuotesKey).Scan(&after); err != nil {
		t.Fatalf("count quotes after second run: %v", err)
	}
	if after != rows {
		t.Fatalf("row count changed on the second run: %d → %d", rows, after)
	}
}
