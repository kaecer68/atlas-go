package main

// 轉檔閘門的**收斂性**守門測試（FU-20260927-08）。
//
// 背景：#2079 的閘門是單向的 —— 只有「CSV 最新日 > JSONL 最新日」才轉。
// JSONL 若被外部寫入者（人工修復、cmd/import-replay、其他工具）寫到**領先或等於**
// CSV，閘門恆為 false，於是：
//   ① 檔案永遠不會被帶回與 CSV 一致；
//   ② 日誌仍寫 `conversion: skipped (already up to date)` ⇒ 把「不一致」講成「已同步」。
//
// 本檔把兩件事變成可執行斷言：
//   1. `TestSyncReplayJSONLFromCSV_JSONLAhead_ConvergesToCSV` — JSONL 領先 ⇒ 必須真的被修好。
//   2. `TestSyncReplayJSONLFromCSV_LogMatchesReality` — 日誌的每個 token 必須與事實相符：
//      「skipped (already up to date)」只允許在兩邊日期**相等**且檔案**未被改寫**時出現；
//      日期不等時必須是 `converted`（真做了事）。
//
// 為什麼 JSONL 領先也要以 CSV 為準：JSONL 是**衍生檔**（CSV ＝ TWSE open data 是來源），
// 衍生檔不得成為權威 ⇒ 不一致就必須收斂回 CSV。這條語意寫在此處以免後人再度只比單向。

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// gateFixtureAhead 建立「JSONL 領先 CSV」的 fixture：
// CSV 最新日 2026-03-19、JSONL 已經有 2026-03-20（外部寫入者的痕跡）。
func gateFixtureAhead(t *testing.T) (csvPath, jsonlPath string) {
	t.Helper()
	_, csvPath, jsonlPath = guardFixture(t)
	// JSONL 先用正式轉檔路徑產生到 2026-03-20…
	guardSeedJSONLFromCSV(t, csvPath, jsonlPath, "2026-03-19", "2026-03-20")
	// …再把 CSV 退回只有 2026-03-19（模擬來源落後／JSONL 被外部工具寫到未來）。
	guardWriteCSV(t, csvPath, "2026-03-19")
	return csvPath, jsonlPath
}

// TestSyncReplayJSONLFromCSV_JSONLAhead_ConvergesToCSV — JSONL 領先 ⇒ 修前假「已同步」。
func TestSyncReplayJSONLFromCSV_JSONLAhead_ConvergesToCSV(t *testing.T) {
	csvPath, jsonlPath := gateFixtureAhead(t)

	f := compareReplayCSVToJSONL(csvPath, jsonlPath)
	if f.csvErr != nil || f.jsonlErr != nil {
		t.Fatalf("fixture must be readable: csvErr=%v jsonlErr=%v", f.csvErr, f.jsonlErr)
	}
	if !f.jsonlLatest.After(f.csvLatest) {
		t.Fatalf("fixture must have JSONL(%s) ahead of CSV(%s)", f.jsonlLatest, f.csvLatest)
	}
	if !f.conversionNeeded() {
		t.Fatal("JSONL 領先 CSV 時閘門必須要求轉檔（不一致就必須收斂回來源 CSV），否則該檔永遠不會被修好")
	}

	before, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read JSONL pre-state: %v", err)
	}
	if got := guardReplayLatestDate(t, jsonlPath); got != "2026-03-20" {
		t.Fatalf("fixture JSONL pre-state = %s, want 2026-03-20", got)
	}

	var syncErr error
	logs := guardCaptureLog(t, func() {
		syncErr = syncReplayJSONLFromCSV(csvPath, jsonlPath)
	})
	if syncErr != nil {
		t.Fatalf("sync returned %v, want nil", syncErr)
	}

	after, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read JSONL post-state: %v", err)
	}
	if bytes.Equal(before, after) {
		t.Error("JSONL 領先時必須真的被重寫（收斂回 CSV），實際內容未變")
	}
	if got := guardReplayLatestDate(t, jsonlPath); got != "2026-03-19" {
		t.Errorf("JSONL latest date after sync = %s, want 2026-03-19 (must converge to the CSV)", got)
	}
	if strings.Count(string(after), "\n") != 1 {
		t.Errorf("JSONL must hold exactly one bar after converging to the single-day CSV, got:\n%s", after)
	}
	if strings.Contains(logs, "already up to date") {
		t.Errorf("日誌不得在修好之前把「不一致」講成『已同步』, got:\n%s", logs)
	}
	if !strings.Contains(logs, "conversion: converted") {
		t.Errorf("expected a 'converted' outcome line, got:\n%s", logs)
	}
	t.Logf("[evidence] logs:\n%s", strings.TrimSpace(logs))
}

// conversionLogLine 抓出 `backfill CSV→JSONL conversion: <token> …` 這一行。
var conversionLogLine = regexp.MustCompile(`(?m)^.*backfill CSV→JSONL conversion: (.+)$`)

// csvLatestInLog 抓出日誌行裡的 csv_latest（缺即為 nil）。
var csvLatestInLog = regexp.MustCompile(`csv_latest=(\S+)`)

// TestSyncReplayJSONLFromCSV_LogMatchesReality — 日誌必須說實話。
//
// 對每一種前置狀態跑一次 syncReplayJSONLFromCSV，然後只相信兩種證據：
// 檔案實際內容（是否被改寫、最新日為何）與日誌行本身。斷言：
//
//	日誌 token                          ⇒ 必須成立的事實
//	skipped (already up to date)         兩邊日期相等、且檔案逐位元組未被改動
//	converted                            日誌裡的 csv_latest 等於轉檔後 JSONL 的最新日
//
// 這條會直接把「單向閘門 + 恆真 skip 訊息」的組合打成紅燈。
func TestSyncReplayJSONLFromCSV_LogMatchesReality(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T) (csvPath, jsonlPath string)
	}{
		{"JSONL 落後（CSV 較新）", func(t *testing.T) (string, string) {
			_, csvPath, jsonlPath := guardFixture(t)
			guardSeedJSONLFromCSV(t, csvPath, jsonlPath, "2026-03-19")
			guardWriteCSV(t, csvPath, "2026-03-19", "2026-03-20")
			return csvPath, jsonlPath
		}},
		{"JSONL 領先（外部寫入者）", gateFixtureAhead},
		{"兩邊一致", func(t *testing.T) (string, string) {
			_, csvPath, jsonlPath := guardFixture(t)
			guardSeedJSONLFromCSV(t, csvPath, jsonlPath, "2026-03-19", "2026-03-20")
			return csvPath, jsonlPath
		}},
		{"JSONL 缺席", func(t *testing.T) (string, string) {
			_, csvPath, jsonlPath := guardFixture(t)
			guardWriteCSV(t, csvPath, "2026-03-19")
			return csvPath, jsonlPath
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			csvPath, jsonlPath := tc.seed(t)
			pre := compareReplayCSVToJSONL(csvPath, jsonlPath)
			before, readErr := os.ReadFile(jsonlPath)
			existedBefore := readErr == nil

			var syncErr error
			logs := guardCaptureLog(t, func() {
				syncErr = syncReplayJSONLFromCSV(csvPath, jsonlPath)
			})
			if syncErr != nil {
				t.Fatalf("sync returned %v, want nil", syncErr)
			}

			after, err := os.ReadFile(jsonlPath)
			if err != nil {
				t.Fatalf("read JSONL post-state: %v", err)
			}
			rewritten := !existedBefore || !bytes.Equal(before, after)
			post := compareReplayCSVToJSONL(csvPath, jsonlPath)

			m := conversionLogLine.FindStringSubmatch(logs)
			if m == nil {
				t.Fatalf("no 'backfill CSV→JSONL conversion:' line in the logs:\n%s", logs)
			}
			outcome := strings.TrimSpace(m[1])

			switch {
			case strings.HasPrefix(outcome, "skipped (already up to date)"):
				if pre.csvErr != nil || pre.jsonlErr != nil {
					t.Errorf("logged 'already up to date' although a file was unreadable: csvErr=%v jsonlErr=%v", pre.csvErr, pre.jsonlErr)
				}
				if !pre.csvLatest.Equal(pre.jsonlLatest) {
					t.Errorf("日誌說『already up to date』但兩邊日期不同：csv_latest=%s jsonl_latest=%s ⇒ 判定失真",
						pre.csvLatest.Format(dateLayout), pre.jsonlLatest.Format(dateLayout))
				}
				if rewritten {
					t.Errorf("日誌說『already up to date』（零工作）但檔案被改寫了")
				}
			case strings.HasPrefix(outcome, "skipped (replay CSV unreadable"):
				if pre.csvErr == nil {
					t.Errorf("logged 'replay CSV unreadable' although the CSV was readable")
				}
				if rewritten {
					t.Errorf("沒有來源可轉時不得改寫 JSONL")
				}
			case strings.HasPrefix(outcome, "needed ("):
				// 方向必須與事實相符：閘門的判準只能有這三種來源。
				switch {
				case pre.jsonlErr != nil:
					if !strings.Contains(outcome, "JSONL unreadable") {
						t.Errorf("JSONL 讀不到時日誌必須說 %q, got %q", "JSONL unreadable", outcome)
					}
				case pre.csvLatest.After(pre.jsonlLatest):
					if !strings.Contains(outcome, "CSV is newer") {
						t.Errorf("CSV 較新時日誌必須說 %q, got %q", "CSV is newer", outcome)
					}
				case pre.jsonlLatest.After(pre.csvLatest):
					if !strings.Contains(outcome, "JSONL is ahead") {
						t.Errorf("JSONL 領先時日誌必須說 %q（不得只說『較新』或『已同步』）, got %q", "JSONL is ahead", outcome)
					}
				default:
					t.Errorf("兩邊日期相等時不得走『needed』路徑, got %q", outcome)
				}
				if post.jsonlErr != nil {
					t.Errorf("logged 'needed' but the JSONL is unreadable afterwards: %v", post.jsonlErr)
				}
				if post.csvErr == nil && !post.csvLatest.Equal(post.jsonlLatest) {
					t.Errorf("日誌說『needed ...』（＝已轉檔）但 JSONL(%s) 仍未與 CSV(%s) 一致 ⇒ 宣告與事實不符",
						post.jsonlLatest.Format(dateLayout), post.csvLatest.Format(dateLayout))
				}
				if !rewritten {
					t.Errorf("日誌說『needed ...』但檔案逐位元組未變")
				}
				if csvInLog := csvLatestInLog.FindStringSubmatch(outcome); csvInLog != nil && post.jsonlErr == nil {
					if want := post.jsonlLatest.Format(dateLayout); csvInLog[1] != want {
						t.Errorf("日誌的 csv_latest=%s 與轉檔後的 JSONL 最新日 %s 不一致", csvInLog[1], want)
					}
				}
			default:
				t.Errorf("未知的轉檔結果 token %q（每個 token 都必須可被斷言）", outcome)
			}
			t.Logf("[evidence %s] pre(csv=%v jsonl=%v) post(csv=%v jsonl=%v) rewritten=%v log=%q",
				tc.name, pre.csvLatest.Format(dateLayout), pre.jsonlLatest.Format(dateLayout),
				post.csvLatest.Format(dateLayout), post.jsonlLatest.Format(dateLayout), rewritten, outcome)
		})
	}
}
