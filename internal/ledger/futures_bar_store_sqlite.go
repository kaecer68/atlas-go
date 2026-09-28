package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SQLiteFuturesBarStore 是 futures_bars 的 SQLite 後端（本機開發用）。
type SQLiteFuturesBarStore struct {
	db *sql.DB
}

// NewSQLiteFuturesBarStore 以既有 *sql.DB 建立 store。
func NewSQLiteFuturesBarStore(db *sql.DB) *SQLiteFuturesBarStore {
	return &SQLiteFuturesBarStore{db: db}
}

// NewSQLiteFuturesBarStoreFromPath 開檔、確保 schema 後建立 store。
func NewSQLiteFuturesBarStoreFromPath(path string) (*SQLiteFuturesBarStore, error) {
	db, err := OpenSQLiteDB(path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if err := InitSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return NewSQLiteFuturesBarStore(db), nil
}

// Compile-time assertion.
var _ FuturesBarStore = (*SQLiteFuturesBarStore)(nil)

// RecordFuturesBars upsert（INSERT OR REPLACE），以四欄複合鍵冪等。
// 缺值以 SQL NULL 寫入（**不寫 0**）。
func (s *SQLiteFuturesBarStore) RecordFuturesBars(ctx context.Context, bars []domain.FuturesBar) (int, error) {
	if len(bars) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint:errcheck
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO futures_bars (
			contract, contract_month, trade_date, session,
			open, high, low, close, volume, settlement_price, open_interest, source, fetched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for _, b := range bars {
		if _, err := stmt.ExecContext(ctx,
			b.Contract, b.ContractMonth, b.TradeDate.Format("2006-01-02"), string(b.Session),
			futuresFloatPtr(b.Open), futuresFloatPtr(b.High), futuresFloatPtr(b.Low), futuresFloatPtr(b.Close),
			futuresIntPtr(b.Volume), futuresFloatPtr(b.SettlementPrice), futuresIntPtr(b.OpenInterest),
			b.Source, now,
		); err != nil {
			return 0, fmt.Errorf("insert futures bar: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return n, nil
}

// LoadFuturesBars 回傳 [start, end] 的 bar（依 trade_date、session 升冪）。
//
// 過濾條件用 NULL-safe 形式（`? = ” OR col = ?`）而非動態拼接 SQL：
// 參數一律綁定，且不需要依條件組字串。
func (s *SQLiteFuturesBarStore) LoadFuturesBars(ctx context.Context, contract, contractMonth string, session domain.FuturesSession, start, end time.Time) ([]domain.FuturesBar, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT contract, contract_month, trade_date, session,
		       open, high, low, close, volume, settlement_price, open_interest, source
		FROM futures_bars
		WHERE contract = ?
		  AND trade_date >= ?
		  AND trade_date <= ?
		  AND (? = '' OR contract_month = ?)
		  AND (? = '' OR session = ?)
		ORDER BY trade_date ASC, session ASC, contract_month ASC`,
		contract, start.Format("2006-01-02"), end.Format("2006-01-02"),
		contractMonth, contractMonth, string(session), string(session))
	if err != nil {
		return nil, fmt.Errorf("query futures bars: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanFuturesBars(rows)
}

// LoadLatestFuturesBars 回傳每個到期月最新一日的 bar。
func (s *SQLiteFuturesBarStore) LoadLatestFuturesBars(ctx context.Context, contract string) ([]domain.FuturesBar, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT contract, contract_month, trade_date, session,
		       open, high, low, close, volume, settlement_price, open_interest, source
		FROM futures_bars f
		WHERE contract = ?
		  AND trade_date = (
			SELECT MAX(trade_date) FROM futures_bars g
			WHERE g.contract = f.contract AND g.contract_month = f.contract_month
		  )
		ORDER BY contract_month ASC, session ASC`, contract)
	if err != nil {
		return nil, fmt.Errorf("query latest futures bars: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanFuturesBars(rows)
}

// rowScanner 是 *sql.Rows 的最小介面（供 SQLite / Postgres 共用 scan 邏輯）。
type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanFuturesBars(rows rowScanner) ([]domain.FuturesBar, error) {
	var out []domain.FuturesBar
	for rows.Next() {
		var (
			b            domain.FuturesBar
			dateStr      string
			sessionStr   string
			open         sql.NullFloat64
			high         sql.NullFloat64
			low          sql.NullFloat64
			close        sql.NullFloat64
			settlement   sql.NullFloat64
			volume       sql.NullInt64
			openInterest sql.NullInt64
		)
		if err := rows.Scan(&b.Contract, &b.ContractMonth, &dateStr, &sessionStr,
			&open, &high, &low, &close, &volume, &settlement, &openInterest, &b.Source); err != nil {
			return nil, fmt.Errorf("scan futures bar: %w", err)
		}
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse trade_date %q: %w", dateStr, err)
		}
		b.TradeDate = date
		b.Session = domain.FuturesSession(sessionStr)
		b.Open = futuresNullFloatPtr(open)
		b.High = futuresNullFloatPtr(high)
		b.Low = futuresNullFloatPtr(low)
		b.Close = futuresNullFloatPtr(close)
		b.SettlementPrice = futuresNullFloatPtr(settlement)
		b.Volume = futuresNullIntPtr(volume)
		b.OpenInterest = futuresNullIntPtr(openInterest)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate futures bars: %w", err)
	}
	return out, nil
}

// ├─ 缺值輔助：nil ↔ SQL NULL（**禁止**以 0 代替 NULL）────────────────────

func futuresFloatPtr(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func futuresIntPtr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func futuresNullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func futuresNullIntPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	i := v.Int64
	return &i
}

// RecordFuturesRollovers upsert splice 事件（冪等鍵為四欄複合主鍵）。
func (s *SQLiteFuturesBarStore) RecordFuturesRollovers(ctx context.Context, rollovers []domain.FuturesRollover) (int, error) {
	if len(rollovers) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() //nolint:errcheck
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO futures_rollovers (
			contract, roll_date, from_month, to_month, price_diff, computed_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare rollover statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for _, r := range rollovers {
		if _, err := stmt.ExecContext(ctx,
			r.Contract, r.RollDate.Format("2006-01-02"), r.FromMonth, r.ToMonth,
			futuresFloatPtr(r.PriceDiff), now,
		); err != nil {
			return 0, fmt.Errorf("insert futures rollover: %w", err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return n, nil
}

// LoadFuturesRollovers 回傳某契約的 splice 事件。
func (s *SQLiteFuturesBarStore) LoadFuturesRollovers(ctx context.Context, contract string) ([]domain.FuturesRollover, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT contract, roll_date, from_month, to_month, price_diff
		FROM futures_rollovers
		WHERE contract = ?
		ORDER BY roll_date ASC, from_month ASC`, contract)
	if err != nil {
		return nil, fmt.Errorf("query futures rollovers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.FuturesRollover
	for rows.Next() {
		var (
			r       domain.FuturesRollover
			dateStr string
			diff    sql.NullFloat64
		)
		if err := rows.Scan(&r.Contract, &dateStr, &r.FromMonth, &r.ToMonth, &diff); err != nil {
			return nil, fmt.Errorf("scan futures rollover: %w", err)
		}
		date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("parse roll_date %q: %w", dateStr, err)
		}
		r.RollDate = date
		r.PriceDiff = futuresNullFloatPtr(diff)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate futures rollovers: %w", err)
	}
	return out, nil
}
