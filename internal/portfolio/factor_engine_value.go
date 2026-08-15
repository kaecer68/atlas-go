package portfolio

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CalculateValueScore computes value based on P/E and P/B from fundamentals.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateValueScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateValueDetail(symbol, quotes).Score
}

// calculateValueDetail returns the full breakdown for value calculation.
// Implements SCOR-02 (industry-relative P/E) and SCOR-03 (negative/undefined P/E handling).
// Precious metals (gold, silver) have no P/E or P/B — returns 0.
//
// Scoring priority:
//  1. If PE is valid (>0, finite): use sector-relative PE (PE/SectorMedianPE)
//     if SectorMedianPE > 0, else fall back to absolute PE range. Always
//     include PB as secondary metric when valid.
//  2. If PE invalid, try PB (PBRangeCenter/Width), then PS (PSRangeCenter/Width).
//  3. All metrics invalid → IsFallback=true, formula="fallback: no valid value metrics".
//  4. No fundamentals provider → ValueFallbackScore, IsFallback=true.
func (fe *FactorEngine) calculateValueDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	_ = quotes

	if isPM, _ := isPreciousMetal(symbol); isPM {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "precious_metal: P/E not applicable",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	fe.mu.RLock()
	fp := fe.fundamentals
	fe.mu.RUnlock()

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		score := 0.0
		count := 0
		raw := map[string]float64{}
		var formula string
		var isFallback bool

		// SCOR-03: Handle negative/undefined P/E
		// Check if P/E is valid (positive and not NaN/Inf)
		peValid := data.PE > 0 && isFinite(data.PE)

		if peValid {
			// SCOR-02: Industry-relative P/E comparison
			sector := data.Sector
			if sector == "" {
				sector = "other" // Default to "other" if no sector specified
			}
			sectorMedianPE := fp.SectorMedianPE(sector)

			var peScore float64
			if sectorMedianPE > 0 {
				// Relative P/E: PE / SectorMedianPE
				// PE = sector median → score 1.0
				// PE = 2x median → score 0.0
				// PE = 0.5x median → score 1.5 (capped)
				relativePE := data.PE / sectorMedianPE
				peScore = 1.0 - (relativePE - 1.0)
				raw["sector_median_pe"] = sectorMedianPE
				raw["relative_pe"] = relativePE
				formula = "clamp(1 - (PE/sectorMedianPE - 1), -1, 1)"
			} else {
				// Fallback to absolute P/E if no sector data available
				peScore = 1.0 - (data.PE-fe.params.Factor.ValuePERangeCenter)/fe.params.Factor.ValuePERangeWidth
				formula = fmt.Sprintf("clamp(1 - (PE-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePERangeCenter, fe.params.Factor.ValuePERangeWidth)
			}

			if peScore > 1.0 {
				peScore = 1.0
			}
			if peScore < -1.0 {
				peScore = -1.0
			}
			score += peScore
			count++
			raw["pe"] = data.PE
			raw["pe_score"] = peScore
		} else {
			// P/E is invalid (negative, zero, NaN, or Inf)
			// Try P/B first
			if data.PB > 0 && isFinite(data.PB) {
				pbScore := 1.0 - (data.PB-fe.params.Factor.ValuePBRangeCenter)/fe.params.Factor.ValuePBRangeWidth
				if pbScore > 1.0 {
					pbScore = 1.0
				}
				if pbScore < -1.0 {
					pbScore = -1.0
				}
				score += pbScore
				count++
				raw["pb"] = data.PB
				raw["pb_score"] = pbScore
				raw["pe_switched_to_pb"] = 1.0 // Mark that we switched from P/E to P/B
				formula = fmt.Sprintf("clamp(1 - (PB-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePBRangeCenter, fe.params.Factor.ValuePBRangeWidth)
			} else if data.PS != nil && *data.PS > 0 && isFinite(*data.PS) {
				// P/B also invalid, try P/S (FIX-7: nil PS = 缺資料, skip)
				psValue := *data.PS
				psScore := 1.0 - (psValue-fe.params.Factor.ValuePSRangeCenter)/fe.params.Factor.ValuePSRangeWidth
				if psScore > 1.0 {
					psScore = 1.0
				}
				if psScore < -1.0 {
					psScore = -1.0
				}
				score += psScore
				count++
				raw["ps"] = psValue
				raw["ps_score"] = psScore
				raw["pe_switched_to_ps"] = 1.0 // Mark that we switched from P/E to P/S
				formula = fmt.Sprintf("clamp(1 - (PS-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePSRangeCenter, fe.params.Factor.ValuePSRangeWidth)
			} else {
				// All value metrics invalid, use fallback
				isFallback = true
				formula = "fallback: no valid value metrics"
			}
		}

		// If P/E was valid, also include P/B as secondary metric
		if peValid && data.PB > 0 && isFinite(data.PB) {
			pbScore := 1.0 - (data.PB-fe.params.Factor.ValuePBRangeCenter)/fe.params.Factor.ValuePBRangeWidth
			if pbScore > 1.0 {
				pbScore = 1.0
			}
			if pbScore < -1.0 {
				pbScore = -1.0
			}
			score += pbScore
			count++
			raw["pb"] = data.PB
			raw["pb_score"] = pbScore
			formula = fmt.Sprintf("avg(clamp(1 - (PE/sectorMedianPE - 1), -1, 1), clamp(1 - (PB-%.2f)/%.2f, -1, 1))", fe.params.Factor.ValuePBRangeCenter, fe.params.Factor.ValuePBRangeWidth)
		}

		if count > 0 {
			return domain.FactorScoreItem{
				Score:      score / float64(count),
				Formula:    formula,
				RawInputs:  raw,
				IsFallback: isFallback,
			}
		}
	}
	return domain.FactorScoreItem{
		Score:      fe.params.Factor.ValueFallbackScore,
		Formula:    "fallback: no fundamentals available",
		RawInputs:  map[string]float64{},
		IsFallback: true,
	}
}
