package orchestrator

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeReplayFixture lays out the same shape the production deployment uses:
// <root>/data/replay/tw_extended_90days.jsonl plus
// <root>/data/state/price_adjustments/TaiwanStockPriceAdj/<SYMBOL>.jsonl, and
// returns the replay path.
func writeReplayFixture(t *testing.T, root string, rawLines []string, officialFiles map[string]string) string {
	t.Helper()
	replayDir := filepath.Join(root, "data", "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	replayPath := filepath.Join(replayDir, "tw_extended_90days.jsonl")
	body := ""
	for _, line := range rawLines {
		body += line + "\n"
	}
	if err := os.WriteFile(replayPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	adjDir := filepath.Join(root, "data", "state", "price_adjustments", "TaiwanStockPriceAdj")
	if err := os.MkdirAll(adjDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range officialFiles {
		if err := os.WriteFile(filepath.Join(adjDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return replayPath
}

func splitFixture(t *testing.T) (string, map[string]float64) {
	t.Helper()
	raw := map[string]float64{
		"2025-06-16": 188.65,
		"2025-06-17": 189.00,
		"2025-06-18": 47.57,
		"2025-06-19": 47.90,
		"2025-06-20": 48.10,
	}
	// The official adjusted series puts the pre-event bars in post-event terms.
	official := map[string]float64{
		"2025-06-16": 188.65 / 4,
		"2025-06-17": 189.00 / 4,
		"2025-06-18": 47.57,
		"2025-06-19": 47.90,
		"2025-06-20": 48.10,
	}
	lines := []string{}
	for date, close := range raw {
		lines = append(lines, `{"symbol":"SPLIT.TW","date":"`+date+`T00:00:00Z","close":`+trimFloat(close)+`}`)
	}
	for date, close := range raw {
		lines = append(lines, `{"symbol":"CALM.TW","date":"`+date+`T00:00:00Z","close":`+trimFloat(close)+`}`)
	}
	officialJSONL := ""
	for date, close := range official {
		officialJSONL += `{"date":"` + date + `","close":` + trimFloat(close) + "}\n"
	}
	calmJSONL := ""
	for date, close := range raw {
		calmJSONL += `{"date":"` + date + `","close":` + trimFloat(close) + "}\n"
	}
	root := t.TempDir()
	path := writeReplayFixture(t, root, lines, map[string]string{
		"SPLIT.TW.jsonl": officialJSONL,
		"CALM.TW.jsonl":  calmJSONL,
		// A symbol that the replay series does not carry: must be skipped.
		"EXTRA.TW.jsonl": `{"date":"2025-06-16","close":1}\n{"date":"2025-06-17","close":2}\n`,
	})
	return path, raw
}

func csvPathFor(jsonlPath string) string {
	return strings.TrimSuffix(jsonlPath, ".jsonl") + ".csv"
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// TestLoadHistoricalPricesRemovesSplitCliff is the acceptance test for #2151:
// the replay series must not show a fake cliff after the corporate action.
//
// MUTATION: deleting the applyOfficialPriceAdjustments call inside
// loadHistoricalPrices (composition.go) makes this test fail, because the raw
// -75% drop reappears.
func TestLoadHistoricalPricesRemovesSplitCliff(t *testing.T) {
	path, raw := splitFixture(t)
	hp := loadHistoricalPrices(csvPathFor(path), path)

	series := hp.GetCloseSeries("SPLIT.TW")
	if len(series) == 0 {
		t.Fatal("SPLIT.TW must be loaded from the replay fixture")
	}
	// ① no fake cliff: pre-event bars are rewritten to post-event terms, so the
	// largest single-day move is the real one (+0.7% here).
	for i := 1; i < len(series); i++ {
		if change := math.Abs(series[i]/series[i-1] - 1); change > 0.40 {
			t.Fatalf("fake cliff survived: day %d changed %.1f%% (series=%v)", i, change*100, series)
		}
	}
	// ② the adjusted series matches the official one (÷4 for the pre-event bars).
	if want := raw["2025-06-16"] / 4; math.Abs(series[0]-want) > 1e-9 {
		t.Errorf("pre-event close = %v, want %v (official series)", series[0], want)
	}
	// ret20-style comparison against the official values.
	if got, want := series[3]/series[0]-1, 47.90/(188.65/4)-1; math.Abs(got-want) > 1e-9 {
		t.Errorf("three-day return = %v, want the official series' %v", got, want)
	}
	// The newest bar keeps its raw value.
	if last := series[len(series)-1]; last != raw["2025-06-20"] {
		t.Errorf("newest close = %v, want the raw %v", last, raw["2025-06-20"])
	}
	// ③ a symbol whose official/raw ratio never changes is untouched.
	calm := hp.GetCloseSeries("CALM.TW")
	for i := range calm {
		want := []float64{raw["2025-06-16"], raw["2025-06-17"], raw["2025-06-18"], raw["2025-06-19"], raw["2025-06-20"]}[i]
		if calm[i] != want {
			t.Errorf("CALM.TW bar %d = %v, want the raw %v", i, calm[i], want)
		}
	}
	if len(hp.ActionEffects("SPLIT.TW")) != 1 {
		t.Errorf("want exactly one recorded action for SPLIT.TW, got %d", len(hp.ActionEffects("SPLIT.TW")))
	}
	if len(hp.ActionEffects("CALM.TW")) != 0 {
		t.Errorf("CALM.TW must have no actions, got %d", len(hp.ActionEffects("CALM.TW")))
	}
}

// TestLoadHistoricalPricesWithoutOfficialDataIsNoOp documents the data-level
// rollback: without the official series directory the replay prices are used
// exactly as loaded.
func TestLoadHistoricalPricesWithoutOfficialDataIsNoOp(t *testing.T) {
	path, raw := splitFixture(t)
	if err := os.RemoveAll(filepath.Join(filepath.Dir(filepath.Dir(path)), "state", "price_adjustments")); err != nil {
		t.Fatal(err)
	}
	hp := loadHistoricalPrices(csvPathFor(path), path)
	series := hp.GetCloseSeries("SPLIT.TW")
	if got, want := series[2], raw["2025-06-18"]; got != want {
		t.Errorf("without official data the raw series must be kept: got %v want %v", got, want)
	}
	if len(hp.ActionEffects("SPLIT.TW")) != 0 {
		t.Error("no action may be recorded when the official series is unavailable")
	}
}

// TestLoadHistoricalPricesConvertsCsvOnlyReplay guards the load ORDER: the
// P1 CSV→JSONL auto-conversion must run before the JSONL is loaded. A
// deployment that only ships the replay CSV (the very case that conversion
// exists for) would otherwise end up with an empty price series, because a load
// failure is only a warning.
func TestLoadHistoricalPricesConvertsCsvOnlyReplay(t *testing.T) {
	root := t.TempDir()
	replayDir := filepath.Join(root, "data", "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	csvPath := filepath.Join(replayDir, "tw_extended_90days.csv")
	// The CSV loader appends ".TW" to the Code column, so Code stays bare here.
	csvBody := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" +
		"2025-06-16,SPLIT,Fixture,1000,188,189,187,188.65\n" +
		"2025-06-18,SPLIT,Fixture,1000,47,48,47,47.57\n"
	if err := os.WriteFile(csvPath, []byte(csvBody), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(replayDir, "tw_extended_90days.jsonl")

	adjDir := filepath.Join(root, "data", "state", "price_adjustments", "TaiwanStockPriceAdj")
	if err := os.MkdirAll(adjDir, 0o755); err != nil {
		t.Fatal(err)
	}
	official := `{"date":"2025-06-16","close":47.1625}` + "\n" + `{"date":"2025-06-18","close":47.57}` + "\n"
	if err := os.WriteFile(filepath.Join(adjDir, "SPLIT.TW.jsonl"), []byte(official), 0o600); err != nil {
		t.Fatal(err)
	}

	hp := loadHistoricalPrices(csvPath, jsonlPath)

	if _, err := os.Stat(jsonlPath); err != nil {
		t.Fatalf("the CSV→JSONL conversion must have created %s: %v", jsonlPath, err)
	}
	series := hp.GetCloseSeries("SPLIT.TW")
	if len(series) == 0 {
		t.Fatal("price series is empty: the load ran before the CSV→JSONL conversion")
	}
	// Step 3 must also have run after the conversion (the official series rewrites
	// the pre-event bar).
	if math.Abs(series[0]-47.1625) > 1e-6 {
		t.Errorf("pre-event close = %v, want the official series' 47.1625 (adjustment did not run)", series[0])
	}
}
