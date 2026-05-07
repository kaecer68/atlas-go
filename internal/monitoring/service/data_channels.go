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
	"github.com/kaecer68/atlas-go/internal/marketdata"
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
	Status        string `json:"status"`
	LastFetchAt   string `json:"last_fetch_at"`
	LastError     string `json:"last_error,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

type DataChannelService struct {
	WorkDir           string
	Pool              *pgxpool.Pool
	MacroIngestor     *narrative.MacroIngestor
	GeoProvider       narrative.GeopoliticalRiskProvider
	TaiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider
	JanusEngine       *janus.Engine
	healthStore       *ChannelHealthStoreAdapter
}

type ChannelHealthStoreAdapter struct {
	pool *pgxpool.Pool
	dir  string
}

func NewChannelHealthStoreAdapter(dir string, pool *pgxpool.Pool) *ChannelHealthStoreAdapter {
	return &ChannelHealthStoreAdapter{pool: pool, dir: dir}
}

func (a *ChannelHealthStoreAdapter) Get(channelID string) *ChannelHealthRecord {
	store := newChannelHealthStore(a.dir, a.pool)
	return store.Get(channelID)
}

func (a *ChannelHealthStoreAdapter) Record(channelID, status, errMsg string) error {
	store := newChannelHealthStore(a.dir, a.pool)
	return store.Record(channelID, status, errMsg)
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

func (s *channelHealthStore) Record(channelID, status, errMsg string) error {
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

func (s *channelHealthStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
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

func NewDataChannelService(workDir string, pool *pgxpool.Pool, macroIngestor *narrative.MacroIngestor, geoProvider narrative.GeopoliticalRiskProvider, taiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider, janusEngine *janus.Engine) *DataChannelService {
	return &DataChannelService{
		WorkDir:           workDir,
		Pool:              pool,
		MacroIngestor:     macroIngestor,
		GeoProvider:       geoProvider,
		TaiwanGeoProvider: taiwanGeoProvider,
		JanusEngine:       janusEngine,
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
	channels = append(channels, s.buildFugleChannel(now))
	channels = append(channels, s.buildFubonChannel(now))
	channels = append(channels, s.buildFinMindChannel(now))
	channels = append(channels, s.buildJPYYahooChannel(now))
	channels = append(channels, s.buildGeopoliticalChannel(now))
	channels = append(channels, s.buildTWSEMarginChannel(now))
	channels = append(channels, s.buildExportStatisticsChannel(now))
	channels = append(channels, s.buildTSMCRevenueChannel(now))
	channels = append(channels, s.buildTaiwanGeopoliticalChannel(now))
	channels = append(channels, s.buildJanusRegimeChannel(now))
	channels = append(channels, s.buildTEJChannel(now))

	return channels, nil
}

func (s *DataChannelService) buildUSYahooChannel(now time.Time) DataChannel {
	macroPath := filepath.Join(s.WorkDir, "data/state/macro/latest.json")
	status, updated := checkMacroHealth(macroPath, now)
	rec := s.healthStore.Get("us_yahoo")
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else {
			updated = "上次抓取: " + rec.LastFetchAt
		}
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
	replayPath := filepath.Join(s.WorkDir, "data/replay/tw_extended_90days.csv")
	status, updated := checkReplayHealth(replayPath, now)
	rec := s.healthStore.Get("twse_replay")
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else if rec.LastSuccessAt != "" {
			updated = "上次成功: " + rec.LastSuccessAt
		}
	}
	return DataChannel{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "OpenAPI / CSV",
		Path:       "openapi.twse.com.tw / www.twse.com.tw",
		Storage:    "data/replay/tw_extended_90days.csv",
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
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else if rec.LastSuccessAt != "" {
			updated = "上次成功: " + rec.LastSuccessAt
		}
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

func (s *DataChannelService) buildFugleChannel(now time.Time) DataChannel {
	fugleKey := os.Getenv("FUGLE_API_KEY")
	if fugleKey == "" {
		fugleKey = os.Getenv("ATLAS_FUGLE_API_KEY")
	}
	status := "inactive"
	updated := "-"
	lastError := ""
	if fugleKey != "" {
		fugleClient := marketdata.NewFugleClient(fugleKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := fugleClient.GetQuote(ctx, "1476")
		cancel()
		if err != nil {
			status = "error"
			updated = "API 連線失敗"
			lastError = err.Error()
		} else {
			status = "ok"
			updated = "API 連線正常"
		}
	} else {
		updated = "未設定 API Key"
	}
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

func (s *DataChannelService) buildFubonChannel(now time.Time) DataChannel {
	fubonKey := os.Getenv("FUBON_API_KEY")
	if fubonKey == "" {
		fubonKey = os.Getenv("ATLAS_FUBON_API_KEY")
	}
	status := "inactive"
	updated := "-"
	lastError := ""
	if fubonKey != "" {
		fubonClient := marketdata.NewFubonClient(fubonKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := fubonClient.HealthCheck(ctx)
		cancel()
		if err != nil {
			status = "error"
			updated = "API 連線失敗"
			lastError = err.Error()
		} else {
			status = "ok"
			updated = "API 連線正常"
		}
	} else {
		updated = "未設定 API Key"
	}
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

func (s *DataChannelService) buildFinMindChannel(now time.Time) DataChannel {
	finmindKey := os.Getenv("FINMIND_API_KEY")
	status := "inactive"
	updated := "-"
	lastError := ""
	if finmindKey != "" {
		finmindClient := marketdata.NewFinMindClient(finmindKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := finmindClient.GetStockPrice(ctx, "2330", time.Now().AddDate(0, 0, -1).Format("2006-01-02"))
		cancel()
		if err != nil {
			status = "error"
			updated = "API 連線失敗"
			lastError = err.Error()
		} else {
			status = "ok"
			updated = "API 連線正常"
		}
	} else {
		updated = "未設定 API Key"
	}
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
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else {
			updated = "上次抓取: " + rec.LastFetchAt
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
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else {
			updated = "上次抓取: " + rec.LastFetchAt
		}
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
	status, updated := checkCapitalFlowHealth(marginDir, now)
	rec := s.healthStore.Get("twse_margin")
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else if rec.LastSuccessAt != "" {
			updated = "上次成功: " + rec.LastSuccessAt
		}
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
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else if rec.LastSuccessAt != "" {
			updated = "上次成功: " + rec.LastSuccessAt
		}
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
	status, updated := checkCapitalFlowHealth(tsmcDir, now)
	rec := s.healthStore.Get("tsmc_revenue")
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else if rec.LastSuccessAt != "" {
			updated = "上次成功: " + rec.LastSuccessAt
		}
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
	twGeoDir := filepath.Join(s.WorkDir, "data/state/geopolitical/taiwan")
	status, updated := checkCapitalFlowHealth(twGeoDir, now)
	rec := s.healthStore.Get("geopolitical_taiwan")
	if rec != nil && rec.Status != "" {
		status = rec.Status
		if rec.LastError != "" {
			updated = "上次失敗: " + rec.LastError
		} else if rec.LastSuccessAt != "" {
			updated = "上次成功: " + rec.LastSuccessAt
		}
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

func (s *DataChannelService) buildTEJChannel(now time.Time) DataChannel {
	status := "inactive"
	updated := "TEJ_API_KEY not configured"
	tejKey := config.GetSecret("TEJ_API_KEY")
	if tejKey != "" {
		status = "ok"
		updated = "TEJ API key configured"
		rec := s.healthStore.Get("tej")
		if rec != nil && rec.Status != "" {
			status = rec.Status
			if rec.LastError != "" {
				updated = "上次失敗: " + rec.LastError
			} else if rec.LastSuccessAt != "" {
				updated = "上次成功: " + rec.LastSuccessAt
			}
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
	switch status {
	case "ok":
		return "正常"
	case "warn":
		return "延遲"
	case "error":
		return "異常"
	case "inactive":
		return "未啟用"
	default:
		return "未知"
	}
}

func lastErrorStr(rec *ChannelHealthRecord) string {
	if rec != nil {
		return rec.LastError
	}
	return ""
}
