package domain

import "time"

// TradeRecord is the persisted audit trail entry for an executed simulation trade.
type TradeRecord struct {
	TradeID   string    `json:"trade_id"`
	SessionID string    `json:"session_id"`
	Symbol    string    `json:"symbol"`
	Side      Side      `json:"side"`
	Quantity  int       `json:"quantity"`
	Price     float64   `json:"price"`
	Amount    float64   `json:"amount"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
