package eventdriven

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// SectorPredictor generates per-sector capital-flow direction predictions
// using a rule-based model over existing data sources:
//   - overall FlowPrediction (baseline direction + confidence)
//   - event calendar affected_industries (event-driven adjustment)
//   - MacroDataSnapshot (macro driver exposure)
//   - CycleTracker (cycle position shift)
//
// All providers are optional; when nil the corresponding adjustment is skipped.
type SectorPredictor struct {
	macro *marketdata.MacroDataSnapshot
	cycle cycleScoreProvider
	prior *sectorallocation.StrategicSectorPrior
}

// cycleScoreProvider abstracts CycleTracker.GetContinuousPhaseScore.
type cycleScoreProvider interface {
	GetContinuousPhaseScore(industryID string) float64
}

// NewSectorPredictor creates a SectorPredictor. Both providers may be nil
// (the predictor degrades gracefully, skipping the corresponding adjustment).
func NewSectorPredictor(macro *marketdata.MacroDataSnapshot, cycle cycleScoreProvider) *SectorPredictor {
	return &SectorPredictor{macro: macro, cycle: cycle}
}

// SetMacroSnapshot updates the macro snapshot.
func (sp *SectorPredictor) SetMacroSnapshot(m *marketdata.MacroDataSnapshot) { sp.macro = m }

// SetCycleProvider updates the cycle provider.
func (sp *SectorPredictor) SetCycleProvider(c cycleScoreProvider) { sp.cycle = c }

// SetStrategicPrior injects the typed strategic prior (SA02 SA-INV-05).
// （已由 StrategicSectorPrior 取代；單一 source of truth——參見 SA02/SA04）
// nil prior 會讓 predictSector 對 prior baseline = 0（spec §4.1: nil prior 不得回 fallback）。
func (sp *SectorPredictor) SetStrategicPrior(p *sectorallocation.StrategicSectorPrior) { sp.prior = p }

// PriorWeight returns the strategic prior weight for sid; 0 if no prior set.
func (sp *SectorPredictor) PriorWeight(sid industry.SectorID) float64 {
	if sp.prior == nil {
		return 0
	}
	w, ok := sp.prior.Weights[sid]
	if !ok {
		return 0
	}
	return w
}

// Predict generates SectorDayPrediction for each forecast day.
// predictions must be exactly 5 days; activeEvents supplies event→sector mapping.
func (sp *SectorPredictor) Predict(predictions []FlowPrediction, activeEvents []EventCalendarItem) []SectorDayPrediction {
	l1s := industry.L1Sectors()
	out := make([]SectorDayPrediction, 0, len(predictions))
	for _, fp := range predictions {
		sdp := SectorDayPrediction{
			Date:    fp.Date.Format("2006-01-02"),
			Sectors: make([]SectorPrediction, 0, len(l1s)),
		}
		for _, sid := range l1s {
			spread := sp.predictSector(sid, fp, activeEvents)
			sdp.Sectors = append(sdp.Sectors, spread)
		}
		out = append(out, sdp)
	}
	return out
}

// predictSector computes the prediction for a single sector on a single day.
func (sp *SectorPredictor) predictSector(
	sid industry.SectorID,
	overall FlowPrediction,
	activeEvents []EventCalendarItem,
) SectorPrediction {
	var scoreIn, scoreOut, scoreNeu float64
	contrib := make(map[string]float64)

	// ── 1. Overall baseline ──────────────────────────────────────────
	sw := sp.PriorWeight(sid)
	switch overall.Direction {
	case "inflow":
		scoreIn += sw * overall.Confidence
	case "outflow":
		scoreOut += sw * overall.Confidence
	default:
		scoreNeu += sw * overall.Confidence
	}
	contrib["overall_baseline"] = sw * overall.Confidence

	// ── 2. Event-driven adjustment ───────────────────────────────────
	for _, e := range activeEvents {
		if !sectorInAffected(sid, e.AffectedIndustries) {
			continue
		}
		w := e.Confidence
		if e.Backfilled {
			w *= backfillDiscountFactor
		}
		switch e.Direction {
		case "bullish":
			scoreIn += w
			contrib[fmt.Sprintf("event:%s", e.Name)] = w
		case "bearish":
			scoreOut += w
			contrib[fmt.Sprintf("event:%s", e.Name)] = w
		case "mixed":
			scoreIn += w * 0.3
			scoreOut += w * 0.3
			contrib[fmt.Sprintf("event:%s", e.Name)] = w * 0.3
		}
	}

	// ── 3. Macro driver adjustment ───────────────────────────────────
	if sp.macro != nil {
		sp.applyMacroDrivers(sid, &scoreIn, &scoreOut, contrib)
	}

	// ── 4. Cycle position adjustment ────────────────────────────────
	if sp.cycle != nil {
		sp.applyCycleShift(sid, &scoreIn, &scoreOut, &scoreNeu, contrib)
	}

	// ── 5. Softmax → distribution ────────────────────────────────────
	dist := softmax3(scoreIn, scoreNeu, scoreOut)

	// ── 6. Direction + confidence ────────────────────────────────────
	dir, conf := directionConfidence(dist)

	// ── 7. JSD consistency vs overall ────────────────────────────────
	ovDist := overall.Distribution
	if jsd(dist, ovDist) > jsdThreshold {
		conf *= confidencePenaltyJSD
		contrib["consistency_warning"] = 0
	}

	// ── 8. Drivers (top 2, excluding consistency warning) ────────────
	drivers := topDrivers(contrib, 2)

	return SectorPrediction{
		SectorID:     string(sid),
		SectorName:   industry.DisplayZHTw[sid],
		Direction:    dir,
		Confidence:   math.Round(conf*100) / 100,
		Distribution: dist,
		Drivers:      drivers,
	}
}

// ── Helpers ────────────────────────────────────────────────────────────

func sectorInAffected(sid industry.SectorID, affected []string) bool {
	if len(affected) == 0 {
		return false
	}
	sidStr := string(sid)
	zhName := industry.DisplayZHTw[sid]
	for _, a := range affected {
		if strings.EqualFold(a, sidStr) {
			return true
		}
		if zhName != "" && strings.Contains(a, zhName) {
			return true
		}
	}
	return false
}

// sectorWeight 已由 PriorWeight 取代。
// 刪除前請確認所有參照已清理完畢。

// applyMacroDrivers adjusts scores based on macro driver change percentages.
func (sp *SectorPredictor) applyMacroDrivers(sid industry.SectorID, scoreIn, scoreOut *float64, contrib map[string]float64) {
	for _, md := range macroDriverDefs {
		if !sectorInSet(sid, md.sectors) {
			continue
		}
		chg := md.changePct(sp.macro)
		if chg == 0 {
			continue
		}
		magnitude := math.Min(math.Abs(chg)/md.typicalMove, 1.0)
		relevance := md.relevanceForSector(sid)
		impact := relevance * magnitude
		if impact == 0 {
			continue
		}
		sign := 1.0
		if (chg > 0 && !md.bullishOnUp) || (chg < 0 && md.bullishOnUp) {
			sign = -1.0
		}
		if sign > 0 {
			*scoreIn += impact
		} else {
			*scoreOut += impact
		}
		contrib[md.label] = impact
	}
}

func sectorInSet(sid industry.SectorID, set []industry.SectorID) bool {
	for _, s := range set {
		if s == sid {
			return true
		}
	}
	return false
}

type macroDriverDef struct {
	label           string
	changePct       func(*marketdata.MacroDataSnapshot) float64
	sectors         []industry.SectorID
	bullishOnUp     bool
	typicalMove     float64
	sectorRelevance map[industry.SectorID]float64
}

func (d *macroDriverDef) relevanceForSector(sid industry.SectorID) float64 {
	if v, ok := d.sectorRelevance[sid]; ok {
		return v
	}
	return 0.5
}

var macroDriverDefs = []macroDriverDef{
	{
		label:     "dxy",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.DXY.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorElectronics, industry.SectorSemiconductor,
			industry.SectorShipping, industry.SectorSteel,
		},
		bullishOnUp: false, // DXY up → export pressure → bearish
		typicalMove: 0.5,
	},
	{
		label:     "us10y",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.US10Y.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorFinancials, industry.SectorConstruction,
		},
		bullishOnUp: true, // rates up → financials bullish (wider NIM), construction bearish handled below
		typicalMove: 0.05,
		sectorRelevance: map[industry.SectorID]float64{
			industry.SectorFinancials:   0.8,  // bullish for financials
			industry.SectorConstruction: -0.4, // bearish for construction → invert
		},
	},
	{
		label:     "tsm_adr",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.TSMADR.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorSemiconductor,
		},
		bullishOnUp: true,
		typicalMove: 2.0,
	},
	{
		label:     "nvda",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.NVDA.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorSemiconductor,
		},
		bullishOnUp: true,
		typicalMove: 3.0,
	},
	{
		label:     "foreign_investor_net",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.ForeignInvestorNet.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorSemiconductor, industry.SectorFinancials,
		},
		bullishOnUp: true,
		typicalMove: 1.0,
	},
	{
		label:     "bdi",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.Bdi.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorShipping, industry.SectorSteel,
		},
		bullishOnUp: true,
		typicalMove: 2.0,
	},
	{
		label:     "sox_index",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.SOXIndex.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorSemiconductor,
		},
		bullishOnUp: true,
		typicalMove: 2.0,
	},
	{
		label:     "taiex",
		changePct: func(m *marketdata.MacroDataSnapshot) float64 { return m.TAIEX.ChangePct },
		sectors: []industry.SectorID{
			industry.SectorSemiconductor, industry.SectorElectronics,
			industry.SectorOptoelectronics, industry.SectorFinancials,
			industry.SectorCement, industry.SectorPlastics, industry.SectorTextiles,
			industry.SectorSteel, industry.SectorShipping, industry.SectorFood,
			industry.SectorAuto, industry.SectorTelecom, industry.SectorChemicals,
			industry.SectorBiotech, industry.SectorConstruction,
			industry.SectorOtherElectronics, industry.SectorMachinery,
			industry.SectorTourism, industry.SectorRetail, industry.SectorEnergy,
		},
		bullishOnUp: true,
		typicalMove: 1.0,
		sectorRelevance: map[industry.SectorID]float64{
			industry.SectorSemiconductor: 1.0,
			industry.SectorElectronics:   0.8,
			industry.SectorFinancials:    0.6,
		},
	},
}

// applyCycleShift adjusts scores based on the cycle position for the sector.
func (sp *SectorPredictor) applyCycleShift(sid industry.SectorID, scoreIn, scoreOut, scoreNeu *float64, contrib map[string]float64) {
	phaseScore := sp.cycle.GetContinuousPhaseScore(string(sid))
	if phaseScore == 0 {
		return
	}
	sens := cycleSensitivity(sid)
	impact := phaseScore * sens
	if impact > 0 {
		*scoreIn += impact
	} else if impact < 0 {
		*scoreOut += -impact
	} else {
		*scoreNeu += 0.2
	}
	if impact != 0 {
		contrib["cycle_position"] = math.Abs(impact)
	}
}

func cycleSensitivity(sid industry.SectorID) float64 {
	switch sid {
	case industry.SectorSemiconductor, industry.SectorShipping, industry.SectorSteel,
		industry.SectorElectronics, industry.SectorAuto, industry.SectorConstruction,
		industry.SectorMachinery, industry.SectorChemicals, industry.SectorPlastics,
		industry.SectorCement:
		return 1.0
	case industry.SectorOptoelectronics, industry.SectorOtherElectronics,
		industry.SectorTelecom, industry.SectorBiotech, industry.SectorEnergy,
		industry.SectorRetail, industry.SectorFood, industry.SectorTextiles,
		industry.SectorTourism:
		return 0.7
	case industry.SectorFinancials:
		return 0.4
	default:
		return 0.5
	}
}

// softmax3 converts three raw scores into a probability distribution.
// Results sum to 1.0 within floating-point tolerance.
func softmax3(a, b, c float64) PredictionDistribution {
	maxV := math.Max(a, math.Max(b, c))
	ea := math.Exp(a - maxV)
	eb := math.Exp(b - maxV)
	ec := math.Exp(c - maxV)
	sum := ea + eb + ec
	if sum == 0 {
		return PredictionDistribution{Inflow: 0.33, Neutral: 0.34, Outflow: 0.33}
	}
	raw := PredictionDistribution{
		Inflow:  ea / sum,
		Neutral: eb / sum,
		Outflow: ec / sum,
	}
	// Normalize after rounding to ensure sum is exactly 1.0.
	dist := PredictionDistribution{
		Inflow:  roundProb(raw.Inflow),
		Neutral: roundProb(raw.Neutral),
		Outflow: roundProb(raw.Outflow),
	}
	residual := 1.0 - (dist.Inflow + dist.Neutral + dist.Outflow)
	// Absorb residual into the largest component.
	switch {
	case dist.Inflow >= dist.Neutral && dist.Inflow >= dist.Outflow:
		dist.Inflow = roundProb(dist.Inflow + residual)
	case dist.Outflow >= dist.Inflow && dist.Outflow >= dist.Neutral:
		dist.Outflow = roundProb(dist.Outflow + residual)
	default:
		dist.Neutral = roundProb(dist.Neutral + residual)
	}
	return dist
}

// directionConfidence derives direction and confidence from a distribution.
func directionConfidence(dist PredictionDistribution) (dir string, conf float64) {
	switch {
	case dist.Inflow >= dist.Neutral && dist.Inflow >= dist.Outflow:
		dir = "inflow"
	case dist.Outflow >= dist.Inflow && dist.Outflow >= dist.Neutral:
		dir = "outflow"
	default:
		dir = "neutral"
	}
	conf = 1.0 - normalizedEntropy(dist)
	if conf < confidenceFloor {
		conf = confidenceFloor
	}
	return dir, conf
}

func normalizedEntropy(dist PredictionDistribution) float64 {
	var h float64
	for _, p := range []float64{dist.Inflow, dist.Neutral, dist.Outflow} {
		if p > 0 {
			h -= p * math.Log(p)
		}
	}
	if h == 0 {
		return 0
	}
	return h / math.Log(3.0)
}

// jsd computes Jensen-Shannon divergence between two distributions.
func jsd(a, b PredictionDistribution) float64 {
	m := PredictionDistribution{
		Inflow:  (a.Inflow + b.Inflow) / 2,
		Neutral: (a.Neutral + b.Neutral) / 2,
		Outflow: (a.Outflow + b.Outflow) / 2,
	}
	return (klDivergence(a, m) + klDivergence(b, m)) / 2
}

func klDivergence(p, q PredictionDistribution) float64 {
	var d float64
	for _, pair := range [][2]float64{
		{p.Inflow, q.Inflow},
		{p.Neutral, q.Neutral},
		{p.Outflow, q.Outflow},
	} {
		if pair[0] > 0 && pair[1] > 0 {
			d += pair[0] * math.Log(pair[0]/pair[1])
		}
	}
	return d
}

func topDrivers(contrib map[string]float64, n int) []string {
	type entry struct {
		name  string
		value float64
	}
	entries := make([]entry, 0, len(contrib))
	for name, val := range contrib {
		if name == "consistency_warning" {
			continue
		}
		if val <= 0 {
			continue
		}
		entries = append(entries, entry{name, val})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].value > entries[j].value })
	out := make([]string, 0, n)
	for i := 0; i < n && i < len(entries); i++ {
		out = append(out, entries[i].name)
	}
	return out
}

// ── Constants ──────────────────────────────────────────────────────────

const (
	jsdThreshold         = 0.25
	confidencePenaltyJSD = 0.85
	confidenceFloor      = 0.40
)
