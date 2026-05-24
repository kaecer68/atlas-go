package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type DataChannel struct {
	ChannelID  string `json:"channel_id"`
	Country    string `json:"country"`
	Platform   string `json:"platform"`
	APIFormat  string `json:"api_format"`
	Path       string `json:"path"`
	Storage    string `json:"storage"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UpdatedAt  string `json:"updated_at"`
	LastError  string `json:"last_error,omitempty"`
}

type ChannelAlert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error"`
}

type ChannelHealthRecord struct {
	Status             string   `json:"status"`
	LastFetchAt        string   `json:"last_fetch_at"`
	LastDataAt         string   `json:"last_data_at,omitempty"`
	LastError          string   `json:"last_error,omitempty"`
	LastSuccessAt      string   `json:"last_success_at,omitempty"`
	RateLimitRemaining int      `json:"rate_limit_remaining,omitempty"`
	LatencyMs          int64    `json:"latency_ms,omitempty"`
	RecordsFetched     int      `json:"records_fetched,omitempty"`
	SymbolsProcessed   int      `json:"symbols_processed,omitempty"`
	Errors             []string `json:"errors,omitempty"`
}

// RecordOption configures optional fields on a ChannelHealthRecord.
type RecordOption func(*ChannelHealthRecord)

// WithLastDataAt sets the last data timestamp.
func WithLastDataAt(t time.Time) RecordOption {
	return func(r *ChannelHealthRecord) { r.LastDataAt = t.Format(time.RFC3339) }
}

// WithLatencyMs sets the latency in milliseconds.
func WithLatencyMs(ms int64) RecordOption {
	return func(r *ChannelHealthRecord) { r.LatencyMs = ms }
}

type DataChannelService struct {
	WorkDir           string
	Pool              *pgxpool.Pool
	MacroIngestor     *narrative.MacroIngestor
	GeoProvider       narrative.GeopoliticalRiskProvider
	TaiwanGeoProvider narrative.GeopoliticalRiskProvider
	JanusEngine       *janus.Engine
	FugleAPIKey       string
	FubonAPIKey       string
	FinMindAPIKey     string
	TejAPIKey         string
	healthStore       *ChannelHealthStoreAdapter
}

// getHealthFromStore returns channel health status from the Gateway-managed health store.
// API key availability is checked separately (not tracked in health store).
func (s *DataChannelService) getHealthFromStore(channelID, apiKey string) (status, updated, lastError string) {
	if apiKey == "" {
		return "inactive", "未設定 API Key", ""
	}
	rec := s.healthStore.Get(channelID)
	if rec == nil {
		return "unknown", "尚未執行健康檢查", ""
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

// getCachedFrankfurterHealth returns Frankfurter FX health from Gateway health store.
func (s *DataChannelService) getCachedFrankfurterHealth() (status, updated, lastError string) {
	return s.getHealthFromStore("jpy_yahoo", "enabled")
}

type ChannelHealthStoreAdapter struct {
	pool  *pgxpool.Pool
	dir   string
	store *channelHealthStore
	once  sync.Once
}

func NewChannelHealthStoreAdapter(dir string, pool *pgxpool.Pool) *ChannelHealthStoreAdapter {
	return &ChannelHealthStoreAdapter{pool: pool, dir: dir}
}

func (a *ChannelHealthStoreAdapter) Get(channelID string) *ChannelHealthRecord {
	a.once.Do(func() {
		a.store = newChannelHealthStore(a.dir, a.pool)
	})
	return a.store.Get(channelID)
}

func (a *ChannelHealthStoreAdapter) Record(channelID, status, errMsg string, opts ...RecordOption) error {
	a.once.Do(func() {
		a.store = newChannelHealthStore(a.dir, a.pool)
	})
	return a.store.Record(channelID, status, errMsg, opts...)
}

func newChannelHealthStore(dir string, pool *pgxpool.Pool) *channelHealthStore {
	return &channelHealthStore{
		path: filepath.Join(dir, "channel_health.json"),
		data: make(map[string]*ChannelHealthRecord),
		pool: pool,
	}
}

type channelHealthStore struct {
	path string
	data map[string]*ChannelHealthRecord
	pool *pgxpool.Pool
	mu   sync.RWMutex
}

func (s *channelHealthStore) Get(channelID string) *ChannelHealthRecord {
	s.load()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if rec, ok := s.data[channelID]; ok {
		cp := *rec
		return &cp
	}
	return nil
}

func (s *channelHealthStore) Record(channelID, status, errMsg string, opts ...RecordOption) error {
	s.load()
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.data[channelID]
	if rec == nil {
		rec = &ChannelHealthRecord{}
		s.data[channelID] = rec
	}
	rec.Status = status
	rec.LastFetchAt = time.Now().Format(time.RFC3339)
	if status == "ok" {
		rec.LastError = ""
		rec.LastSuccessAt = rec.LastFetchAt
	} else {
		rec.LastError = errMsg
	}
	for _, opt := range opts {
		opt(rec)
	}
	return s.saveLocked()
}

func (s *channelHealthStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *channelHealthStore) loadLocked() error {
	s.data = make(map[string]*ChannelHealthRecord)
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read channel health file: %w", err)
	}
	var wrapper struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("unmarshal channel health: %w", err)
	}
	if wrapper.Channels != nil {
		s.data = wrapper.Channels
	}
	return nil
}

func (s *channelHealthStore) saveLocked() error {
	wrapper := struct {
		Channels map[string]*ChannelHealthRecord `json:"channels"`
	}{Channels: s.data}
	b, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal channel health: %w", err)
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	return os.Rename(tmp, s.path)
}

func NewDataChannelService(workDir string, pool *pgxpool.Pool, macroIngestor *narrative.MacroIngestor, geoProvider narrative.GeopoliticalRiskProvider, taiwanGeoProvider narrative.GeopoliticalRiskProvider, janusEngine *janus.Engine, fugleAPIKey, fubonAPIKey, finmindAPIKey, tejAPIKey string) *DataChannelService {
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
		healthStore:       NewChannelHealthStoreAdapter(filepath.Join(workDir, "data/state"), pool),
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

func (s *DataChannelService) GetAllChannelStatuses(ctx context.Context) ([]DataChannel, error) {
	now := time.Now()
	channels := make([]DataChannel, 0, 14)

	channels = append(channels, s.buildUSYahooChannel(now))
	channels = append(channels, s.buildTWSEReplayChannel(now))
	channels = append(channels, s.buildTWSECapitalFlowChannel(now))
	channels = append(channels, s.buildFugleChannel())
	channels = append(channels, s.buildFubonChannel())
	channels = append(channels, s.buildFinMindChannel())
	channels = append(channels, s.buildJPYYahooChannel(now))
	channels = append(channels, s.buildGeopoliticalChannel(now))
	channels = append(channels, s.buildTWSEMarginChannel(now))
	channels = append(channels, s.buildExportStatisticsChannel(now))
	channels = append(channels, s.buildTSMCRevenueChannel(now))
	channels = append(channels, s.buildTaiwanGeopoliticalChannel(now))
	channels = append(channels, s.buildJanusRegimeChannel(now))
	channels = append(channels, s.buildTEJChannel())

	return channels, nil
}

func (s *DataChannelService) buildUSYahooChannel(now time.Time) DataChannel {
	macroPath := filepath.Join(s.WorkDir, "data/state/macro/latest.json")
	status, updated := checkMacroHealth(macroPath, now)
	rec := s.healthStore.Get("us_yahoo")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
	return DataChannel{
		ChannelID:  "us_yahoo",
		Country:    "美國",
		Platform:   "Yahoo Finance",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com/v8/finance/chart",
		Storage:    "data/state/macro/latest.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildTWSEReplayChannel(now time.Time) DataChannel {
	replayPath := config.GetReplayDataPath(s.WorkDir)
	status, updated := checkReplayHealth(replayPath, now)
	rec := s.healthStore.Get("twse_replay")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
	return DataChannel{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "OpenAPI / CSV",
		Path:       "openapi.twse.com.tw / www.twse.com.tw",
		Storage:    config.GetReplayDataPath(s.WorkDir),
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildTWSECapitalFlowChannel(now time.Time) DataChannel {
	capFlowDir := filepath.Join(s.WorkDir, "data/state/capital_flow")
	status, updated := checkCapitalFlowHealth(capFlowDir, now)
	rec := s.healthStore.Get("twse_capital_flow")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
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
		ChannelID:  "fugle",
		Country:    "台灣",
		Platform:   "Fugle 富果",
		APIFormat:  "REST JSON",
		Path:       "api.fugle.tw",
		Storage:    "(live cache / memory)",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildFubonChannel() DataChannel {
	status, updated, lastError := s.getCachedFubonHealth()
	return DataChannel{
		ChannelID:  "fubon",
		Country:    "台灣",
		Platform:   "富邦證券",
		APIFormat:  "REST JSON",
		Path:       "api.fubon.com.tw",
		Storage:    "(live cache / memory)",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildFinMindChannel() DataChannel {
	status, updated, lastError := s.getCachedFinMindHealth()
	return DataChannel{
		ChannelID:  "finmind",
		Country:    "台灣",
		Platform:   "FinMind",
		APIFormat:  "REST JSON",
		Path:       "api.finmindtrade.com",
		Storage:    "(live cache / memory)",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastError,
	}
}

func (s *DataChannelService) buildJPYYahooChannel(now time.Time) DataChannel {
	macroPath := filepath.Join(s.WorkDir, "data/state/macro/latest.json")
	status, updated := checkJPYHealth(macroPath, now)
	rec := s.healthStore.Get("jpy_yahoo")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}

	// Also check Frankfurter FX API as an alternative JPY source.
	fxStatus, _, fxLastError := s.getCachedFrankfurterHealth()
	if status == "error" && fxStatus == "ok" {
		// File data is stale but the Frankfurter API (alternative JPY source) is reachable.
		status = "warn"
		updated = "檔案數據過期，但替代來源 Frankfurter API 連線正常"
		rec = s.healthStore.Get("jpy_yahoo")
		if rec != nil && rec.LastError != "" {
			updated += " · 最後成功: " + rec.LastSuccessAt
		}
	} else if status == "error" && fxStatus == "error" {
		lastError := fxLastError
		lastErrorStr := lastErrorStr(rec)
		if lastErrorStr != "" {
			lastError = lastErrorStr
		}
		return DataChannel{
			ChannelID:  "jpy_yahoo",
			Country:    "日本",
			Platform:   "Yahoo Finance (JPY)",
			APIFormat:  "REST JSON",
			Path:       "query1.finance.yahoo.com/v8/finance/chart",
			Storage:    "data/state/macro/latest.json",
			Status:     status,
			StatusText: statusText(status),
			UpdatedAt:  fmt.Sprintf("Yahoo API 連線失敗 · Frankfurter API 連線失敗: %s", lastError),
			LastError:  lastError,
		}
	}

	return DataChannel{
		ChannelID:  "jpy_yahoo",
		Country:    "日本",
		Platform:   "Yahoo Finance (JPY)",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com/v8/finance/chart",
		Storage:    "data/state/macro/latest.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildGeopoliticalChannel(now time.Time) DataChannel {
	geoPath := filepath.Join(s.WorkDir, "data/state/geopolitical/latest.json")
	status, updated := checkGeopoliticalHealth(geoPath, now)
	rec := s.healthStore.Get("geopolitical")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
	return DataChannel{
		ChannelID:  "geopolitical",
		Country:    "中東/全球",
		Platform:   "RSS + GDELT",
		APIFormat:  "RSS / REST JSON",
		Path:       "feeds.bbci.co.uk / api.gdeltproject.org",
		Storage:    "data/state/geopolitical/latest.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildTWSEMarginChannel(now time.Time) DataChannel {
	marginDir := filepath.Join(s.WorkDir, "data/state/margin")
	status, updated := checkMarginHealth(marginDir, now)
	rec := s.healthStore.Get("twse_margin")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
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
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildExportStatisticsChannel(now time.Time) DataChannel {
	exportDir := filepath.Join(s.WorkDir, "data/state/export")
	status, updated := checkExportHealth(exportDir, now)
	rec := s.healthStore.Get("export_statistics")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
	return DataChannel{
		ChannelID:  "export_statistics",
		Country:    "台灣",
		Platform:   "海關進出口統計 (data.gov.tw)",
		APIFormat:  "CSV",
		Path:       "opendata.customs.gov.tw/data/6053/csv.csv",
		Storage:    "data/state/export/*_export.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
	}
}

func (s *DataChannelService) buildTSMCRevenueChannel(now time.Time) DataChannel {
	tsmcDir := filepath.Join(s.WorkDir, "data/state/tsmc_revenue")
	status, updated := checkTSMCRevenueHealth(tsmcDir, now)
	rec := s.healthStore.Get("tsmc_revenue")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
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
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildTaiwanGeopoliticalChannel(now time.Time) DataChannel {
	twGeoPath := filepath.Join(s.WorkDir, "data/state/geopolitical/taiwan/latest.json")
	status, updated := checkGeopoliticalHealth(twGeoPath, now)
	rec := s.healthStore.Get("geopolitical_taiwan")
	if rec != nil && rec.LastError != "" {
		updated = "上次失敗: " + rec.LastError
	}
	return DataChannel{
		ChannelID:  "geopolitical_taiwan",
		Country:    "台灣",
		Platform:   "CNA / 自由時報 / TVBS RSS",
		APIFormat:  "RSS XML",
		Path:       "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw",
		Storage:    "data/state/geopolitical/taiwan/latest.json",
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
		LastError:  lastErrorStr(rec),
	}
}

func (s *DataChannelService) buildJanusRegimeChannel(now time.Time) DataChannel {
	status, updated := checkJanusHealth(s.JanusEngine, now)
	lastError := ""
	rec := s.healthStore.Get("janus_regime")
	if rec != nil && rec.LastError != "" {
		lastError = rec.LastError
	}
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

func lastErrorStr(rec *ChannelHealthRecord) string {
	if rec != nil {
		return rec.LastError
	}
	return ""
}
