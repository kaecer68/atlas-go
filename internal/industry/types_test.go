package industry

import (
	"testing"
)

func TestDefaultClassification(t *testing.T) {
	tree := DefaultClassification()

	// Test Level 1 count
	level1 := tree.GetLevel1()
	if len(level1) != 11 {
		t.Errorf("expected 10 level-1 industries, got %d", len(level1))
	}

	// Test specific level 1 industry
	semi, ok := tree.GetSegment("semiconductor")
	if !ok {
		t.Fatal("semiconductor segment not found")
	}
	if semi.Name != "半導體" {
		t.Errorf("expected name '半導體', got '%s'", semi.Name)
	}
	if semi.Level != Level1 {
		t.Errorf("expected level 1, got %d", semi.Level)
	}

	// Test Level 2 children
	children := tree.GetChildren("semiconductor")
	if len(children) != 6 {
		t.Errorf("expected 6 semiconductor sub-industries, got %d", len(children))
	}

	// Test Level 3 children
	foundryChildren := tree.GetChildren("foundry")
	if len(foundryChildren) != 2 {
		t.Errorf("expected 2 foundry sub-categories, got %d", len(foundryChildren))
	}

	// Test path
	path := tree.GetPath("advanced_process")
	if len(path) != 3 {
		t.Errorf("expected path length 3, got %d", len(path))
	}
	if path[0].ID != "semiconductor" {
		t.Errorf("expected root semiconductor, got %s", path[0].ID)
	}
	if path[1].ID != "foundry" {
		t.Errorf("expected middle foundry, got %s", path[1].ID)
	}
	if path[2].ID != "advanced_process" {
		t.Errorf("expected leaf advanced_process, got %s", path[2].ID)
	}
}

func TestClassificationTreeValidate(t *testing.T) {
	tree := NewClassificationTree()

	// Valid tree
	tree.AddSegment(&IndustrySegment{
		ID:    "test1",
		Name:  "Test 1",
		Level: Level1,
	})
	tree.AddSegment(&IndustrySegment{
		ID:       "test2",
		Name:     "Test 2",
		Level:    Level2,
		ParentID: "test1",
	})

	if err := tree.Validate(); err != nil {
		t.Errorf("expected valid tree, got error: %v", err)
	}

	// Invalid: missing parent
	tree.AddSegment(&IndustrySegment{
		ID:       "test3",
		Name:     "Test 3",
		Level:    Level2,
		ParentID: "nonexistent",
	})

	if err := tree.Validate(); err == nil {
		t.Error("expected error for missing parent, got nil")
	}
}

func TestIndustrySegmentAttributes(t *testing.T) {
	tree := DefaultClassification()

	// Test risk characteristics
	robotics, ok := tree.GetSegment("robotics")
	if !ok {
		t.Fatal("robotics segment not found")
	}
	if robotics.Cyclicality != CyclicalityMedium {
		t.Errorf("expected robotics cyclicality medium, got %s", robotics.Cyclicality)
	}
	if robotics.TechnologyIntensity != TechIntensityHigh {
		t.Errorf("expected robotics tech intensity high, got %s", robotics.TechnologyIntensity)
	}

	// Test geographic exposure
	financials, ok := tree.GetSegment("financials")
	if !ok {
		t.Fatal("financials segment not found")
	}
	if financials.GeographicExposure != ExposureDomestic {
		t.Errorf("expected financials domestic exposure, got %s", financials.GeographicExposure)
	}
}

func TestClassificationWeights(t *testing.T) {
	tree := DefaultClassification()

	// Test level 1 weights sum to approximately 1.0
	level1 := tree.GetLevel1()
	totalWeight := 0.0
	for _, seg := range level1 {
		totalWeight += seg.Weight
	}

	// Allow small floating point error
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected level 1 weights sum to ~1.0, got %f", totalWeight)
	}
}

func TestGetChildrenEmpty(t *testing.T) {
	tree := DefaultClassification()

	// Test leaf node has no children
	children := tree.GetChildren("advanced_process")
	if len(children) != 0 {
		t.Errorf("expected 0 children for leaf node, got %d", len(children))
	}
}

func TestGetPathRoot(t *testing.T) {
	tree := DefaultClassification()

	// Test path for level 1 (root only)
	path := tree.GetPath("semiconductor")
	if len(path) != 1 {
		t.Errorf("expected path length 1 for root, got %d", len(path))
	}
}
