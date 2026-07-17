package marketdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGovernmentFlowProvider_Latest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20260714.json"), []byte(`{"date":"20260714","total_net":100000000,"source":"operator-imported"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260715.json"), []byte(`{"date":"20260715","total_net":-50000000,"source":"broker-aggregate","raw_url":"https://example.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "garbage.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewGovernmentFlowProvider(dir)
	r, ok, err := p.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if !ok {
		t.Fatal("expected a reading")
	}
	if r.Date != "20260715" {
		t.Errorf("date=%s, want 20260715", r.Date)
	}
	if r.TotalNet != -50000000 {
		t.Errorf("total_net=%d, want -50000000", r.TotalNet)
	}
	if r.Source != "broker-aggregate" {
		t.Errorf("source=%s", r.Source)
	}
}

func TestGovernmentFlowProvider_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	p := NewGovernmentFlowProvider(dir)
	_, ok, err := p.Latest()
	if err != nil {
		t.Fatalf("empty dir should not error: %v", err)
	}
	if ok {
		t.Error("expected no reading for empty dir")
	}
}

func TestGovernmentFlowProvider_RejectsUnknownSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20260716.json"), []byte(`{"total_net":1,"source":"made-up"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewGovernmentFlowProvider(dir)
	_, _, err := p.Latest()
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestGovernmentFlowProvider_NoDirIsNotAnError(t *testing.T) {
	p := NewGovernmentFlowProvider(filepath.Join(t.TempDir(), "nonexistent"))
	_, ok, err := p.Latest()
	if err != nil {
		t.Errorf("missing dir should be silent: %v", err)
	}
	if ok {
		t.Error("expected no reading")
	}
}