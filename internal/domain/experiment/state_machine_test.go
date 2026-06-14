package experiment

import "testing"

func TestCanTransitionExperimentStatus(t *testing.T) {
	tests := []struct {
		name string
		from ExperimentStatus
		to   ExperimentStatus
		want bool
	}{
		// same status
		{"same planned", ExperimentPlanned, ExperimentPlanned, true},
		{"same running", ExperimentRunning, ExperimentRunning, true},
		{"same accepted", ExperimentAccepted, ExperimentAccepted, true},
		{"same rejected", ExperimentRejected, ExperimentRejected, true},
		{"same expired", ExperimentExpired, ExperimentExpired, true},

		// from empty (zero-value / initial)
		{"empty to planned", "", ExperimentPlanned, true},
		{"empty to running", "", ExperimentRunning, true},
		{"empty to accepted", "", ExperimentAccepted, false},
		{"empty to rejected", "", ExperimentRejected, false},
		{"empty to expired", "", ExperimentExpired, false},

		// from planned
		{"planned to running", ExperimentPlanned, ExperimentRunning, true},
		{"planned to rejected", ExperimentPlanned, ExperimentRejected, true},
		{"planned to expired", ExperimentPlanned, ExperimentExpired, true},
		{"planned to accepted", ExperimentPlanned, ExperimentAccepted, false},
		{"planned to planned", ExperimentPlanned, ExperimentPlanned, true},

		// from running
		{"running to accepted", ExperimentRunning, ExperimentAccepted, true},
		{"running to rejected", ExperimentRunning, ExperimentRejected, true},
		{"running to expired", ExperimentRunning, ExperimentExpired, true},
		{"running to planned", ExperimentRunning, ExperimentPlanned, false},
		{"running to running", ExperimentRunning, ExperimentRunning, true},

		// from accepted (terminal)
		{"accepted to planned", ExperimentAccepted, ExperimentPlanned, false},
		{"accepted to running", ExperimentAccepted, ExperimentRunning, false},
		{"accepted to rejected", ExperimentAccepted, ExperimentRejected, false},
		{"accepted to expired", ExperimentAccepted, ExperimentExpired, false},

		// from rejected (terminal)
		{"rejected to planned", ExperimentRejected, ExperimentPlanned, false},
		{"rejected to running", ExperimentRejected, ExperimentRunning, false},
		{"rejected to accepted", ExperimentRejected, ExperimentAccepted, false},
		{"rejected to expired", ExperimentRejected, ExperimentExpired, false},

		// from expired (terminal)
		{"expired to planned", ExperimentExpired, ExperimentPlanned, false},
		{"expired to running", ExperimentExpired, ExperimentRunning, false},
		{"expired to accepted", ExperimentExpired, ExperimentAccepted, false},
		{"expired to rejected", ExperimentExpired, ExperimentRejected, false},

		// unknown status
		{"unknown to planned", ExperimentStatus("unknown"), ExperimentPlanned, false},
		{"unknown to running", ExperimentStatus("unknown"), ExperimentRunning, false},
		{"unknown to accepted", ExperimentStatus("unknown"), ExperimentAccepted, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanTransitionExperimentStatus(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("CanTransitionExperimentStatus(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTransitionExperimentStatus(t *testing.T) {
	t.Run("nil record returns error", func(t *testing.T) {
		err := TransitionExperimentStatus(nil, ExperimentRunning)
		if err == nil {
			t.Fatal("expected error for nil record, got nil")
		}
	})

	t.Run("valid transition mutates status", func(t *testing.T) {
		record := &ExperimentRecord{Status: ExperimentPlanned}
		err := TransitionExperimentStatus(record, ExperimentRunning)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Status != ExperimentRunning {
			t.Errorf("expected status %q, got %q", ExperimentRunning, record.Status)
		}
	})

	t.Run("same status is allowed", func(t *testing.T) {
		record := &ExperimentRecord{Status: ExperimentRunning}
		err := TransitionExperimentStatus(record, ExperimentRunning)
		if err != nil {
			t.Fatalf("unexpected error for same-status transition: %v", err)
		}
		if record.Status != ExperimentRunning {
			t.Errorf("expected status %q, got %q", ExperimentRunning, record.Status)
		}
	})

	t.Run("invalid transition returns error and preserves status", func(t *testing.T) {
		record := &ExperimentRecord{Status: ExperimentAccepted}
		err := TransitionExperimentStatus(record, ExperimentRunning)
		if err == nil {
			t.Fatal("expected error for invalid transition, got nil")
		}
		if record.Status != ExperimentAccepted {
			t.Errorf("expected status to be preserved as %q, got %q", ExperimentAccepted, record.Status)
		}
	})

	t.Run("empty to planned is valid", func(t *testing.T) {
		record := &ExperimentRecord{Status: ""}
		err := TransitionExperimentStatus(record, ExperimentPlanned)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Status != ExperimentPlanned {
			t.Errorf("expected status %q, got %q", ExperimentPlanned, record.Status)
		}
	})

	t.Run("empty to running is valid", func(t *testing.T) {
		record := &ExperimentRecord{Status: ""}
		err := TransitionExperimentStatus(record, ExperimentRunning)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Status != ExperimentRunning {
			t.Errorf("expected status %q, got %q", ExperimentRunning, record.Status)
		}
	})

	t.Run("running to accepted is valid", func(t *testing.T) {
		record := &ExperimentRecord{Status: ExperimentRunning}
		err := TransitionExperimentStatus(record, ExperimentAccepted)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Status != ExperimentAccepted {
			t.Errorf("expected status %q, got %q", ExperimentAccepted, record.Status)
		}
	})

	t.Run("running to rejected is valid", func(t *testing.T) {
		record := &ExperimentRecord{Status: ExperimentRunning}
		err := TransitionExperimentStatus(record, ExperimentRejected)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Status != ExperimentRejected {
			t.Errorf("expected status %q, got %q", ExperimentRejected, record.Status)
		}
	})

	t.Run("running to expired is valid", func(t *testing.T) {
		record := &ExperimentRecord{Status: ExperimentRunning}
		err := TransitionExperimentStatus(record, ExperimentExpired)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record.Status != ExperimentExpired {
			t.Errorf("expected status %q, got %q", ExperimentExpired, record.Status)
		}
	})
}
