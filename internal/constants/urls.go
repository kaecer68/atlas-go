package constants

// External API base URLs used across atlas-go binaries and internal modules.
// When a provider changes its endpoint, updating the constant here propagates
// the change to all consumers (backfill commands, marketdata clients, etc.).

const (
	// FinMindBaseURL is the root endpoint for the FinMind trade API (v4).
	// Used by backfill commands (financial statements, month revenue,
	// institutional investors, replay) and the marketdata FinMind client.
	// API docs: https://finmindtrade.com
	FinMindBaseURL = "https://api.finmindtrade.com/api/v4"

	// TWSEBaseURL is the root endpoint for the Taiwan Stock Exchange (TWSE)
	// web API. Used by marketdata providers (calendar, ETF, OpenAPI, capital
	// flow, odd lot, margin, day trading) and fetch commands.
	TWSEBaseURL = "https://www.twse.com.tw"
)
