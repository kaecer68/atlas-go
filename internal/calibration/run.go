package calibration

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func Run(workDir, module, dataPath string, dryRun, verbose bool, writeback config.CalibrationWriteback) (string, error) {
	replayPath := dataPath
	if replayPath == "" {
		if replayPath = config.GetReplayDataPath(workDir); replayPath == "" {
			replayPath = filepath.Join(workDir, "data", "replay", "tw_extended_90days.csv")
		}
	}
	paramsPath := filepath.Join(workDir, "configs", "parameters.json")
	paramsCfg, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return "", fmt.Errorf("load parameters config: %w", err)
	}

	// Snapshot the configuration as loaded: overlay mode diffs it against the
	// calibrated one so only the changed leaves are persisted (and
	// configs/parameters.json is not written, which matters inside a container
	// where configs/ is not bind-mounted — FU-20260926-07).
	var baseCfg *config.ParametersConfig
	if writeback.IsOverlay() {
		baseCfg, err = config.CloneParametersConfig(paramsCfg)
		if err != nil {
			return "", err
		}
	}
	returns, n, err := LoadReturns(replayPath)
	if err != nil {
		return "", fmt.Errorf("load returns: %w", err)
	}
	results, err := CalibrateModule(module, config.NewInferenceEngine(paramsCfg), returns, n, paramsCfg)
	if err != nil {
		return "", err
	}
	report := FormatReport(results, verbose)
	if dryRun {
		return report + "\n[DRY-RUN] No changes written.\n", nil
	}
	if writeback.IsOverlay() {
		changed, err := config.WriteConfigOverlay("calibrate_parameters", baseCfg, paramsCfg, time.Now())
		if err != nil {
			return "", err
		}
		return report + fmt.Sprintf("\nWrote %d changed value(s) to %s (configs/parameters.json untouched)\n",
			changed, config.GetCalibratedOverlayPath()), nil
	}
	if err := SaveResults(paramsCfg, results, paramsPath); err != nil {
		return "", err
	}
	return report + fmt.Sprintf("\nSaved updated parameters to %s\n", paramsPath), nil
}
