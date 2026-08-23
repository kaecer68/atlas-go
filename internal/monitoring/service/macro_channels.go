package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// loadLatestMacroSnapshot reads data/state/macro/latest.json using MacroService
// when a MacroIngestor is wired, and falls back to a direct read for tests /
// bootstrap paths where the ingestor has not yet been configured.
func (s *DataChannelService) loadLatestMacroSnapshot() (*marketdata.MacroDataSnapshot, error) {
	if s.MacroIngestor != nil {
		ms := NewMacroService(s.WorkDir, s.MacroIngestor, nil)
		return ms.GetLatestSnapshot()
	}

	path := filepath.Join(s.WorkDir, constants.StateMacro, "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read macro snapshot: %w", err)
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode macro snapshot: %w", err)
	}
	return &snap, nil
}

// latestMacroSnapshotOrZero returns the latest macro snapshot, or an empty
// snapshot on error. A missing snapshot is expected during bootstrap/tests;
// the per-field health checks will report the individual channels as missing
// data.
func (s *DataChannelService) latestMacroSnapshotOrZero() *marketdata.MacroDataSnapshot {
	snap, err := s.loadLatestMacroSnapshot()
	if err != nil || snap == nil {
		return &marketdata.MacroDataSnapshot{}
	}
	return snap
}

// checkMacroFieldHealth evaluates a single MacroDataPoint for freshness.
func checkMacroFieldHealth(pt marketdata.MacroDataPoint, now time.Time) (status, updated string) {
	if pt.Symbol == "" {
		return "error", "資料缺失 — 無報價代號"
	}
	if pt.Timestamp == 0 {
		return "error", "無時間戳記"
	}

	dataTime := time.Unix(pt.Timestamp, 0)
	detail := dataTime.Format("2006-01-02 15:04:05")
	age := now.Sub(dataTime)

	switch {
	case age < 24*time.Hour:
		return "ok", detail
	case age < 7*24*time.Hour:
		if isWeekendGap(dataTime, now, 72) {
			return "expected_delay", detail
		}
		return "warn", detail
	default:
		return "error", detail
	}
}

// buildMacroPointChannel creates a DataChannel for a single macro indicator
// backed by the shared latest.json snapshot.
func (s *DataChannelService) buildMacroPointChannel(now time.Time, channelID, label string, pt marketdata.MacroDataPoint) DataChannel {
	fileStatus, fileUpdated := checkMacroFieldHealth(pt, now)
	status, updated, lastError := s.resolveStatusFromStore(channelID, fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:     channelID,
		Country:       "美國",
		Platform:      label,
		APIFormat:     "REST JSON",
		Path:          "query1.finance.yahoo.com/v8/finance/chart",
		Storage:       "data/state/macro/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverityMsg(lastError),
	}
}

// buildUSMacroChannels creates the 8 Yahoo-backed US index / tech stock channels
// that all read from the same macro snapshot.
func (s *DataChannelService) buildUSMacroChannels(now time.Time, snap *marketdata.MacroDataSnapshot) []DataChannel {
	if snap == nil {
		snap = &marketdata.MacroDataSnapshot{}
	}
	return []DataChannel{
		s.buildMacroPointChannel(now, "us_spx", "S&P 500 指數 (^GSPC)", snap.SPXIndex),
		s.buildMacroPointChannel(now, "us_ndx", "NASDAQ 100 指數 (^NDX)", snap.NDXIndex),
		s.buildMacroPointChannel(now, "us_dji", "道瓊工業指數 (^DJI)", snap.DJIIndex),
		s.buildMacroPointChannel(now, "sox_index", "費城半導體指數 (^SOX)", snap.SOXIndex),
		s.buildMacroPointChannel(now, "us_nvda", "NVIDIA (NVDA)", snap.NVDA),
		s.buildMacroPointChannel(now, "us_aapl", "Apple (AAPL)", snap.AAPL),
		s.buildMacroPointChannel(now, "us_msft", "Microsoft (MSFT)", snap.MSFT),
		s.buildMacroPointChannel(now, "tsm_adr", "台積電 ADR (TSM)", snap.TSMADR),
	}
}
