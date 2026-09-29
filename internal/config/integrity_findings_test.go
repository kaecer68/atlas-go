package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Issue #1944 Batch 3, item I31: cmd/calibration-validate reported OK=false while
// the nightly job stayed green (the workflow runs it with `set +e` and a non-final
// `cat`, and the Slack step is skipped when SLACK_WEBHOOK_URL is unset). This batch
// cannot touch .github/workflows, so the fix inside the code lane is to make the
// verdict self-describing: every failure now carries a stable code, a severity and
// the affected segment. These tests pin the codes and the shipped-config shape.

func writeIntegrityParams(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write params: %v", err)
	}
	return path
}

func segmentJSON(id, parent string, level int, reps []string) map[string]any {
	seg := map[string]any{
		"id":    id,
		"name":  id,
		"level": level,
	}
	if parent != "" {
		seg["parent_id"] = parent
	}
	if len(reps) > 0 {
		seg["representative_stocks"] = reps
	}
	return seg
}

func integrityParamsJSON(t *testing.T, updatedAt time.Time, segments []map[string]any) string {
	t.Helper()
	doc := map[string]any{
		"updated_at": updatedAt.Format(time.RFC3339),
		"industry": map[string]any{
			"classification_tree": map[string]any{
				"value": map[string]any{"segments": segments},
			},
		},
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(out)
}

func findingCodes(res *CalibrationValidationResult) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, string(f.Code))
	}
	return out
}

func requireCode(t *testing.T, res *CalibrationValidationResult, code CalibrationFindingCode, segment string) {
	t.Helper()
	for _, f := range res.Findings {
		if f.Code != code {
			continue
		}
		if segment != "" && f.Segment != segment {
			continue
		}
		if f.Severity != CalibrationSeverityError {
			t.Fatalf("finding %s severity = %q, want %q", f.Code, f.Severity, CalibrationSeverityError)
		}
		return
	}
	t.Fatalf("missing finding %s (segment %q); got codes %v", code, segment, findingCodes(res))
}

func TestValidateCalibration_FindingsAreClassified(t *testing.T) {
	now := time.Now()

	t.Run("missing_file", func(t *testing.T) {
		res, err := ValidateCalibration(filepath.Join(t.TempDir(), "absent.json"), 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingParamsStatFailed, "")
	})

	t.Run("invalid_json", func(t *testing.T) {
		path := writeIntegrityParams(t, "{not json")
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingParamsInvalidJSON, "")
	})

	t.Run("zero_updated_at", func(t *testing.T) {
		path := writeIntegrityParams(t, integrityParamsJSON(t, time.Time{},
			[]map[string]any{segmentJSON("semiconductor", "", 1, []string{"2330.TW"})}))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingUpdatedAtZero, "")
	})

	t.Run("stale_updated_at", func(t *testing.T) {
		path := writeIntegrityParams(t, integrityParamsJSON(t, now.Add(-30*24*time.Hour),
			[]map[string]any{segmentJSON("semiconductor", "", 1, []string{"2330.TW"})}))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingUpdatedAtStale, "")
	})

	t.Run("empty_segments", func(t *testing.T) {
		path := writeIntegrityParams(t, integrityParamsJSON(t, now, nil))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingSegmentsEmpty, "")
	})

	t.Run("structural_segment_defects", func(t *testing.T) {
		segments := []map[string]any{
			segmentJSON("", "", 1, []string{"2330.TW"}),                       // empty L1 id
			segmentJSON("semiconductor", "", 1, []string{"2330.TW"}),          // ok
			segmentJSON("no_reps_l1", "", 1, nil),                             // L1 without reps
			segmentJSON("pcb", "semiconductor", 2, nil),                       // L2 without reps
			segmentJSON("orphan", "", 0, nil),                                 // level 0 → ignored
			segmentJSON("no_parent", "semiconductor", 2, []string{"3037.TW"}), // ok (has parent+reps)
		}
		// Force an L2 with empty parent and one with an unknown parent.
		segments = append(segments,
			map[string]any{"id": "empty_parent", "level": 2, "representative_stocks": []string{"1"}},
			map[string]any{"id": "unknown_parent", "level": 2, "parent_id": "ghost", "representative_stocks": []string{"2"}},
		)
		path := writeIntegrityParams(t, integrityParamsJSON(t, now, segments))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		requireCode(t, res, CalibrationFindingL1EmptyID, "")
		requireCode(t, res, CalibrationFindingL1NoRepresentatives, "no_reps_l1")
		requireCode(t, res, CalibrationFindingL2NoRepresentatives, "pcb")
		requireCode(t, res, CalibrationFindingL2EmptyParentID, "empty_parent")
		requireCode(t, res, CalibrationFindingL2UnknownParentID, "unknown_parent")
		if len(res.Issues) != len(res.Findings) {
			t.Fatalf("Issues (%d) must mirror Findings (%d) for backward compatibility", len(res.Issues), len(res.Findings))
		}
		for i, f := range res.Findings {
			if res.Issues[i] != f.Message {
				t.Fatalf("Issues[%d] = %q, want message %q", i, res.Issues[i], f.Message)
			}
		}
	})

	t.Run("clean_tree_passes", func(t *testing.T) {
		segments := []map[string]any{
			segmentJSON("electronics", "", 1, []string{"2317.TW"}),
			segmentJSON("pcb", "electronics", 2, []string{"3037.TW"}),
		}
		path := writeIntegrityParams(t, integrityParamsJSON(t, now, segments))
		res, err := ValidateCalibration(path, 48*time.Hour)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.OK {
			t.Fatalf("expected OK=true for a fully populated tree, findings=%v", findingCodes(res))
		}
		if len(res.Findings) != 0 {
			t.Fatalf("expected no findings, got %v", findingCodes(res))
		}
	})
}

// TestShippedConfigIntegrityFindingsAreClassified pins the exact failure classes of
// the shipped config. It is intentionally an exact-set assertion: refreshing
// updated_at, adding representatives or growing the taxonomy must fail here, so the
// inert-registry entry (I31) is updated in the same commit instead of drifting.
func TestShippedConfigIntegrityFindingsAreClassified(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "parameters.json")

	// 注入時鐘，讓這一條的判定**不再取決於 checkout 何時做的**：這份檔案在版本庫裡
	// 的 updated_at 是固定的舊值（永遠 stale），但它的 mtime 是「你上次 checkout 的
	// 時間」。用真時鐘跑 ⇒ 在同一份 checkout 上，mtime 會隨時間老化，超過 48h 之後
	// 就會多出一個 MTIME_STALE，讓下面那個**精確集合**斷言自己翻紅（clone 當天綠、
	// 兩天後紅）。下面把 now 釘在 mtime + 1h：
	//   · mtime 永遠不 stale（與 checkout 時間無關）；
	//   · updated_at 仍然相對它過期 ⇒ 這一條要守的性質（「shipped 設定的 updated_at
	//     就是舊的，任何人刷新它都必須在這裡紅燈」）一字不減。
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat shipped params: %v", err)
	}
	res, err := ValidateCalibrationWithOptions(path, CalibrationValidationOptions{
		MaxAge: 48 * time.Hour,
		Now:    info.ModTime().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	if res.OK {
		t.Fatalf("shipped config unexpectedly passes the nightly integrity check — update docs/reference/inert-registry.md (I31), which documents why it cannot pass in a fresh checkout")
	}

	want := []string{
		string(CalibrationFindingUpdatedAtStale),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL2NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL1NoRepresentatives),
		string(CalibrationFindingL2NoRepresentatives),
	}
	got := findingCodes(res)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("shipped-config finding codes changed:\n got %v\nwant %v", got, want)
	}

	// The five zero-weight strategy/asset-class buckets are the L1 entries without
	// representatives; pcb and thermal are the two L2 ones.
	segments := map[string]int{}
	for _, f := range res.Findings {
		if f.Code == CalibrationFindingL1NoRepresentatives || f.Code == CalibrationFindingL2NoRepresentatives {
			segments[f.Segment]++
		}
	}
	for _, id := range []string{"defensive", "etf_rotation", "high_dividend", "small_cap", "tech", "pcb", "thermal"} {
		if segments[id] != 1 {
			t.Fatalf("segment %q: got %d no-representative findings, want 1 (all=%v)", id, segments[id], segments)
		}
	}
}

// ---------------------------------------------------------------------------
// 時鐘 seam（2026-09-29）
// ---------------------------------------------------------------------------
//
// 為什麼這幾條測試存在：`collectCalibrationFindings` 原本用 `time.Since(...)`
// （真實時鐘）判 mtime/updated_at 是否過期，於是 **freshness 判定是 wall clock 的
// 函式**：任何以固定時戳驗證的呼叫端（監控探針、測試）都會在真實時間走過
// `fixture_updated_at + maxAge` 的那一天自己翻紅 —— 一顆與程式碼無關的日期炸彈。
// `CalibrationValidationOptions.Now` 就是那個 seam。下面兩組測試分別釘住：
//
//	① 注入的 now 完全決定判定（含精確邊界），且檔案 mtime 也必須跟著它；
//	② 判定與 **真實時鐘無關**：把注入時鐘放在真實時鐘的 ±100 天，預期值不變
//	   （舊實作在這兩格會各紅一次，方向相反）。

// clockSeamFixture 寫一份結構乾淨的 parameters.json，並把 **mtime 與 updated_at
// 都釘在 updatedAt**。mtime 必須一起釘：`collectCalibrationFindings` 對 mtime 也發
// freshness finding，只釘 updated_at 會留下第二條 wall-clock 依賴。
func clockSeamFixture(t *testing.T, updatedAt time.Time) string {
	t.Helper()
	segments := []map[string]any{segmentJSON("semiconductor", "", 1, []string{"2330.TW"})}
	path := writeIntegrityParams(t, integrityParamsJSON(t, updatedAt, segments))
	if err := os.Chtimes(path, updatedAt, updatedAt); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

// freshnessCodesOnly 回傳結果中的 freshness finding codes（其他類別與本測試無關）。
func freshnessCodesOnly(res *CalibrationValidationResult) []string {
	out := []string{}
	for _, f := range res.Findings {
		if IsFreshnessFinding(f.Code) {
			out = append(out, string(f.Code))
		}
	}
	return out
}

func TestValidateCalibrationWithOptions_InjectedNowOwnsTheVerdict(t *testing.T) {
	const maxAge = 48 * time.Hour
	fixed := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	updatedAt := fixed.Add(-time.Hour) // fixture 自述的校準時間（= mtime）

	cases := []struct {
		name  string
		now   time.Time
		ok    bool
		codes []string
	}{
		{
			name:  "同一時刻 ⇒ 新鮮",
			now:   fixed,
			ok:    true,
			codes: []string{},
		},
		{
			name:  "注入的 now 往後 100 天 ⇒ 不新鮮（兩個 freshness code 都必須出現）",
			now:   fixed.Add(100 * 24 * time.Hour),
			ok:    false,
			codes: []string{string(CalibrationFindingUpdatedAtStale), string(CalibrationFindingMTimeStale)},
		},
		{
			name:  "注入的 now 往前 100 天（產物時間戳在未來）⇒ 仍然新鮮",
			now:   fixed.Add(-100 * 24 * time.Hour),
			ok:    true,
			codes: []string{},
		},
		{
			name:  "剛好等於 maxAge ⇒ 新鮮（判定是 > 不是 >=）",
			now:   updatedAt.Add(maxAge),
			ok:    true,
			codes: []string{},
		},
		{
			name:  "maxAge 再多 1ns ⇒ 不新鮮",
			now:   updatedAt.Add(maxAge + time.Nanosecond),
			ok:    false,
			codes: []string{string(CalibrationFindingUpdatedAtStale), string(CalibrationFindingMTimeStale)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := clockSeamFixture(t, updatedAt)
			res, err := ValidateCalibrationWithOptions(path, CalibrationValidationOptions{MaxAge: maxAge, Now: tc.now})
			if err != nil {
				t.Fatalf("ValidateCalibrationWithOptions: %v", err)
			}
			if res.OK != tc.ok {
				t.Fatalf("OK=%v，want %v（now=%s updated_at=%s codes=%v）",
					res.OK, tc.ok, tc.now.Format(time.RFC3339), updatedAt.Format(time.RFC3339), findingCodes(res))
			}
			if got := freshnessCodesOnly(res); !sameStringSet(got, tc.codes) {
				t.Fatalf("freshness codes=%v，want %v", got, tc.codes)
			}
			// StaleBy 也必須由注入的時鐘導出（CLI 會把它印出來，是值班讀的那一行）。
			if want := tc.now.Sub(updatedAt) - maxAge; res.StaleBy != want {
				t.Errorf("StaleBy=%v，want %v（必須由注入的 now 導出）", res.StaleBy, want)
			}
		})
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := slices.Clone(a)
	y := slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// 這一條是「與 wall clock 無關」的可執行證明：注入時鐘被放在真實時鐘的
// ±100 天（兩個方向都遠超 48h 契約），fixture 跟著注入時鐘釘死。
//
// 舊實作（內部 `time.Since`）在這一條會**兩個子案例都紅、且方向相反**：
//
//	· 「現在 −100 天」那一格，真時鐘看到 updated_at 大約是 100 天前 ⇒ 誤判 stale；
//	· 「現在 +100 天」那一格，真時鐘看到 updated_at 在未來（負 age）⇒ 誤判 fresh。
//
// ⇒ 任何把 `Now` 接線拿掉的改動都會在這裡留下可診斷的紅燈（見 PR 的突變證據）。
func TestValidateCalibrationWithOptions_VerdictIsIndependentOfWallClock(t *testing.T) {
	const maxAge = 48 * time.Hour
	realNow := time.Now() // 只用來把注入時鐘推離真實時鐘，不用來判定

	cases := []struct {
		name     string
		offset   time.Duration // 注入時鐘相對真實時鐘的偏移
		ageHours time.Duration // 產物自述年齡（相對注入時鐘）
		ok       bool
	}{
		{
			name:     "注入時鐘在真實時鐘前 100 天，產物只有 65 分鐘舊 ⇒ 新鮮",
			offset:   -100 * 24 * time.Hour,
			ageHours: 65 * time.Minute,
			ok:       true,
		},
		{
			name:     "注入時鐘在真實時鐘後 100 天，產物已 72 小時舊 ⇒ 不新鮮",
			offset:   +100 * 24 * time.Hour,
			ageHours: 72 * time.Hour,
			ok:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			injected := realNow.Add(tc.offset).Truncate(time.Second)
			updatedAt := injected.Add(-tc.ageHours)
			path := clockSeamFixture(t, updatedAt)

			res, err := ValidateCalibrationWithOptions(path, CalibrationValidationOptions{MaxAge: maxAge, Now: injected})
			if err != nil {
				t.Fatalf("ValidateCalibrationWithOptions: %v", err)
			}
			if res.OK != tc.ok {
				t.Fatalf("OK=%v，want %v —— 判定跟隨了真實時鐘而不是注入的 now=%s（codes=%v）",
					res.OK, tc.ok, injected.Format(time.RFC3339), findingCodes(res))
			}
		})
	}
}
