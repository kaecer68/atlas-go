package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// SectorDataDirRel is the single authority for the directory that holds
// sector_data.json, relative to the work dir (issue #1944 Batch 2, Q6 I14).
//
// Before this constant the readers disagreed, so the channel was silently dead
// in production while a shipped file existed:
//
//	dashboard_api.go / register_adapters.go : <workDir>/data/state/sector_data
//	scheduler strategy-evolution deps       : <workDir>/sector_data.json
//	orchestrator system.go                  : <ledgerDir>/sector_data.json
//	the file actually committed to the repo : <workDir>/data/sector_data/sector_data.json
const SectorDataDirRel = "data/sector_data"

// ResolveSectorDataDir returns the directory that holds sector_data.json for
// the given work dir. Use this instead of spelling the path out; see
// SectorDataDirRel for the paths that used to disagree.
func ResolveSectorDataDir(workDir string) string {
	return filepath.Join(workDir, SectorDataDirRel)
}

// SectorDataState is the machine-readable outcome of the last FetchSnapshot
// call on a SectorDataProvider. The provider keeps its graceful-degradation
// contract (a missing file still yields a zero snapshot with a nil error), but
// the absence must be observable: this state is what
// apigateway.SectorDataChannelAdapter.HealthCheck reports on, so a dead channel
// no longer looks healthy (issue #1944 Batch 2, Q6 I14).
// The fields carry no json tags on purpose: this is an internal diagnostic
// value (health-check input, human-readable LastError text), not an API
// payload, so it must not widen the frontend field contract.
type SectorDataState struct {
	// Path is the file the provider tried to read.
	Path string
	// Found reports whether the file existed and parsed.
	Found bool
	// DataUpdatedAt is the file's own updated_at (zero when absent/unparseable).
	DataUpdatedAt time.Time
	// Reason is "ok", "missing", "invalid_json" or "unparseable_timestamp".
	Reason string
}

// SectorDataProvider reads sector-specific data from JSON files to feed the StructuralTrend engine.
type SectorDataProvider struct {
	dataDir string

	state SectorDataState

	mu sync.RWMutex
	// Last-fetched values are cached in memory so ChangePct can be derived
	// from the previous fetch (Bug#5 root cause — JSON file has no historical
	// column, so ChangePct was hardcoded to 0).
	lastValues struct {
		AIRevenueGrowth    float64
		CoWoSUtilization   float64
		CapexGrowth        float64
		SemiconductorIndex float64
	}
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

// FilePath returns the absolute-or-relative path this provider reads.
func (p *SectorDataProvider) FilePath() string {
	return filepath.Join(p.dataDir, "sector_data.json")
}

// State returns the outcome of the last FetchSnapshot call (see SectorDataState).
func (p *SectorDataProvider) State() SectorDataState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// setState records the load outcome for State()/HealthCheck.
func (p *SectorDataProvider) setState(found bool, updatedAt time.Time, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = SectorDataState{
		Path:          p.FilePath(),
		Found:         found,
		DataUpdatedAt: updatedAt,
		Reason:        reason,
	}
}

// FetchSnapshot reads the sector data JSON file and maps it to a MacroDataSnapshot.
//
// Graceful degradation is retained — a missing or malformed file yields a zero
// snapshot with a nil error — but the outcome is recorded in State() so callers
// (notably the apigateway channel HealthCheck) can distinguish "no data" from
// "zero data" (issue #1944 Batch 2, Q6 I14).
func (p *SectorDataProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	path := p.FilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if p.dataDir != "" {
				logging.Warn("sector_data_provider", "file_missing", "path", path)
			}
			p.setState(false, time.Time{}, "missing")
			return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
		}
		p.setState(false, time.Time{}, "read_error")
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, fmt.Errorf("sector_data: %w", err)
	}

	var parsed sectorDataJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		logging.Warn("sector_data_provider", "invalid_json", "path", path, logging.Err(err))
		p.setState(false, time.Time{}, "invalid_json")
		return MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
	}

	var ts, dataAt time.Time
	reason := "ok"
	if parsed.UpdatedAt != "" {
		if parsedTS, perr := time.Parse(time.RFC3339, parsed.UpdatedAt); perr != nil {
			// The snapshot keeps its legacy "now" fallback, but DataUpdatedAt
			// stays zero so a health check does not mistake an unparseable
			// timestamp for fresh data.
			reason = "unparseable_timestamp"
		} else {
			dataAt, ts = parsedTS, parsedTS
		}
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	p.setState(true, dataAt, reason)

	p.mu.Lock()
	aiChange := pctChange(parsed.AIRevenueGrowth, p.lastValues.AIRevenueGrowth)
	cowosChange := pctChange(parsed.CoWoSUtilization, p.lastValues.CoWoSUtilization)
	capexChange := pctChange(parsed.CapexGrowth, p.lastValues.CapexGrowth)
	soxChange := pctChange(parsed.SemiconductorIndex, p.lastValues.SemiconductorIndex)
	p.lastValues.AIRevenueGrowth = parsed.AIRevenueGrowth
	p.lastValues.CoWoSUtilization = parsed.CoWoSUtilization
	p.lastValues.CapexGrowth = parsed.CapexGrowth
	p.lastValues.SemiconductorIndex = parsed.SemiconductorIndex
	p.mu.Unlock()

	return MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    "TSMC_AI_REVENUE",
			Value:     parsed.AIRevenueGrowth,
			ChangePct: aiChange,
			Timestamp: ts.Unix(),
		},
		SOXIndex: MacroDataPoint{
			Symbol:    "^SOX",
			Value:     parsed.SemiconductorIndex,
			ChangePct: soxChange,
			Timestamp: ts.Unix(),
		},
		CoWoSUtilization: MacroDataPoint{
			Symbol:    "COWOS_UTILIZATION",
			Value:     parsed.CoWoSUtilization,
			ChangePct: cowosChange,
			Timestamp: ts.Unix(),
		},
		CapexGrowth: MacroDataPoint{
			Symbol:    "CAPEX_GROWTH",
			Value:     parsed.CapexGrowth,
			ChangePct: capexChange,
			Timestamp: ts.Unix(),
		},
		RecordedAt: ts.Unix(),
	}, nil
}

// pctChange returns (current-previous)/previous*100, or 0 when previous is 0
// (cold start) or current is unchanged.
func pctChange(current, previous float64) float64 {
	if previous == 0 {
		return 0
	}
	return (current - previous) / previous * 100
}
