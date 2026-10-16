package importer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeFixtureCSV 產生 days × codes 列的最小 TWSE open-data CSV
// （欄位＝internal/replay.LoadTWSEOpenDataCSV 的 required 清單）。
func writeFixtureCSV(t *testing.T, path string, days, codes int) {
	t.Helper()
	var b strings.Builder
	b.WriteString("Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction\n")
	base := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	for d := 0; d < days; d++ {
		date := base.AddDate(0, 0, d).Format("2006-01-02")
		for c := 0; c < codes; c++ {
			fmt.Fprintf(&b, "%s,%04d,SYM%04d,32001234,25801234567,790,795,788,792,2,19555\n", date, c+1, c+1)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write fixture CSV %s: %v", path, err)
	}
}

// TestImportTWOpenDataCSVToJSONL_ReplaceIsAtomic 把「讀端可能看到 0-byte／半截檔」
// 變成可執行斷言（FU-20260927-07）。
//
// 手法：轉檔進行中，另一個 goroutine 以**無 sleep 的緊密輪詢**統計目標檔的 size
// （＝FactorEngine 讀端在 composition 階段開檔時會看到的大小）。合法觀測只有兩種：
//
//	old size — 舊的完整檔（尚未 rename）
//	new size — 新的完整檔（rename 之後）
//
// 觀測到 0 bytes（truncate 先發生）或任何中間 size（逐列寫入中）都是缺陷。
// 非空洞性：取樣器必須在轉檔**之前**與**之後**各留下至少一個觀測（首筆 == old、
// 末筆 == new），否則斷言無效。
func TestImportTWOpenDataCSVToJSONL_ReplaceIsAtomic(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "big.csv")
	target := filepath.Join(dir, "bars.jsonl")
	// 40 日 × 400 檔 = 16,000 列 ⇒ 寫入期間足以被緊密輪詢取樣到。
	writeFixtureCSV(t, csvPath, 40, 400)

	// 期望的最終內容：寫到另一個目錄量測，避免與被觀測的檔案混淆。
	expectedPath := filepath.Join(t.TempDir(), "expected.jsonl")
	if err := ImportTWOpenDataCSVToJSONL(csvPath, expectedPath); err != nil {
		t.Fatalf("build expected JSONL: %v", err)
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected JSONL: %v", err)
	}
	if len(expected) == 0 {
		t.Fatal("expected JSONL is empty; the sampler below would be vacuous")
	}

	oldContent := []byte("{\"date\":\"2025-12-31T00:00:00Z\",\"symbol\":\"OLD.TW\"}\n")
	if err := os.WriteFile(target, oldContent, 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	oldSize := int64(len(oldContent))
	newSize := int64(len(expected))
	if newSize <= oldSize {
		t.Fatalf("fixture must grow the file (old=%d new=%d)", oldSize, newSize)
	}

	var mu sync.Mutex
	var sizes []int64
	sampleCount := func() int { mu.Lock(); defer mu.Unlock(); return len(sizes) }
	lastSize := func() int64 {
		mu.Lock()
		defer mu.Unlock()
		if len(sizes) == 0 {
			return -1
		}
		return sizes[len(sizes)-1]
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			fi, err := os.Stat(target)
			if err != nil {
				continue
			}
			mu.Lock()
			sizes = append(sizes, fi.Size())
			mu.Unlock()
			runtime.Gosched()
		}
	}()

	// 等取樣器真的在跑（否則下面的斷言可能是空的）。
	waitFor(t, func() bool { return sampleCount() > 0 }, "sampler never observed the target file before the conversion")

	convErr := ImportTWOpenDataCSVToJSONL(csvPath, target)

	// 等取樣器看到轉檔後的新檔（證明它跨越了整個寫入窗口）。
	waitFor(t, func() bool { return lastSize() == newSize }, "sampler never observed the post-conversion file")
	close(stop)
	wg.Wait()

	if convErr != nil {
		t.Fatalf("conversion failed: %v", convErr)
	}

	mu.Lock()
	observed := append([]int64(nil), sizes...)
	mu.Unlock()

	if observed[0] != oldSize {
		t.Fatalf("first sample = %d, want the pre-conversion size %d (the sampler must start before the conversion)", observed[0], oldSize)
	}

	var zeroBytes, truncated int
	var examples []int64
	minSeen := int64(math.MaxInt64)
	for _, s := range observed {
		if s < minSeen {
			minSeen = s
		}
		switch {
		case s == 0:
			zeroBytes++
		case s != oldSize && s != newSize:
			truncated++
			if len(examples) < 5 {
				examples = append(examples, s)
			}
		}
	}
	if zeroBytes > 0 || truncated > 0 {
		t.Errorf("a concurrent reader observed %d/%d samples of a 0-byte file and %d/%d samples of a truncated file (size examples=%v, min=%d, old=%d, new=%d); the reader must only ever see the complete old file or the complete new file",
			zeroBytes, len(observed), truncated, len(observed), examples, minSeen, oldSize, newSize)
	}
	t.Logf("[evidence] %d samples, sizes ∈ {old=%d, new=%d}, 0-byte=%d truncated=%d min=%d",
		len(observed), oldSize, newSize, zeroBytes, truncated, minSeen)

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read converted target: %v", err)
	}
	// 內容比對用「排序後的列集合」：Loader 對同一天的股票是 map 走訪
	// （internal/replay/twse_csv.go 的 ByDate[symbol]），列序在合法範圍內不保證穩定。
	if diff := jsonlLineSetDiff(t, got, expected); diff != "" {
		t.Fatalf("converted target content differs from the expected JSONL: %s", diff)
	}
}

// jsonlLineSetDiff 比較兩份 JSONL 的「列集合」（排序後逐列比對），
// 回傳空字串代表等價。
func jsonlLineSetDiff(t *testing.T, got, want []byte) string {
	t.Helper()
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	sort.Strings(gotLines)
	sort.Strings(wantLines)
	if len(gotLines) != len(wantLines) {
		return fmt.Sprintf("line count %d != %d", len(gotLines), len(wantLines))
	}
	for i := range gotLines {
		if gotLines[i] != wantLines[i] {
			return fmt.Sprintf("line %d: got %q want %q", i, gotLines[i], wantLines[i])
		}
	}
	return ""
}

// waitFor 以 5 秒上限輪詢 cond（失敗時 fatal 並附訊息）。
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	t.Fatal(msg)
}

// TestImportTWOpenDataCSVToJSONL_FailureLeavesNoPartialFile 是原子寫入的負向對照：
// 目標路徑不可替換（這裡是目錄）⇒ 轉檔必須回錯，且**不得**留下暫存檔或半成品。
//
// 舊碼的失敗點在 os.Create（EISDIR，還沒寫任何東西）；新碼的失敗點在
// os.Rename（暫存檔已寫完）。因此這條測試守的是「新的失敗模式不會留下垃圾」。
func TestImportTWOpenDataCSVToJSONL_FailureLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	target := filepath.Join(dir, "bars.jsonl")
	writeFixtureCSV(t, csvPath, 3, 4)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("seed target path as a directory: %v", err)
	}
	before := listDir(t, dir)

	err := ImportTWOpenDataCSVToJSONL(csvPath, target)
	if err == nil {
		t.Fatal("converting onto a directory path must fail")
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		t.Errorf("the injected directory must be untouched (err=%v)", statErr)
	}
	after := listDir(t, dir)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("a failed conversion changed the directory contents:\n before=%v\n after =%v\n (temp or partial files must not survive a failure)", before, after)
	}
	t.Logf("[evidence] conversion error = %v; directory unchanged: %v", err, after)
}

// TestImportTWOpenDataCSVToJSONL_PreservesFileMode 守「轉檔不得默默改變目標檔權限」。
//
// os.CreateTemp 固定以 0600 建立檔案，而舊碼的 os.Create 是 0666 & ~umask
// （本機 umask 022 ⇒ 0644）⇒ 換成 temp+rename 時若不處理就會**默默收緊**權限。
//
// 兩條斷言都不依賴 umask 的實際值：
//   - 新檔：與同目錄的 os.Create 對照組權限相同（kernel 套用同一個 umask）；
//   - 既有檔：沿用既有權限（0640 ⇒ 轉檔後仍是 0640）。
func TestImportTWOpenDataCSVToJSONL_PreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	writeFixtureCSV(t, csvPath, 2, 3)

	// 對照組：舊碼的建立方式。
	control := filepath.Join(dir, "control-by-os-create")
	cf, err := os.Create(control)
	if err != nil {
		t.Fatalf("create control file: %v", err)
	}
	if err := cf.Close(); err != nil {
		t.Fatalf("close control file: %v", err)
	}
	controlInfo, err := os.Stat(control)
	if err != nil {
		t.Fatalf("stat control file: %v", err)
	}

	fresh := filepath.Join(dir, "fresh.jsonl")
	if err := ImportTWOpenDataCSVToJSONL(csvPath, fresh); err != nil {
		t.Fatalf("convert to a new path: %v", err)
	}
	freshInfo, err := os.Stat(fresh)
	if err != nil {
		t.Fatalf("stat fresh target: %v", err)
	}
	if freshInfo.Mode().Perm() != controlInfo.Mode().Perm() {
		t.Errorf("new target mode = %#o, want %#o (the same mode os.Create would have produced)", freshInfo.Mode().Perm(), controlInfo.Mode().Perm())
	}

	existing := filepath.Join(dir, "existing.jsonl")
	if err := os.WriteFile(existing, []byte("{}\n"), 0o640); err != nil {
		t.Fatalf("seed existing target: %v", err)
	}
	if err := os.Chmod(existing, 0o640); err != nil {
		t.Fatalf("chmod existing target: %v", err)
	}
	if err := ImportTWOpenDataCSVToJSONL(csvPath, existing); err != nil {
		t.Fatalf("convert over an existing path: %v", err)
	}
	existingInfo, err := os.Stat(existing)
	if err != nil {
		t.Fatalf("stat existing target: %v", err)
	}
	if existingInfo.Mode().Perm() != 0o640 {
		t.Errorf("existing target mode = %#o, want 0640 (the conversion must not silently change the file mode)", existingInfo.Mode().Perm())
	}
}

// listDir 回傳（排序後的）目錄項目名稱。
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
