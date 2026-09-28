// Command futures-shadow-eval 對既有的期貨日行情跑「影子評估」：計算跨市場特徵、
// 產生影子預測、寫入**獨立**的影子儲存，並印出命中率。
//
// 這不是交易訊號，也**不會**接進任何引擎路徑（R3 未修前不得接線；見
// docs/specs/futures-crossmarket-signal-spec.md §2）。
//
// 兩個刻意的設計：
//
//  1. **零隱藏係數**：所有門檻與權重都必須以 flag 明確提供，且**沒有預設值**。
//     少給任何一個 ⇒ 直接錯誤結束。這是 product-positioning.md §8
//     「heuristic 一律經過驗證管道」的落地：未校準的門檻不得躲在程式裡。
//  2. **資料閘門**：指定的區間內沒有 futures bars ⇒ **no-op**（exit 0、不寫任何列、
//     不產生假訊號）。生產在回補完成前就是這個狀態。
//
// 用法（全部參數必填）：
//
//	futures-shadow-eval -contracts TX -start 2026-09-01 -end 2026-09-24 \
//	  -flat-spread-points 10 -oi-flat-pct 0.5 \
//	  -w-term-structure 1 -w-oi 1 -w-foreign 1 -w-pcr 1 -direction-threshold 0.2
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/futures"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// defaultModelVersion 是本評估器的模型版本標記。
//
// 這是**識別字串**，不是可調參數：影子列必須與 live 校準可區分（見 spec §7）。
// 權重變更時應遞增版本（例：futures-shadow-v2），以免不同參數的列被當成同一組。
const defaultModelVersion = "futures-shadow-v1"

// requiredFloat 是「必填數值 flag」：區分「未提供」與「提供 0」。
type requiredFloat struct {
	name  string
	value float64
	set   bool
}

func (r *requiredFloat) String() string { return fmt.Sprintf("%v", r.value) }

func (r *requiredFloat) Set(v string) error {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return fmt.Errorf("flag -%s: %q is not a number", r.name, v)
	}
	r.value = parsed
	r.set = true
	return nil
}

type cliConfig struct {
	contracts []string
	start     time.Time
	end       time.Time
	backend   string
	modelVer  string
	dryRun    bool
	params    futures.ShadowParams
}

// barReader 是 CLI 對期貨 bar 儲存的最小需求（測試可注入 fake）。
type barReader interface {
	LoadFuturesBars(ctx context.Context, contract, contractMonth string, session domain.FuturesSession, start, end time.Time) ([]domain.FuturesBar, error)
}

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("futures-shadow-eval: %v", err)
	}
}

func runFromOSArgs() error {
	cfg, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		return err
	}
	return run(cfg)
}

// parseArgs 以**獨立的 FlagSet** 解析命令列（不使用全域 flag.CommandLine）。
//
// 為什麼要獨立 FlagSet：全域 flag 的重複註冊會 panic，而本研究工具需要可重複呼叫的
// 參數解析（測試要驗證「少給係數必須失敗」）。獨立 FlagSet 讓解析成為純函式。
func parseArgs(args []string, errOut io.Writer) (cliConfig, error) {
	fs := flag.NewFlagSet("futures-shadow-eval", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var (
		contracts = fs.String("contracts", "", "逗號分隔的契約代碼（必填，例如 TX）")
		start     = fs.String("start", "", "起日 YYYY-MM-DD（必填）")
		end       = fs.String("end", "", "迄日 YYYY-MM-DD（必填）")
		backend   = fs.String("backend", "", "儲存後端 jsonl|sqlite|postgres（預設：讀 ATLAS_STORE_BACKEND）")
		modelVer  = fs.String("model-version", defaultModelVersion, "影子列模型版本標記（權重變更時請遞增）")
		dryRun    = fs.Bool("dry-run", false, "只計算不寫入")
	)

	// 門檻與權重：**沒有預設值**（零隱藏係數）。
	flatSpread := &requiredFloat{name: "flat-spread-points"}
	oiFlatPct := &requiredFloat{name: "oi-flat-pct"}
	wTerm := &requiredFloat{name: "w-term-structure"}
	wOI := &requiredFloat{name: "w-oi"}
	wForeign := &requiredFloat{name: "w-foreign"}
	wPCR := &requiredFloat{name: "w-pcr"}
	threshold := &requiredFloat{name: "direction-threshold"}
	fs.Var(flatSpread, "flat-spread-points", "價差視為平坦的門檻（指數點；必填）")
	fs.Var(oiFlatPct, "oi-flat-pct", "OI 變化視為持平的門檻（百分比；必填）")
	fs.Var(wTerm, "w-term-structure", "跨月價差結構的假設權重（必填）")
	fs.Var(wOI, "w-oi", "OI 變化的假設權重（必填）")
	fs.Var(wForeign, "w-foreign", "外資淨部位變化的假設權重（必填）")
	fs.Var(wPCR, "w-pcr", "PCR 變化的假設權重（必填）")
	fs.Var(threshold, "direction-threshold", "|score| 超過此值才給方向（必填）")

	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}

	for _, r := range []*requiredFloat{flatSpread, oiFlatPct, wTerm, wOI, wForeign, wPCR, threshold} {
		if !r.set {
			return cliConfig{}, fmt.Errorf("flag -%s is required: this evaluator has no hidden coefficients (see docs/specs/futures-crossmarket-signal-spec.md §6)", r.name)
		}
	}

	if strings.TrimSpace(*contracts) == "" {
		return cliConfig{}, fmt.Errorf("flag -contracts is required")
	}
	if strings.TrimSpace(*start) == "" || strings.TrimSpace(*end) == "" {
		return cliConfig{}, fmt.Errorf("flags -start and -end are required")
	}

	loc := marketdata.TaiwanLocation()
	startTime, err := time.ParseInLocation("2006-01-02", *start, loc)
	if err != nil {
		return cliConfig{}, fmt.Errorf("parse -start: %w", err)
	}
	endTime, err := time.ParseInLocation("2006-01-02", *end, loc)
	if err != nil {
		return cliConfig{}, fmt.Errorf("parse -end: %w", err)
	}
	if endTime.Before(startTime) {
		return cliConfig{}, fmt.Errorf("-end %s is before -start %s", *end, *start)
	}

	var list []string
	for _, part := range strings.Split(*contracts, ",") {
		code := strings.ToUpper(strings.TrimSpace(part))
		if code == "" {
			continue
		}
		if _, ok := domain.FuturesContractSpecFor(code); !ok {
			return cliConfig{}, fmt.Errorf("unknown futures contract %q (known: %v)", code, domain.FuturesContractCodes())
		}
		list = append(list, code)
	}
	if len(list) == 0 {
		return cliConfig{}, fmt.Errorf("-contracts must list at least one contract")
	}

	return cliConfig{
		contracts: list,
		start:     startTime,
		end:       endTime,
		backend:   *backend,
		modelVer:  *modelVer,
		dryRun:    *dryRun,
		params: futures.ShadowParams{
			Features:           futures.FeatureParams{FlatSpreadPoints: flatSpread.value, OIFlatChangePct: oiFlatPct.value},
			WTermStructure:     wTerm.value,
			WOIChange:          wOI.value,
			WForeignNet:        wForeign.value,
			WPCR:               wPCR.value,
			DirectionThreshold: threshold.value,
		},
	}, nil
}

// run 建立真實的 bar reader 與影子儲存後執行。
func run(cfg cliConfig) error {
	appCfg := config.Load()

	readStore, err := ledger.NewFuturesBarStore(resolveBackend(cfg, appCfg))
	if err != nil {
		// 後端解析失敗 ⇒ 先確認是否只是「讀取端」不可用。
		return err
	}
	writer, err := ledger.NewFuturesShadowStore(resolveBackend(cfg, appCfg))
	if err != nil {
		return err
	}
	return runWith(context.Background(), cfg, readStore, writer, os.Stdout)
}

// resolveBackend 讓 -backend 覆寫組態；未給則沿用 ATLAS_STORE_BACKEND（不得硬編）。
func resolveBackend(cfg cliConfig, appCfg config.Config) config.Config {
	if cfg.backend != "" {
		appCfg.StoreBackend = cfg.backend
	}
	return appCfg
}

// runWith 是可測核心：bar reader 與影子 writer 都可注入。
func runWith(ctx context.Context, cfg cliConfig, reader barReader, writer ledger.FuturesShadowStore, out *os.File) error {
	if err := cfg.params.Validate(); err != nil {
		return err
	}

	totalSamples := 0
	var allRows []ledger.FuturesShadowRow

	for _, contract := range cfg.contracts {
		bars, err := reader.LoadFuturesBars(ctx, contract, "", "", cfg.start, cfg.end)
		if err != nil {
			return fmt.Errorf("load futures bars for %s: %w", contract, err)
		}
		if len(bars) == 0 {
			// 資料閘門：沒有 bars ⇒ no-op（不報錯、不寫列、不產生假訊號）。
			_, _ = fmt.Fprintf(out, "futures-shadow-eval: %s: no futures bars in %s..%s ⇒ no-op (run the backfill first)\n",
				contract, cfg.start.Format("2006-01-02"), cfg.end.Format("2006-01-02"))
			continue
		}

		// 外部輸入（三大法人期貨部位、PCR）在本版本**尚未接線**：
		// 兩者的 first-party 歷史深度不足（PCR 端點僅回滾動約 19 個交易日；
		// 法人期貨部位 OpenAPI 無日期參數），因此這裡傳 nil，
		// 對應的假設項不會被計入（TermsUsed 會如實反映）。
		samples, err := futures.BuildShadowSeries(bars, contract, cfg.params, nil)
		if err != nil {
			return fmt.Errorf("build shadow series for %s: %w", contract, err)
		}
		for _, s := range samples {
			allRows = append(allRows, ledger.FuturesShadowRow{ModelVersion: cfg.modelVer, Sample: s})
		}
		hr := futures.SummarizeHitRate(samples)
		_, _ = fmt.Fprintf(out, "futures-shadow-eval: %s: samples=%d labeled=%d hits=%d hit_rate=%.4f skipped=%d\n",
			contract, len(samples), hr.Total, hr.Hits, hr.Rate, hr.Skipped)
		totalSamples += len(samples)
	}

	if totalSamples == 0 {
		_, _ = fmt.Fprintf(out, "futures-shadow-eval: no samples produced (data gate) — nothing written\n")
		return nil
	}

	hr := futures.SummarizeHitRate(samplesOfRows(allRows))
	_, _ = fmt.Fprintf(out, "futures-shadow-eval: total samples=%d labeled=%d hits=%d hit_rate=%.4f model_version=%s\n",
		totalSamples, hr.Total, hr.Hits, hr.Rate, cfg.modelVer)

	if cfg.dryRun {
		_, _ = fmt.Fprintf(out, "futures-shadow-eval: dry-run — %d rows NOT written\n", len(allRows))
		return nil
	}

	written, err := writer.RecordFuturesShadow(ctx, allRows)
	if err != nil {
		return fmt.Errorf("record futures shadow rows: %w", err)
	}
	_, _ = fmt.Fprintf(out, "futures-shadow-eval: wrote %d shadow rows (isolated namespace; live calibration untouched)\n", written)
	return nil
}

func samplesOfRows(rows []ledger.FuturesShadowRow) []futures.ShadowSample {
	out := make([]futures.ShadowSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Sample)
	}
	return out
}
