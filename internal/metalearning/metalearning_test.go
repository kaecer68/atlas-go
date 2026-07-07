package metalearning

import (
	"os"
	"testing"
	"time"
)

func TestMetaLearnerPersistence(t *testing.T) {
	ml1 := NewMetaLearner(DefaultMetaLearningConfig())

	// Save state
	path := t.TempDir() + "/metalearner_state.json"
	if err := ml1.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into fresh MetaLearner
	ml2 := NewMetaLearner(DefaultMetaLearningConfig())
	if err := ml2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(ml2.Strategies()) == 0 {
		t.Fatal("expected strategies restored from persistence")
	}
	t.Logf("Restored %d strategies from disk", len(ml2.Strategies()))
}

func TestNewMetaLearner_DefaultConfig(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	if ml == nil {
		t.Fatal("expected NewMetaLearner to return non-nil")
	}
	if len(ml.Strategies()) == 0 {
		t.Fatal("expected initial population from default config")
	}
	t.Logf("Initial population: %d strategies", len(ml.Strategies()))
}

func TestMetaLearner_GetTopStrategies(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	top := ml.GetTopStrategies(3)
	if len(top) == 0 {
		t.Fatal("expected top strategies from initial population")
	}
	if len(top) > 3 {
		t.Errorf("expected at most 3, got %d", len(top))
	}
}

func TestMetaLearner_SaveLoad_Roundtrip(t *testing.T) {
	ml1 := NewMetaLearner(DefaultMetaLearningConfig())
	path := t.TempDir() + "/roundtrip.json"
	if err := ml1.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ml2 := NewMetaLearner(DefaultMetaLearningConfig())
	if err := ml2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(ml1.Strategies()) != len(ml2.Strategies()) {
		t.Errorf("strategy count mismatch: %d vs %d", len(ml1.Strategies()), len(ml2.Strategies()))
	}
}

func TestMetaLearner_Save_ErrorOnBogusPath(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	err := ml.Save("/nonexistent/dir/state.json")
	if err == nil {
		t.Error("expected error saving to nonexistent directory")
	}
}

func TestMetaLearner_Load_ErrorOnNonexistentFile(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	err := ml.Load("/nonexistent/file.json")
	if err == nil {
		t.Error("expected error loading nonexistent file")
	}
}

func TestMetaLearner_SubmitSwarmData(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	// SubmitSwarmData should not panic on valid data
	ml.SubmitSwarmData(SwarmLearningData{
		FishID:    "test-fish",
		Scenario:  "Test Scenario",
		Timestamp: time.Now(),
	})
	if len(ml.Strategies()) == 0 {
		t.Error("expected strategies still available after submission")
	}
}

func TestMetaLearner_GenerateReport(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	report := ml.GenerateReport()
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.TotalStrategies == 0 {
		t.Error("expected non-zero total strategies in report")
	}
	t.Logf("Report: %d strategies", report.TotalStrategies)
}

func TestMetaLearner_Save_PersistenceToRealFile(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	path := t.TempDir() + "/save_test.json"

	if err := ml.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected save file to exist after Save()")
	}
}
