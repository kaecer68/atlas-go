// Package ledger — PostgresFuturesBarStore
//
// futures_bars 的 PostgreSQL 後端。生產為 Postgres-first（ATLAS_STORE_BACKEND=postgres），
// 因此這是回補 CLI 在生產實際使用的後端（見 docs/specs/futures-bars-firstparty-spec.md §5）。
//
// trade_date 以 DATE 儲存（migration 000025），pgx 直讀 time.Time。
// 缺值一律 NULL：來源的 "-"/"NULL" 在 provider 層已轉成 nil，本層不得補 0。
package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PostgresFuturesBarStore 是 FuturesBarStore 的 Postgres 實作。
type PostgresFuturesBarStore struct {
	pool *pgxpool.Pool
}

// NewPostgresFuturesBarStore 綁定已開啟的 pgxpool。
func NewPostgresFuturesBarStore(pool *pgxpool.Pool) *PostgresFuturesBarStore {
	return &PostgresFuturesBarStore{pool: pool}
}

// Compile-time assertion.
var _ FuturesBarStore = (*PostgresFuturesBarStore)(nil)

// RecordFuturesBars 以 ON CONFLICT upsert 寫入（冪等鍵 = 四欄複合主鍵）。
func (s *PostgresFuturesBarStore) RecordFuturesBars(ctx context.Context, bars []domain.FuturesBar) (int, error) {
	if len(bars) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, b := range bars {
		batch.Queue(`
			INSERT INTO futures_bars (
				contract, contract_month, trade_date, session,
				open, high, low, close, volume, settlement_price, open_interest, source, fetched_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())
			ON CONFLICT (contract, contract_month, trade_date, session) DO UPDATE SET
				open = excluded.open,
				high = excluded.high,
				low = excluded.low,
				close = excluded.close,
				volume = excluded.volume,
				settlement_price = excluded.settlement_price,
				open_interest = excluded.open_interest,
				source = excluded.source,
				fetched_at = excluded.fetched_at
		`, b.Contract, b.ContractMonth, b.TradeDate.Format("2006-01-02"), string(b.Session),
			futuresFloatPtr(b.Open), futuresFloatPtr(b.High), futuresFloatPtr(b.Low), futuresFloatPtr(b.Close),
			futuresIntPtr(b.Volume), futuresFloatPtr(b.SettlementPrice), futuresIntPtr(b.OpenInterest), b.Source)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	if _, err := br.Exec(); err != nil {
		return 0, fmt.Errorf("insert futures bars: %w", err)
	}
	return len(bars), nil
}

// LoadFuturesBars 回傳 [start, end] 的 bar。
//
// 過濾條件用 NULL-safe 形式（`$n = ” OR col = $n`）而非動態拼接 SQL。
func (s *PostgresFuturesBarStore) LoadFuturesBars(ctx context.Context, contract, contractMonth string, session domain.FuturesSession, start, end time.Time) ([]domain.FuturesBar, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT contract, contract_month, to_char(trade_date, 'YYYY-MM-DD'), session,
		       open, high, low, close, volume, settlement_price, open_interest, source
		FROM futures_bars
		WHERE contract = $1
		  AND trade_date >= $2::date
		  AND trade_date <= $3::date
		  AND ($4 = '' OR contract_month = $4)
		  AND ($5 = '' OR session = $5)
		ORDER BY trade_date ASC, session ASC, contract_month ASC`,
		contract, start.Format("2006-01-02"), end.Format("2006-01-02"), contractMonth, string(session))
	if err != nil {
		return nil, fmt.Errorf("query futures bars: %w", err)
	}
	defer rows.Close()
	return scanFuturesBarsPG(rows)
}

// LoadLatestFuturesBars 回傳每個到期月最新一日的 bar。
func (s *PostgresFuturesBarStore) LoadLatestFuturesBars(ctx context.Context, contract string) ([]domain.FuturesBar, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.contract, f.contract_month, to_char(f.trade_date, 'YYYY-MM-DD'), f.session,
		       f.open, f.high, f.low, f.close, f.volume, f.settlement_price, f.open_interest, f.source
		FROM futures_bars f
		WHERE f.contract = $1
		  AND f.trade_date = (
			SELECT MAX(g.trade_date) FROM futures_bars g
			WHERE g.contract = f.contract AND g.contract_month = f.contract_month
		  )
		ORDER BY f.contract_month ASC, f.session ASC`, contract)
	if err != nil {
		return nil, fmt.Errorf("query latest futures bars: %w", err)
	}
	defer rows.Close()
	return scanFuturesBarsPG(rows)
}

// pgFuturesBarRows 是 pgx.Rows 的最小介面。
type pgFuturesBarRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// scanFuturesBarsPG 用 pgx 原生可空型別掃描（`*float64` / `*int64` 對 NULL 會給 nil）。
//
// 刻意**不**與 SQLite 路徑共用 scanner：SQLite 走 database/sql（`sql.NullFloat64`），
// Postgres 走 pgx（原生指標）。兩者對 NULL 的表示法不同，共用會讓其中一條路徑
// 只在真實 PG 上才會壞掉（而本機沒有 PG 可測）。
func scanFuturesBarsPG(rows pgFuturesBarRows) ([]domain.FuturesBar, error) {
	var out []domain.FuturesBar
	for rows.Next() {
		var (
			b          domain.FuturesBar
			dateStr    string
			sessionStr string
		)
		if err := rows.Scan(&b.Contract, &b.ContractMonth, &dateStr, &sessionStr,
			&b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.SettlementPrice, &b.OpenInterest, &b.Source); err != nil {
			return nil, fmt.Errorf("scan futures bar: %w", err)
		}
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse trade_date %q: %w", dateStr, err)
		}
		b.TradeDate = date
		b.Session = domain.FuturesSession(sessionStr)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate futures bars: %w", err)
	}
	return out, nil
}

// RecordFuturesRollovers upsert splice 事件。
func (s *PostgresFuturesBarStore) RecordFuturesRollovers(ctx context.Context, rollovers []domain.FuturesRollover) (int, error) {
	if len(rollovers) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, r := range rollovers {
		batch.Queue(`
			INSERT INTO futures_rollovers (
				contract, roll_date, from_month, to_month, price_diff, computed_at
			) VALUES ($1, $2::date, $3, $4, $5, now())
			ON CONFLICT (contract, roll_date, from_month, to_month) DO UPDATE SET
				price_diff = excluded.price_diff,
				computed_at = excluded.computed_at
		`, r.Contract, r.RollDate.Format("2006-01-02"), r.FromMonth, r.ToMonth, futuresFloatPtr(r.PriceDiff))
	}
	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	if _, err := br.Exec(); err != nil {
		return 0, fmt.Errorf("insert futures rollovers: %w", err)
	}
	return len(rollovers), nil
}

// LoadFuturesRollovers 回傳某契約的 splice 事件。
func (s *PostgresFuturesBarStore) LoadFuturesRollovers(ctx context.Context, contract string) ([]domain.FuturesRollover, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT contract, to_char(roll_date, 'YYYY-MM-DD'), from_month, to_month, price_diff
		FROM futures_rollovers
		WHERE contract = $1
		ORDER BY roll_date ASC, from_month ASC`, contract)
	if err != nil {
		return nil, fmt.Errorf("query futures rollovers: %w", err)
	}
	defer rows.Close()

	var out []domain.FuturesRollover
	for rows.Next() {
		var (
			r       domain.FuturesRollover
			dateStr string
			diff    *float64
		)
		if err := rows.Scan(&r.Contract, &dateStr, &r.FromMonth, &r.ToMonth, &diff); err != nil {
			return nil, fmt.Errorf("scan futures rollover: %w", err)
		}
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse roll_date %q: %w", dateStr, err)
		}
		r.RollDate = date
		r.PriceDiff = diff
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate futures rollovers: %w", err)
	}
	return out, nil
}
