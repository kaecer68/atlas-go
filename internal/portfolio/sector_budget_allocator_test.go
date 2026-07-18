package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

func TestSectorBudgetAllocator_Normal(t *testing.T) {
	alloc := NewSectorBudgetAllocator()
	snap := &sectorallocation.SectorAllocationSnapshot{
		Target:        map[industry.SectorID]float64{"semiconductor": 0.40, "electronics": 0.30, "financials": 0.30},
		EffectiveFrom: "2026-07-21",
		MutationReceipt: &sectorallocation.MutationReceipt{
			ReceiptID: "abc123",
		},
	}

	result, err := alloc.Allocate(snap, 1000000.0)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if result.PolicyReceiptID != "abc123" {
		t.Errorf("receipt mismatch: %s", result.PolicyReceiptID)
	}
	if result.PortfolioValue != 1000000.0 {
		t.Errorf("portfolio value mismatch: %f", result.PortfolioValue)
	}

	expect := map[industry.SectorID]float64{
		"semiconductor": 400000.0,
		"electronics":   300000.0,
		"financials":    300000.0,
	}
	for s, w := range expect {
		if result.Budgets[s] != w {
			t.Errorf("sector %s: got %f, want %f", s, result.Budgets[s], w)
		}
	}
}

func TestSectorBudgetAllocator_NilSnapshot(t *testing.T) {
	alloc := NewSectorBudgetAllocator()
	_, err := alloc.Allocate(nil, 1000.0)
	if err == nil {
		t.Fatal("expected error for nil snapshot")
	}
}

func TestSectorBudgetAllocator_EmptyTarget(t *testing.T) {
	alloc := NewSectorBudgetAllocator()
	snap := &sectorallocation.SectorAllocationSnapshot{
		Target: map[industry.SectorID]float64{},
	}
	_, err := alloc.Allocate(snap, 1000.0)
	if err == nil {
		t.Fatal("expected error for empty target")
	}
}

func TestSectorBudgetAllocator_BadSum(t *testing.T) {
	alloc := NewSectorBudgetAllocator()
	snap := &sectorallocation.SectorAllocationSnapshot{
		Target: map[industry.SectorID]float64{"a": 0.30, "b": 0.30}, // sum = 0.60
	}
	_, err := alloc.Allocate(snap, 1000.0)
	if err == nil {
		t.Fatal("expected error for bad sum")
	}
}

func TestSectorBudgetAllocator_Minimal(t *testing.T) {
	alloc := NewSectorBudgetAllocator()
	snap := &sectorallocation.SectorAllocationSnapshot{
		Target: map[industry.SectorID]float64{"semiconductor": 1.0},
	}
	result, err := alloc.Allocate(snap, 100.0)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if result.Budgets["semiconductor"] != 100.0 {
		t.Errorf("budget: got %f, want 100.0", result.Budgets["semiconductor"])
	}
}
