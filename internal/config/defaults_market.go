package config

func defaultForwardReturnParameters() ForwardReturnParameters {
	return ForwardReturnParameters{
		RiskOnMean: ParameterMetadata[float64]{
			Value:     0.0008,
			Rationale: "Risk-on regime expected positive daily drift on TWSE; long-term equity premium",
			Source:    SourceHeuristic,
		},
		RiskOffMean: ParameterMetadata[float64]{
			Value:     0.0001,
			Rationale: "Risk-off regime near-zero drift reflects defensive positioning",
			Source:    SourceHeuristic,
		},
		RiskOnStdDev: ParameterMetadata[float64]{
			Value:     0.015,
			Rationale: "Risk-on regime 1.5% daily vol matches typical TWSE large-cap",
			Source:    SourceHeuristic,
		},
		RiskOffStdDev: ParameterMetadata[float64]{
			Value:     0.008,
			Rationale: "Risk-off regime 0.8% daily vol reflects compressed trading ranges",
			Source:    SourceHeuristic,
		},
	}
}

func defaultTaxParameters() TaxParameters {
	return TaxParameters{
		DividendTaxRate: ParameterMetadata[float64]{
			Value:     0.28,
			Rationale: "Taiwan individual income tax rate on dividend income (28% bracket)",
			Source:    SourceLiterature,
		},
		TransactionTaxRate: ParameterMetadata[float64]{
			Value:     0.003,
			Rationale: "Taiwan securities transaction tax (0.3%, sell-side only)",
			Source:    SourceLiterature,
		},
		NHISurchargeRate: ParameterMetadata[float64]{
			Value:     0.0211,
			Rationale: "Taiwan NHI supplementary premium (二代健保補充保費) rate 2.11% effective 2021",
			Source:    SourceLiterature,
		},
	}
}

func defaultRealtimeParameters() RealtimeParameters {
	return RealtimeParameters{
		VolatilityThreshold: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "2% daily volatility threshold for regime detection",
			Source:    SourceHeuristic,
		},
		VolumeSpikeThreshold: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "2x average volume indicates unusual activity",
			Source:    SourceHeuristic,
		},
		PriceChangeThreshold: ParameterMetadata[float64]{
			Value:     0.01,
			Rationale: "1% price move threshold for signal detection",
			Source:    SourceHeuristic,
		},
		MinConfidence: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "70% minimum confidence for real-time signals",
			Source:    SourceHeuristic,
		},
		WeightAdjustmentRate: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "10% weight adjustment per signal for stability",
			Source:    SourceHeuristic,
		},
		MaxWeightChange: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Maximum 50% weight change per adjustment",
			Source:    SourceHeuristic,
		},
		MinWeight: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "Minimum 10% weight to maintain position",
			Source:    SourceHeuristic,
		},
		UpdateIntervalMs: ParameterMetadata[int]{
			Value:     100,
			Rationale: "100ms update interval for real-time processing",
			Source:    SourceHeuristic,
		},
	}
}

func defaultMarketdataParameters() MarketdataParameters {
	return MarketdataParameters{
		TWSEAPIRateLimit: ParameterMetadata[float64]{
			Value:     0.6,
			Rationale: "TWSE OpenAPI rate limit: 3 requests per 5 seconds = 0.6 req/s",
			Source:    SourceHeuristic,
			Todo:      "Verify: check TWSE documentation for current limits",
		},
		TWSEAPIRateBurst: ParameterMetadata[int]{
			Value:     3,
			Rationale: "TWSE OpenAPI burst limit: 3 requests per 5-second window",
			Source:    SourceHeuristic,
			Todo:      "Verify: check TWSE documentation for current burst limits",
		},
		TWSEAPITimeoutSec: ParameterMetadata[int]{
			Value:     20,
			Rationale: "HTTP timeout for TWSE API calls; balances responsiveness vs slow responses. N1 S4 (2026-08-18): 15s 在 TWSE 官方慢時段 (07:17–07:58 台北) 實測被 STOCK_DAY_ALL >15s 拖垮，校準至 20s (見 investigation-twse-timeout-2026-08-18.md §3.2)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [10, 30] range based on observed latency distribution",
		},
		FubonIntradayLimit: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Fubon intraday API burst limit; prevents overwhelming the proxy",
			Source:    SourceHeuristic,
			Todo:      "Verify: check Fubon DMA documentation for actual limits",
		},
		FubonHistoricalLimit: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Fubon historical data API rate limit; conservative to avoid throttling",
			Source:    SourceHeuristic,
			Todo:      "Verify: check Fubon DMA documentation for actual limits",
		},
		FubonAPITimeoutSec: ParameterMetadata[int]{
			Value:     10,
			Rationale: "HTTP timeout for Fubon API calls; proxy adds latency, keep short",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [5, 15] range based on proxy latency",
		},
		TEJCallsPerSecond: ParameterMetadata[int]{
			Value:     5,
			Rationale: "TEJ free tier rate limit: 500 calls/day, burst at 5 calls/second",
			Source:    SourceHeuristic,
			Todo:      "Verify: check TEJ subscription tier for actual limits",
		},
		TEJAPITimeoutSec: ParameterMetadata[int]{
			Value:     30,
			Rationale: "HTTP timeout for TEJ API; historical queries can be slow",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [20, 45] range",
		},
		FugleRateLimit: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Fugle API free tier: 30 req/min (conservative, below measured ~39/min 429 point; manifest fugle-unified-access F2/A2). Developer: 600/min. Advanced: 2000/min",
			Source:    SourceEmpirical,
			Todo:      "Set FUGLE_TIER env var if using paid plan",
		},
		FugleAPITimeoutSec: ParameterMetadata[int]{
			Value:     10,
			Rationale: "HTTP timeout for Fugle API; premium service, expect fast responses",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [5, 15] range",
		},
		BDIAPITimeoutSec: ParameterMetadata[int]{
			Value:     10,
			Rationale: "HTTP timeout for CNBC BDI API; public free endpoint, accept slower response",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [5, 15] range based on observed CNBC latency",
		},
		BDIEndpoint: ParameterMetadata[string]{
			Value:     "https://quote.cnbc.com/quote-html-webservice/quote.htm?symbols=.BADI&output=json",
			Rationale: "CNBC free REST JSON API for Baltic Dry Index; no API key required, /BADI symbol includes change_pct and last_time_msec",
			Source:    SourceEmpirical,
			Todo:      "",
		},
		MaxRetryAttempts: ParameterMetadata[int]{
			Value:     3,
			Rationale: "Maximum retry attempts for transient failures; exponential backoff",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [2, 5] range based on failure rate analysis",
		},
		RetryBackoffMs: ParameterMetadata[int]{
			Value:     1000,
			Rationale: "Base backoff between retries in milliseconds; doubles each attempt",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [500, 2000] range",
		},
	}
}

func defaultPreciousMetalsParameters() PreciousMetalsParameters {
	return PreciousMetalsParameters{
		CentralBankBuyingTrend: ParameterMetadata[string]{
			Value:     "stable",
			Rationale: "WGC quarterly CB gold buying trend: accelerating, stable, or decelerating",
			Source:    SourceLiterature,
			Todo:      "Update quarterly from WGC Gold Demand Trends report",
			Citation: &ParameterCitation{
				SourceType:      "report",
				SourceReference: "World Gold Council, Gold Demand Trends Q1 2026",
				EvidenceQuality: "medium",
				UpdatePolicy:    "quarterly",
				LastValidated:   "2026-05-01",
			},
		},
		CentralBankNetBuy: ParameterMetadata[float64]{
			Value:     800.0,
			Rationale: "Annualized central bank net gold purchases in tonnes (~800t in 2025)",
			Source:    SourceLiterature,
			Todo:      "Update from WGC quarterly report",
		},
		IndiaGoldImportsYoY: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "India gold imports YoY % change; 0 means no change from prior year",
			Source:    SourceLiterature,
			Todo:      "Update monthly from India Ministry of Commerce data",
		},
		ChinaGoldImportsYoY: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "China SGE withdrawal YoY % change; 0 means no change from prior year",
			Source:    SourceLiterature,
			Todo:      "Update monthly from Shanghai Gold Exchange data",
		},
		COMEXDefaultNetLong: ParameterMetadata[float64]{
			Value:     150000,
			Rationale: "CFTC COT managed money net long default (typical mid-cycle level)",
			Source:    SourceEmpirical,
			Todo:      "Update weekly from CFTC Commitment of Traders report",
		},
	}
}
