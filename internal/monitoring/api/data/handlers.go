package data

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// DataChannel represents a single data source configuration and health status.
type DataChannel struct {
	Country    string `json:"country"`
	Platform   string `json:"platform"`
	APIFormat  string `json:"api_format"`
	Path       string `json:"path"`
	Storage    string `json:"storage"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UpdatedAt  string `json:"updated_at"`
	LastError  string `json:"last_error,omitempty"`
	ChannelID  string `json:"channel_id"`
}

// Handlers provides data channel and ingest API endpoints.
type Handlers struct {
	WorkDir           string
	Pool              *pgxpool.Pool
	MacroIngestor     *narrative.MacroIngestor
	GeoProvider       narrative.GeopoliticalRiskProvider
	TaiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider
	JanusEngine       *janus.Engine
	HealthRecorder    ChannelHealthRecorder
}

// NewHandlers creates a new data Handlers.
func NewHandlers(workDir string, pool *pgxpool.Pool, macroIngestor *narrative.MacroIngestor, geoProvider narrative.GeopoliticalRiskProvider, taiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider, janusEngine *janus.Engine, healthRecorder ChannelHealthRecorder) *Handlers {
	return &Handlers{
		WorkDir:           workDir,
		Pool:              pool,
		MacroIngestor:     macroIngestor,
		GeoProvider:       geoProvider,
		TaiwanGeoProvider: taiwanGeoProvider,
		JanusEngine:       janusEngine,
		HealthRecorder:    healthRecorder,
	}
}

// RegisterRoutes mounts data channel endpoints on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/data-channels", h.HandleDataChannels)
	mux.HandleFunc("/api/channels/ingest", h.HandleChannelsIngest)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
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

// HandleDataChannels handles GET /api/dashboard/data-channels.
func (h *Handlers) HandleDataChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	channels := make([]DataChannel, 0)

	// 1. Yahoo Finance Macro (US + Global)
	macroPath := filepath.Join(h.WorkDir, "data/state/macro/latest.json")
	macroStatus, macroUpdated := checkMacroHealth(macroPath, now)
	macroRec := h.HealthRecorder.Get("us_yahoo")
	if macroRec != nil && macroRec.Status != "" {
		macroStatus = macroRec.Status
		if macroRec.LastError != "" {
			macroUpdated = "上次失敗: " + macroRec.LastError
		} else {
			macroUpdated = "上次抓取: " + macroRec.LastFetchAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "us_yahoo",
		Country:    "美國",
		Platform:   "Yahoo Finance",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com/v8/finance/chart",
		Storage:    "data/state/macro/latest.json",
		Status:     macroStatus,
		StatusText: statusText(macroStatus),
		UpdatedAt:  macroUpdated,
		LastError: func() string {
			if macroRec != nil {
				return macroRec.LastError
			}
			return ""
		}(),
	})

	// 2. TWSE OpenAPI / T86 - Replay data
	replayPath := filepath.Join(h.WorkDir, "data/replay/tw_extended_90days.csv")
	replayStatus, replayUpdated := checkReplayHealth(replayPath, now)
	replayRec := h.HealthRecorder.Get("twse_replay")
	if replayRec != nil && replayRec.Status != "" {
		replayStatus = replayRec.Status
		if replayRec.LastError != "" {
			replayUpdated = "上次失敗: " + replayRec.LastError
		} else if replayRec.LastSuccessAt != "" {
			replayUpdated = "上次成功: " + replayRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "OpenAPI / CSV",
		Path:       "openapi.twse.com.tw / www.twse.com.tw",
		Storage:    "data/replay/tw_extended_90days.csv",
		Status:     replayStatus,
		StatusText: statusText(replayStatus),
		UpdatedAt:  replayUpdated,
		LastError: func() string {
			if replayRec != nil {
				return replayRec.LastError
			}
			return ""
		}(),
	})

	// 3. TWSE Capital Flow
	capFlowDir := filepath.Join(h.WorkDir, "data/state/capital_flow")
	capStatus, capUpdated := checkCapitalFlowHealth(capFlowDir, now)
	channels = append(channels, DataChannel{
		ChannelID:  "twse_capital_flow",
		Country:    "台灣",
		Platform:   "TWSE 三大法人",
		APIFormat:  "T86 JSON",
		Path:       "www.twse.com.tw/rwd/zh/fund/T86",
		Storage:    "data/state/capital_flow/*.json",
		Status:     capStatus,
		StatusText: statusText(capStatus),
		UpdatedAt:  capUpdated,
	})

	// 4. Fugle (optional/live)
	fugleKey := os.Getenv("FUGLE_API_KEY")
	if fugleKey == "" {
		fugleKey = os.Getenv("ATLAS_FUGLE_API_KEY")
	}
	fugleStatus := "inactive"
	fugleUpdated := "-"
	fugleLastError := ""
	if fugleKey != "" {
		fugleClient := marketdata.NewFugleClient(fugleKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		// Fugle free tier API key 只授權單一 symbol，先用 1476 測試連線
		_, err := fugleClient.GetQuote(ctx, "1476")
		cancel()
		if err != nil {
			fugleStatus = "error"
			fugleUpdated = "API 連線失敗"
			fugleLastError = err.Error()
		} else {
			fugleStatus = "ok"
			fugleUpdated = "API 連線正常"
		}
	} else {
		fugleUpdated = "未設定 API Key"
	}
	channels = append(channels, DataChannel{
		ChannelID:  "fugle",
		Country:    "台灣",
		Platform:   "Fugle 富果",
		APIFormat:  "REST JSON",
		Path:       "api.fugle.tw",
		Storage:    "(live cache / memory)",
		Status:     fugleStatus,
		StatusText: statusText(fugleStatus),
		UpdatedAt:  fugleUpdated,
		LastError:  fugleLastError,
	})

	// 4a. Fubon (optional/live)
	// 富邦證券 API 目前連線異常（TLS handshake timeout），暫時標記為未啟用
	channels = append(channels, DataChannel{
		ChannelID:  "fubon",
		Country:    "台灣",
		Platform:   "富邦證券",
		APIFormat:  "REST JSON",
		Path:       "api.fubon.com.tw",
		Storage:    "(live cache / memory)",
		Status:     "inactive",
		StatusText: statusText("inactive"),
		UpdatedAt:  "API 連線異常",
		LastError:  "",
	})

	// 4b. FinMind (optional/data backfill)
	finmindKey := os.Getenv("FINMIND_API_KEY")
	finmindStatus := "inactive"
	finmindUpdated := "-"
	finmindLastError := ""
	if finmindKey != "" {
		finmindClient := marketdata.NewFinMindClient(finmindKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		// 用近交易日測試（今天可能是假日無交易資料）
		_, err := finmindClient.GetStockPrice(ctx, "2330", time.Now().AddDate(0, 0, -1).Format("2006-01-02"))
		cancel()
		if err != nil {
			finmindStatus = "error"
			finmindUpdated = "API 連線失敗"
			finmindLastError = err.Error()
		} else {
			finmindStatus = "ok"
			finmindUpdated = "API 連線正常"
		}
	} else {
		finmindUpdated = "未設定 API Key"
	}
	channels = append(channels, DataChannel{
		ChannelID:  "finmind",
		Country:    "台灣",
		Platform:   "FinMind",
		APIFormat:  "REST JSON",
		Path:       "api.finmindtrade.com",
		Storage:    "(live cache / memory)",
		Status:     finmindStatus,
		StatusText: statusText(finmindStatus),
		UpdatedAt:  finmindUpdated,
		LastError:  finmindLastError,
	})

	// 5. JPY via Yahoo (Japan indicator, same endpoint as US)
	jpyStatus, jpyUpdated := checkJPYHealth(macroPath, now)
	jpyRec := h.HealthRecorder.Get("jpy_yahoo")
	if jpyRec != nil && jpyRec.Status != "" {
		jpyStatus = jpyRec.Status
		if jpyRec.LastError != "" {
			jpyUpdated = "上次失敗: " + jpyRec.LastError
		} else {
			jpyUpdated = "上次抓取: " + jpyRec.LastFetchAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "jpy_yahoo",
		Country:    "日本",
		Platform:   "Yahoo Finance (JPY)",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com/v8/finance/chart",
		Storage:    "data/state/macro/latest.json",
		Status:     jpyStatus,
		StatusText: statusText(jpyStatus),
		UpdatedAt:  jpyUpdated,
		LastError: func() string {
			if jpyRec != nil {
				return jpyRec.LastError
			}
			return ""
		}(),
	})

	// 6. Geopolitical Risk (RSS + GDELT)
	geoPath := filepath.Join(h.WorkDir, "data/state/geopolitical/latest.json")
	geoStatus, geoUpdated := checkGeopoliticalHealth(geoPath, now)
	geoRec := h.HealthRecorder.Get("geopolitical")
	if geoRec != nil && geoRec.Status != "" {
		geoStatus = geoRec.Status
		if geoRec.LastError != "" {
			geoUpdated = "上次失敗: " + geoRec.LastError
		} else {
			geoUpdated = "上次抓取: " + geoRec.LastFetchAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "geopolitical",
		Country:    "中東/全球",
		Platform:   "RSS + GDELT",
		APIFormat:  "RSS / REST JSON",
		Path:       "feeds.bbci.co.uk / api.gdeltproject.org",
		Storage:    "data/state/geopolitical/latest.json",
		Status:     geoStatus,
		StatusText: statusText(geoStatus),
		UpdatedAt:  geoUpdated,
		LastError: func() string {
			if geoRec != nil {
				return geoRec.LastError
			}
			return ""
		}(),
	})

	// 7. TWSE Margin (Retail Leverage — reverse indicator)
	marginDir := filepath.Join(h.WorkDir, "data/state/margin")
	marginStatus, marginUpdated := checkCapitalFlowHealth(marginDir, now)
	marginRec := h.HealthRecorder.Get("twse_margin")
	if marginRec != nil && marginRec.Status != "" {
		marginStatus = marginRec.Status
		if marginRec.LastError != "" {
			marginUpdated = "上次失敗: " + marginRec.LastError
		} else if marginRec.LastSuccessAt != "" {
			marginUpdated = "上次成功: " + marginRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "twse_margin",
		Country:    "台灣",
		Platform:   "TWSE 融資融券",
		APIFormat:  "Miantane JSON",
		Path:       "www.twse.com.tw/rwd/zh/marginTradingMiantane",
		Storage:    "data/state/margin/*_margin.json",
		Status:     marginStatus,
		StatusText: statusText(marginStatus),
		UpdatedAt:  marginUpdated,
		LastError: func() string {
			if marginRec != nil {
				return marginRec.LastError
			}
			return ""
		}(),
	})

	// 8. Export Statistics (Electronics export proxy for tech sector health)
	// TWSE FAS210 decommissioned; replaced with customs open data (data.gov.tw dataset 6053).
	exportDir := filepath.Join(h.WorkDir, "data/state/export")
	exportStatus, exportUpdated := checkExportHealth(exportDir, now)
	exportRec := h.HealthRecorder.Get("export_statistics")
	if exportRec != nil && exportRec.Status != "" {
		exportStatus = exportRec.Status
		if exportRec.LastError != "" {
			exportUpdated = "上次失敗: " + exportRec.LastError
		} else if exportRec.LastSuccessAt != "" {
			exportUpdated = "上次成功: " + exportRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "export_statistics",
		Country:    "台灣",
		Platform:   "海關進出口統計 (data.gov.tw)",
		APIFormat:  "CSV",
		Path:       "opendata.customs.gov.tw/data/6053/csv.csv",
		Storage:    "data/state/export/*_export.json",
		Status:     exportStatus,
		StatusText: statusText(exportStatus),
		UpdatedAt:  exportUpdated,
		LastError:  "",
	})

	// 9. TSMC Revenue (AI capex sentiment proxy)
	tsmcDir := filepath.Join(h.WorkDir, "data/state/tsmc_revenue")
	tsmcStatus, tsmcUpdated := checkCapitalFlowHealth(tsmcDir, now)
	tsmcRec := h.HealthRecorder.Get("tsmc_revenue")
	if tsmcRec != nil && tsmcRec.Status != "" {
		tsmcStatus = tsmcRec.Status
		if tsmcRec.LastError != "" {
			tsmcUpdated = "上次失敗: " + tsmcRec.LastError
		} else if tsmcRec.LastSuccessAt != "" {
			tsmcUpdated = "上次成功: " + tsmcRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "tsmc_revenue",
		Country:    "台灣",
		Platform:   "TWSE 台積電月營收",
		APIFormat:  "TWT49U JSON",
		Path:       "www.twse.com.tw/rwd/zh/fund/TWT49U",
		Storage:    "data/state/tsmc_revenue/*_revenue.json",
		Status:     tsmcStatus,
		StatusText: statusText(tsmcStatus),
		UpdatedAt:  tsmcUpdated,
		LastError: func() string {
			if tsmcRec != nil {
				return tsmcRec.LastError
			}
			return ""
		}(),
	})

	// 10. Taiwan Geopolitical Risk (RSS + Cross-Strait monitoring)
	twGeoDir := filepath.Join(h.WorkDir, "data/state/geopolitical/taiwan")
	twGeoStatus, twGeoUpdated := checkCapitalFlowHealth(twGeoDir, now)
	twGeoRec := h.HealthRecorder.Get("geopolitical_taiwan")
	if twGeoRec != nil && twGeoRec.Status != "" {
		twGeoStatus = twGeoRec.Status
		if twGeoRec.LastError != "" {
			twGeoUpdated = "上次失敗: " + twGeoRec.LastError
		} else if twGeoRec.LastSuccessAt != "" {
			twGeoUpdated = "上次成功: " + twGeoRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "geopolitical_taiwan",
		Country:    "台灣",
		Platform:   "CNA / 自由時報 / TVBS RSS",
		APIFormat:  "RSS XML",
		Path:       "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw",
		Storage:    "data/state/geopolitical/taiwan/latest.json",
		Status:     twGeoStatus,
		StatusText: statusText(twGeoStatus),
		UpdatedAt:  twGeoUpdated,
		LastError: func() string {
			if twGeoRec != nil {
				return twGeoRec.LastError
			}
			return ""
		}(),
	})

	// 11. JANUS Regime (Meta-layer regime detection)
	janusStatus, janusUpdated := checkJanusHealth(h.JanusEngine, now)
	janusRec := h.HealthRecorder.Get("janus_regime")
	if janusRec != nil && janusRec.Status != "" {
		janusStatus = janusRec.Status
		if janusRec.LastError != "" {
			janusUpdated = "上次失敗: " + janusRec.LastError
		} else if janusRec.LastSuccessAt != "" {
			janusUpdated = "上次成功: " + janusRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "janus_regime",
		Country:    "全域",
		Platform:   "JANUS Engine",
		APIFormat:  "Internal",
		Path:       "internal/janus",
		Storage:    "(in-memory state)",
		Status:     janusStatus,
		StatusText: statusText(janusStatus),
		UpdatedAt:  janusUpdated,
		LastError: func() string {
			if janusRec != nil {
				return janusRec.LastError
			}
			return ""
		}(),
	})

	// 12. TEJ (Taiwan Economic Journal - premium financial data)
	tejStatus := "inactive"
	tejUpdated := "TEJ_API_KEY not configured"
	tejKey := os.Getenv("TEJ_API_KEY")
	if tejKey != "" {
		tejStatus = "ok"
		tejUpdated = "TEJ API key configured"
		tejRec := h.HealthRecorder.Get("tej")
		if tejRec != nil && tejRec.Status != "" {
			tejStatus = tejRec.Status
			if tejRec.LastError != "" {
				tejUpdated = "上次失敗: " + tejRec.LastError
			} else if tejRec.LastSuccessAt != "" {
				tejUpdated = "上次成功: " + tejRec.LastSuccessAt
			}
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "tej",
		Country:    "台灣",
		Platform:   "TEJ 台灣經濟新報",
		APIFormat:  "REST JSON",
		Path:       "TEJ API (premium)",
		Storage:    "N/A (live query)",
		Status:     tejStatus,
		StatusText: statusText(tejStatus),
		UpdatedAt:  tejUpdated,
		LastError:  "",
	})

	// Build alerts list from FRESH channel status only.
	// KnownInactive channels are permanently disabled and never produce alerts.
	knownInactive := map[string]bool{
		"fubon": true,
	}
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels":  channels,
		"alerts":    alerts,
		"generated": now.Format("2006-01-02 15:04:05"),
	})
}

// HandleChannelsIngest handles POST /api/channels/ingest.
func (h *Handlers) HandleChannelsIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stateDir := filepath.Join(h.WorkDir, "data/state")
	var wg sync.WaitGroup
	var macroErr, geoErr, capFlowErr, exportErr, tsmcErr, twGeoErr, janusErr, tejErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		events, snap, err := h.MacroIngestor.Ingest(r.Context())
		if err != nil {
			macroErr = err
			h.HealthRecorder.Record("us_yahoo", "error", err.Error())
			h.HealthRecorder.Record("jpy_yahoo", "error", err.Error())
			log.Printf("[HandleChannelsIngest] macro ingest failed: %v", err)
			return
		}
		h.HealthRecorder.Record("us_yahoo", "ok", "")
		h.HealthRecorder.Record("jpy_yahoo", "ok", "")
		log.Printf("[HandleChannelsIngest] macro ingest succeeded: %d events, recorded_at=%d", len(events), snap.RecordedAt)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		score, err := h.GeoProvider.FetchScore(r.Context())
		if err != nil {
			geoErr = err
			h.HealthRecorder.Record("geopolitical", "error", err.Error())
			log.Printf("[HandleChannelsIngest] geo ingest failed: %v", err)
			return
		}
		store := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical"))
		if err := store.Save(score); err != nil {
			geoErr = err
			h.HealthRecorder.Record("geopolitical", "error", err.Error())
			log.Printf("[HandleChannelsIngest] geo save failed: %v", err)
			return
		}
		h.HealthRecorder.Record("geopolitical", "ok", "")
		log.Printf("[HandleChannelsIngest] geo ingest succeeded: intensity=%.2f", score.Intensity)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(stateDir, "capital_flow"))
		_, err := capFlowProvider.FetchSnapshot(r.Context())
		if err != nil {
			capFlowErr = err
			h.HealthRecorder.Record("twse_capital_flow", "error", err.Error())
			log.Printf("[HandleChannelsIngest] capital flow ingest failed: %v", err)
			return
		}
		h.HealthRecorder.Record("twse_capital_flow", "ok", "")
		log.Printf("[HandleChannelsIngest] capital flow ingest succeeded")
	}()

	// Export statistics (customs open data — replaced TWSE FAS210)
	wg.Add(1)
	go func() {
		defer wg.Done()
		exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(stateDir, "export"))
		_, err := exportProvider.FetchSnapshot(r.Context())
		if err != nil {
			exportErr = err
			h.HealthRecorder.Record("export_statistics", "error", err.Error())
			log.Printf("[HandleChannelsIngest] export statistics ingest failed: %v", err)
			return
		}
		h.HealthRecorder.Record("export_statistics", "ok", "")
		log.Printf("[HandleChannelsIngest] export statistics ingest succeeded")
	}()

	// TWSE balance / margin (retail sentiment proxy)
	wg.Add(1)
	go func() {
		defer wg.Done()
		marginProvider := marketdata.NewTWSEBalanceProvider(filepath.Join(stateDir, "margin"))
		_, err := marginProvider.FetchSnapshot(r.Context())
		if err != nil {
			h.HealthRecorder.Record("twse_margin", "error", err.Error())
			log.Printf("[HandleChannelsIngest] TWSE margin balance ingest failed: %v", err)
			return
		}
		h.HealthRecorder.Record("twse_margin", "ok", "")
		log.Printf("[HandleChannelsIngest] TWSE margin balance ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tsmcProvider := marketdata.NewTSMCRevenueProvider(filepath.Join(stateDir, "tsmc_revenue"))
		_, err := tsmcProvider.FetchSnapshot(r.Context())
		if err != nil {
			tsmcErr = err
			h.HealthRecorder.Record("tsmc_revenue", "error", err.Error())
			log.Printf("[HandleChannelsIngest] TSMC revenue ingest failed: %v", err)
			return
		}
		h.HealthRecorder.Record("tsmc_revenue", "ok", "")
		log.Printf("[HandleChannelsIngest] TSMC revenue ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		twGeoScore, err := h.TaiwanGeoProvider.FetchScore(r.Context())
		if err != nil {
			twGeoErr = err
			h.HealthRecorder.Record("geopolitical_taiwan", "error", err.Error())
			log.Printf("[HandleChannelsIngest] Taiwan geopolitical ingest failed: %v", err)
			return
		}
		twStore := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical", "taiwan"))
		if err := twStore.Save(twGeoScore); err != nil {
			twGeoErr = err
			h.HealthRecorder.Record("geopolitical_taiwan", "error", err.Error())
			log.Printf("[HandleChannelsIngest] Taiwan geopolitical save failed: %v", err)
			return
		}
		h.HealthRecorder.Record("geopolitical_taiwan", "ok", "")
		log.Printf("[HandleChannelsIngest] Taiwan geopolitical ingest succeeded: intensity=%.2f", twGeoScore.Intensity)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if h.JanusEngine == nil {
			janusErr = fmt.Errorf("JANUS engine not initialized")
			h.HealthRecorder.Record("janus_regime", "error", janusErr.Error())
			log.Printf("[HandleChannelsIngest] JANUS regime ingest skipped: engine not initialized")
			return
		}
		h.JanusEngine.Update()
		status := h.JanusEngine.GetStatus()
		if status.LastUpdated.IsZero() {
			janusErr = fmt.Errorf("JANUS engine has no data after update")
			h.HealthRecorder.Record("janus_regime", "error", janusErr.Error())
			log.Printf("[HandleChannelsIngest] JANUS regime ingest failed: %v", janusErr)
			return
		}
		h.HealthRecorder.Record("janus_regime", "ok", "")
		log.Printf("[HandleChannelsIngest] JANUS regime ingest succeeded: class=%s", status.Classification)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tejKey := os.Getenv("TEJ_API_KEY")
		if tejKey == "" {
			h.HealthRecorder.Record("tej", "inactive", "TEJ_API_KEY not set")
			log.Printf("[HandleChannelsIngest] TEJ ingest skipped: TEJ_API_KEY not set")
			return
		}
		tejClient := marketdata.NewTEJClient(tejKey)
		if err := tejClient.Ping(r.Context()); err != nil {
			tejErr = err
			h.HealthRecorder.Record("tej", "error", err.Error())
			log.Printf("[HandleChannelsIngest] TEJ ingest failed: %v", err)
			return
		}
		h.HealthRecorder.Record("tej", "ok", "")
		log.Printf("[HandleChannelsIngest] TEJ ingest succeeded")
	}()

	wg.Wait()

	result := map[string]any{
		"macro_ok":    macroErr == nil,
		"geo_ok":      geoErr == nil,
		"cap_flow_ok": capFlowErr == nil,
		"export_ok":   exportErr == nil,
		"tsmc_ok":     tsmcErr == nil,
		"tw_geo_ok":   twGeoErr == nil,
		"janus_ok":    janusErr == nil,
		"tej_ok":      tejErr == nil,
	}
	if macroErr != nil {
		result["macro_error"] = macroErr.Error()
	}
	if geoErr != nil {
		result["geo_error"] = geoErr.Error()
	}
	if capFlowErr != nil {
		result["cap_flow_error"] = capFlowErr.Error()
	}
	if exportErr != nil {
		result["export_error"] = exportErr.Error()
	}
	if tsmcErr != nil {
		result["tsmc_error"] = tsmcErr.Error()
	}
	if twGeoErr != nil {
		result["tw_geo_error"] = twGeoErr.Error()
	}
	if janusErr != nil {
		result["janus_error"] = janusErr.Error()
	}
	if tejErr != nil {
		result["tej_error"] = tejErr.Error()
	}

	if macroErr != nil && geoErr != nil && capFlowErr != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("all core ingests failed: macro=%v, geo=%v, cap_flow=%v", macroErr, geoErr, capFlowErr))
		return
	}

	writeJSON(w, http.StatusOK, result)
}
