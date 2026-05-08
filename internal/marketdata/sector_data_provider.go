package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// SectorDataProvider reads sector-specific data from JSON files to feed the StructuralTrend engine.
type SectorDataProvider struct {
	dataDir string
}

type sectorDataJSON struct {
	AIRevenueGrowth    float64 `json:"ai_revenue_growth"`
	CoWoSUtilization   float64 `json:"cowos_utilization"`
	CapexGrowth        float64 `json:"capex_growth"`
	SemiconductorIndex float64 `json:"semiconductor_index"`
	UpdatedAt          string  `json:"updated_at"`
}

// NewSectorDataProvider creates a new sector data provider.
// If the directory does not exist, the provider returns zero values (graceful degradation).
func NewSectorDataProvider(dataDir string) *SectorDataProvider {
	return &SectorDataProvider{dataDir: dataDir}
}

// Name returns the provider name.
func (p *SectorDataProvider) Name() string {
	return "sector_data"
}

// FetchSnapshot reads the sector data JSON file and maps it to a MacroDataSnapshot.
func (p *SectorDataProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	path := filepath.Join(p.dataDir, "sector_data.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
		}
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, fmt.Errorf("sector_data: %w", err)
	}

	var parsed sectorDataJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		logging.Warn("sector_data_provider", "invalid_json", logging.Err(err))
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	var ts time.Time
	if parsed.UpdatedAt != "" {
		ts, _ = time.Parse(time.RFC3339, parsed.UpdatedAt)
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	return MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    "TSMC_AI_REVENUE",
			Value:     parsed.AIRevenueGrowth,
			ChangePct: 0,
			Timestamp: ts.Unix(),
		},
		SOXIndex: MacroDataPoint{
			Symbol:    "^SOX",
			Value:     parsed.SemiconductorIndex,
			ChangePct: 0,
			Timestamp: ts.Unix(),
		},
		CoWoSUtilization: MacroDataPoint{
			Symbol:    "COWOS_UTILIZATION",
			Value:     parsed.CoWoSUtilization,
			ChangePct: 0,
			Timestamp: ts.Unix(),
		},
		CapexGrowth: MacroDataPoint{
			Symbol:    "CAPEX_GROWTH",
			Value:     parsed.CapexGrowth,
			ChangePct: 0,
			Timestamp: ts.Unix(),
		},
		RecordedAt: ts.Unix(),
	}, nil
}
