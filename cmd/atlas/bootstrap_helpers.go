package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/portprobe"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// universeWatchlistMu is the package-level mutex shared by all universe-builder
// task closures to serialize concurrent read-modify-write on universe_watchlist.json.
// Defined here because it is used exclusively by newUniverseBuilderDeps below.
var universeWatchlistMu sync.Mutex

// experimentMonitorAdapter wraps *monitoring.Monitor to match experiment.AutoExperimentMonitor interface.
type experimentMonitorAdapter struct {
	m *monitoring.Monitor
}

// ResolveAlert implements experiment.EvolutionAlertResolver (#1787):
// clears open alerts whose condition has recovered. Empty identity means
// category-wide (identity-less alert families).
func (a *experimentMonitorAdapter) ResolveAlert(category, identity, reason string) {
	if a.m != nil {
		a.m.ResolveByIdentity(category, identity, reason)
	}
}

func (a *experimentMonitorAdapter) Alert(level string, category, message string, details map[string]any) {
	if a.m != nil {
		var al monitoring.AlertLevel
		switch level {
		case "error":
			al = monitoring.AlertLevelError
		case "warning":
			al = monitoring.AlertLevelWarning
		default:
			al = monitoring.AlertLevelInfo
		}
		a.m.Alert(al, category, message, details)
	}
}

func defaultAppDeps() appDeps {
	return appDeps{
		loadConfig: config.Load,
		newDashboardAPI: func(workDir, ledgerDir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			// Gateway initialization failed; use a no-op fetcher instead of silently falling back
			// to the legacy CompositeMacroProvider path. This makes the degraded state explicit.
			logging.Warn("bootstrap", "gateway_unavailable_using_noop_fetcher")
			return monitoring.NewDashboardAPIWithGateway(workDir, ledgerDir, collector, monitoring.NoopFetcher())
		},
		listenAndServe: func(srv *http.Server) error {
			// http.Server default Addr is "" which ListenAndServe maps to ":http".
			// Skip portprobe for empty Addr: lsof + classification assume bindable
			// loopback and would probe port 80, which needs root on most systems.
			if srv.Addr == "" {
				return srv.ListenAndServe()
			}
			ln, err := portprobe.Listen(srv.Addr)
			if err != nil {
				return err
			}
			return srv.Serve(ln)
		},
		shutdown: make(chan struct{}),
	}
}

// getLatestReplayDate reads the replay CSV and returns the latest date.
func getLatestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	var latest time.Time
	_, _ = reader.Read()
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if len(row) == 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		if d.After(latest) {
			latest = d
		}
	}
	if latest.IsZero() {
		return time.Time{}, fmt.Errorf("no valid dates found")
	}
	return latest, nil
}

func publishBootstrapEvents(bus eventbus.EventBus, replayPath, baselinePath string) {
	now := time.Now()

	// Check data status
	replayStatus := "已載入"
	replayDate := ""
	if _, err := os.Stat(replayPath); os.IsNotExist(err) {
		replayStatus = "未找到"
	} else if d, err := getLatestReplayDate(replayPath); err == nil {
		replayDate = d.Format("2006-01-02")
	} else {
		replayStatus = "載入失敗"
	}

	baselineStatus := "已載入"
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		baselineStatus = "未找到"
	}

	bus.Publish(eventbus.BusEvent{
		ID:        "bootstrap-" + now.Format("150405"),
		Type:      eventbus.EventSystemStart,
		Timestamp: now,
		Description: "Atlas 系統啟動完成 · replay 資料 " + replayStatus + func() string {
			if replayDate != "" {
				return "（" + replayDate + "）"
			}
			return ""
		}() + " · 基線策略 " + baselineStatus,
		Severity: "info",
		Payload: map[string]any{
			"replay_status":   replayStatus,
			"replay_date":     replayDate,
			"baseline_status": baselineStatus,
		},
	})
}

// shouldStartFubonProxy 判斷是否應自動啟動 fubon-proxy 服務。
// 兩種情境觸發：
//   - broker mode 為 "live"（即時交易模式，需要 proxy 處理 broker API 請求）
//   - FUBON_API_KEY 已設定（即使非 live 模式，dashboard API 也需要 fubon 資料通道）
//
// 目標：讓 `atlas -api` 搭配 FUBON_API_KEY 設定時能一鍵啟動 fubon-proxy，
// 不需要使用者手動啟動 Python FastAPI 微服務。
func shouldStartFubonProxy(mode string, fubonAPIKey string) bool {
	return mode == "live" || fubonAPIKey != ""
}

// narrativeFeedFetcher adapts *apigateway.Gateway into a monitoring.FeedFetcher.
// It bridges the monitoring↔apigateway package boundary without creating an
// import cycle (apigateway already imports monitoring for WithLatencyMs).
func narrativeFeedFetcher(gw *apigateway.Gateway) monitoring.FeedFetcher {
	if gw == nil {
		return nil
	}
	return func(ctx context.Context, channelID string) (*monitoring.FeedData, error) {
		result, err := gw.Fetch(ctx, channelID)
		if err != nil {
			return nil, err
		}
		return &monitoring.FeedData{Data: result.Data, Stale: result.Stale, LastError: result.LastError}, nil
	}
}

// newUniverseBuilderDeps constructs the SmartUniverseBuilder dependency
// struct with all parameters wired from SmartUniverseConfig. Callers
// should invoke this once at startup; the returned deps are safe to
// share across daily/weekly scheduler tasks.
func newUniverseBuilderDeps(
	cfg config.Config,
	classTreeAdapter monitoring.ClassificationTreeAccessor,
	gateway *apigateway.Gateway,
	um *metrics.UniverseMetrics,
	suCfg config.SmartUniverseConfig,
) monitoring.UniverseBuilderDeps {
	riskFilter := monitoring.NewRiskExclusionFilter(nil, nil, portfolio.NewHistoricalPrices())
	riskFilter.Configure(suCfg)
	narrativeBridge := monitoring.NewNarrativeEventBridgeWithFetcher(
		filepath.Join(cfg.WorkDir, "data", "state", "narrative_cache.json"),
		narrativeFeedFetcher(gateway),
	)
	narrativeBridge.Configure(suCfg)
	factorEngine := portfolio.NewFactorEngine()
	return monitoring.UniverseBuilderDeps{
		Mapper:          monitoring.NewTreeBasedMapper(classTreeAdapter),
		Tree:            classTreeAdapter,
		SupplyChain:     monitoring.AdaptSupplyChainGraph(industry.NewSupplyChainGraph()),
		Screener:        screener.NewEngine(factorEngine, portfolio.NewFundamentalProvider()),
		FactorEng:       monitoring.AdaptFactorEngine(factorEngine),
		Quotes:          nil,
		RiskFilter:      riskFilter,
		NarrativeBridge: narrativeBridge,
		UniverseMetrics: um,
		Config:          suCfg,
		WorkDir:         cfg.WorkDir,
		WatchlistMu:     &universeWatchlistMu,
	}
}
