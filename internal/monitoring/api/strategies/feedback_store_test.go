package strategies

import (
	"path/filepath"
	"testing"
)

func TestFeedbackStore_WriteAccumulates(t *testing.T) {
	store := NewFeedbackStore(t.TempDir())
	if err := store.Write(Record{StrategyID: "L4-01", TotalTests: 50, TotalHits: 30}); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(Record{StrategyID: "L4-01", TotalTests: 10, TotalHits: 6}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Load("L4-01")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.TotalTests != 60 {
		t.Errorf("TotalTests=%d, want 60 (accumulated)", got.TotalTests)
	}
	if got.TotalHits != 36 {
		t.Errorf("TotalHits=%d, want 36", got.TotalHits)
	}
	want := float64(36) / float64(60)
	if got.HitRate != want {
		t.Errorf("HitRate=%f, want %f", got.HitRate, want)
	}
}

func TestFeedbackStore_LoadMissingIsNotAnError(t *testing.T) {
	store := NewFeedbackStore(t.TempDir())
	_, ok, err := store.Load("missing")
	if err != nil {
		t.Fatalf("missing strategy should not error: %v", err)
	}
	if ok {
		t.Error("expected ok=false")
	}
}

func TestFeedbackStore_PathEscapeDefended(t *testing.T) {
	store := NewFeedbackStore(t.TempDir())
	// "../etc" must not write outside the store root.
	if err := store.Write(Record{StrategyID: "../etc/passwd", TotalTests: 1, TotalHits: 1}); err != nil {
		t.Fatal(err)
	}
	// The resolved file must live inside t.TempDir().
	_ = filepath.Join(store.root, ".._.._etc_passwd.json")
}