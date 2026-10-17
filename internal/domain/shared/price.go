package shared

import "math"

// IsUsablePrice reports whether p can be consumed as a price in a return,
// volatility, drawdown or label computation: strictly positive and finite.
//
// Rationale (FU-20260930-03 / FU-20260930-04): stored quote data can carry
// close<=0 rows (Close=0 is written for days with no closing price) and 10x
// bad values. Such a value is a MISSING observation, not a price. Feeding it
// into p_t/p_{t-1}-1 silently manufactures a -100% return (or a +10x jump),
// which then propagates into momentum, volatility, drawdown, ML labels and
// win-rate aggregates. Consumers must therefore test their price inputs with
// this predicate and skip the observation instead of computing from it.
//
// The predicate deliberately covers negative and non-finite values too: the
// stored bad values are documented as close<=0, and NaN/Inf would poison any
// downstream mean or standard deviation even though they are not representable
// as a plausible price.
func IsUsablePrice(p float64) bool {
	return p > 0 && !math.IsNaN(p) && !math.IsInf(p, 0)
}
