package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kaecer68/atlas-go/internal/calibration"
	"github.com/kaecer68/atlas-go/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "calibrate-parameters:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("calibrate-parameters", flag.ContinueOnError)
	module := fs.String("module", "all", "Module: garch, var, darwinian, factor, all")
	dryRun := fs.Bool("dry-run", false, "Preview changes without saving")
	dataPath := fs.String("data", "", "Replay path (CSV or JSONL). Defaults to ATLAS_REPLAY_DATA_PATH")
	verbose := fs.Bool("v", false, "Verbose output")
	writebackFlag := fs.String("writeback", string(config.WritebackSSOT), config.WritebackFlagUsage)
	if err := fs.Parse(args); err != nil {
		return err
	}
	writeback, err := config.ParseCalibrationWriteback(*writebackFlag)
	if err != nil {
		return err
	}
	workDir, _ := os.Getwd()
	if err := writeback.RegisterForWorkDir(workDir); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// darwinian 校準會讀 ledger outcomes：生產注入 PG-backed store（讀真相來源，
	// 而不是 session_summary_reconcile 每 12 小時回填的 flat JSONL 鏡射）；
	// 解析或開池失敗時 calibrationStoreForRun 會記 WARN 並降級成 nil store，
	// 週任務因此不會因為設定問題而失敗。非 darwinian 模組不開池、不讀 DSN。
	store, closeStore := calibrationStoreForRun(ctx, *module, workDir, config.Load(), defaultStoreDeps())
	defer closeStore()

	report, err := calibration.RunWithStore(workDir, *module, *dataPath, *dryRun, *verbose, writeback, store)
	if err != nil {
		return err
	}
	fmt.Print(report)
	return nil
}
