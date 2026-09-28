package domain

import "time"

// 期貨（futures）領域型別。
//
// 設計約束（見 docs/specs/futures-bars-firstparty-spec.md §4）：
//   - 期貨型別與既有股票型別**完全獨立**：不改動 DailyBar，也不在共用型別上新增
//     AssetClass 欄位。期貨 bar 有自己的契約（contract）與到期月（contract month）鍵，
//     語意與股票 symbol 不同，混用會污染兩邊的查詢與聚合。
//   - 可空欄位一律用指標：上游以 "-"/"NULL"/空字串表示缺值，映射時一律轉 nil。
//     **禁止**以 0 代表缺值（2026-09-27 #2107 同型事故：預設值把「沒資料」變成「有資料」）。
//     0 是合法值（例：成交量 0 口），必須與 nil（無此欄位值）可區分。

// FuturesSession 是期貨交易時段。
type FuturesSession string

const (
	// SessionRegular 是一般交易時段（08:45–13:45；最後交易日 08:45–13:30）。
	SessionRegular FuturesSession = "regular"
	// SessionAfterHours 是盤後交易時段（15:00–次日 05:00）。
	SessionAfterHours FuturesSession = "after_hours"
	// SessionUnknown 是無法辨識的時段。不得靜默歸類為一般。
	SessionUnknown FuturesSession = "unknown"
)

// FuturesBar 是單一契約、單一到期月、單一交易日、單一時段的一根日 bar。
//
// TradeDate 一律為 Asia/Taipei 日曆日（時間部分歸零）。
type FuturesBar struct {
	Contract        string         `json:"contract"`
	ContractMonth   string         `json:"contract_month"`
	TradeDate       time.Time      `json:"trade_date"`
	Session         FuturesSession `json:"session"`
	Open            *float64       `json:"open,omitempty"`
	High            *float64       `json:"high,omitempty"`
	Low             *float64       `json:"low,omitempty"`
	Close           *float64       `json:"close,omitempty"`
	Volume          *int64         `json:"volume,omitempty"`
	SettlementPrice *float64       `json:"settlement_price,omitempty"`
	OpenInterest    *int64         `json:"open_interest,omitempty"`
	Source          string         `json:"source"`
}

// Traded 回報此 bar 是否代表實際成交（有收盤價且成交量 > 0）。
//
// 判別式刻意不使用 `Close == 0`：0 是合法指數值，而缺值是 nil。
func (b FuturesBar) Traded() bool {
	return b.Close != nil && b.Volume != nil && *b.Volume > 0
}

// IsMonthlyContractMonth 回報 contract month 是否為標準月契約（YYYYMM，6 位數字）。
//
// 週契約（例：202609W5）與價差組合（例：202706/202709）皆回 false。
func IsMonthlyContractMonth(cm string) bool {
	if len(cm) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if cm[i] < '0' || cm[i] > '9' {
			return false
		}
	}
	return true
}

// FuturesContractSpec 是契約參考表的一列（見規格 §3，來源為期交所官網契約規格頁）。
type FuturesContractSpec struct {
	Code        string  // "TX" / "MTX"
	Name        string  // 臺股期貨 / 小型臺指期貨
	Multiplier  float64 // 每點價值（新臺幣元）：TX 200、MTX 50
	TickSize    float64 // 最小升降單位（指數點）：1
	HasWeeklies bool    // 是否加掛週契約
}

// futuresContractSpecs 是已知契約的參考表。
//
// 值全部來自期交所官網契約規格頁原文（2026-09-28 抓取）：
//   - TX  https://www.taifex.com.tw/cht/2/tX  「臺股期貨指數乘上新臺幣200元」
//   - MTX https://www.taifex.com.tw/cht/2/mTX 「小型臺指期貨指數乘上新臺幣50元」
var futuresContractSpecs = map[string]FuturesContractSpec{
	"TX":  {Code: "TX", Name: "臺股期貨", Multiplier: 200, TickSize: 1, HasWeeklies: false},
	"MTX": {Code: "MTX", Name: "小型臺指期貨", Multiplier: 50, TickSize: 1, HasWeeklies: true},
}

// FuturesContractSpecFor 回傳契約參考表的一列。
//
// 未知契約回 (zero, false)。**不得**回傳預設乘數 —— 用錯乘數會把點數錯算成金額。
func FuturesContractSpecFor(code string) (FuturesContractSpec, bool) {
	spec, ok := futuresContractSpecs[code]
	return spec, ok
}

// FuturesContractCodes 回傳已知契約代碼（排序穩定，供 CLI 預設值使用）。
func FuturesContractCodes() []string {
	return []string{"TX", "MTX"}
}

// FuturesLastTradingDay 回傳某年某月契約的表定最後交易日＝該月**第三個星期三**
// （期交所契約規格原文：TX「各契約的最後交易日為各該契約交割月份第三個星期三」；
// MTX 同）。
//
// 注意：這是**表定**日期。若該日為假日，期交所順延至最近之次一營業日；實務上必須以
// 實際資料驗證（見規格 §6.2），本函式只提供日曆候選值。
func FuturesLastTradingDay(year int, month time.Month) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	// Weekday(): Sunday=0 … Wednesday=3
	offset := (int(time.Wednesday) - int(first.Weekday()) + 7) % 7
	third := first.AddDate(0, 0, offset+14)
	return third
}
