package eventlogic

import (
	"sync"
	"testing"
)

func TestRegistryCRUD(t *testing.T) {
	reg := NewRegistry()
	initial := reg.Count()

	// Add
	rule := NewEventRule(
		"test-rule-1",
		"test pattern",
		[]Condition{{Field: "Test.Field", Operator: "gt", Value: 10.0}},
		[]string{"tech"},
		DirUp,
	)
	if err := reg.Add(rule); err != nil {
		t.Fatalf("unexpected error adding rule: %v", err)
	}
	if reg.Count() != initial+1 {
		t.Errorf("after Add, Count() = %d, want %d", reg.Count(), initial+1)
	}

	// Get
	got, found := reg.GetByID("test-rule-1")
	if !found {
		t.Fatal("GetByID returned false for existing rule")
	}
	if got.ID != "test-rule-1" {
		t.Errorf("GetByID ID = %q, want %q", got.ID, "test-rule-1")
	}

	// Update
	rule.Direction = DirDown
	if err := reg.Update(rule); err != nil {
		t.Fatalf("unexpected error updating rule: %v", err)
	}
	updated, _ := reg.GetByID("test-rule-1")
	if updated.Direction != DirDown {
		t.Errorf("after Update, Direction = %q, want %q", updated.Direction, DirDown)
	}

	// Delete
	if err := reg.Delete("test-rule-1"); err != nil {
		t.Fatalf("unexpected error deleting rule: %v", err)
	}
	if reg.Count() != initial {
		t.Errorf("after Delete, Count() = %d, want %d", reg.Count(), initial)
	}
	_, found = reg.GetByID("test-rule-1")
	if found {
		t.Error("GetByID returned true for deleted rule")
	}
}

func TestSeedRules(t *testing.T) {
	reg := NewRegistry()
	count := reg.Count()
	if count != 6 {
		t.Fatalf("expected 6 seed rules, got %d", count)
	}

	tests := []struct {
		name           string
		pattern        string
		wantConds      int
		wantDir        string
		wantFirstSector string
	}{
		{
			name:           "sox-foreignflow-semiconductor",
			wantConds:      2,
			wantDir:        DirUp,
			wantFirstSector: "semiconductor",
		},
		{
			name:           "usmarket-taiwan-lag",
			wantConds:      1,
			wantDir:        DirUp,
			wantFirstSector: "semiconductor",
		},
		{
			name:           "dxy-strong-export-boost",
			wantConds:      2,
			wantDir:        DirUp,
			wantFirstSector: "shipping",
		},
		{
			name:           "foreign-outflow-bearish",
			wantConds:      2,
			wantDir:        DirDown,
			wantFirstSector: "*",
		},
		{
			name:           "nvidia-earnings-ai-chain",
			wantConds:      1,
			wantDir:        DirUp,
			wantFirstSector: "ai_supply_chain",
		},
		{
			name:           "usd-twd-managed-float",
			wantConds:      1,
			wantDir:        DirUp,
			wantFirstSector: "electronics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, found := reg.GetByID(tt.name)
			if !found {
				t.Fatalf("seed rule %q not found", tt.name)
			}
			if len(rule.Conditions) != tt.wantConds {
				t.Errorf("conditions count = %d, want %d", len(rule.Conditions), tt.wantConds)
			}
			if rule.Direction != tt.wantDir {
				t.Errorf("direction = %q, want %q", rule.Direction, tt.wantDir)
			}
			if rule.Status != StatusActive {
				t.Errorf("status = %q, want %q", rule.Status, StatusActive)
			}
			if rule.HitRate != 0.5 {
				t.Errorf("hit_rate = %f, want 0.5", rule.HitRate)
			}
			if rule.ConfidenceSource != SourceManual {
				t.Errorf("confidence_source = %q, want %q", rule.ConfidenceSource, SourceManual)
			}
			if len(rule.AffectedSectors) == 0 {
				t.Error("affected_sectors is empty")
			}
			if rule.AffectedSectors[0] != tt.wantFirstSector {
				t.Errorf("first affected sector = %q, want %q", rule.AffectedSectors[0], tt.wantFirstSector)
			}
		})
	}
}

func TestListActive(t *testing.T) {
	reg := NewRegistry()

	// All seed rules are active, so active count should equal total
	activeBefore := reg.CountActive()
	allBefore := reg.Count()
	if activeBefore != allBefore {
		t.Fatalf("expected all %d rules active, got %d active", allBefore, activeBefore)
	}

	// Add an expired rule
	expired := NewEventRule(
		"expired-rule",
		"should not appear in active list",
		nil,
		[]string{"banking"},
		DirDown,
	)
	expired.Status = StatusExpired
	if err := reg.Add(expired); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	active := reg.ListActive()
	if len(active) != allBefore {
		t.Errorf("ListActive() = %d rules, want %d", len(active), allBefore)
	}
	for _, r := range active {
		if r.Status != StatusActive {
			t.Errorf("ListActive() included rule %q with status %q", r.ID, r.Status)
		}
	}
}

func TestListExpired(t *testing.T) {
	reg := NewRegistry()

	// Initially no expired rules
	expired := reg.ListExpired()
	if len(expired) != 0 {
		t.Errorf("expected 0 expired rules initially, got %d", len(expired))
	}

	// Add an expired rule
	rule := NewEventRule(
		"expired-rule-2",
		"expired pattern",
		nil,
		[]string{"banking"},
		DirDown,
	)
	rule.Status = StatusExpired
	if err := reg.Add(rule); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expired = reg.ListExpired()
	if len(expired) != 1 {
		t.Errorf("ListExpired() = %d rules, want 1", len(expired))
	}
	if expired[0].ID != "expired-rule-2" {
		t.Errorf("ListExpired()[0].ID = %q, want %q", expired[0].ID, "expired-rule-2")
	}
}

func TestDuplicateAdd(t *testing.T) {
	reg := NewRegistry()

	rule := NewEventRule(
		"dup-rule",
		"duplicate pattern",
		nil,
		[]string{"tech"},
		DirUp,
	)
	if err := reg.Add(rule); err != nil {
		t.Fatalf("first Add should succeed: %v", err)
	}
	if err := reg.Add(rule); err == nil {
		t.Error("second Add should return error for duplicate ID")
	}
}

func TestGetNonExistent(t *testing.T) {
	reg := NewRegistry()

	_, found := reg.GetByID("nonexistent-rule")
	if found {
		t.Error("GetByID should return false for nonexistent rule")
	}
}

func TestUpdateNonExistent(t *testing.T) {
	reg := NewRegistry()

	rule := NewEventRule(
		"nonexistent",
		"will not be found",
		nil,
		[]string{"tech"},
		DirUp,
	)
	if err := reg.Update(rule); err == nil {
		t.Error("Update should return error for nonexistent rule")
	}
}

func TestDeleteNonExistent(t *testing.T) {
	reg := NewRegistry()

	if err := reg.Delete("nonexistent"); err == nil {
		t.Error("Delete should return error for nonexistent rule")
	}
}

func TestConcurrentAccess(t *testing.T) {
	reg := NewRegistry()
	count := reg.Count()

	var wg sync.WaitGroup
	const numWorkers = 20

	// Concurrent adds
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "concurrent-rule-" + string(rune('a'+n%26)) + string(rune('0'+n/26))
			rule := NewEventRule(id, "concurrent pattern", nil, []string{"tech"}, DirUp)
			_ = reg.Add(rule) // ignore errors from duplicates
		}(i)
	}

	// Concurrent reads while writes happen
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reg.List()
			_ = reg.Count()
		}()
	}

	wg.Wait()

	// Verify registry is not corrupted
	finalCount := reg.Count()
	if finalCount < count {
		t.Errorf("final count %d < initial count %d, data loss suspected", finalCount, count)
	}
}

func TestCountActive(t *testing.T) {
	reg := NewRegistry()
	initialActive := reg.CountActive()

	// Add an expired rule
	expired := NewEventRule("expired-for-count", "test", nil, []string{"tech"}, DirUp)
	expired.Status = StatusExpired
	_ = reg.Add(expired)

	if reg.CountActive() != initialActive {
		t.Errorf("CountActive() = %d after adding expired, want %d", reg.CountActive(), initialActive)
	}
}