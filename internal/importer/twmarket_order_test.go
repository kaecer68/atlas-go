package importer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/replay"
)

// rowKey 是 JSONL 檔內每一列的排序鍵（date, symbol），也是「全序」契約的定義。
type rowKey struct {
	Date   string `json:"date"`
	Symbol string `json:"symbol"`
}

// codesInFixture 刻意**不是**字母序：CSV 的列序與排序後的 symbol 序不同，
// 這樣「輸出被排序」才不會退化成「輸出等於輸入列序」的空洞斷言。
var codesInFixture = []string{"2330", "0050", "2317", "2603", "2382"}

// writeOrderedFixtureCSV 產生 days × len(codes) 列的 TWSE open-data CSV。
//
// dateBlocksPermuted ⇒ 日期區塊的順序被反轉；rowsReversed ⇒ 每個日期區塊內的列序
// 被反轉。兩者都只改變**輸入的排列**，不改變內容集合 ⇒ 轉檔輸出必須完全相同
// （FU-20261016-31 的「同輸入」定義是同一組 (date, symbol, 值)）。
func writeOrderedFixtureCSV(t *testing.T, path string, days int, dateBlocksPermuted, rowsReversed bool) {
	t.Helper()

	base := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	var blocks []string
	for d := 0; d < days; d++ {
		date := base.AddDate(0, 0, d).Format("2006-01-02")
		var rows []string
		for c, code := range codesInFixture {
			rows = append(rows, fmt.Sprintf("%s,%s,SYM%s,%d,1000,%d,%d,%d,%d,0,1",
				date, code, code, 100000+c, 100+c, 110+c, 90+c, 105+c))
		}
		if rowsReversed {
			slices.Reverse(rows)
		}
		blocks = append(blocks, strings.Join(rows, "\n"))
	}
	if dateBlocksPermuted {
		slices.Reverse(blocks)
	}

	content := "Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction\n" +
		strings.Join(blocks, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture CSV %s: %v", path, err)
	}
}

// jsonlKeys 把 JSONL 檔的每一列解成排序鍵（同時驗證每一列都是合法 JSON）。
func jsonlKeys(t *testing.T, path string) []rowKey {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	keys := make([]rowKey, 0, len(lines))
	for i, line := range lines {
		var key rowKey
		if err := json.Unmarshal([]byte(line), &key); err != nil {
			t.Fatalf("line %d of %s is not valid JSON: %v (%q)", i+1, path, err, line)
		}
		keys = append(keys, key)
	}
	return keys
}

// TestImportTWOpenDataCSVToJSONL_RowOrderIsDeterministic 是 FU-20261016-31 的
// 契約測試：同一份 CSV 轉檔多次 ⇒ **逐位元組相同**。
//
// 修前：`ds.ByDate[dateKey]` 是 map，Go 的 map 走訪序每次不同 ⇒ 同一天內的列序
// 互換。實測 10 次連續轉檔得到 10 個不同的 sha256。
// 這裡跑 8 次（遠多於 map 走訪序在 5 檔/日下「全部相同」的機率 1/120^7）以確保
// 紅燈不是靠一次巧合。
func TestImportTWOpenDataCSVToJSONL_RowOrderIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "fixture.csv")
	writeOrderedFixtureCSV(t, csvPath, 6, false, false)

	const runs = 8
	digests := make([]string, 0, runs)
	bytesOf := make([][]byte, 0, runs)
	for i := 0; i < runs; i++ {
		target := filepath.Join(dir, fmt.Sprintf("out%02d.jsonl", i))
		if err := ImportTWOpenDataCSVToJSONL(csvPath, target); err != nil {
			t.Fatalf("conversion %d failed: %v", i, err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read conversion %d: %v", i, err)
		}
		sum := sha256.Sum256(got)
		digests = append(digests, fmt.Sprintf("%x", sum))
		bytesOf = append(bytesOf, got)
	}

	distinct := map[string]bool{}
	for _, d := range digests {
		distinct[d] = true
	}
	if len(distinct) != 1 {
		t.Fatalf("converting the same CSV %d times produced %d distinct sha256 digests (%v); the file must be byte-identical across conversions",
			runs, len(distinct), digests)
	}
	for i := range bytesOf {
		if !slices.Equal(bytesOf[0], bytesOf[i]) {
			t.Fatalf("conversion %d differs byte-wise from conversion 0 (sha256 %s vs %s)", i, digests[i], digests[0])
		}
	}
	t.Logf("[evidence] %d conversions, 1 distinct sha256 = %s", runs, digests[0])
}

// TestImportTWOpenDataCSVToJSONL_RowsAreTotallyOrdered 直接斷言檔案的列序契約：
// date 遞增，其次 symbol 遞增（byte order）；且輸出的**內容集合**與 loader
// 看到的資料完全相同（只有順序被固定，沒有列被增刪或改寫）。
func TestImportTWOpenDataCSVToJSONL_RowsAreTotallyOrdered(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "fixture.csv")
	target := filepath.Join(dir, "out.jsonl")
	writeOrderedFixtureCSV(t, csvPath, 4, true, true)

	if err := ImportTWOpenDataCSVToJSONL(csvPath, target); err != nil {
		t.Fatalf("conversion failed: %v", err)
	}
	keys := jsonlKeys(t, target)
	if len(keys) != 4*len(codesInFixture) {
		t.Fatalf("row count = %d, want %d", len(keys), 4*len(codesInFixture))
	}

	// 全序：date 遞增，date 相同時 symbol 遞增。
	for i := 1; i < len(keys); i++ {
		prev, cur := keys[i-1], keys[i]
		switch {
		case cur.Date < prev.Date:
			t.Fatalf("row %d has date %s before row %d's date %s; dates must ascend", i+1, cur.Date, i, prev.Date)
		case cur.Date == prev.Date && cur.Symbol < prev.Symbol:
			t.Fatalf("rows %d/%d on %s are out of symbol order (%s before %s)", i, i+1, cur.Date, prev.Symbol, cur.Symbol)
		}
	}

	// 內容集合不變：與 loader 的 (date, symbol) 集合逐鍵相同。
	ds, err := replay.LoadTWSEOpenDataCSV(csvPath)
	if err != nil {
		t.Fatalf("load fixture with the loader: %v", err)
	}
	want := make([]string, 0, len(keys))
	for dateKey, day := range ds.ByDate {
		for symbol := range day {
			want = append(want, dateKey+"|"+symbol)
		}
	}
	got := make([]string, 0, len(keys))
	for _, k := range keys {
		got = append(got, k.Date[:10]+"|"+k.Symbol)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Fatalf("content set changed: got %v, want %v", got, want)
	}
}

// TestImportTWOpenDataCSVToJSONL_OutputIgnoresInputRowOrder 把「同輸入」的正確
// 語意釘住：內容集合相同、排列不同的兩份 CSV（日期區塊反轉、區塊內列序反轉）
// ⇒ 轉檔輸出**逐位元組相同**。
//
// 這同時守住 date 軸：若日後 loader 不再回傳遞增的日期序列，第 5、6 列的
// 位元組會不同（GetLatestDate 的尾行讀取也同步失效）。
func TestImportTWOpenDataCSVToJSONL_OutputIgnoresInputRowOrder(t *testing.T) {
	dir := t.TempDir()
	canonicalCSV := filepath.Join(dir, "canonical.csv")
	shuffledCSV := filepath.Join(dir, "shuffled.csv")
	writeOrderedFixtureCSV(t, canonicalCSV, 5, false, false)
	writeOrderedFixtureCSV(t, shuffledCSV, 5, true, true)

	canonicalJSONL := filepath.Join(dir, "canonical.jsonl")
	shuffledJSONL := filepath.Join(dir, "shuffled.jsonl")
	if err := ImportTWOpenDataCSVToJSONL(canonicalCSV, canonicalJSONL); err != nil {
		t.Fatalf("convert canonical CSV: %v", err)
	}
	if err := ImportTWOpenDataCSVToJSONL(shuffledCSV, shuffledJSONL); err != nil {
		t.Fatalf("convert shuffled CSV: %v", err)
	}

	gotCanonical, err := os.ReadFile(canonicalJSONL)
	if err != nil {
		t.Fatalf("read canonical JSONL: %v", err)
	}
	gotShuffled, err := os.ReadFile(shuffledJSONL)
	if err != nil {
		t.Fatalf("read shuffled JSONL: %v", err)
	}
	if !slices.Equal(gotCanonical, gotShuffled) {
		t.Fatalf("the same rows in a different input order produced different files:\ncanonical sha256=%x\nshuffled  sha256=%x",
			sha256.Sum256(gotCanonical), sha256.Sum256(gotShuffled))
	}
	t.Logf("[evidence] input-order-independent, sha256=%x", sha256.Sum256(gotCanonical))
}
