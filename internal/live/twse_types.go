package live

// TWSEOrderRequest represents a TWSE order submission payload.
// All fields use snake_case JSON tags for API compatibility.
type TWSEOrderRequest struct {
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`          // "B" for Buy, "S" for Sell
	Quantity    int     `json:"quantity"`      // shares in board lots (1 lot = 1000 shares)
	Price       float64 `json:"price"`         // limit price, 0 for market order
	OrderType   string  `json:"order_type"`    // "L" for Limit, "M" for Market
	TimeInForce string  `json:"time_in_force"` // "ROD", "IOC", "FOK"
	AccountID   string  `json:"account_id"`    // trading account identifier
}

// TWSEOrderResponse represents a TWSE order submission response.
type TWSEOrderResponse struct {
	OrderID      string  `json:"order_id"`
	Status       string  `json:"status"`
	FillPrice    float64 `json:"fill_price"`
	FilledQty    int     `json:"filled_qty"`
	RejectReason string  `json:"reject_reason,omitempty"`
	Message      string  `json:"message,omitempty"`
	Timestamp    string  `json:"timestamp"`
}

// TWSEOrderStatus tracks the current state of an order.
type TWSEOrderStatus string

const (
	TWSEStatusPending   TWSEOrderStatus = "pending"
	TWSEStatusSubmitted TWSEOrderStatus = "submitted"
	TWSEStatusPartial   TWSEOrderStatus = "partial"
	TWSEStatusFilled    TWSEOrderStatus = "filled"
	TWSEStatusCancelled TWSEOrderStatus = "cancelled"
	TWSEStatusRejected  TWSEOrderStatus = "rejected"
	TWSEStatusError     TWSEOrderStatus = "error"
)

// TWSETradeResult represents the result of a TWSE order execution.
type TWSETradeResult struct {
	OrderID    string          `json:"order_id"`
	Status     TWSEOrderStatus `json:"status"`
	FillPrice  float64         `json:"fill_price"`
	FilledQty  int             `json:"filled_qty"`
	RemainQty  int             `json:"remain_qty"`
	Symbol     string          `json:"symbol"`
	Side       string          `json:"side"`
	Message    string          `json:"message,omitempty"`
	LastUpdate string          `json:"last_update"`
}
