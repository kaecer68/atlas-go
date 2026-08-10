package apigateway

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// RegisterChannelAdapters creates concrete market-data clients from cfg,
// wraps each in a channel adapter, and registers them in the Gateway's
// ChannelRegistry. Clients that require API keys are silently skipped
// when the key is not configured.
func RegisterChannelAdapters(g *Gateway, workDir string, cfg config.Config, janusEngine *janus.Engine) error {
	if g == nil {
		return fmt.Errorf("gateway is nil")
	}

	// --- Fugle ---
	if cfg.FugleAPIKey != "" {
		fugleClient := marketdata.GetSharedFugleClient(cfg.FugleAPIKey)
		fugleAdapter := NewFugleChannelAdapter(fugleClient)
		g.registry.Register("fugle", fugleAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "fugle")
	}

	// --- Fubon ---
	// Startup probe: skip registration if the local proxy is not reachable,
	// avoiding constant connection-refused errors at runtime.
	//
	// 雙位址 probe:127.0.0.1 (本機開發,go run in macOS host) 先測,
	// 再測 fubon-proxy (Docker DNS,容器內).根據結果選取正確的 proxy host:
	//   - 只有 127.0.0.1 可達 → 叫 SetProxyHost("127.0.0.1")
	//   - fubon-proxy 可達 → 用預設值(不需覆寫)
	fubonKey := cfg.FubonAPIKey
	if fubonKey == "" {
		fubonKey = config.GetSecret("ATLAS_FUBON_API_KEY")
	}
	if fubonKey != "" {
		fubonPort := fubonproxy.GetFubonProxyPort()
		localAddr := fmt.Sprintf("127.0.0.1:%d", fubonPort)
		dockerAddr := fubonproxy.ProxyHostPort()

		// probeAddr 嘗試一次 TCP dial,回傳成功/失敗.
		probeAddr := func(addr string) bool {
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return false
			}
			_ = conn.Close()
			return true
		}

		localOK := probeAddr(localAddr)
		proxyOK := probeAddr(dockerAddr)

		if !localOK && !proxyOK {
			logging.Info("apigateway", "fubon_proxy_not_reachable",
				"msg", fmt.Sprintf("skipping fubon adapter registration — fubon-proxy not reachable on %s or %s", localAddr, dockerAddr))
		} else {
			// 若只有本機 loopback 可達,覆寫 proxy host 為 127.0.0.1
			if localOK && !proxyOK {
				fubonproxy.SetProxyHost("127.0.0.1")
				logging.Info("apigateway", "fubon_proxy_host_override",
					"msg", "fubon-proxy only reachable on 127.0.0.1, overriding proxy host")
			}
			fubonClient := marketdata.GetSharedFubonClient()
			fubonAdapter := NewFubonChannelAdapter(fubonClient)
			g.registry.Register("fubon", fubonAdapter)
			logging.Info("apigateway", "adapter_registered", "channel", "fubon")
		}
	}

	// --- FinMind ---
	if cfg.FinMindAPIKey != "" {
		finmindClient := marketdata.GetSharedFinMindClient(cfg.FinMindAPIKey)
		finmindAdapter := NewFinMindChannelAdapter(finmindClient)
		g.registry.Register("finmind", finmindAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "finmind")
	}

	// --- TWSE (no API key required) ---
	twseClient := marketdata.GetSharedTWSEClient()
	twseAdapter := NewTWSEChannelAdapter(twseClient)
	g.registry.Register("twse_replay", twseAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_replay")

	// --- Yahoo Finance Macro ---
	if cfg.YahooEnabled {
		yahooProvider := marketdata.NewYahooFinanceMacroProvider()
		yahooAdapter := NewYahooMacroChannelAdapter(yahooProvider)
		g.registry.Register("us_yahoo", yahooAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "us_yahoo")
	}

	// --- TWSE Capital Flow (no API key required) ---
	capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, constants.StateCapitalFlow))
	capFlowAdapter := NewTWSECapitalFlowChannelAdapter(capFlowProvider)
	g.registry.Register("twse_capital_flow", capFlowAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_capital_flow")

	// --- TWSE Margin Balance (no API key required) ---
	marginProvider := marketdata.NewTWSEMarginBalanceProvider(filepath.Join(workDir, "data/state/margin"))
	marginAdapter := NewTWSEMarginChannelAdapter(marginProvider)
	g.registry.Register("twse_margin", marginAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_margin")

	// --- Export Statistics (no API key required) ---
	exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(workDir, constants.StateExport))
	exportAdapter := NewExportStatisticsChannelAdapter(exportProvider)
	g.registry.Register("export_statistics", exportAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "export_statistics")

	// --- TEJ ---
	// DISABLED 2026-08-03 — TEJ free trial API key (AAA003) expired on 2026-07-31.
	// Source-layer audit (PR chore/20260803-disable-tej) confirmed no mission-required
	// endpoint consumes TEJ data. Channel registration is gated on TEJ_API_KEY so the
	// adapter only joins the registry when a valid key is configured. The matching
	// tej_refresh scheduler in cmd/atlas/main.go reads the same secret, keeping
	// channel + scheduler in lock-step (T3-A47 fix). To re-enable, set TEJ_API_KEY
	// (and TEJ_TIER=paid if upgraded) in the atlas env.
	if tejKey := config.GetSecret("TEJ_API_KEY"); tejKey != "" {
		tejClient := marketdata.GetSharedTEJClient(tejKey)
		tejAdapter := NewTEJChannelAdapter(tejClient)
		g.registry.Register("tej", tejAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "tej")
	} else {
		// Mark TEJ as inactive so dashboard + Alerts() stop reporting the
		// stale AAA003 error from before the 2026-08-03 disable. The status
		// "inactive" is filtered by UnifiedHealthStore.Alerts() (status != "ok"
		// && status != "inactive"), keeping this from generating false alerts,
		// and renders as "未啟用" via monitoring/service/session.go StatusText.
		// Pair with the tej_refresh scheduler gate in cmd/atlas/main.go:1670
		// so channel + scheduler + health record all stay in lock-step.
		if err := g.Health().Record("tej", "inactive", "TEJ_API_KEY not configured (PR chore/20260803-disable-tej)"); err != nil {
			logging.Warn("apigateway", "tej_inactive_record_failed", "err", err.Error())
		}
	}

	// --- Geopolitical (RSS + GDELT) ---
	geoAdapter := NewGeopoliticalChannelAdapter(workDir)
	g.registry.Register("geopolitical", geoAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "geopolitical")

	// --- JPY (Frankfurter API for USD/JPY rate) ---
	// The channel was historically named "jpy_yahoo" but has always used the
	// Frankfurter FX API (api.frankfurter.app). Renamed to frankfurter_fx to
	// reflect the actual data source. us_yahoo no longer fetches JPY=X to avoid overlap.
	jpyProvider := marketdata.NewFrankfurterFXProvider()
	jpyAdapter := NewFrankfurterFXChannelAdapter(jpyProvider)
	g.registry.Register("frankfurter_fx", jpyAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "frankfurter_fx")

	// --- TSMC Revenue (FinMind, requires API key) ---
	tsmcProvider := marketdata.NewTSMCRevenueProviderWithStorage(cfg.FinMindAPIKey, filepath.Join(workDir, "data/state/tsmc_revenue"))
	tsmcProvider.OnDegraded = func(channelID, reason string) {
		_ = g.Health().Record(channelID, "degraded", reason)
	}
	tsmcAdapter := NewTSMCRevenueChannelAdapter(tsmcProvider)
	g.registry.Register("tsmc_revenue", tsmcAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "tsmc_revenue")

	// --- Taiwan Geopolitical (CNA / 自由時報 / TVBS RSS) ---
	taiwanGeoAdapter := NewTaiwanGeopoliticalChannelAdapter(workDir)
	g.registry.Register("geopolitical_taiwan", taiwanGeoAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "geopolitical_taiwan")

	// --- Exchange Rate (Frankfurter API) ---
	exchangeProvider := marketdata.NewExchangeRateProvider()
	exchangeAdapter := NewExchangeRateChannelAdapter(exchangeProvider)
	g.registry.Register("exchange_rate", exchangeAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "exchange_rate")

	// --- SOX Index (Philadelphia Semiconductor Index) ---
	soxProvider := marketdata.NewSOXIndexProvider()
	soxAdapter := NewSOXIndexChannelAdapter(soxProvider)
	g.registry.Register("sox_index", soxAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "sox_index")

	// --- TAIEX (Taiwan Stock Exchange Capitalization Weighted Stock Index) ---
	if cfg.YahooEnabled {
		taiexProvider := marketdata.NewTAIEXIndexProvider()
		taiexAdapter := NewTAIEXIndexChannelAdapter(taiexProvider)
		g.registry.Register("taiex_index", taiexAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "taiex_index")

		// --- TAIEX 20-day annualized volatility (^TWII) ---
		// Provides MacroDataSnapshot.HistoricalVolatility consumed by
		// internal/strategy_techniques/evaluator.go (resolveField "HistoricalVolatility").
		// See docs/data-sources.md §tw_vol for the ChangePct semantic caveat.
		twVolProvider := marketdata.NewTaiwanVolatilityProviderWithStore(filepath.Join(workDir, "data/state", "taiwan_index_history.json"))
		twVolAdapter := NewTaiwanVolatilityChannelAdapter(twVolProvider)
		g.registry.Register("tw_vol", twVolAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "tw_vol")
	}

	// --- US Indexes (S&P 500, Nasdaq Composite, Dow Jones) ---
	if cfg.YahooEnabled {
		spxProvider := marketdata.NewSPXIndexProvider()
		g.registry.Register("us_spx", NewUSSPXIndexChannelAdapter(spxProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "us_spx")

		ndxProvider := marketdata.NewNDXIndexProvider()
		g.registry.Register("us_ndx", NewUSNDXIndexChannelAdapter(ndxProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "us_ndx")

		djiProvider := marketdata.NewDJIIndexProvider()
		g.registry.Register("us_dji", NewUSDJIIndexChannelAdapter(djiProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "us_dji")
	}

	// --- US Tech Stocks (NVDA, AAPL, MSFT) ---
	if cfg.YahooEnabled {
		nvdaProvider := marketdata.NewNVDAProvider()
		g.registry.Register("us_nvda", NewUSNVDAChannelAdapter(nvdaProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "us_nvda")

		aaplProvider := marketdata.NewAAPLProvider()
		g.registry.Register("us_aapl", NewUSAAPLChannelAdapter(aaplProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "us_aapl")

		msftProvider := marketdata.NewMSFTProvider()
		g.registry.Register("us_msft", NewUSMSFTChannelAdapter(msftProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "us_msft")
	}

	// --- TSM ADR (TSMC NYSE-listed American Depositary Receipt) ---
	if cfg.YahooEnabled {
		tsmProvider := marketdata.NewTSMADRProvider()
		g.registry.Register("tsm_adr", NewTSMADRChannelAdapter(tsmProvider))
		logging.Info("apigateway", "adapter_registered", "channel", "tsm_adr")
	}

	// --- DRAM Spot Price (Micron MU stock proxy) ---
	dramProvider := marketdata.NewDRAMSpotPriceProvider()
	dramAdapter := NewDRAMSpotPriceChannelAdapter(dramProvider)
	g.registry.Register("dram_spot_price", dramAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "dram_spot_price")

	// --- TWSE Sector Index (Taiwan Semiconductor Index, TAISEMI proxy) ---
	twseSectorProvider := marketdata.NewTWSESectorIndexProvider(filepath.Join(workDir, "data/state/sector_index"))
	twseSectorAdapter := NewTWSESectorIndexChannelAdapter(twseSectorProvider)
	g.registry.Register("twse_sector_index", twseSectorAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_sector_index")

	// --- BDI (Baltic Dry Index from CNBC) ---
	bdiProvider := marketdata.NewBDIProvider()
	bdiAdapter := NewBDIChannelAdapter(bdiProvider)
	g.registry.Register("bdi", bdiAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "bdi")

	// --- Sector Data (TWSE sector classification) ---
	sectorProvider := marketdata.NewSectorDataProvider(filepath.Join(workDir, "data/state/sector_data"))
	sectorAdapter := NewSectorDataChannelAdapter(sectorProvider)
	g.registry.Register("sector_data", sectorAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "sector_data")

	// --- TWSE Day Trading (no API key required) ---
	dayTradingAdapter := NewDayTradingChannelAdapter()
	g.registry.Register("day_trading", dayTradingAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "day_trading")

	// --- TWSE Market Volume (集中市場成交金額, no API key required) ---
	marketVolumeAdapter := NewMarketVolumeChannelAdapter()
	g.registry.Register("market_volume", marketVolumeAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "market_volume")

	// --- TAIFEX Daily (PCR, retail futures OI — no API key required) ---
	taifexAdapter := NewTaifexChannelAdapter()
	g.registry.Register("taifex_daily", taifexAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "taifex_daily")

	// --- TAIFEX Institutional (三大法人 期貨 OI — no API key required) ---
	taifexInstAdapter := NewTaifexInstitutionalAdapter()
	g.registry.Register("taifex_institutional", taifexInstAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "taifex_institutional")

	// --- Government (官股行庫 readings — operator-imported state files; #E04) ---
	govProvider := marketdata.NewGovernmentFlowProvider(filepath.Join(workDir, "data/state/government_flow"))
	govAdapter := NewGovernmentFlowAdapter(govProvider)
	g.registry.Register("government_flow", govAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "government_flow")

	// --- Government Broker Aggregate (TWSE broker-level fetcher; C06) ---
	brokerProvider := marketdata.NewGovernmentBrokerAggregator(filepath.Join(workDir, "data/state/government_flow"))
	brokerAdapter := NewGovernmentBrokerChannelAdapter(brokerProvider)
	g.registry.Register("government_broker", brokerAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "government_broker")

	// --- TWSE Odd-Lot Trading (no API key required) ---
	oddlotAdapter := NewTWSEOddLotChannelAdapter()
	g.registry.Register("twse_oddlot", oddlotAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_oddlot")

	// --- TWSE ETF Net Subscription (no API key required) ---
	etfAdapter := NewTWSEETFChannelAdapter()
	g.registry.Register("twse_etf", etfAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_etf")

	// --- TWSE SBL (Securities Borrowing & Lending) — G02 ---
	// STUB: provider returns "endpoint not yet confirmed" (see
	// internal/marketdata/twse_sbl_provider.go). The channel is
	// registered so the dashboard can show "not yet live" via
	// Metadata().Stub, but HealthCheck returns "inactive" so the
	// alerting path in monitoring/service/data_channels.go skips it.
	sblAdapter := NewTWSESBLChannelAdapter()
	g.registry.Register("twse_sbl", sblAdapter)
	logging.Warn("apigateway", "stub_adapter_registered", "channel", "twse_sbl", "gate", "G02")

	// --- TDCC Equity Dispersion (集保股權分散) — G01 ---
	// STUB: provider returns "API access not yet configured" (see
	// internal/marketdata/tdcc_provider.go). Auto-fetch is not
	// scheduled; the channel is registered so the dashboard shows
	// it as "not yet live" via Metadata().Stub. HealthCheck returns
	// "inactive" so the alerting path skips it.
	tdccAdapter := NewTDCClientChannelAdapter()
	g.registry.Register("tdcc_equity_dispersion", tdccAdapter)
	logging.Warn("apigateway", "stub_adapter_registered", "channel", "tdcc_equity_dispersion", "gate", "G01")

	// TwseInsider — TWSE OpenAPI 內部人持股轉讓 (t187ap12_L).
	insiderAdapter := NewTWSEInsiderChannelAdapter(filepath.Join(workDir, "data/state/insider_flow"))
	g.registry.Register("twse_insider", insiderAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_insider")

	// --- JANUS Regime (internal computed engine, optional) ---
	if janusEngine != nil {
		janusAdapter := NewJANUSRegimeChannelAdapter(janusEngine)
		g.registry.Register("janus_regime", janusAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "janus_regime")
	}

	return nil
}

func saveSnapshot(channelID string, data []byte) {
	dir := filepath.Join("data", "state", channelID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "latest.json"), data, 0o644)
}
