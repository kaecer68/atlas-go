package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// Error severity levels for frontend display.
const (
	ErrorSeverityInfo     = "info"     // Expected: off-hours, no new data available
	ErrorSeverityWarn     = "warn"     // Transient: rate limits, network timeouts
	ErrorSeverityError    = "error"    // Config/Infra: misconfiguration, service down
	ErrorSeverityCritical = "critical" // Auth: API key missing or invalid
)

func classifyErrorSeverity(errMsg string) string {
	if errMsg == "" {
		return ""
	}
	// Service / infra down patterns — requires operator action (check FIRST, before generic timeout keywords)
	if containsAny(errMsg, "connection refused", "no such host", "dial tcp") {
		return ErrorSeverityError
	}
	// Transient network / rate-limit patterns — usually self-healing (check BEFORE auth, so "rate limit: 403" stays warn)
	if containsAny(errMsg, "rate limit", "context deadline exceeded", "timeout", "connection reset") {
		return ErrorSeverityWarn
	}
	// Auth / credential patterns — urgent
	if containsAny(errMsg, "api key", "unauthorized", "401", "403", "forbidden") {
		return ErrorSeverityCritical
	}
	// Configuration / registration patterns — needs admin fix
	if containsAny(errMsg, "channel not registered", "not found", "invalid") {
		return ErrorSeverityError
	}
	// Off-hours / no-data patterns — expected behavior, not actionable (check LAST, "data available in the last" is specific enough)
	if containsAny(errMsg, "data available in the last") {
		return ErrorSeverityInfo
	}
	// Unknown errors default to warn
	return ErrorSeverityWarn
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

type DataChannel struct {
	ChannelID     string `json:"channel_id"`
	Country       string `json:"country"`
	Platform      string `json:"platform"`
	APIFormat     string `json:"api_format"`
	Path          string `json:"path"`
	Storage       string `json:"storage"`
	Status        string `json:"status"`
	StatusText    string `json:"status_text"`
	UpdatedAt     string `json:"updated_at"`
	LastError     string `json:"last_error,omitempty"`
	ErrorSeverity string `json:"error_severity,omitempty"`
	// Enabled reflects the operator's toggle state from data/state/channels.json.
	// When false the channel is intentionally disabled — frontends should surface
	// this as a 停用 / muted badge even if health status is otherwise "ok".
	// Absence in channels.json is treated as enabled=true (default-on).
	Enabled bool `json:"enabled"`
}

type ChannelAlert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error"`
}

// ChannelHealthRecord aliases the canonical apigateway definition.
type ChannelHealthRecord = apigateway.ChannelHealthRecord

// RecordOption aliases the canonical apigateway definition.
type RecordOption = apigateway.RecordOption

// WithLastDataAt wraps apigateway.WithLastDataAt.
func WithLastDataAt(t time.Time) RecordOption { return apigateway.WithLastDataAt(t) }

// WithLatencyMs wraps apigateway.WithLatencyMs.
func WithLatencyMs(ms int64) RecordOption { return apigateway.WithLatencyMs(ms) }

type DataChannelService struct {
	WorkDir              string
	Pool                 *pgxpool.Pool
	MacroIngestor        *narrative.MacroIngestor
	GeoProvider          geopolitical.GeopoliticalRiskProvider
	TaiwanGeoProvider    geopolitical.GeopoliticalRiskProvider
	JanusEngine          *janus.Engine
	FugleAPIKey          string
	FubonAPIKey          string
	FinMindAPIKey        string
	TejAPIKey            string
	healthStore          *apigateway.ChannelHealthStore
	RegisteredChannelIDs []string
}

// getHealthFromStore returns channel health status from the Gateway-managed health store.
// API key availability is checked separately (not tracked in health store).
func (s *DataChannelService) getHealthFromStore(channelID, apiKey string) (status, updated, lastError string) {
	if apiKey == "" {
		return "inactive", "未設定 API Key", ""
	}
	rec := s.healthStore.Get(channelID)
	if rec == nil {
		return "warn", "API Key 已設定，等待首次健康檢查", ""
	}
	return rec.Status, rec.LastFetchAt, rec.LastError
}

// getCachedFugleHealth returns Fugle health from Gateway health store.
func (s *DataChannelService) getCachedFugleHealth() (status, updated, lastError string) {
	return s.getHealthFromStore("fugle", s.FugleAPIKey)
}

// getCachedFubonHealth returns Fubon health from Gateway health store.
func (s *DataChannelService) getCachedFubonHealth() (status, updated, lastError string) {
	return s.getHealthFromStore("fubon", s.FubonAPIKey)
}

// getCachedFinMindHealth returns FinMind health from Gateway health store.
func (s *DataChannelService) getCachedFinMindHealth() (status, updated, lastError string) {
	return s.getHealthFromStore("finmind", s.FinMindAPIKey)
}

func NewDataChannelService(workDir string, pool *pgxpool.Pool, macroIngestor *narrative.MacroIngestor, geoProvider geopolitical.GeopoliticalRiskProvider, taiwanGeoProvider geopolitical.GeopoliticalRiskProvider, janusEngine *janus.Engine, fugleAPIKey, fubonAPIKey, finmindAPIKey, tejAPIKey string) *DataChannelService {
	return &DataChannelService{
		WorkDir:           workDir,
		Pool:              pool,
		MacroIngestor:     macroIngestor,
		GeoProvider:       geoProvider,
		TaiwanGeoProvider: taiwanGeoProvider,
		JanusEngine:       janusEngine,
		FugleAPIKey:       fugleAPIKey,
		FubonAPIKey:       fubonAPIKey,
		FinMindAPIKey:     finmindAPIKey,
		TejAPIKey:         tejAPIKey,
		healthStore:       apigateway.NewChannelHealthStoreWithPool(filepath.Join(workDir, "data/state"), pool),
	}
}

func (s *DataChannelService) GetChannelStatus(ctx context.Context, channel string) (DataChannel, error) {
	channels, err := s.GetAllChannelStatuses(ctx)
	if err != nil {
		return DataChannel{}, fmt.Errorf("get all channel statuses: %w", err)
	}
	for _, c := range channels {
		if c.ChannelID == channel {
			return c, nil
		}
	}
	return DataChannel{}, fmt.Errorf("channel not found: %s", channel)
}

// resolveStatusFromStore delegates to the package-level resolver so the
// DataChannelService and SystemService apply identical precedence rules.
// See channel_status_resolver.go for behavior details.
func (s *DataChannelService) resolveStatusFromStore(channelID string, fileStatus, fileUpdated string) (status, updated, lastError string) {
	return resolveChannelStatusFromStore(s.healthStore, channelID, fileStatus, fileUpdated)
}

func (s *DataChannelService) GetAllChannelStatuses(ctx context.Context) ([]DataChannel, error) {
	now := time.Now()
	enabledStates := s.loadEnabledStates()
	mergeEnabled := func(c DataChannel) DataChannel {
		// Default-on: starting from true and only flipping to false when channels.json
		// explicitly records enabled=false keeps absent entries (the common case)
		// equal to "operator has not touched this channel — leave running".
		c.Enabled = true
		if e, ok := enabledStates[c.ChannelID]; ok {
			c.Enabled = e
		}
		return c
	}

	channels := []DataChannel{
		mergeEnabled(s.buildUSYahooChannel(now)),
		mergeEnabled(s.buildTWSEReplayChannel(now)),
		mergeEnabled(s.buildTWSECapitalFlowChannel(now)),
		mergeEnabled(s.buildFugleChannel()),
		mergeEnabled(s.buildFubonChannel()),
		mergeEnabled(s.buildFinMindChannel()),
		mergeEnabled(s.buildFrankfurterFXChannel(now)),
		mergeEnabled(s.buildGeopoliticalChannel(now)),
		mergeEnabled(s.buildTWSEMarginChannel(now)),
		mergeEnabled(s.buildExportStatisticsChannel(now)),
		mergeEnabled(s.buildTSMCRevenueChannel(now)),
		mergeEnabled(s.buildTaiwanGeopoliticalChannel(now)),
		mergeEnabled(s.buildJanusRegimeChannel(now)),
		mergeEnabled(s.buildTEJChannel()),
	}
	channels = append(channels, s.buildUSMacroChannels(now, s.latestMacroSnapshotOrZero())...)
	for i, c := range channels {
		channels[i] = mergeEnabled(c)
	}

	// Manifest #G05: append a fallback entry for every registered channel ID
	// not already covered by a static builder (incl. US macro group), so the
	// admin page reflects the full ChannelRegistry instead of a hand-maintained
	// subset. The minimal fallback carries no status; the health probe fills
	// it on next refresh.
	seen := make(map[string]bool, len(channels))
	for _, c := range channels {
		seen[c.ChannelID] = true
	}
	for _, id := range s.RegisteredChannelIDs {
		if seen[id] {
			continue
		}
		// P2-3: channels in the registry without a static builder used to be
		// hardcoded "inactive", which misled the admin page into showing every
		// registered channel as disabled even when 17/18 had a live fetcher.
		// Resolve the real health from the Gateway health store instead; the
		// health probe fills it on the next refresh.
		status, updated, lastError := s.resolveStatusFromStore(id, "", "")
		c := DataChannel{
			ChannelID:     id,
			Country:       "台灣",
			Platform:      "registered channel",
			APIFormat:     "n/a",
			Path:          "(see ChannelRegistry)",
			Status:        status,
			StatusText:    statusText(status),
			UpdatedAt:     updated,
			LastError:     lastError,
			ErrorSeverity: classifyErrorSeverity(lastError),
			Enabled:       true,
		}
		channels = append(channels, mergeEnabled(c))
	}

	return channels, nil
}

// loadEnabledStates reads data/state/channels.json (the same file written by
// internal/monitoring/api/dashboard.setChannelEnabled). Returns nil if the file
// is missing or malformed — callers should treat nil as "no overrides recorded,
// default-on". Channels explicitly listed with enabled=false are honored.
// Malformed-file behavior matches internal/monitoring/api/dashboard.channel_state.go
// (silent nil → default-on) — both reads share the same file so the contract is
// "you get the operator's toggles, or you get default-on; never partial".
func (s *DataChannelService) loadEnabledStates() map[string]bool {
	path := filepath.Join(s.WorkDir, constants.StateChannels+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	out := make(map[string]bool, len(raw))
	for k, v := range raw {
		out[k] = v.Enabled
	}
	return out
}

func (s *DataChannelService) buildUSYahooChannel(now time.Time) DataChannel {
	macroPath := filepath.Join(s.WorkDir, "data/state/macro/latest.json")
	fileStatus, fileUpdated := checkMacroHealth(macroPath, now)
	status, updated, lastError := s.resolveStatusFromStore("us_yahoo", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:     "us_yahoo",
		Country:       "美國",
		Platform:      "Yahoo Finance",
		APIFormat:     "REST JSON",
		Path:          "query1.finance.yahoo.com/v8/finance/chart",
		Storage:       "data/state/macro/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildTWSEReplayChannel(now time.Time) DataChannel {
	replayPath := config.GetReplayDataPath(s.WorkDir)
	fileStatus, fileUpdated := checkReplayHealth(replayPath, now)
	status, updated, lastError := s.resolveStatusFromStore("twse_replay", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:     "twse_replay",
		Country:       "台灣",
		Platform:      "TWSE 證交所",
		APIFormat:     "OpenAPI / CSV",
		Path:          "openapi.twse.com.tw / www.twse.com.tw",
		Storage:       config.GetReplayDataPath(s.WorkDir),
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildTWSECapitalFlowChannel(now time.Time) DataChannel {
	capFlowDir := filepath.Join(s.WorkDir, constants.StateCapitalFlow)
	fileStatus, fileUpdated := checkCapitalFlowHealth(capFlowDir, now)
	status, updated, _ := s.resolveStatusFromStore("twse_capital_flow", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:  "twse_capital_flow",
		Country:    "台灣",
		Platform:   "TWSE 三大法人",
		APIFormat:  "T86 JSON",
		Path:       "www.twse.com.tw/rwd/zh/fund/T86",
		Storage:    "data/state/capital_flow/*.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
	}
}

func (s *DataChannelService) buildFugleChannel() DataChannel {
	status, updated, lastError := s.getCachedFugleHealth()
	return DataChannel{
		ChannelID:     "fugle",
		Country:       "台灣",
		Platform:      "Fugle 富果",
		APIFormat:     "REST JSON",
		Path:          "api.fugle.tw",
		Storage:       "data/state/fugle/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildFubonChannel() DataChannel {
	status, updated, lastError := s.getCachedFubonHealth()
	return DataChannel{
		ChannelID:     "fubon",
		Country:       "台灣",
		Platform:      "富邦證券",
		APIFormat:     "REST JSON",
		Path:          "api.fubon.com.tw",
		Storage:       "data/state/fubon/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildFinMindChannel() DataChannel {
	status, updated, lastError := s.getCachedFinMindHealth()
	return DataChannel{
		ChannelID:     "finmind",
		Country:       "台灣",
		Platform:      "FinMind",
		APIFormat:     "REST JSON",
		Path:          "api.finmindtrade.com",
		Storage:       "data/state/finmind/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildFrankfurterFXChannel(now time.Time) DataChannel {
	macroPath := filepath.Join(s.WorkDir, "data/state/macro/latest.json")
	fileStatus, fileUpdated := checkJPYHealth(macroPath, now)
	status, updated, lastError := s.resolveStatusFromStore("frankfurter_fx", fileStatus, fileUpdated)

	// Edge case: file data is stale but the fetch mechanism is working.
	// resolveStatusFromStore returns "ok" in this case, which is correct
	// for the channel health, but we surface a note about stale file data.
	if fileStatus == "error" && status == "ok" {
		status = "warn"
		updated = "檔案數據過期，但 Frankfurter API 連線正常"
		rec := s.healthStore.Get("frankfurter_fx")
		if rec != nil && rec.LastSuccessAt != "" {
			updated += " · 最後成功: " + rec.LastSuccessAt
		}
	}

	return DataChannel{
		ChannelID:  "frankfurter_fx",
		Country:    "日本",
		Platform:   "Frankfurter (USD/JPY)",
		APIFormat:  "REST JSON",
		Path:       "api.frankfurter.app/latest?from=USD&to=JPY",
		Storage:    "data/state/macro/latest.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildGeopoliticalChannel(now time.Time) DataChannel {
	geoPath := filepath.Join(s.WorkDir, constants.StateGeopolitical+"/latest.json")
	fileStatus, fileUpdated := checkGeopoliticalHealth(geoPath, now)
	status, updated, lastError := s.resolveStatusFromStore("geopolitical", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:     "geopolitical",
		Country:       "中東/全球",
		Platform:      "RSS + GDELT",
		APIFormat:     "RSS / REST JSON",
		Path:          "feeds.bbci.co.uk / api.gdeltproject.org",
		Storage:       constants.StateGeopolitical + "/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildTWSEMarginChannel(now time.Time) DataChannel {
	marginDir := filepath.Join(s.WorkDir, "data/state/margin")
	fileStatus, fileUpdated := checkMarginHealth(marginDir, now)
	status, updated, lastError := s.resolveStatusFromStore("twse_margin", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:  "twse_margin",
		Country:    "台灣",
		Platform:   "TWSE 融資融券",
		APIFormat:  "Miantane JSON",
		Path:       "www.twse.com.tw/rwd/zh/marginTradingMiantane",
		Storage:    "data/state/margin/*_margin.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildExportStatisticsChannel(now time.Time) DataChannel {
	exportDir := filepath.Join(s.WorkDir, constants.StateExport)
	fileStatus, fileUpdated := checkExportHealth(exportDir, now)
	status, updated, lastError := s.resolveStatusFromStore("export_statistics", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:     "export_statistics",
		Country:       "台灣",
		Platform:      "海關進出口統計 (data.gov.tw)",
		APIFormat:     "CSV",
		Path:          "opendata.customs.gov.tw/data/6053/csv.csv",
		Storage:       "data/state/export/*_export.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildTSMCRevenueChannel(now time.Time) DataChannel {
	tsmcDir := filepath.Join(s.WorkDir, "data/state/tsmc_revenue")
	fileStatus, fileUpdated := checkTSMCRevenueHealth(tsmcDir, now)
	status, updated, lastError := s.resolveStatusFromStore("tsmc_revenue", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:  "tsmc_revenue",
		Country:    "台灣",
		Platform:   "TWSE 台積電月營收",
		APIFormat:  "TWT49U JSON",
		Path:       "www.twse.com.tw/rwd/zh/fund/TWT49U",
		Storage:    "data/state/tsmc_revenue/*_revenue.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildTaiwanGeopoliticalChannel(now time.Time) DataChannel {
	twGeoPath := filepath.Join(s.WorkDir, constants.StateGeopolitical+"/taiwan/latest.json")
	fileStatus, fileUpdated := checkGeopoliticalHealth(twGeoPath, now)
	status, updated, lastError := s.resolveStatusFromStore("geopolitical_taiwan", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:     "geopolitical_taiwan",
		Country:       "台灣",
		Platform:      "CNA / 自由時報 / TVBS RSS",
		APIFormat:     "RSS XML",
		Path:          "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw",
		Storage:       constants.StateGeopolitical + "/taiwan/latest.json",
		Status:        status,
		StatusText:    statusText(status),
		UpdatedAt:     updated,
		LastError:     lastError,
		ErrorSeverity: classifyErrorSeverity(lastError),
	}
}

func (s *DataChannelService) buildJanusRegimeChannel(now time.Time) DataChannel {
	fileStatus, fileUpdated := checkJanusHealth(s.JanusEngine, now)
	status, updated, lastError := s.resolveStatusFromStore("janus_regime", fileStatus, fileUpdated)
	return DataChannel{
		ChannelID:  "janus_regime",
		Country:    "全域",
		Platform:   "JANUS Engine",
		APIFormat:  "Internal",
		Path:       "internal/janus",
		Storage:    "(in-memory state)",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildTEJChannel() DataChannel {
	status := "inactive"
	updated := "TEJ_API_KEY not configured"
	tejKey := s.TejAPIKey
	if tejKey != "" {
		status = "ok"
		updated = "TEJ API key configured"
		rec := s.healthStore.Get("tej")
		if rec != nil && rec.LastError != "" {
			status = "error"
			updated = "上次失敗: " + rec.LastError
		}
	}
	return DataChannel{
		ChannelID:  "tej",
		Country:    "台灣",
		Platform:   "TEJ 台灣經濟新報",
		APIFormat:  "REST JSON",
		Path:       "TEJ API (premium)",
		Storage:    "N/A (live query)",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
	}
}

func (s *DataChannelService) GetAlerts(ctx context.Context) ([]ChannelAlert, error) {
	channels, err := s.GetAllChannelStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all channel statuses: %w", err)
	}
	knownInactive := map[string]bool{}
	var alerts []ChannelAlert
	for _, c := range channels {
		if c.Status == "error" || c.Status == "warn" {
			if knownInactive[c.ChannelID] {
				continue
			}
			alerts = append(alerts, ChannelAlert{
				ChannelID: c.ChannelID,
				Status:    c.Status,
				Error:     c.LastError,
			})
		}
	}
	if alerts == nil {
		alerts = []ChannelAlert{}
	}
	return alerts, nil
}

func statusText(status string) string {
	return StatusText(status)
}
