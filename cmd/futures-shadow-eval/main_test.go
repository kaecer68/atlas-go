package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/futures"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

type fakeReader struct {
	bars     map[string][]domain.FuturesBar
	err      error
	requests int
}

func (f *fakeReader) LoadFuturesBars(_ context.Context, contract, _ string, _ domain.FuturesSession, _, _ time.Time) ([]domain.FuturesBar, error) {
	f.requests++
	if f.err != nil {
		return nil, f.err
	}
	return f.bars[contract], nil
}

func tDay(s string) time.Time {
	d, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return d
}

func tPtrF(v float64) *float64 { return &v }
func tPtrI(v int64) *int64     { return &v }

func tBar(month, date string, close float64, oi int64) domain.FuturesBar {
	return domain.FuturesBar{
		Contract: "TX", ContractMonth: month, TradeDate: tDay(date),
		Session: domain.SessionRegular, Close: tPtrF(close), OpenInterest: tPtrI(oi),
		Volume: tPtrI(100), Source: "test",
	}
}

// txBars 是三個交易日的近月/遠月序列（與 internal/futures 的 golden 同構）。
func txBars() []domain.FuturesBar {
	return []domain.FuturesBar{
		tBar("202601", "2026-01-05", 100, 1000),
		tBar("202602", "2026-01-05", 110, 500),
		tBar("202601", "2026-01-06", 102, 1100),
		tBar("202602", "2026-01-06", 111, 520),
		tBar("202601", "2026-01-07", 101, 1150),
		tBar("202602", "2026-01-07", 112, 540),
	}
}

func testCLIConfig() cliConfig {
	return cliConfig{
		contracts: []string{"TX"},
		start:     tDay("2026-01-01"),
		end:       tDay("2026-01-31"),
		modelVer:  defaultModelVersion,
		params: futures.ShadowParams{
			Features:           futures.FeatureParams{FlatSpreadPoints: 0, OIFlatChangePct: 0.1},
			WTermStructure:     1,
			WOIChange:          1,
			WForeignNet:        1,
			WPCR:               1,
			DirectionThreshold: 0,
		},
	}
}

func newOutFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout.txt"))
	if err != nil {
		t.Fatalf("create out: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func readOut(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	return string(b)
}

// TestRunWith_DataGateIsNoOp 資料閘門：沒有 bars ⇒ 不報錯、不寫列。
func TestRunWith_DataGateIsNoOp(t *testing.T) {
	store, err := ledger.NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open shadow store: %v", err)
	}
	out := newOutFile(t)
	reader := &fakeReader{bars: map[string][]domain.FuturesBar{}} // TX 沒有任何 bar

	cfg := testCLIConfig()
	if err := runWith(context.Background(), cfg, reader, store, out); err != nil {
		t.Fatalf("data gate must be a no-op, got error: %v", err)
	}
	got := readOut(t, out)
	if !strings.Contains(got, "no-op") {
		t.Fatalf("output should explain the no-op, got %q", got)
	}
	rows, err := store.LoadFuturesShadow(context.Background(), "TX", "", cfg.start, cfg.end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("data gate must write nothing, got %d rows", len(rows))
	}
}

// TestRunWith_WritesShadowRowsAndReportsHitRate 端到端（真 sqlite 影子儲存）。
func TestRunWith_WritesShadowRowsAndReportsHitRate(t *testing.T) {
	store, err := ledger.NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open shadow store: %v", err)
	}
	out := newOutFile(t)
	reader := &fakeReader{bars: map[string][]domain.FuturesBar{"TX": txBars()}}

	cfg := testCLIConfig()
	if err := runWith(context.Background(), cfg, reader, store, out); err != nil {
		t.Fatalf("runWith: %v", err)
	}
	rows, err := store.LoadFuturesShadow(context.Background(), "TX", defaultModelVersion, cfg.start, cfg.end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 shadow rows, got %d", len(rows))
	}
	got := readOut(t, out)
	if !strings.Contains(got, "labeled=1") || !strings.Contains(got, "hits=0") {
		t.Fatalf("output should contain the measured hit rate, got %q", got)
	}
	if !strings.Contains(got, "wrote 3 shadow rows") {
		t.Fatalf("output should report rows written, got %q", got)
	}
}

// TestRunWith_DryRunDoesNotWrite dry-run 不得落地。
func TestRunWith_DryRunDoesNotWrite(t *testing.T) {
	store, err := ledger.NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open shadow store: %v", err)
	}
	out := newOutFile(t)
	reader := &fakeReader{bars: map[string][]domain.FuturesBar{"TX": txBars()}}

	cfg := testCLIConfig()
	cfg.dryRun = true
	if err := runWith(context.Background(), cfg, reader, store, out); err != nil {
		t.Fatalf("runWith dry-run: %v", err)
	}
	rows, err := store.LoadFuturesShadow(context.Background(), "TX", defaultModelVersion, cfg.start, cfg.end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("dry-run wrote %d rows", len(rows))
	}
	if got := readOut(t, out); !strings.Contains(got, "dry-run") {
		t.Fatalf("output should say dry-run, got %q", got)
	}
}

// TestRunWith_ReaderErrorPropagates 讀取失敗必須回報（不得靜默）。
func TestRunWith_ReaderErrorPropagates(t *testing.T) {
	store, err := ledger.NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open shadow store: %v", err)
	}
	out := newOutFile(t)
	reader := &fakeReader{err: errors.New("db down")}
	if err := runWith(context.Background(), testCLIConfig(), reader, store, out); err == nil {
		t.Fatal("reader error must propagate")
	}
}

// TestRunWith_ValidatesParams 非法參數必須在寫入前被擋下。
func TestRunWith_ValidatesParams(t *testing.T) {
	store, err := ledger.NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open shadow store: %v", err)
	}
	out := newOutFile(t)
	reader := &fakeReader{bars: map[string][]domain.FuturesBar{"TX": txBars()}}

	cfg := testCLIConfig()
	cfg.params.DirectionThreshold = -1
	if err := runWith(context.Background(), cfg, reader, store, out); err == nil {
		t.Fatal("negative threshold must be rejected before writing")
	}
}

// TestParseArgs_RequiresEveryCoefficient 是「零隱藏係數」的釘子：
// 少給任一個門檻/權重 ⇒ 必須錯誤，且訊息說明沒有預設值。
func TestParseArgs_RequiresEveryCoefficient(t *testing.T) {
	_, err := parseArgs([]string{"-contracts", "TX", "-start", "2026-01-01", "-end", "2026-01-31"}, io.Discard)
	if err == nil {
		t.Fatal("missing coefficients must be rejected (no hidden defaults)")
	}
	if !strings.Contains(err.Error(), "no hidden coefficients") {
		t.Fatalf("error must explain the no-defaults rule, got %v", err)
	}

	// 逐一拿掉，每一個都必須失敗（不是只有「全缺」才失敗）。
	all := map[string]string{
		"-flat-spread-points":  "10",
		"-oi-flat-pct":         "0.5",
		"-w-term-structure":    "1",
		"-w-oi":                "1",
		"-w-foreign":           "1",
		"-w-pcr":               "1",
		"-direction-threshold": "0.2",
	}
	for drop := range all {
		var args []string
		for k, v := range all {
			if k == drop {
				continue
			}
			args = append(args, k, v)
		}
		args = append(args, "-contracts", "TX", "-start", "2026-01-01", "-end", "2026-01-31")
		if _, err := parseArgs(args, io.Discard); err == nil {
			t.Fatalf("dropping %s must fail (every coefficient is required)", drop)
		}
	}

	// 給齊全部 ⇒ 通過係數閘門，並正確帶入參數。
	var full []string
	for k, v := range all {
		full = append(full, k, v)
	}
	full = append(full, "-contracts", "TX", "-start", "2026-01-01", "-end", "2026-01-31")
	cfg, err := parseArgs(full, io.Discard)
	if err != nil {
		t.Fatalf("all coefficients provided must parse: %v", err)
	}
	if cfg.params.WTermStructure != 1 || cfg.params.DirectionThreshold != 0.2 || cfg.params.Features.OIFlatChangePct != 0.5 {
		t.Fatalf("parsed params = %+v", cfg.params)
	}
	if len(cfg.contracts) != 1 || cfg.contracts[0] != "TX" {
		t.Fatalf("contracts = %v", cfg.contracts)
	}
}

// TestParseArgs_RejectsUnknownContract 未知契約必須擋下（不得給預設乘數／預設語意）。
func TestParseArgs_RejectsUnknownContract(t *testing.T) {
	args := []string{
		"-flat-spread-points", "10", "-oi-flat-pct", "0.5",
		"-w-term-structure", "1", "-w-oi", "1", "-w-foreign", "1", "-w-pcr", "1",
		"-direction-threshold", "0.2",
		"-contracts", "NOPE", "-start", "2026-01-01", "-end", "2026-01-31",
	}
	_, err := parseArgs(args, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unknown futures contract") {
		t.Fatalf("want unknown-contract error, got %v", err)
	}
}
