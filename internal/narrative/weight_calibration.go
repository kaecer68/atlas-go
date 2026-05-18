package narrative

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
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
			Date:          dt,
			Snapshot:      snap,
			ForeignNet:    foreignNet,
			Outflow:       outflow,
			OutflowTarget: outflow,
		})
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("load historical data: no paired macro/flow records found")
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
		return defaultCalibrationWeights()
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
		return defaultCalibrationWeights()
	}

	floor := 0.05
	remaining := 1.0 - floor*float64(len(factors))
	if remaining < 0 {
		return defaultCalibrationWeights()
	}
	var sumAboveFloor float64
	for _, factor := range factors {
		if raw[factor] > 0 {
			sumAboveFloor += raw[factor]
		}
	}
	if sumAboveFloor == 0 {
		return defaultCalibrationWeights()
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
	cfg := StressIndexWeightsConfig{Scaling: scaling, Weights: normalizeWeights(weights), Thresholds: thresholds}
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
		return snap.JPY.ChangePct
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

func sameDirection(a, b float64) bool {
	return (a >= 0 && b >= 0) || (a <= 0 && b <= 0)
}

func normalizeWeights(w StressIndexWeights) StressIndexWeights {
	sum := w.DXY + w.US10Y + w.ForeignFlow + w.VIX + w.JPY + w.Geopolitical + w.Oil + w.Gold
	if sum <= 0 {
		return defaultCalibrationWeights()
	}
	w.DXY /= sum
	w.US10Y /= sum
	w.ForeignFlow /= sum
	w.VIX /= sum
	w.JPY /= sum
	w.Geopolitical /= sum
	w.Oil /= sum
	w.Gold /= sum
	return w
}

func defaultCalibrationWeights() StressIndexWeights {
	return StressIndexWeights{DXY: 0.13, US10Y: 0.18, ForeignFlow: 0.22, VIX: 0.13, JPY: 0.08, Geopolitical: 0.13, Oil: 0.07, Gold: 0.06}
}
