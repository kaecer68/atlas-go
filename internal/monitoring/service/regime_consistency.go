// File: regime_consistency.go
// Package: service
//
// Phase 2 Reconciler v1 — P1: regime three-endpoint consistency check.
//
// The atlas codebase stores market regime in three places that use two
// vocabularies plus a session enumeration:
//
//  1. session-level regime — LoadSessions reads sessions/<id>/summary.json
//     (domain.Regime vocabulary RISK_ON/RISK_OFF/NEUTRAL; defaults to
//     "unknown" when the field is absent or empty — see LoadSessions).
//  2. regime_history SQLite — the authoritative time series (Janus vocabulary
//     RISK_ON/RISK_OFF/NEUTRAL/TRANSITIONAL), written continuously by
//     DashboardAPI.persistRegimeHistory and seeded by stage-4 backfill.
//  3. stress_index_history — TaiwanStressCalculator vocabulary
//     (low/alert/high/crisis), cross-walked into the canonical regime
//     vocabulary via narrative.RegimeVocabularyMapping / NormalizeRegime.
//
// This checker reconciles the three endpoints over a look-back window and
// reports agreement (matches), drift, and the unknown-session ratio — the
// latter is a writer-gap signal: sessions whose summary.json carries an empty
// regime field (backfill/summaries.go writes "") or whose summary.json is
// missing entirely (orphan session). regime_history is the authoritative
// endpoint (persistRegimeHistory writes it continuously from the live stress
// index), so drift is measured against it.
package service

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

const (
	// RegimeConsistencyAuthoritative is the endpoint the reconciliation
	// treats as truth (Janus vocabulary, continuously written by
	// DashboardAPI.persistRegimeHistory).
	RegimeConsistencyAuthoritative = "regime_history"
	// RegimeConsistencyDefaultDays is the default look-back window in
	// calendar days (UTC, ending today).
	RegimeConsistencyDefaultDays = 30
	// RegimeConsistencyMaxDays caps the look-back window.
	RegimeConsistencyMaxDays = 365
	// RegimeUnknown is the LoadSessions fallback label for empty/missing
	// summary regime.
	RegimeUnknown = "unknown"

	// regimeUnknownRatioThreshold: an unknown-session ratio above this flips
	// the report status to unknown_high (writer-gap alert).
	regimeUnknownRatioThreshold = 0.20
)

// RegimeConsistency status values.
const (
	RegimeConsistencyOK          = "ok"
	RegimeConsistencyDrift       = "drift"
	RegimeConsistencyUnknownHigh = "unknown_high"
	RegimeConsistencyDegraded    = "degraded"
)

// RegimeConsistencyReport is the reconciliation output for the three regime
// endpoints. Status is one of ok / drift / unknown_high / degraded:
//   - drift: at least one date where stress_index_history or a session disagrees
//     with the authoritative regime_history after cross-walking vocabularies.
//   - unknown_high: unknown-session ratio in the window exceeds the threshold.
//   - degraded: HistoricalStore not wired (legacy deployment) — only the
//     session endpoint is available.
type RegimeConsistencyReport struct {
	Authoritative string             `json:"authoritative"`
	WindowDays    int                `json:"window_days"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Availability  RegimeAvailability `json:"availability"`

	RegimeHistory EndpointRegimeSummary `json:"regime_history"`
	Sessions      SessionRegimeSummary  `json:"sessions"`
	StressIndex   StressRegimeSummary   `json:"stress_index"`

	ComparedDays int     `json:"compared_days"`
	Matches      int     `json:"matches"`
	Drifts       int     `json:"drifts"`
	UnknownCount int     `json:"unknown_count"`
	UnknownRatio float64 `json:"unknown_ratio"`
	Status       string  `json:"status"`

	DriftDetails []RegimeDrift    `json:"drift_details,omitempty"`
	WriterGap    *RegimeWriterGap `json:"writer_gap,omitempty"`
}

// RegimeAvailability reports which endpoints were readable for this run.
type RegimeAvailability struct {
	RegimeHistory bool `json:"regime_history"`
	StressIndex   bool `json:"stress_index"`
	Sessions      bool `json:"sessions"`
}

// EndpointRegimeSummary is a per-endpoint distribution over the window,
// keyed by canonical regime label ("unknown" included where applicable).
type EndpointRegimeSummary struct {
	Rows         int            `json:"rows"`
	Regimes      map[string]int `json:"regimes"`
	LatestDate   string         `json:"latest_date"`
	LatestRegime string         `json:"latest_regime"`
}

// SessionRegimeSummary summarizes session-level regimes in the window.
type SessionRegimeSummary struct {
	Scanned      int            `json:"scanned"` // all sessions on disk (window-unfiltered)
	Total        int            `json:"total"`   // sessions whose trading date falls in the window
	Regimes      map[string]int `json:"regimes"`
	UnknownCount int            `json:"unknown_count"`
	UnknownRatio float64        `json:"unknown_ratio"`
}

// StressRegimeSummary summarizes stress_index_history rows in the window.
// Regimes uses the raw stress vocabulary (low/alert/high/crisis); Normalized
// shows the same rows after cross-walking into the canonical vocabulary.
type StressRegimeSummary struct {
	Rows         int            `json:"rows"`
	Regimes      map[string]int `json:"regimes"`
	Normalized   map[string]int `json:"normalized"`
	LatestDate   string         `json:"latest_date"`
	LatestRegime string         `json:"latest_regime"`
}

// RegimeDrift is one date where a non-authoritative endpoint disagreed with
// regime_history after cross-walking.
type RegimeDrift struct {
	Date          string `json:"date"`
	Authoritative string `json:"authoritative"`
	Endpoint      string `json:"endpoint"` // "stress_index" | "session"
	Actual        string `json:"actual"`
	Normalized    string `json:"normalized"`
}

// RegimeWriterGap explains why sessions report "unknown": the summary.json
// either exists with an empty regime field (writer wrote "") or is missing
// (orphan session). Both are writer-side field-mapping gaps of the same
// family as N2 (DualWrite NULL scan), not read-side drift.
type RegimeWriterGap struct {
	UnknownSessionIDs    []string `json:"unknown_session_ids,omitempty"`
	EmptyRegimeInSummary int      `json:"empty_regime_in_summary"`
	MissingSummary       int      `json:"missing_summary"`
	RootCause            string   `json:"root_cause"`
}

const regimeWriterGapRootCause = `session summary regime 欄位未寫入 (writer 缺口, 與 N2 同族欄位映射缺口): ` +
	`(a) backfill/summaries.go 為孤兒 session 寫入 Regime: "" (empty), ` +
	`(b) legacy SQLite session_summaries rows (BL-01 前) 無 summary_json, LoadSessionSummaries 投影無 regime 欄. ` +
	`LoadSessions 對空 regime 預設 "unknown" (pipeline.go LoadSessions). ` +
	`修復方向: backfill 由 regime_history cross-walk 填補, 或 Reconciler 以 authoritative 值覆寫。`

// CheckRegimeConsistency reconciles the three regime endpoints over the last
// `days` calendar days (UTC, ending today). regime_history is the
// authoritative endpoint; stress_index_history rows and session-level regimes
// are cross-walked into the canonical regime vocabulary before comparison.
//
// When the HistoricalStore is not wired (legacy deployments), the
// authoritative and stress endpoints are unavailable: the report degrades to
// session-only with Status=degraded instead of failing the request.
func (s *PipelineService) CheckRegimeConsistency(ctx context.Context, days int) (*RegimeConsistencyReport, error) {
	days = clampRegimeWindow(days)
	report := &RegimeConsistencyReport{
		Authoritative: RegimeConsistencyAuthoritative,
		WindowDays:    days,
		GeneratedAt:   time.Now().UTC(),
		Availability: RegimeAvailability{
			RegimeHistory: s.historicalStore != nil,
			StressIndex:   s.historicalStore != nil,
			Sessions:      true,
		},
		DriftDetails: []RegimeDrift{},
	}

	// Session endpoint: always computable from the ledger dir.
	sessions, err := s.LoadSessions()
	if err != nil {
		return nil, err
	}
	sessionSum, sessionsByDate := summarizeSessions(sessions, days)
	report.Sessions = sessionSum
	report.UnknownCount = sessionSum.UnknownCount
	if sessionSum.Total > 0 {
		report.UnknownRatio = float64(sessionSum.UnknownCount) / float64(sessionSum.Total)
	}
	if report.UnknownCount > 0 {
		report.WriterGap = attributeRegimeWriterGap(sessions, days, s.LedgerDir)
	}

	if s.historicalStore == nil {
		report.Status = RegimeConsistencyDegraded
		return report, nil
	}

	// Authoritative endpoint: regime_history SQLite (both live persistRegimeHistory
	// rows and stage-4 backfill rows — LoadRegimeHistoryAll, not the synthetic-
	// filtered variant, so backfilled dates participate in the reconciliation).
	authByDate, authSummary, err := s.loadRegimeHistoryForConsistency(ctx, days)
	if err != nil {
		return nil, err
	}
	report.RegimeHistory = authSummary

	// Stress endpoint: stress_index_history SQLite (same include-synthetic rule).
	stressByDate, stressSummary, err := s.loadStressHistoryForConsistency(ctx, days)
	if err != nil {
		return nil, err
	}
	report.StressIndex = stressSummary

	// Cross-walk + drift detection, per authoritative date.
	for date, auth := range authByDate {
		report.ComparedDays++
		drifted := false

		// stress endpoint: stress vocabulary → canonical, then compare.
		if row, ok := stressByDate[date]; ok && row.Regime != "" {
			norm := narrative.NormalizeRegime(row.Regime)
			if norm != auth {
				drifted = true
				report.DriftDetails = append(report.DriftDetails, RegimeDrift{
					Date: date, Authoritative: auth, Endpoint: "stress_index",
					Actual: row.Regime, Normalized: norm,
				})
			}
		}

		// session endpoint: domain vocabulary is already canonical. "unknown"
		// sessions are a writer gap (counted via UnknownRatio), not a drift.
		for _, sm := range sessionsByDate[date] {
			if sm.Regime == "" || sm.Regime == RegimeUnknown {
				continue
			}
			norm := narrative.NormalizeRegime(sm.Regime)
			if norm != auth {
				drifted = true
				report.DriftDetails = append(report.DriftDetails, RegimeDrift{
					Date: date, Authoritative: auth, Endpoint: "session",
					Actual: sm.Regime, Normalized: norm,
				})
			}
		}

		if drifted {
			report.Drifts++
		} else {
			report.Matches++
		}
	}

	switch {
	case report.Drifts > 0:
		report.Status = RegimeConsistencyDrift
	case report.UnknownRatio > regimeUnknownRatioThreshold:
		report.Status = RegimeConsistencyUnknownHigh
	default:
		report.Status = RegimeConsistencyOK
	}
	return report, nil
}

func clampRegimeWindow(days int) int {
	if days <= 0 {
		return RegimeConsistencyDefaultDays
	}
	if days > RegimeConsistencyMaxDays {
		return RegimeConsistencyMaxDays
	}
	return days
}

// windowMinDate returns the inclusive window start as a UTC date string.
func windowMinDate(days int) string {
	return time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
}

// windowMaxDate returns today's UTC date string (inclusive window end).
func windowMaxDate() string {
	return time.Now().UTC().Format("2006-01-02")
}

// sessionTradingDate derives the trading date a session should be bucketed
// under: the session ID date (authoritative, same convention as LoadSessions
// sorting) with RecordedAt as fallback.
func sessionTradingDate(sm SessionMeta) string {
	d := domain.SessionDateFromID(sm.SessionID)
	if d.IsZero() {
		d = sm.RecordedAt
	}
	if d.IsZero() {
		return ""
	}
	return d.UTC().Format("2006-01-02")
}

// summarizeSessions buckets sessions into the window and tallies the regime
// distribution (raw labels, including "unknown").
func summarizeSessions(sessions []SessionMeta, days int) (SessionRegimeSummary, map[string][]SessionMeta) {
	minDate, maxDate := windowMinDate(days), windowMaxDate()
	sum := SessionRegimeSummary{
		Scanned: len(sessions),
		Regimes: map[string]int{},
	}
	byDate := map[string][]SessionMeta{}
	for _, sm := range sessions {
		date := sessionTradingDate(sm)
		if date == "" || date < minDate || date > maxDate {
			continue
		}
		sum.Total++
		sum.Regimes[sm.Regime]++
		if sm.Regime == "" || sm.Regime == RegimeUnknown {
			sum.UnknownCount++
		}
		byDate[date] = append(byDate[date], sm)
	}
	if sum.Total > 0 {
		sum.UnknownRatio = float64(sum.UnknownCount) / float64(sum.Total)
	}
	return sum, byDate
}

// attributeRegimeWriterGap classifies each in-window unknown session as either
// "summary.json exists but regime empty" (writer wrote an empty value) or
// "summary.json missing" (orphan session).
func attributeRegimeWriterGap(sessions []SessionMeta, days int, ledgerDir string) *RegimeWriterGap {
	minDate, maxDate := windowMinDate(days), windowMaxDate()
	gap := &RegimeWriterGap{
		RootCause: regimeWriterGapRootCause,
	}
	for _, sm := range sessions {
		date := sessionTradingDate(sm)
		if date == "" || date < minDate || date > maxDate {
			continue
		}
		if sm.Regime != RegimeUnknown {
			continue
		}
		gap.UnknownSessionIDs = append(gap.UnknownSessionIDs, sm.SessionID)
		summaryPath := filepath.Join(ledgerDir, "sessions", sm.SessionID, "summary.json")
		if _, err := os.Stat(summaryPath); err != nil {
			gap.MissingSummary++
		} else {
			gap.EmptyRegimeInSummary++
		}
	}
	if len(gap.UnknownSessionIDs) == 0 {
		return nil
	}
	return gap
}

// loadRegimeHistoryForConsistency loads regime_history rows for the window.
// LoadRegimeHistoryAll is used so stage-4 synthetic backfill rows (one of the
// two documented sources of regime_history) participate in the reconciliation;
// the filtered variant would silently drop every backfilled date.
func (s *PipelineService) loadRegimeHistoryForConsistency(ctx context.Context, days int) (map[string]string, EndpointRegimeSummary, error) {
	rows, err := s.historicalStore.LoadRegimeHistoryAll(ctx, regimeStoreLimit(days))
	if err != nil {
		return nil, EndpointRegimeSummary{}, err
	}
	minDate := windowMinDate(days)
	byDate := map[string]string{}
	sum := EndpointRegimeSummary{Regimes: map[string]int{}}
	for _, r := range rows {
		if r.Date < minDate {
			continue
		}
		regime := r.Regime
		if regime == "" {
			regime = RegimeUnknown
		}
		byDate[r.Date] = regime
		sum.Rows++
		sum.Regimes[regime]++
		if sum.LatestDate == "" || r.Date > sum.LatestDate {
			sum.LatestDate = r.Date
			sum.LatestRegime = regime
		}
	}
	return byDate, sum, nil
}

// loadStressHistoryForConsistency loads stress_index_history rows for the
// window (include-synthetic for the same backfill reason as regime_history).
func (s *PipelineService) loadStressHistoryForConsistency(ctx context.Context, days int) (map[string]ledger.StressRow, StressRegimeSummary, error) {
	rows, err := s.historicalStore.LoadStressHistoryAll(ctx, regimeStoreLimit(days))
	if err != nil {
		return nil, StressRegimeSummary{}, err
	}
	minDate := windowMinDate(days)
	byDate := map[string]ledger.StressRow{}
	sum := StressRegimeSummary{
		Regimes:    map[string]int{},
		Normalized: map[string]int{},
	}
	for _, r := range rows {
		if r.Date < minDate {
			continue
		}
		byDate[r.Date] = r
		sum.Rows++
		regime := r.Regime
		if regime == "" {
			regime = RegimeUnknown
		}
		sum.Regimes[regime]++
		norm := narrative.NormalizeRegime(regime)
		if norm == "" {
			norm = RegimeUnknown
		}
		sum.Normalized[norm]++
		if sum.LatestDate == "" || r.Date > sum.LatestDate {
			sum.LatestDate = r.Date
			sum.LatestRegime = regime
		}
	}
	return byDate, sum, nil
}

// regimeStoreLimit mirrors loadRegimeHistoryFromStoreDays: a generous row
// limit so the in-memory window filter is authoritative.
func regimeStoreLimit(days int) int {
	limit := max(days*2, 90)
	return limit
}
