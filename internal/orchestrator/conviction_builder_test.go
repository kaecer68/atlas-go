package orchestrator

import (
	"testing"
)

func TestNewConvictionBuilder(t *testing.T) {
	tests := []struct {
		name      string
		base      int
		floor     int
		wantBase  int
		wantFloor int
		wantFinal int
		wantSteps int
	}{
		{"basic values", 50, 20, 50, 20, 50, 0},
		{"zero base", 0, 10, 0, 10, 0, 0},
		{"negative floor", 30, -5, 30, -5, 30, 0},
		{"large values", 100, 80, 100, 80, 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newConvictionBuilder(tt.base, tt.floor)
			if b.base != tt.wantBase {
				t.Errorf("base = %d, want %d", b.base, tt.wantBase)
			}
			if b.floor != tt.wantFloor {
				t.Errorf("floor = %d, want %d", b.floor, tt.wantFloor)
			}
			if b.final != tt.wantFinal {
				t.Errorf("final = %d, want %d", b.final, tt.wantFinal)
			}
			if len(b.steps) != tt.wantSteps {
				t.Errorf("steps len = %d, want %d", len(b.steps), tt.wantSteps)
			}
		})
	}
}

func TestConvictionBuilderAdd(t *testing.T) {
	b := newConvictionBuilder(50, 20)

	b.add("rule1", 10, "reason1")
	if b.final != 60 {
		t.Errorf("after add +10: final = %d, want 60", b.final)
	}
	if len(b.steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(b.steps))
	}
	if b.steps[0].Rule != "rule1" || b.steps[0].Delta != 10 || b.steps[0].Reason != "reason1" {
		t.Errorf("step mismatch: %+v", b.steps[0])
	}

	b.add("rule2", -5, "reason2")
	if b.final != 55 {
		t.Errorf("after add -5: final = %d, want 55", b.final)
	}
	if len(b.steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(b.steps))
	}
}

func TestConvictionBuilderCap(t *testing.T) {
	t.Run("capped when above max", func(t *testing.T) {
		b := newConvictionBuilder(50, 20)
		b.add("test", 30, "test add")
		b.cap(60)

		if b.final != 60 {
			t.Errorf("final = %d, want 60 (capped)", b.final)
		}
		if len(b.steps) != 2 {
			t.Fatalf("steps len = %d, want 2 (original + cap)", len(b.steps))
		}
		capStep := b.steps[1]
		if capStep.Rule != "cap" || capStep.Delta != -20 {
			t.Errorf("cap step mismatch: %+v", capStep)
		}
	})

	t.Run("no change when below max", func(t *testing.T) {
		b := newConvictionBuilder(50, 20)
		b.add("test", 5, "test add")
		b.cap(70)

		if b.final != 55 {
			t.Errorf("final = %d, want 55 (unchanged)", b.final)
		}
		if len(b.steps) != 1 {
			t.Fatalf("steps len = %d, want 1", len(b.steps))
		}
	})

	t.Run("cap at exact value", func(t *testing.T) {
		b := newConvictionBuilder(50, 20)
		b.add("test", 10, "test add")
		b.cap(60)

		if b.final != 60 {
			t.Errorf("final = %d, want 60", b.final)
		}
		if len(b.steps) != 1 {
			t.Errorf("steps len = %d, want 1 (no cap step when final == max)", len(b.steps))
		}
	})
}

func TestConvictionBuilderFloorCheck(t *testing.T) {
	t.Run("raises when below floor", func(t *testing.T) {
		b := newConvictionBuilder(50, 30)
		b.add("test", -30, "test add")

		result := b.floorCheck()
		if result != false {
			t.Errorf("floorCheck = %v, want false", result)
		}
		if b.final != 20 {
			t.Errorf("final = %d, want 20 (original after add)", b.final)
		}
		if len(b.steps) != 1 {
			t.Fatalf("steps len = %d, want 1 (no floor step added)", len(b.steps))
		}
	})

	t.Run("returns true when above floor", func(t *testing.T) {
		b := newConvictionBuilder(50, 20)
		b.add("test", 10, "test add")

		result := b.floorCheck()
		if result != true {
			t.Errorf("floorCheck = %v, want true", result)
		}
		if b.final != 60 {
			t.Errorf("final = %d, want 60", b.final)
		}
		if len(b.steps) != 1 {
			t.Fatalf("steps len = %d, want 1", len(b.steps))
		}
	})

	t.Run("floor at exact value", func(t *testing.T) {
		b := newConvictionBuilder(50, 30)
		b.add("test", -20, "test add")

		result := b.floorCheck()
		if result != true {
			t.Errorf("floorCheck = %v, want true (at floor)", result)
		}
		if b.final != 30 {
			t.Errorf("final = %d, want 30 (at floor)", b.final)
		}
		if len(b.steps) != 1 {
			t.Errorf("steps len = %d, want 1 (no floor step added)", len(b.steps))
		}
	})
}

func TestConvictionBuilderBuild(t *testing.T) {
	b := newConvictionBuilder(50, 20)
	b.add("rule1", 15, "reason1")
	b.add("rule2", -5, "reason2")

	final, breakdown := b.build()

	if final != 60 {
		t.Errorf("final = %d, want 60", final)
	}
	if breakdown.Base != 50 {
		t.Errorf("breakdown.Base = %d, want 50", breakdown.Base)
	}
	if breakdown.Floor != 20 {
		t.Errorf("breakdown.Floor = %d, want 20", breakdown.Floor)
	}
	if breakdown.Final != 60 {
		t.Errorf("breakdown.Final = %d, want 60", breakdown.Final)
	}
	if len(breakdown.Steps) != 2 {
		t.Errorf("breakdown.Steps len = %d, want 2", len(breakdown.Steps))
	}
}

func TestConvictionBuilderCombinedWorkflow(t *testing.T) {
	b := newConvictionBuilder(50, 30)

	b.add("positive1", 20, "first positive")
	b.add("negative1", -15, "first negative")

	b.cap(60)

	b.floorCheck()

	final, breakdown := b.build()

	if final != 55 {
		t.Errorf("final = %d, want 55", final)
	}
	if breakdown.Base != 50 {
		t.Errorf("breakdown.Base = %d, want 50", breakdown.Base)
	}
	if breakdown.Floor != 30 {
		t.Errorf("breakdown.Floor = %d, want 30", breakdown.Floor)
	}

	if len(breakdown.Steps) != 2 {
		t.Errorf("steps len = %d, want 2", len(breakdown.Steps))
	}
}

func TestConvictionBuilderCombinedWorkflowFloorTriggered(t *testing.T) {
	b := newConvictionBuilder(50, 40)

	b.add("positive1", 10, "first positive")
	b.add("negative1", -30, "first negative")

	result := b.floorCheck()
	if result != false {
		t.Errorf("floorCheck = %v, want false", result)
	}

	b.cap(60)

	final, breakdown := b.build()

	if final != 30 {
		t.Errorf("final = %d, want 30 (floorCheck does not mutate on false)", final)
	}

	if len(breakdown.Steps) != 2 {
		t.Errorf("steps len = %d, want 2", len(breakdown.Steps))
	}
}
