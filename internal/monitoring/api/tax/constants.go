package tax

// Data status values returned by HandleTaxSnapshot.
const (
	DataStatusOK               = "ok"
	DataStatusDegraded         = "degraded"
	DataStatusMissingPositions = "missing_positions"
)

// Failed-reason identifiers returned by HandleTaxSnapshot.
const (
	FailedReasonNoPositions                   = "no_positions_file_or_empty_portfolio"
	FailedReasonDividendProviderNotConfigured = "dividend_provider_not_configured"
	FailedReasonZeroQuantityOrPricePrefix     = "zero_quantity_or_price"
)
