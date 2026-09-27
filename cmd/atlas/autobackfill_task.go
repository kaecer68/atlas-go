package main

// auto_backfill 的缺口回補 / CSV→JSONL 轉檔解耦（2026-09-27，E33 的 B 票）。
//
// 為什麼要解耦（讀碼 + fixture 實證，非推論）
// ------------------------------------------
// 改動前 `registerOperationsTasks` 的 auto_backfill closure 把 CSV→JSONL 轉檔
// 寫在「有缺口」分支的最後面：
//
//     if start.After(end) { return nil }          // ← 無缺口從這裡離開
//     ... 跑 daily-replay-sync 補缺口 ...
//     importer.ImportTWOpenDataCSVToJSONL(...)    // ← 轉檔只在這條路徑上
//
// 後果：只要每日 cron 正常落地（每個交易日都寫進 CSV ⇒ start > end ⇒ 無缺口），
// auto_backfill 每天都會在 `return nil` 離開，**永遠到不了轉檔**。CSV 每天前進、
// JSONL 凍結 —— 2026-08-24 ~ 2026-09-24 的停擺就是這個形狀（CSV 有 2026-09-24，
// JSONL 停在 2026-08-24），期間所有「抓取層」訊號全程正常。
// 回歸釘：`TestAutoBackfill_NoGapStillConvertsJSONL`（在無缺口 fixture 上必須紅轉綠）。
//
// 修法（兩段式；缺口回補與轉檔各自獨立）
// -------------------------------------
//  1. 缺口回補：有缺口才跑 daily-replay-sync（行為不變，日期窗推導不變）。
//  2. 轉檔閘門：**每次 tick 都評估**，閘門是「CSV 最新資料日 > JSONL 最新資料日」。
//     只有真的落後（或 JSONL 缺席／讀不到）才轉；否則記「已最新、跳過」且
//     **不重寫檔案**（zero work、mtime/size 不變）。
//
// 閘門與可觀測性不得漂移
// ----------------------
//   - 日期語意：閘門兩邊都用 `replayLatestDate`（replay_freshness_metrics_task.go），
//     與 #2063 匯出的 `atlas_replay_{csv,jsonl}_latest_date_timestamp_seconds` 是
//     同一組語意、同一組正規化（該日曆日的 UTC 00:00）。
//   - 產物路徑：JSONL 由 `replayJSONLPath` 推導（同一個函式，不新增第二條推導）——
//     否則「檢查 A、寫入 B」是下一個同型缺陷。
//   - 讀與寫同一條解析：`resolveReplayPaths` 同時供「讀 CSV 最新日」與「寫 JSONL」
//     使用（舊碼讀的是相對 process CWD 的 `cfg.ReplayDataPath`、寫的是相對
//     WorkDir 的路徑，本身就已經是「檢查 A、寫入 B」的形狀）。
//
// 失敗語意（刻意維持）
// --------------------
//   - 轉檔失敗**不致命**：回 nil，只記 `state=failed`（BTM 不得因資料檔問題進入
//     task_failed 的失敗處理；同一件事不要有兩個出口）。
//   - 讀不到 CSV 最新日 ⇒ 不做任何重寫（來源未知時不覆寫產物），記 `state=failed`
//     並附 reason，避免「靜默不轉檔」再次無觀測。

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/importer"
)

// replayDateLayout 是 replay 資料日的字串格式。CSV 的 Date 欄與 JSONL 的
// date 欄前綴（`2026-09-24T00:00:00Z`）都是這個格式。
const replayDateLayout = "2006-01-02"

// replayConversionState 是轉檔閘門的三態。三態各自對應一行可辨識的日誌
// （`state=converted` / `state=up_to_date` / `state=failed`）—— 沒有這個區分，
// 下一次「JSONL 停擺」仍然無法從日誌判斷是「沒跑」、「跑了但判斷已最新」還是「轉檔失敗」。
type replayConversionState string

const (
	// replayConversionConverted：CSV 較新（或 JSONL 缺席／讀不到）且轉檔成功。
	replayConversionConverted replayConversionState = "converted"
	// replayConversionUpToDate：JSONL 已最新 ⇒ 跳過，**不重寫檔案**。
	replayConversionUpToDate replayConversionState = "up_to_date"
	// replayConversionFailed：無從判定或轉檔失敗（非致命）。
	replayConversionFailed replayConversionState = "failed"
)

// autoBackfillWindow 由 replay 最新資料日與現在時間推導要回補的日期窗。
//
// 推導規則與改動前完全相同（15:30 Taipei 前算前一交易日；頭尾都避開週末）：
// hasGap=false 表示沒有缺口 —— **這只是「不回補」，不是「不轉檔」**（本次修法的重點）。
func autoBackfillWindow(latestDate, now time.Time) (start, end time.Time, hasGap bool) {
	if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
		now = now.In(tz)
	}
	end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
		end = end.AddDate(0, 0, -1)
	}
	start = latestDate.AddDate(0, 0, 1)
	for start.Weekday() == time.Saturday || start.Weekday() == time.Sunday {
		start = start.AddDate(0, 0, 1)
	}
	for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
		end = end.AddDate(0, 0, -1)
	}
	return start, end, !start.After(end)
}

// resolveReplayPaths 由 (workDir, replayDataPath) 推導 replay CSV 與其 JSONL 產物路徑。
//
// 相對路徑以 workDir 為基準（容器內 `/app` + `data/replay/tw_extended_90days.csv`
// 即 `/app/data/replay/...`；與 daily-replay-sync 的 `-csv` 預設值一致）。
// JSONL 一律由 `replayJSONLPath` 推導，與 #2063 的觀測端共用同一條規則。
func resolveReplayPaths(workDir, replayDataPath string) (csvPath, jsonlPath string) {
	csvPath = replayDataPath
	if !filepath.IsAbs(csvPath) {
		csvPath = filepath.Join(absWorkDir(workDir), csvPath)
	}
	return csvPath, replayJSONLPath(csvPath)
}

// absWorkDir 取得 workDir 的絕對路徑；失敗時退回原值（與改動前相同）。
func absWorkDir(workDir string) string {
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return workDir
	}
	return abs
}

// syncReplayJSONLIfStale 是轉檔閘門：以「CSV 最新資料日 > JSONL 最新資料日」決定
// 是否重建 JSONL，並回傳三態之一（同時寫出可辨識的日誌）。
//
// 永不回傳 error：失敗（CSV 讀不到、產物不可寫）一律記 `state=failed` 後回
// replayConversionFailed，維持「轉檔失敗不致命」的既有語意。
func syncReplayJSONLIfStale(csvPath, jsonlPath string) replayConversionState {
	csvDate, csvErr := replayLatestDate(csvPath)
	if csvErr != nil {
		// 來源未知 ⇒ 不覆寫產物（重寫一份「不知道對不對」的 JSONL 比不寫更糟）。
		log.Printf("[Gateway] auto_backfill CSV→JSONL: state=%s reason=csv_latest_date_unreadable csv=%s err=%v",
			replayConversionFailed, csvPath, csvErr)
		return replayConversionFailed
	}

	jsonlDate, jsonlErr := replayLatestDate(jsonlPath)
	if jsonlErr == nil && !csvDate.After(jsonlDate) {
		log.Printf("[Gateway] auto_backfill CSV→JSONL: state=%s (skip; JSONL already latest, no rewrite) csv_latest=%s jsonl_latest=%s jsonl=%s",
			replayConversionUpToDate, csvDate.Format(replayDateLayout), jsonlDate.Format(replayDateLayout), jsonlPath)
		return replayConversionUpToDate
	}

	reason := "csv_ahead"
	if jsonlErr != nil {
		reason = "jsonl_latest_date_unreadable"
	}
	if err := importer.ImportTWOpenDataCSVToJSONL(csvPath, jsonlPath); err != nil {
		log.Printf("[Gateway] auto_backfill CSV→JSONL: state=%s (non-fatal) reason=%s csv=%s jsonl=%s err=%v",
			replayConversionFailed, reason, csvPath, jsonlPath, err)
		return replayConversionFailed
	}
	jsonlBefore := "-"
	if jsonlErr == nil {
		jsonlBefore = jsonlDate.Format(replayDateLayout)
	}
	log.Printf("[Gateway] auto_backfill CSV→JSONL: state=%s csv_latest=%s jsonl_before=%s jsonl=%s",
		replayConversionConverted, csvDate.Format(replayDateLayout), jsonlBefore, jsonlPath)
	return replayConversionConverted
}

// backfillRunner 執行 daily-replay-sync 回補一個日期窗（測試可注入）。
type backfillRunner func(ctx context.Context, workDir, csvPath, startStr, endStr string) ([]byte, error)

// runAutoBackfill 是一次 auto_backfill tick（production 入口，clock 取 time.Now()）。
func runAutoBackfill(ctx context.Context, cfg config.Config) error {
	return runAutoBackfillAt(ctx, cfg, time.Now(), runDailyReplaySyncBackfill)
}

// runAutoBackfillAt 是可測的 tick 主體：缺口回補（有缺口才跑）+ 轉檔閘門（每次都評估）。
func runAutoBackfillAt(ctx context.Context, cfg config.Config, now time.Time, runBackfill backfillRunner) error {
	csvPath, jsonlPath := resolveReplayPaths(cfg.WorkDir, cfg.ReplayDataPath)

	latestDate, err := getLatestReplayDate(csvPath)
	if err != nil {
		return fmt.Errorf("backfill replay read: %w", err)
	}

	start, end, hasGap := autoBackfillWindow(latestDate, now)
	if hasGap {
		startStr := start.Format(replayDateLayout)
		endStr := end.Format(replayDateLayout)
		log.Printf("[Gateway] backfill gap detected: %s to %s", startStr, endStr)
		bgCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		out, err := runBackfill(bgCtx, cfg.WorkDir, cfg.ReplayDataPath, startStr, endStr)
		if err != nil {
			return fmt.Errorf("backfill failed: %w, output: %s", err, string(out))
		}
		log.Printf("[Gateway] backfill success: %s", string(out))
	} else {
		// 無缺口 ⇒ 只跳過回補。**不得**在此離開：轉檔與回補無關（本次修法的根因）。
		log.Printf("[Gateway] backfill: no gap (replay latest %s, window end %s) — sync skipped, CSV→JSONL conversion still evaluated",
			latestDate.Format(replayDateLayout), end.Format(replayDateLayout))
	}

	// 轉檔閘門：與回補結果無關，每次都評估（失敗不致命 ⇒ 不回傳 error）。
	_ = syncReplayJSONLIfStale(csvPath, jsonlPath)
	return nil
}

// runDailyReplaySyncBackfill 呼叫 daily-replay-sync 回補日期窗。
//
// 二選一：workDir 下已編譯的 binary，否則 `go run`（本地開發）；兩者皆無 ⇒ 錯誤。
// 行為（binary 優先、`cmd.Dir = absWorkDir`、5 分鐘 timeout 由呼叫端給）與改動前相同。
func runDailyReplaySyncBackfill(ctx context.Context, workDir, csvPath, startStr, endStr string) ([]byte, error) {
	absWorkDirPath := absWorkDir(workDir)
	var cmd *exec.Cmd
	if _, err := os.Stat(filepath.Join(absWorkDirPath, "daily-replay-sync")); err == nil {
		cmd = exec.CommandContext(ctx, filepath.Join(absWorkDirPath, "daily-replay-sync"),
			"-csv", csvPath, "-backfill-start", startStr, "-backfill-end", endStr)
	} else if _, err := exec.LookPath("go"); err == nil {
		cmd = exec.CommandContext(ctx, "go", "run", "./cmd/daily-replay-sync",
			"-csv", csvPath, "-backfill-start", startStr, "-backfill-end", endStr)
	} else {
		return nil, errors.New("backfill binary not found")
	}
	cmd.Dir = absWorkDirPath
	return cmd.CombinedOutput()
}
