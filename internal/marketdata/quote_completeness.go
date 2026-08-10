package marketdata

import "github.com/kaecer68/atlas-go/internal/domain"

// QuoteComplete 判定 domain.Quote 是否為「完整」行情（manifest Phase B1）。
// 統一完整性語義，取代各消費層各自內聯的殘缺判斷：
//
//   - Last == 0 → 無資料（incomplete）
//   - Last > 0 但 Open/High/Low 全 0 → closePrice-only 殘缺
//     （Fugle 非交易時段/破表鎖定前的典型回應模式）
//   - 其餘 → complete
//
// 消費層：HandleQuote（殘缺 → fallback/標記）、HybridProvider
// hasInvalidQuotes（原內聯判斷提升為此共用函數）。
func QuoteComplete(q domain.Quote) bool {
	if q.Last == 0 {
		return false
	}
	if q.Open == 0 && q.High == 0 && q.Low == 0 {
		return false
	}
	return true
}
