package marketdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewTWSEMarginBalanceProvider(t *testing.T) {
	p := NewTWSEMarginBalanceProvider("")
	if p.Name() != "twse_margin_balance" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.storageDir != "" {
		t.Fatalf("expected empty storageDir, got %s", p.storageDir)
	}
}

func TestTWSEMarginBalanceProvider_SaveMargin(t *testing.T) {
	dir := t.TempDir()
	p := NewTWSEMarginBalanceProvider(dir)

	if err := p.saveMargin("20260513", 3500.5, 1.25); err != nil {
		t.Fatalf("saveMargin failed: %v", err)
	}

	fpath := filepath.Join(dir, "20260513_margin.json")
	data, err := os.ReadFile(fpath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if result["date"] != "20260513" {
		t.Fatalf("unexpected date: %v", result["date"])
	}
	if result["margin_balance"] != 3500.5 {
		t.Fatalf("unexpected margin_balance: %v", result["margin_balance"])
	}
	if result["change_pct"] != 1.25 {
		t.Fatalf("unexpected change_pct: %v", result["change_pct"])
	}
}

func TestTWSEMarginBalanceProvider_SaveMargin_EmptyDir(t *testing.T) {
	p := NewTWSEMarginBalanceProvider("")

	if err := p.saveMargin("20260513", 3500.5, 1.25); err != nil {
		t.Fatalf("saveMargin with empty dir should not error: %v", err)
	}
}
