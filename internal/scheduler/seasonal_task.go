// Package scheduler 擴充:季節性模式校準背景任務。
//
// 本檔提供 NewSeasonalCalibrationTask 工廠函式,將
// `cmd/calibrate-seasonal` CLI 工具包裝成 BackgroundCalibrationScheduler
// 可消費的 CalibrationTask。每次排程觸發會 spawn 一個子行程執行
// `calibrate-seasonal -update` 流程,並把 stdout/stderr 透傳到 logging。
//
// 設計取捨:不將 cmd/calibrate-seasonal/main.go 重構為 library 是為了
// 保留 CLI 入口的可獨立呼叫性與既有 flag 介面。Subprocess overhead 對
// 「每 N 個交易日」級距(預設 7 天)可忽略。
package scheduler

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// SeasonalCalibrationDefaults 提供季節性校準任務的預設值。
// 集中管理便於測試與未來調參。
var SeasonalCalibrationDefaults = struct {
	Interval    time.Duration
	MinMaturity domain.SystemMaturity
}{
	Interval:    7 * 24 * time.Hour, // 7 calendar days ≈ 5 trading days
	MinMaturity: domain.MaturityCalibrating,
}

// NewSeasonalCalibrationTask 建立一個會以子行程方式執行
// `calibrate-seasonal -update` 的 CalibrationTask。
//
// Deprecated: 此工廠產出的 *CalibrationTask 只能被 BackgroundCalibrationScheduler
// 消費,但目前 main.go 是用 apigateway.BackgroundTaskManager 註冊背景任務,
// 直接呼叫 SeasonalCalibrationTaskFunc(binaryPath) 作為 ScheduledTask.Task。
// 保留此工廠以備未來切換到 BackgroundCalibrationScheduler,但目前無生產消費者。
// 新代碼請使用 SeasonalCalibrationTaskFunc。
//
// 參數說明:
//   - binaryPath:`calibrate-seasonal` 二進位絕對路徑(由呼叫端注入,
//     避免在 scheduler 套件硬編路徑,符合 dependency injection 原則)。
//   - interval:可選;若 <= 0 則使用 SeasonalCalibrationDefaults.Interval。
//   - minMaturity:可選;若空字串則使用 SeasonalCalibrationDefaults.MinMaturity。
//
// Interval 採用 calendar days 作為交易日近似值(7 days ≈ 5 trading days)。
// 若需精確的「每 N 個交易日」語意,需從 marketdata 推導交易日曆,
// 並改寫 Run 為 ticker 觸發 + 交易日查詢;此屬後續工作。
func NewSeasonalCalibrationTask(binaryPath string, interval time.Duration, minMaturity domain.SystemMaturity) *CalibrationTask {
	if interval <= 0 {
		interval = SeasonalCalibrationDefaults.Interval
	}
	if minMaturity == "" {
		minMaturity = SeasonalCalibrationDefaults.MinMaturity
	}
	if binaryPath == "" {
		// 延遲至 Run 階段報錯,避免工廠呼叫期 panicking。
		binaryPath = "calibrate-seasonal"
	}

	return &CalibrationTask{
		Name:        "seasonal_calibration",
		MinMaturity: minMaturity,
		Interval:    interval,
		Run: func(ctx context.Context) error {
			return runSeasonalCalibration(ctx, binaryPath)
		},
	}
}

func runSeasonalCalibration(ctx context.Context, binaryPath string) error {
	return runSeasonalCalibrationWithReplay(ctx, binaryPath, "")
}

// runSeasonalCalibrationWithReplay exec's calibrate-seasonal -update with an
// optional --replay path. When replayPath is empty, calibrate-seasonal will
// refuse --update (it requires real data) and the task will fail — this is
// the expected behavior until a replay dataset is provided at registration.
func runSeasonalCalibrationWithReplay(ctx context.Context, binaryPath, replayPath string) error {
	if binaryPath == "" {
		return fmt.Errorf("seasonal calibration: binary path is empty")
	}

	args := []string{"-update"}
	if replayPath != "" {
		args = append(args, "--replay", replayPath)
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Error("seasonal_calibration", "exec_failed",
			"binary", binaryPath,
			"args", args,
			"output_len", len(output),
			"err", err)
		return fmt.Errorf("calibrate-seasonal -update: %w", err)
	}

	logging.Info("seasonal_calibration", "exec_ok",
		"binary", binaryPath,
		"output_len", len(output))
	return nil
}

// SeasonalCalibrationTaskFunc returns a closure compatible with
// apigateway.ScheduledTask.Task that exec's the calibrate-seasonal binary.
// Empty/missing binaryPath surfaces as a non-nil error; caller must resolve
// the path at registration time and log a skip if resolution fails.
func SeasonalCalibrationTaskFunc(binaryPath string) func(context.Context) error {
	return func(ctx context.Context) error {
		return runSeasonalCalibration(ctx, binaryPath)
	}
}

// SeasonalCalibrationTaskFuncWithReplay is like SeasonalCalibrationTaskFunc
// but passes --replay <replayPath> to the calibrate-seasonal binary.
// The replay path should point to a replay JSONL/CSV file (e.g.
// data/replay/finmind_2020_2024.jsonl). When replayPath is empty, the task
// will run without --replay (same as SeasonalCalibrationTaskFunc).
func SeasonalCalibrationTaskFuncWithReplay(binaryPath, replayPath string) func(context.Context) error {
	return func(ctx context.Context) error {
		return runSeasonalCalibrationWithReplay(ctx, binaryPath, replayPath)
	}
}
