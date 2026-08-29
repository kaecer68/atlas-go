package dailyreport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// Cross-day claim tracking.
//
// Each generated report emits up to three tracked claims (strategy
// recommendation, period call, risk warning). Claims carry their own
// independent state machine — orthogonal to the report workflow state:
//
//	tracking ──(verify after N trading days)──▶ verified | expired
//
//   - tracking: claim created and waiting for its verification window.
//   - verified: realized market data (or recommendation outcomes) was found
//     and the claim was evaluated; outcome holds the human-readable verdict.
//   - expired: the verification window passed but no data was available, so
//     the claim cannot be evaluated and leaves the tracking set.
//
// Claims persist to <ledgerDir>/report_tracked_claims.jsonl (append-only for
// creation, full rewrite when verification updates a claim).

// ClaimType classifies a tracked claim.
type ClaimType string

const (
	ClaimStrategyRecommendation ClaimType = "strategy_recommendation"
	ClaimPeriodCall             ClaimType = "period_call"
	ClaimRiskWarning            ClaimType = "risk_warning"
)

// ClaimStatus is the cross-day tracking state machine value.
type ClaimStatus string

const (
	ClaimTracking ClaimStatus = "tracking"
	ClaimVerified ClaimStatus = "verified"
	ClaimExpired  ClaimStatus = "expired"
)

// TrackedClaim is one persistable cross-day claim.
type TrackedClaim struct {
	ID          string      `json:"id"`
	ReportDate  string      `json:"report_date"`
	ClaimType   ClaimType   `json:"claim_type"`
	ClaimText   string      `json:"claim_text"`
	Symbol      string      `json:"symbol,omitempty"`
	VerifyAfter string      `json:"verify_after"` // YYYY-MM-DD, N trading days after report
	Status      ClaimStatus `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	VerifiedAt  time.Time   `json:"verified_at,omitzero"`
	Outcome     string      `json:"outcome,omitempty"`
}

// VerifyTradingDays is the default claim horizon: 5 trading days.
const VerifyTradingDays = 5

// claimsFileName is the tracker's JSONL persistence file under the ledger dir.
const claimsFileName = "report_tracked_claims.jsonl"

// taipei returns the Asia/Taipei location with a fixed-offset fallback.
func taipei() *time.Location {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return tz
}

// tradingDaysAfter returns the Nth trading day strictly after date.
// The report date itself is not counted (verify_after = 5 trading days later).
func tradingDaysAfter(date string, n int) (string, error) {
	d, err := time.ParseInLocation("2006-01-02", date, taipei())
	if err != nil {
		return "", fmt.Errorf("tradingDaysAfter: parse %s: %w", date, err)
	}
	count := 0
	for count < n {
		d = d.AddDate(0, 0, 1)
		if marketdata.IsTaiwanTradingDay(d) {
			count++
		}
	}
	return d.Format("2006-01-02"), nil
}

// periodDirection derives a trading direction from the seven-period
// classification, used to make period_call claims machine-verifiable.
func periodDirection(marketPeriod, nameZH string) string {
	s := strings.ToLower(marketPeriod + " " + nameZH)
	switch {
	case strings.Contains(s, "bull"), strings.Contains(s, "turnaround_up"), strings.Contains(s, "多"):
		return "偏多"
	case strings.Contains(s, "downturn"), strings.Contains(s, "black_swan"), strings.Contains(s, "空"):
		return "偏空"
	default:
		return "中性"
	}
}

// ClaimVerifier evaluates a due claim against realized data.
type ClaimVerifier interface {
	// Verify returns a human-readable outcome and ok=true when data was
	// available to evaluate the claim. ok=false means "no data" — the caller
	// marks the claim expired.
	Verify(claim *TrackedClaim) (outcome string, ok bool)
}

// Tracker persists and verifies cross-day claims.
type Tracker struct {
	mu        sync.Mutex
	path      string
	ledgerDir string
	replayDir string
	verifier  ClaimVerifier
	now       func() time.Time
}

// NewTracker creates a claim tracker persisted at
// <ledgerDir>/report_tracked_claims.jsonl. replayDir roots the price data
// (data/replay) used by the default verifier.
func NewTracker(ledgerDir, replayDir string) *Tracker {
	t := &Tracker{
		path:      filepath.Join(ledgerDir, claimsFileName),
		ledgerDir: ledgerDir,
		replayDir: replayDir,
		now:       time.Now,
	}
	t.verifier = NewMarketClaimVerifier(ledgerDir, replayDir)
	return t
}

// SetVerifier overrides the default claim verifier (test seam).
func (t *Tracker) SetVerifier(v ClaimVerifier) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.verifier = v
}

// SetNow injects a clock for deterministic tests.
func (t *Tracker) SetNow(fn func() time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = fn
}

// loadLocked reads all persisted claims. Caller must hold t.mu.
func (t *Tracker) loadLocked() ([]TrackedClaim, error) {
	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("tracker: open claims: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	claims := make([]TrackedClaim, 0, 32)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var c TrackedClaim
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("tracker: decode claim: %w", err)
		}
		claims = append(claims, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("tracker: scan claims: %w", err)
	}
	return claims, nil
}

// writeAllLocked rewrites the claims file. Caller must hold t.mu.
func (t *Tracker) writeAllLocked(claims []TrackedClaim) error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return fmt.Errorf("tracker: mkdir: %w", err)
	}
	tmp := t.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("tracker: create claims file: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, c := range claims {
		if err := enc.Encode(c); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("tracker: encode claim: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("tracker: close claims file: %w", err)
	}
	return os.Rename(tmp, t.path)
}

// appendLocked appends new claims. Caller must hold t.mu.
func (t *Tracker) appendLocked(claims []TrackedClaim) error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return fmt.Errorf("tracker: mkdir: %w", err)
	}
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("tracker: open claims: %w", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, c := range claims {
		if err := enc.Encode(c); err != nil {
			return fmt.Errorf("tracker: encode claim: %w", err)
		}
	}
	return nil
}

// strategyClaim builds the strategy recommendation claim.
func (t *Tracker) strategyClaim(r *Report) TrackedClaim {
	direction := r.Strategy.Direction
	if direction == "" {
		direction = "中性"
	}
	verifyAfter, err := tradingDaysAfter(r.Date, VerifyTradingDays)
	if err != nil {
		return TrackedClaim{}
	}
	return TrackedClaim{
		ID:          r.Date + "-" + string(ClaimStrategyRecommendation),
		ReportDate:  r.Date,
		ClaimType:   ClaimStrategyRecommendation,
		ClaimText:   fmt.Sprintf("策略:%s 方向:%s 進場:%s", r.Strategy.Active, direction, r.Strategy.EntryCond),
		Symbol:      "TAIEX",
		VerifyAfter: verifyAfter,
		Status:      ClaimTracking,
		CreatedAt:   t.now(),
	}
}

// periodClaim builds the period classification claim (only when a period
// section exists).
func (t *Tracker) periodClaim(r *Report) TrackedClaim {
	verifyAfter, err := tradingDaysAfter(r.Date, VerifyTradingDays)
	if err != nil {
		return TrackedClaim{}
	}
	dir := periodDirection(r.Period.MarketPeriod, r.Period.PeriodNameZH)
	return TrackedClaim{
		ID:          r.Date + "-" + string(ClaimPeriodCall),
		ReportDate:  r.Date,
		ClaimType:   ClaimPeriodCall,
		ClaimText:   fmt.Sprintf("週期:%s(%s) 方向:%s 現金水位:%.0f%%", r.Period.MarketPeriod, r.Period.PeriodNameZH, dir, r.Period.CashReserve),
		Symbol:      "TAIEX",
		VerifyAfter: verifyAfter,
		Status:      ClaimTracking,
		CreatedAt:   t.now(),
	}
}

// riskClaim builds the risk warning claim (only when a warning exists).
func (t *Tracker) riskClaim(r *Report) TrackedClaim {
	verifyAfter, err := tradingDaysAfter(r.Date, VerifyTradingDays)
	if err != nil {
		return TrackedClaim{}
	}
	text := fmt.Sprintf("風險等級:%s 壓力指數:%.2f", r.Risk.RiskLevel, r.Risk.StressIndex)
	if r.Risk.Warning != "" {
		text += " 警告:" + r.Risk.Warning
	}
	return TrackedClaim{
		ID:          r.Date + "-" + string(ClaimRiskWarning),
		ReportDate:  r.Date,
		ClaimType:   ClaimRiskWarning,
		ClaimText:   text,
		Symbol:      "TAIEX",
		VerifyAfter: verifyAfter,
		Status:      ClaimTracking,
		CreatedAt:   t.now(),
	}
}

// CreateClaimsFromReport creates the tracked claims for a report. Idempotent:
// claims already persisted for the report date are left untouched, so a
// regenerate or repeated call never duplicates rows.
func (t *Tracker) CreateClaimsFromReport(r *Report) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	existing, err := t.loadLocked()
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(existing))
	for _, c := range existing {
		seen[c.ID] = true
	}

	var fresh []TrackedClaim
	if c := t.strategyClaim(r); c.ID != "" && !seen[c.ID] {
		fresh = append(fresh, c)
		seen[c.ID] = true
	}
	if r.Period != nil {
		if c := t.periodClaim(r); c.ID != "" && !seen[c.ID] {
			fresh = append(fresh, c)
			seen[c.ID] = true
		}
	}
	if r.Risk.Warning != "" || r.Risk.DrawdownAlert || r.Risk.StressIndex >= 0.7 {
		if c := t.riskClaim(r); c.ID != "" && !seen[c.ID] {
			fresh = append(fresh, c)
			seen[c.ID] = true
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	return t.appendLocked(fresh)
}

// VerifyDueClaims evaluates every tracking claim whose verification window
// has passed by now (date comparison). Returns the number of claims that
// changed state. Idempotent: verified/expired claims are never re-evaluated.
func (t *Tracker) VerifyDueClaims(now time.Time) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	claims, err := t.loadLocked()
	if err != nil {
		return 0, err
	}
	today := now.In(taipei()).Format("2006-01-02")
	changed := 0
	for i := range claims {
		c := &claims[i]
		if c.Status != ClaimTracking {
			continue
		}
		if c.VerifyAfter == "" || c.VerifyAfter > today {
			continue
		}
		outcome, ok := t.verifier.Verify(c)
		if ok {
			c.Status = ClaimVerified
			c.Outcome = outcome
		} else {
			c.Status = ClaimExpired
			c.Outcome = "unverified:no_data"
		}
		c.VerifiedAt = t.now()
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	return changed, t.writeAllLocked(claims)
}

// ListClaims returns a copy of all persisted claims.
func (t *Tracker) ListClaims() []TrackedClaim {
	t.mu.Lock()
	defer t.mu.Unlock()
	claims, err := t.loadLocked()
	if err != nil {
		return nil
	}
	out := make([]TrackedClaim, len(claims))
	copy(out, claims)
	return out
}

// GetClaim returns the claim with id, or nil.
func (t *Tracker) GetClaim(id string) *TrackedClaim {
	t.mu.Lock()
	defer t.mu.Unlock()
	claims, err := t.loadLocked()
	if err != nil {
		return nil
	}
	for i := range claims {
		if claims[i].ID == id {
			c := claims[i]
			return &c
		}
	}
	return nil
}

// ---------- Default verifier ----------------------------------------------

// MarketClaimVerifier verifies claims against realized market data:
//   - symbol-level claims: recommendation outcomes recorded in the window
//     (ledgerDir/recommendation_outcomes.jsonl) are preferred;
//   - market-level claims (TAIEX) and fallback: index/stock prices from the
//     replay store (data/replay/twse_index_6months.jsonl /
//     twse_stocks_6months.jsonl).
type MarketClaimVerifier struct {
	ledgerDir string
	replayDir string
}

// NewMarketClaimVerifier creates the default verifier.
func NewMarketClaimVerifier(ledgerDir, replayDir string) *MarketClaimVerifier {
	return &MarketClaimVerifier{ledgerDir: ledgerDir, replayDir: replayDir}
}

// Verify implements ClaimVerifier.
func (v *MarketClaimVerifier) Verify(claim *TrackedClaim) (string, bool) {
	if outcome, ok := v.verifyFromOutcomes(claim); ok {
		return outcome, true
	}
	return v.verifyFromPrices(claim)
}

// verifyFromOutcomes checks recommendation outcomes for symbol-level claims.
func (v *MarketClaimVerifier) verifyFromOutcomes(claim *TrackedClaim) (string, bool) {
	if claim.Symbol == "" || claim.Symbol == "TAIEX" {
		return "", false
	}
	path := filepath.Join(v.ledgerDir, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	start, _ := time.ParseInLocation("2006-01-02", claim.ReportDate, taipei())
	end, _ := time.ParseInLocation("2006-01-02", claim.VerifyAfter, taipei())
	end = end.Add(24*time.Hour - time.Second)

	hits, total, sumFwd := 0, 0, 0.0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec struct {
			Symbol        string    `json:"symbol"`
			ForwardReturn float64   `json:"forward_return"`
			Hit           bool      `json:"hit"`
			RecordedAt    time.Time `json:"recorded_at"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Symbol != claim.Symbol {
			continue
		}
		if rec.RecordedAt.Before(start) || rec.RecordedAt.After(end) {
			continue
		}
		total++
		sumFwd += rec.ForwardReturn
		if rec.Hit {
			hits++
		}
	}
	if total == 0 {
		return "", false
	}
	verdict := "miss"
	if float64(hits)/float64(total) >= 0.5 {
		verdict = "hit"
	}
	return fmt.Sprintf("%s:%d/%d outcomes avg_fwd=%+.2f%%", verdict, hits, total, sumFwd/float64(total)*100), true
}

// verifyFromPrices computes the forward return of the claim's symbol between
// report date and verify_after from the replay price store.
func (v *MarketClaimVerifier) verifyFromPrices(claim *TrackedClaim) (string, bool) {
	base, ok := v.priceAt(claim.Symbol, claim.ReportDate)
	if !ok {
		return "", false
	}
	end, ok := v.priceAt(claim.Symbol, claim.VerifyAfter)
	if !ok {
		return "", false
	}
	fwd := end/base - 1
	return v.verdict(claim, fwd), true
}

// verdict maps a forward return to a claim verdict by claim type.
func (v *MarketClaimVerifier) verdict(claim *TrackedClaim, fwd float64) string {
	dir := claimDirection(claim.ClaimText)
	switch claim.ClaimType {
	case ClaimRiskWarning:
		if fwd <= -0.02 {
			return fmt.Sprintf("confirmed:%+.2f%%", fwd*100)
		}
		return fmt.Sprintf("not_confirmed:%+.2f%%", fwd*100)
	case ClaimPeriodCall:
		switch dir {
		case "偏多":
			if fwd >= 0 {
				return fmt.Sprintf("hit:%+.2f%%", fwd*100)
			}
			return fmt.Sprintf("miss:%+.2f%%", fwd*100)
		case "偏空":
			if fwd <= 0 {
				return fmt.Sprintf("hit:%+.2f%%", fwd*100)
			}
			return fmt.Sprintf("miss:%+.2f%%", fwd*100)
		default: // 中性 — sideways range-bound call
			if fwd > -0.02 && fwd < 0.02 {
				return fmt.Sprintf("hit:sideways:%+.2f%%", fwd*100)
			}
			return fmt.Sprintf("miss:move:%+.2f%%", fwd*100)
		}
	default: // strategy_recommendation
		switch dir {
		case "偏空":
			if fwd <= 0 {
				return fmt.Sprintf("hit:%+.2f%%", fwd*100)
			}
			return fmt.Sprintf("miss:%+.2f%%", fwd*100)
		default: // 偏多 / 中性 treated as long bias
			if fwd >= 0 {
				return fmt.Sprintf("hit:%+.2f%%", fwd*100)
			}
			return fmt.Sprintf("miss:%+.2f%%", fwd*100)
		}
	}
}

// claimDirection extracts the embedded 方向: marker from a claim text.
func claimDirection(text string) string {
	const marker = "方向:"
	_, after, ok := strings.Cut(text, marker)
	if !ok {
		return ""
	}
	rest := after
	if end := strings.IndexAny(rest, " "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// pricePoint is a single replay price row.
type pricePoint struct {
	date  string
	close float64
}

// priceAt returns the first available price for symbol on or after date.
// TAIEX (or empty) uses the index file; other symbols use the stocks file.
func (v *MarketClaimVerifier) priceAt(symbol, date string) (float64, bool) {
	rows, ok := v.loadPrices(symbol)
	if !ok {
		return 0, false
	}
	for _, p := range rows {
		if p.date >= date {
			return p.close, true
		}
	}
	return 0, false
}

// loadPrices loads and sorts the replay price rows for a symbol.
func (v *MarketClaimVerifier) loadPrices(symbol string) ([]pricePoint, bool) {
	var path string
	if symbol == "" || symbol == "TAIEX" {
		path = filepath.Join(v.replayDir, "twse_index_6months.jsonl")
	} else {
		path = filepath.Join(v.replayDir, "twse_stocks_6months.jsonl")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()

	rows := make([]pricePoint, 0, 128)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec struct {
			Date   string  `json:"date"`
			Symbol string  `json:"symbol"`
			Index  float64 `json:"index"`
			Close  float64 `json:"close"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if symbol != "" && symbol != "TAIEX" && rec.Symbol != symbol {
			continue
		}
		price := rec.Index
		if price == 0 {
			price = rec.Close
		}
		if price <= 0 {
			continue
		}
		rows = append(rows, pricePoint{date: rec.Date, close: price})
	}
	if len(rows) == 0 {
		return nil, false
	}
	// Insertion sort by date (files are already chronological; keep it simple
	// and correct for the rare out-of-order row).
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j-1].date > rows[j].date; j-- {
			rows[j-1], rows[j] = rows[j], rows[j-1]
		}
	}
	return rows, true
}
