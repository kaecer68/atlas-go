package sectorallocation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

func TestFileClosureStore_StoreAndLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	snap := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30, "electronics": 0.20},
		Current:           map[industry.SectorID]float64{"semiconductor": 0.28, "electronics": 0.22},
		Delta:             map[industry.SectorID]float64{"semiconductor": +0.02, "electronics": -0.02},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}

	receipt, err := store.Store(snap)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if receipt == nil {
		t.Fatal("receipt is nil")
	}
	if receipt.ReceiptID == "" {
		t.Error("receipt ID is empty")
	}
	if receipt.SHA256 == "" {
		t.Error("sha256 is empty")
	}

	// Verify file exists
	if _, err := os.Stat(store.filePath()); os.IsNotExist(err) {
		t.Fatal("policy file not created")
	}

	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest == nil {
		t.Fatal("latest snapshot is nil")
	}
	if latest.AsOfTradingDate != "2026-07-17" {
		t.Errorf("as_of_trading_date: got %q, want %q", latest.AsOfTradingDate, "2026-07-17")
	}
	if latest.EffectiveFrom != "2026-07-18" {
		t.Errorf("effective_from: got %q, want %q", latest.EffectiveFrom, "2026-07-18")
	}
	if latest.MutationReceipt == nil {
		t.Fatal("mutation receipt is nil")
	}
	if !latest.Applied {
		t.Error("applied should be true after store")
	}
}

func TestFileClosureStore_LatestNone(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest != nil {
		t.Fatal("expected nil snapshot for empty store")
	}
}

func TestFileClosureStore_Consume(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	snap := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}

	receipt, err := store.Store(snap)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Before consume, should be available
	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected snapshot before consume")
	}

	// Consume it
	cr, err := store.Consume(receipt.ReceiptID, "session-001")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if cr == nil {
		t.Fatal("consumption receipt is nil")
	}
	if cr.FromReceiptID != receipt.ReceiptID {
		t.Errorf("from_receipt_id: got %q, want %q", cr.FromReceiptID, receipt.ReceiptID)
	}

	// After consume, Latest should return nil (no unconsumed snapshots)
	latest, err = store.Latest()
	if err != nil {
		t.Fatalf("Latest after consume: %v", err)
	}
	if latest != nil {
		t.Fatal("expected nil after all consumed")
	}
}

func TestFileClosureStore_StoreMultipleThenConsume(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	snap1 := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}
	snap2 := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-18",
		EffectiveFrom:     "2026-07-21",
		Target:            map[industry.SectorID]float64{"electronics": 0.25},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}

	r1, _ := store.Store(snap1)
	store.Store(snap2)

	// Latest should be snap2 (most recent)
	latest, _ := store.Latest()
	if latest.AsOfTradingDate != "2026-07-18" {
		t.Errorf("expected snap2, got as_of=%s", latest.AsOfTradingDate)
	}

	// Consume snap2
	store.Consume(latest.MutationReceipt.ReceiptID, "sess-2")

	// Latest should now be snap1
	latest, _ = store.Latest()
	if latest == nil {
		t.Fatal("expected snap1 after consuming snap2")
	}
	if latest.MutationReceipt.ReceiptID != r1.ReceiptID {
		t.Errorf("expected snap1 receipt, got %s", latest.MutationReceipt.ReceiptID)
	}

	// Consume snap1 too → nil
	store.Consume(r1.ReceiptID, "sess-1")
	latest, _ = store.Latest()
	if latest != nil {
		t.Fatal("expected nil after all consumed")
	}
}

func TestFileClosureStore_StoreEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	snap := SectorAllocationSnapshot{
		AsOfTradingDate: "2026-07-17",
		EffectiveFrom:   "2026-07-18",
		Target:          map[industry.SectorID]float64{},
		ModelVersion:    "1.0.0",
	}
	_, err := store.Store(snap)
	if err == nil {
		t.Fatal("expected error for empty target")
	}
}

func TestFileClosureStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	snap := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}
	receipt, _ := store.Store(snap)
	err := store.Delete(receipt.ReceiptID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestFileClosureStore_LatestSnapshot(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	// No snapshot → nil
	if snap := store.LatestSnapshot(); snap != nil {
		t.Fatal("expected nil")
	}

	snap := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}
	store.Store(snap)

	sn := store.LatestSnapshot()
	if sn == nil {
		t.Fatal("expected snapshot")
	}
	if sn.AsOfTradingDate != "2026-07-17" {
		t.Errorf("as_of mismatch: %s", sn.AsOfTradingDate)
	}
}

func TestFileClosureStore_Concurrent(t *testing.T) {
	dir := t.TempDir()
	store := NewFileClosureStore(dir)

	done := make(chan bool, 4)
	storeFn := func(prefix string) {
		for i := 0; i < 10; i++ {
			snap := SectorAllocationSnapshot{
				AsOfTradingDate:   prefix,
				EffectiveFrom:     prefix,
				Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
				ModelVersion:      "1.0.0",
				CalibrationStatus: "calibrating",
				WeightSource:      "heuristic",
			}
			store.Store(snap)
		}
		done <- true
	}

	for i := 0; i < 4; i++ {
		go storeFn(string(rune('A' + i)))
	}

	for i := 0; i < 4; i++ {
		<-done
	}

	// Should not panic or produce corrupt data
	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest after concurrent writes: %v", err)
	}
	if latest == nil {
		t.Fatal("expected at least one snapshot")
	}
}

func TestFileClosureStore_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	_ = filepath.Join(dir, closurePolicyFileName) // verify path construction compiles

	// Store via first instance
	s1 := NewFileClosureStore(dir)
	snap := SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}
	receipt, err := s1.Store(snap)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Read via second instance
	s2 := NewFileClosureStore(dir)
	latest, err := s2.Latest()
	if err != nil {
		t.Fatalf("Latest from second instance: %v", err)
	}
	if latest == nil {
		t.Fatal("expected snapshot from second instance")
	}
	if latest.MutationReceipt.ReceiptID != receipt.ReceiptID {
		t.Errorf("receipt mismatch across instances")
	}
}
