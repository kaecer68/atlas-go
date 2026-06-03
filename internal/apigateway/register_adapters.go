package apigateway

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/config"
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
		fugleClient := marketdata.NewFugleClient(cfg.FugleAPIKey)
		fugleAdapter := NewFugleChannelAdapter(fugleClient)
		g.registry.Register("fugle", fugleAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "fugle")
	}

	// --- Fubon ---
	fubonKey := cfg.FubonAPIKey
	if fubonKey == "" {
		fubonKey = config.GetSecret("ATLAS_FUBON_API_KEY")
	}
	if fubonKey != "" {
		fubonClient := marketdata.NewFubonClient(fubonKey)
		fubonAdapter := NewFubonChannelAdapter(fubonClient)
		g.registry.Register("fubon", fubonAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "fubon")
	}

	// --- FinMind ---
	if cfg.FinMindAPIKey != "" {
		finmindClient := marketdata.GetSharedFinMindClient(cfg.FinMindAPIKey)
		finmindAdapter := NewFinMindChannelAdapter(finmindClient)
		g.registry.Register("finmind", finmindAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "finmind")
	}

	// --- TWSE (no API key required) ---
	twseClient := marketdata.NewTWSEClient()
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
	capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow"))
	capFlowAdapter := NewTWSECapitalFlowChannelAdapter(capFlowProvider)
	g.registry.Register("twse_capital_flow", capFlowAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_capital_flow")

	// --- TWSE Margin Balance (no API key required) ---
	marginProvider := marketdata.NewTWSEMarginBalanceProvider(filepath.Join(workDir, "data/state/margin"))
	marginAdapter := NewTWSEMarginChannelAdapter(marginProvider)
	g.registry.Register("twse_margin", marginAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_margin")

	// --- Export Statistics (no API key required) ---
	exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export"))
	exportAdapter := NewExportStatisticsChannelAdapter(exportProvider)
	g.registry.Register("export_statistics", exportAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "export_statistics")

	// --- TEJ ---
	if tejKey := config.GetSecret("TEJ_API_KEY"); tejKey != "" {
		tejClient := marketdata.NewTEJClient(tejKey)
		tejAdapter := NewTEJChannelAdapter(tejClient)
		g.registry.Register("tej", tejAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "tej")
	}

	// --- Geopolitical (RSS + GDELT) ---
	geoAdapter := NewGeopoliticalChannelAdapter(workDir)
	g.registry.Register("geopolitical", geoAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "geopolitical")

	// --- JPY (Frankfurter API for USD/JPY rate) ---
	// Despite the "jpy_yahoo" channel ID, this uses Frankfurter FX API (api.frankfurter.app),
	// not Yahoo Finance. The channel name is historical.
	jpyProvider := marketdata.NewFrankfurterFXProvider()
	jpyAdapter := NewJPYYahooChannelAdapter(jpyProvider)
	g.registry.Register("jpy_yahoo", jpyAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "jpy_yahoo")

	// --- TSMC Revenue (FinMind, requires API key) ---
	tsmcProvider := marketdata.NewTSMCRevenueProvider(cfg.FinMindAPIKey)
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

	// --- TAIFEX Daily (PCR, retail futures OI — no API key required) ---
	taifexAdapter := NewTaifexChannelAdapter()
	g.registry.Register("taifex-daily", taifexAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "taifex-daily")

	// --- TWSE Odd-Lot Trading (no API key required) ---
	oddlotAdapter := NewTWSEOddLotChannelAdapter()
	g.registry.Register("twse-oddlot", oddlotAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse-oddlot")

	// --- TWSE ETF Net Subscription (no API key required) ---
	etfAdapter := NewTWSEETFChannelAdapter()
	g.registry.Register("twse-etf", etfAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse-etf")

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
