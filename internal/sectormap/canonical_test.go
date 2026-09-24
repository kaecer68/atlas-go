package sectormap

import (
	"slices"
	"testing"
)

func TestCanonicalL1IDs_IsThe20SectorTaxonomy(t *testing.T) {
	ids := CanonicalL1IDs()
	if len(ids) != 20 {
		t.Fatalf("canonical L1 count = %d, want 20", len(ids))
	}
	if !slices.IsSorted(ids) {
		t.Errorf("CanonicalL1IDs() must be sorted: %v", ids)
	}
	for _, id := range ids {
		if !IsCanonicalL1(id) {
			t.Errorf("IsCanonicalL1(%q) = false for an ID returned by CanonicalL1IDs()", id)
		}
		if IsCanonicalL2(id) {
			t.Errorf("%q is declared as both L1 and L2", id)
		}
	}
}

func TestCanonicalL2IDs_IsThe18SubIndustryTaxonomy(t *testing.T) {
	ids := CanonicalL2IDs()
	if len(ids) != 18 {
		t.Fatalf("canonical L2 count = %d, want 18", len(ids))
	}
	if !slices.IsSorted(ids) {
		t.Errorf("CanonicalL2IDs() must be sorted: %v", ids)
	}
}

func TestCanonicalIDs_NoDuplicates(t *testing.T) {
	ids := CanonicalIDs()
	if len(ids) != 38 {
		t.Fatalf("canonical ID count = %d, want 38", len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate canonical ID %q", id)
		}
		seen[id] = true
	}
}

func TestParentL1Of(t *testing.T) {
	for _, id := range CanonicalL1IDs() {
		got, ok := ParentL1Of(id)
		if !ok || got != id {
			t.Errorf("ParentL1Of(%q) = (%q, %v), want (%q, true) — an L1 sector is its own parent", id, got, ok, id)
		}
	}

	// Every L2 must declare a parent except etf_rotation, which is an
	// asset-class rotation bucket rather than an equity industry.
	for _, id := range CanonicalL2IDs() {
		parent, ok := ParentL1Of(id)
		if id == "etf_rotation" {
			if ok {
				t.Errorf("ParentL1Of(etf_rotation) = (%q, true), want no parent: it is an asset-class bucket", parent)
			}
			continue
		}
		if !ok {
			t.Errorf("ParentL1Of(%q) has no declared L1 parent", id)
			continue
		}
		if !IsCanonicalL1(parent) {
			t.Errorf("ParentL1Of(%q) = %q which is not a canonical L1 sector", id, parent)
		}
	}

	if _, ok := ParentL1Of("not_a_sector"); ok {
		t.Error("ParentL1Of(unknown) must report false")
	}
}

func TestL2ParentL1Table_CoversEveryMappableL2(t *testing.T) {
	table := L2ParentL1Table()
	for _, id := range CanonicalL2IDs() {
		if id == "etf_rotation" {
			if _, declared := table[id]; declared {
				t.Error("etf_rotation must not have a declared L1 parent")
			}
			continue
		}
		if _, declared := table[id]; !declared {
			t.Errorf("L2 %q is missing from the declared L2→L1 parent table", id)
		}
	}
	// The returned table must be a copy: mutation must not leak into the package.
	table["etf_rotation"] = "electronics"
	if _, ok := ParentL1Of("etf_rotation"); ok {
		t.Error("L2ParentL1Table() returned an alias, not a copy")
	}
}
