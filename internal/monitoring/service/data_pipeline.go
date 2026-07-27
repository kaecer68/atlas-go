package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
)

// PipelineSourceStatus represents the freshness status of a single data source.
type PipelineSourceStatus struct {
	SourceID     string `json:"source_id"`
	Producer     string `json:"producer"`
	Consumer     string `json:"consumer"`
	LastProduced string `json:"last_produced"`
	LastConsumed string `json:"last_consumed"`
	Status       string `json:"status"`
	LagHuman     string `json:"lag_human"`
	FilePath     string `json:"file_path"`
}

// DataPipelineService tracks data freshness across all sources.
// ChannelHealthJSONRecord is the on-disk shape of channel_health.json
// (subset of apigateway.ChannelHealthRecord used by data_pipeline). Exported
// so gentags collects it for the JS field contract.
type ChannelHealthJSONRecord struct {
	Status        string    `json:"status"`
	LastFetchAt   time.Time `json:"last_fetch_at"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastError     string    `json:"last_error,omitempty"`
}

type DataPipelineService struct {
	WorkDir   string
	LedgerDir string
}

// NewDataPipelineService creates a new pipeline monitoring service.
func NewDataPipelineService(workDir, ledgerDir string) *DataPipelineService {
	return &DataPipelineService{
		WorkDir:   workDir,
		LedgerDir: ledgerDir,
	}
}

// GetPipelineStatus returns freshness status for all known data sources.
func (s *DataPipelineService) GetPipelineStatus() ([]PipelineSourceStatus, error) {
	now := time.Now()

	sources := []struct {
		id       string
		producer string
		consumer string
		path     string
	}{
		{"twse_replay", "daily-replay-sync", "orchestrator", config.GetReplayDataPath(s.WorkDir)},
		{"us_yahoo", "Yahoo Finance API", "MacroEngine", "data/state/macro/latest.json"},
		{"twse_capital_flow", "TWSE API", "CapitalFlowProvider", constants.StateCapitalFlow},
		{"twse_margin", "TWSE API", "RiskManager", "data/state/margin"},
		{"export_statistics", "Customs API", "NarrativeEngine", constants.StateExport},
		{"tsmc_revenue", "TWSE API", "IndustryAnalysis", "data/state/tsmc_revenue"},
		{"geopolitical", "RSS/GDELT", "GeopoliticalProvider", constants.StateGeopolitical + "/latest.json"},
		{"geopolitical_taiwan", "Taiwan RSS", "GeopoliticalProvider", constants.StateGeopolitical + "/taiwan/latest.json"},
	}

	// Read channel_health.json for last_produced times
	channelHealth := make(map[string]ChannelHealthJSONRecord)
	healthPath := filepath.Join(s.WorkDir, "data", "state", "channel_health.json")
	if data, err := os.ReadFile(healthPath); err == nil {
		var wrapper struct {
			Channels map[string]ChannelHealthJSONRecord `json:"channels"`
		}
		if err := json.Unmarshal(data, &wrapper); err == nil {
			channelHealth = wrapper.Channels
		}
	}

	result := make([]PipelineSourceStatus, 0, len(sources))
	for _, src := range sources {
		status := PipelineSourceStatus{
			SourceID: src.id,
			Producer: src.producer,
			Consumer: src.consumer,
			FilePath: filepath.Join(s.WorkDir, src.path),
		}

		// Read from channel_health.json
		if rec, ok := channelHealth[src.id]; ok {
			status.LastProduced = rec.LastSuccessAt.Format(time.RFC3339)
			status.Status = rec.Status
			if rec.LastError != "" {
				status.LagHuman = "error: " + rec.LastError
			}
		}

		// Fallback to file system mod time
		if status.LastProduced == "" {
			info, err := os.Stat(status.FilePath)
			if err == nil {
				status.LastProduced = info.ModTime().Format(time.RFC3339)
				if info.ModTime().After(now.Add(-24 * time.Hour)) {
					status.Status = "ok"
				} else if info.ModTime().After(now.Add(-7 * 24 * time.Hour)) {
					status.Status = "warn"
				} else {
					status.Status = "error"
				}
			} else {
				status.Status = "unknown"
				status.LagHuman = "no data"
			}
		}

		// Calculate lag
		if status.LastProduced != "" {
			if t, err := time.Parse(time.RFC3339, status.LastProduced); err == nil {
				lag := now.Sub(t)
				status.LagHuman = formatDuration(lag)
			}
		}

		result = append(result, status)
	}

	return result, nil
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, hours)
}
