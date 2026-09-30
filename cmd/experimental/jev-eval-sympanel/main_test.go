package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFixture builds quotes/industry/netbuy JSONL for nSymbols over nDates
// sessions, deterministic values (no randomness) so the panel is reproducible.
func writeFixture(t *testing.T, dir string, nSymbols, nDates int) (string, string, string) {
	t.Helper()
	qPath := filepath.Join(dir, "quotes.jsonl")
	iPath := filepath.Join(dir, "industry.jsonl")
	nPath := filepath.Join(dir, "netbuy.jsonl")
	qf, _ := os.Create(qPath)
	nf, _ := os.Create(nPath)
	inf, _ := os.Create(iPath)
	defer func() { _ = qf.Close(); _ = nf.Close(); _ = inf.Close() }()
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for s := 0; s < nSymbols; s++ {
		sym := fmt.Sprintf("S%03d.TW", s)
		fmt.Fprintf(inf, "{\"symbol\":%q,\"industry_id\":%q}\n", sym, fmt.Sprintf("ind%d", s%3))
		close := 100.0 + float64(s)
		for d := 0; d < nDates; d++ {
			day := base.AddDate(0, 0, d)
			close *= 1 + (float64((s+d)%7)-3)/1000.0
			fmt.Fprintf(qf, "{\"date\":%q,\"symbol\":%q,\"close\":%.6f}\n", day.Format("2006-01-02"), sym, close)
			fmt.Fprintf(nf, "{\"date\":%q,\"symbol\":%q,\"net_buy\":%d}\n", day.Format("2006-01-02"), sym, (s*7+d)%1000)
		}
	}
	return qPath, iPath, nPath
}

func TestRunIsReproducible(t *testing.T) {
	dir := t.TempDir()
	q, ind, nb := writeFixture(t, dir, 6, 90)
	out1 := filepath.Join(dir, "p1.jsonl")
	out2 := filepath.Join(dir, "p2.jsonl")
	for _, out := range []string{out1, out2} {
		if err := run([]string{"-quotes", q, "-industry", ind, "-netbuy", nb, "-out", out, "-quiet"}, os.Stdout); err != nil {
			t.Fatalf("run: %v", err)
		}
	}
	b1, _ := os.ReadFile(out1)
	b2, _ := os.ReadFile(out2)
	if string(b1) != string(b2) {
		t.Fatal("panel is not reproducible for identical inputs")
	}
	if len(b1) == 0 {
		t.Fatal("empty panel")
	}
}

func TestRowsCarryContractFieldsAndPitHasNoForwardInfo(t *testing.T) {
	dir := t.TempDir()
	q, ind, nb := writeFixture(t, dir, 6, 90)
	out := filepath.Join(dir, "p.jsonl")
	if err := run([]string{"-quotes", q, "-industry", ind, "-netbuy", nb, "-out", out, "-quiet"}, os.Stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	body, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) < 10 {
		t.Fatalf("expected many rows, got %d", len(lines))
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &raw); err != nil {
		t.Fatalf("row is not JSON: %v", err)
	}
	for _, key := range []string{"date", "symbol", "industry_id", "hold_days", "cost_rate", "pit", "backward", "forward", "baselines"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("row missing contract field %q: %s", key, lines[0])
		}
	}
	var pit map[string]json.RawMessage
	if err := json.Unmarshal(raw["pit"], &pit); err != nil {
		t.Fatalf("pit is not an object: %v", err)
	}
	// pit 採**白名單**：只允許 t 及以前的特徵（trailing returns/vol/距高/日報酬/history
	// ＋ 兩個 baseline 訊號）。白名單比「黑名單字串比對」精確：ret_20d_pct 是**落後**報酬 ✓，
	// 不該被誤判為洩漏 ✗；而任何新欄位都必須經過這裡才會被接受 ✓。
	allowed := map[string]bool{
		"ret_5d_pct": true, "ret_20d_pct": true, "ret_60d_pct": true, "vol_20d_pct": true,
		"dist_high_60d_pct": true, "daily_return_pct": true, "history_days": true,
		"momentum_20d_pct": true, "net_buy_3d_sum": true,
	}
	for key := range pit {
		if !allowed[key] {
			t.Fatalf("pit carries an unexpected (possibly forward-looking) key %q", key)
		}
	}
	for _, required := range []string{"ret_5d_pct", "ret_20d_pct", "vol_20d_pct", "dist_high_60d_pct", "history_days"} {
		if _, ok := pit[required]; !ok {
			t.Fatalf("pit missing %q", required)
		}
	}
	var fwd map[string]json.RawMessage
	if err := json.Unmarshal(raw["forward"], &fwd); err != nil {
		t.Fatalf("forward is not an object: %v", err)
	}
	if _, ok := fwd["hit"]; !ok {
		t.Fatal("forward.hit missing")
	}
}

func TestNetBuyOptional(t *testing.T) {
	dir := t.TempDir()
	q, ind, _ := writeFixture(t, dir, 4, 80)
	out := filepath.Join(dir, "p.jsonl")
	if err := run([]string{"-quotes", q, "-industry", ind, "-out", out, "-quiet"}, os.Stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	body, _ := os.ReadFile(out)
	line := strings.SplitN(string(body), "\n", 2)[0]
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Fatalf("row is not JSON: %v", err)
	}
	var base map[string]float64
	if err := json.Unmarshal(raw["baselines"], &base); err != nil {
		t.Fatalf("baselines is not an object: %v", err)
	}
	if _, ok := base["net_buy_3d"]; ok {
		t.Fatalf("baselines.net_buy_3d must be absent when -netbuy is not given: %s", line)
	}
	if strings.Contains(string(raw["pit"]), "net_buy_3d_sum") {
		t.Fatalf("pit.net_buy_3d_sum must be omitted (not 0) when -netbuy is absent: %s", line)
	}
}
