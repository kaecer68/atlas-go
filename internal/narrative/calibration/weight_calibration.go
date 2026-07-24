package calibration

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SignalStrategy determines how factor signals are computed.
//   - SignalChange (default): uses the raw change-pct or value signal (original behavior).
//   - SignalLevel: uses the z-score deviation from rolling baseline mean.
//   - SignalHybrid: uses the signal with the larger absolute magnitude between change and level.
type SignalStrategy int

const (
	SignalChange = iota // default: original change-pct signal
	SignalLevel         // level-based z-score from rolling baseline
	SignalHybrid        // max(|change|, |level deviation|)
)

type CalibrationRecord struct {
	Date          time.Time
	Snapshot      marketdata.MacroDataSnapshot
	ForeignNet    float64
	Outflow       float64
	OutflowTarget float64
}

type WeightCalibrationEngine struct{}

func (e *WeightCalibrationEngine) LoadHistoricalData(workDir string, windowDays int) ([]CalibrationRecord, error) {
	if windowDays <= 0 {
		return nil, fmt.Errorf("load historical data: windowDays must be positive")
	}

	macroDir := filepath.Join(workDir, "data", "state", "macro")
	flowDir := filepath.Join(workDir, "data", "state", "capital_flow")

	entries, err := os.ReadDir(macroDir)
	if err != nil {
		return nil, fmt.Errorf("load historical data: read macro dir: %w", err)
	}

	type datedFile struct {
		date string
		path string
	}
	var macros []datedFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "latest.json" {
			continue
		}
		date := strings.TrimSuffix(entry.Name(), ".json")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		macros = append(macros, datedFile{date: date, path: filepath.Join(macroDir, entry.Name())})
	}
	sort.Slice(macros, func(i, j int) bool { return macros[i].date < macros[j].date })

	if len(macros) == 0 {
		return nil, fmt.Errorf("load historical data: no macro snapshots found")
	}
	if windowDays < len(macros) {
		macros = macros[len(macros)-windowDays:]
	}

	records := make([]CalibrationRecord, 0, len(macros))
	for _, mf := range macros {
		macroData, err := os.ReadFile(mf.path)
		if err != nil {
			return nil, fmt.Errorf("load historical data: read macro %s: %w", mf.date, err)
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(macroData, &snap); err != nil {
			return nil, fmt.Errorf("load historical data: unmarshal macro %s: %w", mf.date, err)
		}

		flowPath := filepath.Join(flowDir, strings.ReplaceAll(mf.date, "-", "")+".json")
		flowData, err := os.ReadFile(flowPath)
		if err != nil {
			continue
		}
		var flow struct {
			Date               string  `json:"date"`
			ForeignInvestorNet float64 `json:"foreign_investor_net"`
			DomesticFundNet    float64 `json:"domestic_fund_net"`
			DealerNet          float64 `json:"dealer_net"`
			TotalNet           float64 `json:"total_net"`
		}
		if err := json.Unmarshal(flowData, &flow); err != nil {
			return nil, fmt.Errorf("load historical data: unmarshal capital flow %s: %w", mf.date, err)
		}

		dt, err := time.Parse("2006-01-02", mf.date)
		if err != nil {
			return nil, fmt.Errorf("load historical data: parse date %s: %w", mf.date, err)
		}
		foreignNet := flow.ForeignInvestorNet
		outflow := -foreignNet
		records = append(records, CalibrationRecord{
			Date:       dt,
			Snapshot:   snap,
			ForeignNet: foreignNet,
			Outflow:    outflow,
		})
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("load historical data: no paired macro/flow records found")
	}

	// OutflowTarget uses forward-looking data (t+5) as a lead indicator for
	// calibration. This is intentional: we want to measure whether each factor
	// predicts future outflow direction, not current outflow. The 5-day window
	// aligns with typical foreign fund settlement cycles.
	//
	// WARNING: This introduces look-ahead bias if used for real-time prediction.
	// It is safe for offline weight calibration only.
	for i := range records {
		if i+5 < len(records) {
			records[i].OutflowTarget = records[i+5].Outflow
		} else {
			records[i].OutflowTarget = records[i].Outflow
		}
	}

	return records, nil
}

func (e *WeightCalibrationEngine) ComputeFactorAccuracy(records []CalibrationRecord) map[string]float64 {
	accuracies := map[string]float64{}
	if len(records) == 0 {
		return accuracies
	}

	factors := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	for _, factor := range factors {
		correct := 0
		total := 0
		for _, r := range records {
			pred := factorSignal(factor, r.Snapshot, r.ForeignNet)
			if pred == 0 || r.Outflow == 0 {
				continue
			}
			total++
			if sameDirection(pred, r.Outflow) {
				correct++
			}
		}
		if total == 0 {
			accuracies[factor] = 0
			continue
		}
		accuracies[factor] = float64(correct) / float64(total)
	}
	return accuracies
}

func (e *WeightCalibrationEngine) CalibrateWeights(accuracies map[string]float64) StressIndexWeights {
	factors := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	weights := StressIndexWeights{}
	if len(accuracies) == 0 {
		return DefaultCalibrationWeights()
	}

	raw := make(map[string]float64, len(factors))
	total := 0.0
	for _, factor := range factors {
		acc := accuracies[factor]
		if acc < 0 {
			acc = 0
		}
		if acc > 0 {
			raw[factor] = acc
			total += acc
		}
	}
	if total == 0 {
		return DefaultCalibrationWeights()
	}

	floor := 0.05
	remaining := 1.0 - floor*float64(len(factors))
	if remaining < 0 {
		return DefaultCalibrationWeights()
	}
	var sumAboveFloor float64
	for _, factor := range factors {
		if raw[factor] > 0 {
			sumAboveFloor += raw[factor]
		}
	}
	if sumAboveFloor == 0 {
		return DefaultCalibrationWeights()
	}

	for _, factor := range factors {
		w := floor
		if raw[factor] > 0 {
			w += (raw[factor] / sumAboveFloor) * remaining
		}
		switch factor {
		case "dxy":
			weights.DXY = w
		case "us10y":
			weights.US10Y = w
		case "foreign_flow":
			weights.ForeignFlow = w
		case "vix":
			weights.VIX = w
		case "jpy":
			weights.JPY = w
		case "geopolitical":
			weights.Geopolitical = w
		case "oil":
			weights.Oil = w
		case "gold":
			weights.Gold = w
		}
	}
	return weights
}

func (e *WeightCalibrationEngine) ExportConfig(workDir string, weights StressIndexWeights, scaling StressIndexScaling, thresholds StressIndexThresholds) error {
	weights = normalizeWeights(weights)
	cfg := StressIndexWeightsConfig{Scaling: scaling, Weights: weights, Thresholds: thresholds}
	if !cfg.IsValid() {
		return fmt.Errorf("export config: invalid weights config")
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("export config: marshal: %w", err)
	}
	path := filepath.Join(workDir, "configs", "stress_index_weights.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("export config: mkdir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("export config: write: %w", err)
	}
	return nil
}

func factorSignal(factor string, snap marketdata.MacroDataSnapshot, foreignNet float64) float64 {
	switch factor {
	case "dxy":
		return snap.DXY.ChangePct
	case "us10y":
		return snap.US10Y.Value
	case "foreign_flow":
		return -foreignNet
	case "vix":
		return snap.VIX.Value - 20
	case "jpy":
		// Use raw USD/JPY exchange rate (e.g., 150.5) rather than ChangePct.
		// Carry trade unwinds are state-driven (extreme JPY level vs history),
		// not event-driven (single-day move). On flat days ChangePct=0 would
		// zero out the level signal; using the rate produces a non-zero z-score
		// whenever JPY is at an extreme level relative to its 60d history.
		return snap.JPY.Value
	case "geopolitical":
		return snap.Gold.ChangePct + snap.Oil.ChangePct
	case "oil":
		return snap.Oil.ChangePct
	case "gold":
		return snap.Gold.ChangePct
	default:
		return 0
	}
}

func normalizeWeights(w StressIndexWeights) StressIndexWeights {
	sum := w.DXY + w.US10Y + w.ForeignFlow + w.VIX + w.JPY + w.Geopolitical + w.Oil + w.Gold
	if sum == 0 {
		return w
	}
	return StressIndexWeights{
		DXY:          w.DXY / sum,
		US10Y:        w.US10Y / sum,
		ForeignFlow:  w.ForeignFlow / sum,
		VIX:          w.VIX / sum,
		JPY:          w.JPY / sum,
		Geopolitical: w.Geopolitical / sum,
		Oil:          w.Oil / sum,
		Gold:         w.Gold / sum,
	}
}

func sameDirection(a, b float64) bool {
	return (a >= 0 && b >= 0) || (a <= 0 && b <= 0)
}

func DefaultCalibrationWeights() StressIndexWeights {
	return StressIndexWeights{DXY: 0.13, US10Y: 0.18, ForeignFlow: 0.22, VIX: 0.13, JPY: 0.08, Geopolitical: 0.13, Oil: 0.07, Gold: 0.06}
}

// factorSignalWithStrategy computes the factor signal according to the given strategy.
// Falls back to SignalChange when baseline is nil or the strategy is SignalChange.
func factorSignalWithStrategy(factor string, snap marketdata.MacroDataSnapshot, foreignNet float64, strategy SignalStrategy, cfg *BaselineConfig) float64 {
	switch strategy {
	case SignalHybrid:
		if cfg != nil {
			return ComputeHybridSignal(factor, snap, foreignNet, cfg)
		}
		return factorSignal(factor, snap, foreignNet)
	case SignalLevel:
		if cfg != nil {
			return ComputeLevelSignal(factor, snap, foreignNet, cfg)
		}
		return factorSignal(factor, snap, foreignNet)
	default:
		return factorSignal(factor, snap, foreignNet)
	}
}

// ComputeFactorAccuracyWithBaseline measures how well each factor predicts
// outflow direction, using the specified signal strategy. When strategy is
// SignalChange or cfg is nil, behavior is identical to ComputeFactorAccuracy.
func (e *WeightCalibrationEngine) ComputeFactorAccuracyWithBaseline(records []CalibrationRecord, strategy SignalStrategy, cfg *BaselineConfig) map[string]float64 {
	accuracies := map[string]float64{}
	if len(records) == 0 {
		return accuracies
	}

	factors := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	for _, factor := range factors {
		correct := 0
		total := 0
		for _, r := range records {
			pred := factorSignalWithStrategy(factor, r.Snapshot, r.ForeignNet, strategy, cfg)
			if pred == 0 || r.Outflow == 0 {
				continue
			}
			if math.IsNaN(pred) || math.IsInf(pred, 0) {
				continue
			}
			total++
			if sameDirection(pred, r.Outflow) {
				correct++
			}
		}
		if total == 0 {
			accuracies[factor] = 0
			continue
		}
		accuracies[factor] = float64(correct) / float64(total)
	}
	return accuracies
}

// CalibrationTask orchestrates the complete rolling calibration lifecycle.
// It loads historical data, computes baselines, calibrates scales and weights,
// validates the results, and exports the configuration.
type CalibrationTask struct {
	engine  *WeightCalibrationEngine
	workDir string
}

// NewCalibrationTask creates a calibration task with the given work directory.
func NewCalibrationTask(workDir string) *CalibrationTask {
	return &CalibrationTask{
		engine:  &WeightCalibrationEngine{},
		workDir: workDir,
	}
}

// RunCalibrationCycle executes one complete calibration cycle:
//  1. Load historical data from workDir (window = params.CalibrationBaselineWindow)
//  2. Compute baselines from training data
//  3. Initialize calibrators with target median
//  4. Calibrate scales via ScaleCalibrator.CalibrateScales(records)
//  5. Calibrate regime-aware weights via RegimeAwareCalibrator.CalibrateWeightsByRegime(records)
//  6. Build new config vs old config
//  7. Validate new config vs old config via ValidateCalibration
//  8. If validated: export new config via ExportConfig
//  9. If degraded: return validation result with IsDegradation=true, skip export
//
// Returns error if data loading fails. Returns CalibrationValidation on success (even if degraded).
func (t *CalibrationTask) RunCalibrationCycle() (*CalibrationValidation, error) {
	p := config.GetParametersConfig()
	if p == nil {
		return nil, fmt.Errorf("calibration: no parameters config available")
	}
	n := p.Narrative

	records, err := t.engine.LoadHistoricalData(t.workDir, n.CalibrationBaselineWindow.Value)
	if err != nil {
		return nil, fmt.Errorf("calibration: load data: %w", err)
	}
	if len(records) < n.CalibrationMinRecords.Value {
		return nil, fmt.Errorf("calibration: insufficient records: %d < %d", len(records), n.CalibrationMinRecords.Value)
	}

	baselineCfg := &BaselineConfig{Window: n.CalibrationBaselineWindow.Value}
	baselines := ComputeBaselines(records, baselineCfg)
	if err := SaveBaselines(t.workDir, baselines); err != nil {
		logging.Warn("calibration", "save_baselines_failed", "error", err.Error())
	}

	scaleCalibrator := NewScaleCalibrator().WithTarget(n.CalibrationTargetMedian.Value)
	newScaling := scaleCalibrator.CalibrateScales(records)

	regimeCalibrator := NewRegimeAwareCalibrator()
	regimeConfig := regimeCalibrator.CalibrateWeightsByRegime(records)

	newConfig := StressIndexWeightsConfig{
		Scaling:    newScaling,
		Weights:    regimeConfig.Normal.Weights,
		Thresholds: regimeConfig.Normal.Thresholds,
	}

	oldConfig := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY:          n.TaiwanStressDXYScale.Value,
			US10Y:        n.TaiwanStressUS10YScale.Value,
			ForeignFlow:  n.TaiwanStressForeignScale.Value,
			VIX:          n.TaiwanStressVIXScale.Value,
			JPY:          n.TaiwanStressJPYScale.Value,
			Geopolitical: n.TaiwanStressGeoScale.Value,
			Oil:          n.TaiwanStressOilScale.Value,
			Gold:         n.TaiwanStressGoldScale.Value,
		},
		Weights: StressIndexWeights{
			DXY:          n.TaiwanStressDXYWeight.Value,
			US10Y:        n.TaiwanStressUS10YWeight.Value,
			ForeignFlow:  n.TaiwanStressForeignWeight.Value,
			VIX:          n.TaiwanStressVIXWeight.Value,
			JPY:          n.TaiwanStressJPYWeight.Value,
			Geopolitical: n.TaiwanStressGeoWeight.Value,
			Oil:          n.TaiwanStressOilWeight.Value,
			Gold:         n.TaiwanStressGoldWeight.Value,
		},
		Thresholds: StressIndexThresholds{
			Crisis: n.TaiwanStressCrisisThreshold.Value,
			High:   n.TaiwanStressHighThreshold.Value,
			Alert:  n.TaiwanStressAlertThreshold.Value,
		},
	}

	validation := ValidateCalibration(records, oldConfig, newConfig)

	if validation.IsDegradation {
		return &validation, nil
	}

	if err := t.engine.ExportConfig(t.workDir, newConfig.Weights, newConfig.Scaling, newConfig.Thresholds); err != nil {
		return &validation, fmt.Errorf("calibration: export: %w", err)
	}

	return &validation, nil
}
