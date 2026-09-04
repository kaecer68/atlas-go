package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSnap(t *testing.T, path string) map[string]map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(raw, &outer); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]interface{}{}
	for k, v := range outer {
		var m map[string]interface{}
		if json.Unmarshal(v, &m) == nil {
			out[k] = m
		}
	}
	return out
}

func mustRun(t *testing.T, workdir, source string) {
	t.Helper()
	if _, err := run(workdir, source, "", "2024-07-01", time.Now().Format("2006-01-02"), false); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestMergeCapitalFlowFillsMissingOnly(t *testing.T) {
	root := t.TempDir()
	// Source files use both YYYYMMDD and YYYY-MM-DD names (both shapes exist
	// in data/state/capital_flow on production).
	write(t, filepath.Join(root, "data/state/capital_flow/20250502.json"), map[string]interface{}{
		"date": "2025-05-02", "foreign_investor_net": 2.82, "domestic_fund_net": 0, "dealer_net": -2.01,
	})
	write(t, filepath.Join(root, "data/state/capital_flow/20250505.json"), map[string]interface{}{
		"date": "20250505", "foreign_investor_net": -3.39, "domestic_fund_net": 0.49, "dealer_net": -7.78,
	})
	write(t, filepath.Join(root, "data/state/macro/2025-05-02.json"), map[string]interface{}{
		"taiex": map[string]interface{}{"symbol": "^TWII", "value": 23000.0, "change_pct": 0.5, "timestamp": 1},
	})
	// 2025-05-05 already has a non-zero foreign_investor_net: must not be overwritten.
	write(t, filepath.Join(root, "data/state/macro/2025-05-05.json"), map[string]interface{}{
		"foreign_investor_net": map[string]interface{}{"symbol": "TAIWAN_FOREIGN", "value": 9.9, "change_pct": 1, "timestamp": 2},
	})

	mustRun(t, root, "capital_flow")

	s2 := readSnap(t, filepath.Join(root, "data/state/macro/2025-05-02.json"))
	fi := s2["foreign_investor_net"]
	if fi == nil {
		t.Fatalf("foreign_investor_net not written: %v", s2)
	}
	if fi["symbol"] != "TAIWAN_FOREIGN" || fi["value"] != 2.82 {
		t.Fatalf("unexpected merged point: %v", fi)
	}
	if _, ok := s2["domestic_fund_net"]; ok {
		t.Fatalf("zero-value source must not create a point")
	}
	if dn := s2["dealer_net"]; dn == nil || dn["value"] != -2.01 {
		t.Fatalf("dealer_net not merged: %v", dn)
	}

	s5 := readSnap(t, filepath.Join(root, "data/state/macro/2025-05-05.json"))
	if got := s5["foreign_investor_net"]["value"]; got != 9.9 {
		t.Fatalf("existing non-zero foreign_investor_net overwritten: %v", s5)
	}
	// dealer_net on 05-05 is missing → should be filled while existing stays.
	if dn := s5["dealer_net"]; dn == nil || dn["value"] != -7.78 {
		t.Fatalf("dealer_net not merged on 05-05: %v", dn)
	}
}

func TestMergeTaifexOI(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "data/state/taifex_oi/2024-07-01.json"), map[string]interface{}{
		"date": "2024-07-01",
		"contracts": map[string]interface{}{
			"TX": map[string]interface{}{
				"date":    "2024-07-01",
				"foreign": map[string]interface{}{"oi_net": -25944},
			},
		},
	})
	write(t, filepath.Join(root, "data/state/macro/2024-07-01.json"), map[string]interface{}{
		"taiex": map[string]interface{}{"symbol": "^TWII", "value": 23000.0, "change_pct": 0.5, "timestamp": 1},
	})

	mustRun(t, root, "taifex_oi")

	s := readSnap(t, filepath.Join(root, "data/state/macro/2024-07-01.json"))
	oi := s["foreign_futures_oi_net"]
	if oi == nil || oi["symbol"] != "TX_FOREIGN_OI_NET" || oi["value"] != -25944.0 {
		t.Fatalf("foreign_futures_oi_net not merged: %v", oi)
	}
}

func TestMissingSnapshotSkipped(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "data/state/capital_flow/20250707.json"), map[string]interface{}{
		"date": "2025-07-07", "foreign_investor_net": 1.5,
	})
	mustRun(t, root, "capital_flow")
	if _, err := os.Stat(filepath.Join(root, "data/state/macro/2025-07-07.json")); !os.IsNotExist(err) {
		t.Fatalf("snapshot must not be created for a missing date")
	}
}
