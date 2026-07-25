package apigateway

// USMarketChannels returns the 8 US market channel IDs that hit Yahoo Finance
// v8 chart API. These channels are refreshed individually by BackgroundTaskManager
// tasks in cmd/atlas/data_sync_health_tasks.go so each channel has its own
// ChannelID, failure isolation, and BTM telemetry. The shared Yahoo limiters
// (yahooIndexLimiter / yahooTechLimiter / ExportStatisticsRate) inside
// Gateway.Fetch serialize requests to the same endpoint group.
func USMarketChannels() []string {
	return []string{
		"us_spx",
		"us_ndx",
		"us_dji",
		"sox_index",
		"us_nvda",
		"us_aapl",
		"us_msft",
		"tsm_adr",
	}
}
