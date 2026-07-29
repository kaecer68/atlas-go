package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTWSEStub(t *testing.T, wantStatus int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(wantStatus)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		twseMIIndexURL = "https://www.twse.com.tw/exchangeReport/MI_INDEX"
	})
	twseMIIndexURL = srv.URL
}

func writeSnap(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRun_NormalBackfill_WritesKeyAndAppendsLog(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-07-24.json", "{\n  \"us10y\": {\"symbol\":\"X\",\"value\":1,\"change_pct\":0,\"timestamp\":0}\n}\n")

	body := `{"stat":"OK","tables":[{"title":"Indices","fields":["指數","收盤指數","漲跌點數","漲跌百分比(%)"],"data":[["發行量加權股價指數","43,654.84","1,195.97","-2.67"]]}]}`
	setupTWSEStub(t, 200, body)

	if err := run(args{dir: dir, date: "2026-07-24", field: fieldTAIEX}); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "2026-07-24.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]json.RawMessage
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	rawPt, ok := s["taiex"]
	if !ok {
		t.Fatal("taiex key not present after backfill")
	}
	var pt macroDataPoint
	if err := json.Unmarshal(rawPt, &pt); err != nil {
		t.Fatal(err)
	}
	if pt.Symbol != "^TWII" {
		t.Errorf("symbol = %q, want ^TWII", pt.Symbol)
	}
	if pt.Value != 43654.84 {
		t.Errorf("value = %v, want 43654.84", pt.Value)
	}
	if pt.Timestamp == 0 {
		t.Errorf("timestamp = 0, want non-zero")
	}

	// Verify the original us10y bytes are preserved untouched.
	if !strings.Contains(string(raw), "\"us10y\": {\"symbol\":\"X\",\"value\":1,\"change_pct\":0,\"timestamp\":0}") {
		t.Errorf("original us10y bytes not preserved exactly: %s", string(raw))
	}

	logRaw, err := os.ReadFile(filepath.Join(dir, "backfill_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var log backfillLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(logRaw))), &log); err != nil {
		t.Fatal(err)
	}
	if log.Date != "2026-07-24" {
		t.Errorf("log.date = %q", log.Date)
	}
	if log.Field != "taiex" {
		t.Errorf("log.field = %q", log.Field)
	}
	if log.Value != 43654.84 {
		t.Errorf("log.value = %v", log.Value)
	}
	if log.BaselineDate != "" {
		t.Errorf("baseline_date = %q, want empty", log.BaselineDate)
	}
}

func TestRun_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-07-24.json", "{\n  \"taiex\": {\"symbol\":\"^TWII\",\"value\":44825.78,\"change_pct\":1.34,\"timestamp\":1700000000}\n}\n")

	err := run(args{dir: dir, date: "2026-07-24", field: fieldTAIEX})
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !strings.Contains(err.Error(), "refuse") {
		t.Errorf("error = %v, want contains 'refuse'", err)
	}
}

func TestRun_RejectsNonTAIEXField(t *testing.T) {
	dir := t.TempDir()
	err := run(args{dir: dir, date: "2026-07-24", field: "vix"})
	if err == nil {
		t.Fatal("expected error for non-taiex field")
	}
	if !strings.Contains(err.Error(), "taiex") {
		t.Errorf("error = %v, want mentions taiex", err)
	}
}

func TestRun_RefusesWeekend(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-07-25.json", "{\n  \"us10y\": {\"symbol\":\"X\",\"value\":1}\n}\n")
	err := run(args{dir: dir, date: "2026-07-25", field: fieldTAIEX})
	if err == nil {
		t.Fatal("expected refusal for weekend")
	}
	if !strings.Contains(err.Error(), "weekend") {
		t.Errorf("error = %v, want mentions weekend", err)
	}
}

func TestRun_ChangePctCalculation(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-07-23.json", "{\n  \"taiex\": {\"symbol\":\"^TWII\",\"value\":44825.78,\"change_pct\":0,\"timestamp\":1753315200}\n}\n")
	writeSnap(t, dir, "2026-07-24.json", "{\n  \"us10y\": {\"symbol\":\"X\",\"value\":1,\"change_pct\":0,\"timestamp\":0}\n}\n")

	body := `{"stat":"OK","tables":[{"title":"X","fields":["指數","收盤指數","漲跌點數","漲跌百分比(%)"],"data":[["發行量加權股價指數","43,654.84","1,195.97","-2.67"]]}]}`
	setupTWSEStub(t, 200, body)

	if err := run(args{dir: dir, date: "2026-07-24", field: fieldTAIEX}); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "2026-07-24.json"))
	var s map[string]json.RawMessage
	json.Unmarshal(raw, &s)
	var pt macroDataPoint
	json.Unmarshal(s["taiex"], &pt)

	want := -2.6125
	if absDiff(pt.ChangePct, want) > 0.01 {
		t.Errorf("change_pct = %v, want ≈ %v", pt.ChangePct, want)
	}

	logRaw, _ := os.ReadFile(filepath.Join(dir, "backfill_log.jsonl"))
	var log backfillLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(logRaw))), &log); err != nil {
		t.Fatal(err)
	}
	if log.BaselineDate != "2026-07-23" {
		t.Errorf("log.baseline_date = %q, want 2026-07-23", log.BaselineDate)
	}
	if log.BaselineValue != 44825.78 {
		t.Errorf("log.baseline_value = %v, want 44825.78", log.BaselineValue)
	}
}

func TestRun_LogAppendsAcrossTwoInvocations(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-07-24.json", "{\n  \"us10y\": {\"symbol\":\"X\",\"value\":1,\"change_pct\":0,\"timestamp\":0}\n}\n")

	body1 := `{"stat":"OK","tables":[{"title":"X","fields":["指數","收盤指數","漲跌點數","漲跌百分比(%)"],"data":[["發行量加權股價指數","43,654.84","1,195.97","-2.67"]]}]}`
	setupTWSEStub(t, 200, body1)
	if err := run(args{dir: dir, date: "2026-07-24", field: fieldTAIEX}); err != nil {
		t.Fatal(err)
	}

	writeSnap(t, dir, "2026-07-27.json", "{\n  \"us10y\": {\"symbol\":\"X\",\"value\":1,\"change_pct\":0,\"timestamp\":0}\n}\n")
	body2 := `{"stat":"OK","tables":[{"title":"X","fields":["指數","收盤指數","漲跌點數","漲跌百分比(%)"],"data":[["發行量加權股價指數","43,634.19","20.65","-0.05"]]}]}`
	setupTWSEStub(t, 200, body2)
	if err := run(args{dir: dir, date: "2026-07-27", field: fieldTAIEX}); err != nil {
		t.Fatal(err)
	}

	logRaw, err := os.ReadFile(filepath.Join(dir, "backfill_log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(logRaw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want 2", len(lines))
	}
	var e1, e2 backfillLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &e1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &e2); err != nil {
		t.Fatal(err)
	}
	if e1.Date != "2026-07-24" || e2.Date != "2026-07-27" {
		t.Errorf("log dates = %q, %q", e1.Date, e2.Date)
	}
	if e2.BaselineDate != "2026-07-24" {
		t.Errorf("e2.baseline_date = %q, want 2026-07-24 (just backfilled)", e2.BaselineDate)
	}
}

func TestParseTAIEXClosing(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		err  bool
	}{
		{"43,654.84", 43654.84, false},
		{"43,634.19", 43634.19, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := parseTAIEXClosing(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseTAIEXClosing(%q): want error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTAIEXClosing(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseTAIEXClosing(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLooksLikeDate(t *testing.T) {
	cases := map[string]bool{
		"2026-07-24.json": true,
		"2026-07-24":      false,
		"latest.json":     false,
		"foo-bar.json":    false,
	}
	for in, want := range cases {
		if got := looksLikeDate(in); got != want {
			t.Errorf("looksLikeDate(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRound2AndRound4(t *testing.T) {
	if got := round2(43654.839); got != 43654.84 {
		t.Errorf("round2(43654.839) = %v", got)
	}
	if got := round4(-2.61253); got != -2.6125 {
		t.Errorf("round4(-2.61253) = %v", got)
	}
}

func TestRewriteMergePreservingOrder_OnlyAppendsKey(t *testing.T) {
	raw := []byte("{\n  \"a\": 1,\n  \"b\": {\"x\": 2},\n  \"c\": \"hi\"\n}\n")
	pointBytes, _ := json.Marshal(macroDataPoint{Symbol: "^TWII", Value: 100, ChangePct: 1.0, Timestamp: 1234})
	out, err := rewriteMergePreservingOrder(raw, "taiex", pointBytes)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	ia := strings.Index(s, `"a"`)
	ib := strings.Index(s, `"b"`)
	ic := strings.Index(s, `"c"`)
	it := strings.Index(s, `"taiex"`)
	if !(ia < ib && ib < ic && ic < it) {
		t.Errorf("key order broken: a=%d b=%d c=%d taiex=%d in %s", ia, ib, ic, it, s)
	}
	if strings.Count(s, `"taiex"`) != 1 {
		t.Errorf("taiex appears %d times, want 1", strings.Count(s, `"taiex"`))
	}
	if !strings.Contains(s, "  \"a\": 1,") {
		t.Errorf("expected indented a:1, got: %s", s)
	}
}

func TestRewriteMergePreservingOrder_SingleKeyObject(t *testing.T) {
	raw := []byte("{\n  \"only\": 1\n}\n")
	pointBytes, _ := json.Marshal(macroDataPoint{Symbol: "^TWII", Value: 100, ChangePct: 1.0, Timestamp: 1234})
	out, err := rewriteMergePreservingOrder(raw, "taiex", pointBytes)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "},") {
		t.Errorf("single-key object should not have trailing comma before closing brace: %s", s)
	}
	if strings.Count(s, `"taiex"`) != 1 {
		t.Errorf("taiex appears %d times, want 1", strings.Count(s, `"taiex"`))
	}
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
