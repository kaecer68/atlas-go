package pricegate

import (
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// warnCapPerComponent bounds the number of rejected-observation WARN lines a
// single consumer may emit. Rejections come from corrupted stored data, and a
// corrupted series can span thousands of symbols and hundreds of bars; without
// a cap the gate itself would become a log flood. One extra line announces the
// suppression, so the audit trail never claims there were no further rejects.
const warnCapPerComponent = 100

var (
	warnMu   sync.Mutex
	warnSeen = make(map[string]int)
)

// Check reports whether the consumer may use the return between prev and curr.
//
// false means "treat the observation as missing", never "the return is 0":
//   - either close is not a usable price (see shared.IsUsablePrice) — the
//     FU-20260930-04 case, skipped silently because it is already expected and
//     already documented;
//   - the return exceeds shared.MaxPlausibleReturn(horizonDays) outside the IPO
//     window — the corrupted-multiple case, logged as a WARN so the rejected
//     row is auditable.
//
// pos is the 0-based position of curr in the symbol's stored series and drives
// the IPO exemption; horizonDays is the number of trading days the return spans
// (1 for a daily return). component names the consumer in the audit log; symbol
// may be empty when the caller does not know it.
func Check(component, symbol string, prev, curr float64, pos, horizonDays int) bool {
	if !shared.IsUsablePrice(prev) || !shared.IsUsablePrice(curr) {
		return false
	}
	r := curr/prev - 1
	if shared.PlausibleReturn(r, horizonDays) {
		return true
	}
	if math.IsNaN(r) || math.IsInf(r, 0) {
		// Unreachable for two finite positives; kept so the contract is total
		// and a future non-finite bypass cannot leak through.
		return false
	}
	if shared.InIPOExemptWindow(pos, horizonDays) {
		logging.Info(component, "gap_multiple_ipo_exempt",
			"symbol", symbol, "pos", pos, "horizon_days", horizonDays, "return", r)
		return true
	}
	warnRejected(component, symbol, prev, curr, pos, horizonDays, r)
	return false
}

func warnRejected(component, symbol string, prev, curr float64, pos, horizonDays int, r float64) {
	warnMu.Lock()
	warnSeen[component]++
	n := warnSeen[component]
	warnMu.Unlock()

	if n > warnCapPerComponent {
		return
	}
	if n == warnCapPerComponent {
		logging.Warn(component, "gap_multiple_rejected_suppressed",
			"reason", "warn_cap_reached", "cap", warnCapPerComponent,
			"hint", "further gap-multiple rejections for this component are not logged individually")
		return
	}
	logging.Warn(component, "gap_multiple_rejected",
		"reason", "implausible_return",
		"symbol", symbol,
		"pos", pos,
		"horizon_days", horizonDays,
		"prev", prev,
		"curr", curr,
		"return", r,
		"bound", shared.MaxPlausibleReturn(horizonDays),
	)
}
