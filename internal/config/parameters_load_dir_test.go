package config

import (
	"path/filepath"
	"testing"
)

func TestLoadParametersDir_MatchesFileLoad(t *testing.T) {
	// Test paths are relative to repo root
	root := filepath.Join("..", "..")
	filePath := filepath.Join(root, "configs", "parameters.json")
	dirPath := filepath.Join(root, "configs", "parameters")

	fileCfg, err := loadParametersFile(filePath)
	if err != nil {
		t.Skipf("skipping: cannot load file: %v", err)
	}

	dirCfg, err := loadParametersDir(dirPath)
	if err != nil {
		t.Fatalf("load dir: %v", err)
	}

	if fileCfg.Darwinian.WeightMax.Value != dirCfg.Darwinian.WeightMax.Value {
		t.Errorf("darwinian.weight_max: file=%v dir=%v", fileCfg.Darwinian.WeightMax.Value, dirCfg.Darwinian.WeightMax.Value)
	}
	if fileCfg.Experiment.WelchTTestThreshold.Value != dirCfg.Experiment.WelchTTestThreshold.Value {
		t.Errorf("experiment.welch_t: file=%v dir=%v", fileCfg.Experiment.WelchTTestThreshold.Value, dirCfg.Experiment.WelchTTestThreshold.Value)
	}
	if fileCfg.Narrative.MinConfidence.Value != dirCfg.Narrative.MinConfidence.Value {
		t.Errorf("narrative.min_confidence: file=%v dir=%v", fileCfg.Narrative.MinConfidence.Value, dirCfg.Narrative.MinConfidence.Value)
	}
}
