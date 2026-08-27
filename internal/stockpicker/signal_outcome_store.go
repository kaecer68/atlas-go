package stockpicker

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// signalOutcomeSchema 是 stock_signal_outcomes 的 SQLite DDL。
// 與 sql/migrations/000018_stock_signal_outcomes.up.sql（PostgreSQL mirror）
// 保持欄位一致；SQLite 沒有 BOOLEAN 型別，hit 以 INTEGER 0/1 儲存。
const signalOutcomeSchema = `
CREATE TABLE IF NOT EXISTS stock_signal_outcomes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	symbol TEXT NOT NULL,
	trigger_date TEXT NOT NULL,
	source TEXT NOT NULL,
	forward_return REAL,
	net_forward_return REAL,
	hit INTEGER,
	cost_rate REAL,
	regime TEXT,
	created_at TEXT NOT NULL,
	UNIQUE(symbol, trigger_date, source)
);
`

// ensureSignalOutcomeSchema 在 SQLite 上建立 stock_signal_outcomes 表（冪等）。
//
// 儲存層複用 internal/ledger/ 的 SQLite 模式：由呼叫端提供已開啟的
// *sql.DB（例如經 ledger.OpenSQLiteDB 開啟的共享連線），本 package 不自行
// 建立第二套連線/事務層。此函式只做 CREATE TABLE IF NOT EXISTS，與
// ledger.InitSchema 的冪等 DDL 精神一致。
func ensureSignalOutcomeSchema(db *sql.DB) error {
	if _, err := db.Exec(signalOutcomeSchema); err != nil {
		return fmt.Errorf("stockpicker: ensure stock_signal_outcomes schema: %w", err)
	}
	return nil
}

// SignalOutcomeStore 是 stock_signal_outcomes 的 SQLite 儲存層薄包裝。
// 它持有已開啟的 *sql.DB，不負責開啟或關閉連線。
type SignalOutcomeStore struct {
	db *sql.DB
}

// NewSignalOutcomeStore 建立 SignalOutcomeStore 並確保底層表存在。
func NewSignalOutcomeStore(db *sql.DB) (*SignalOutcomeStore, error) {
	if err := ensureSignalOutcomeSchema(db); err != nil {
		return nil, err
	}
	return &SignalOutcomeStore{db: db}, nil
}

// RecordOutcomes 批次寫入 raw signal outcomes（冪等，見 package-level 版本）。
func (s *SignalOutcomeStore) RecordOutcomes(ctx context.Context, outcomes []SignalOutcome) error {
	return RecordOutcomes(ctx, s.db, outcomes)
}

// LoadOutcomes 依 (symbol, source) 讀取 raw signal outcomes（滾動窗口語意見 package-level 版本）。
func (s *SignalOutcomeStore) LoadOutcomes(ctx context.Context, symbol, source, window string) ([]SignalOutcome, error) {
	return LoadOutcomes(ctx, s.db, symbol, source, window)
}

// RecordOutcomes 將一批 raw signal outcomes 寫入 stock_signal_outcomes。
//
// 寫入採 INSERT OR IGNORE：遇到 (symbol, trigger_date, source) 重複鍵時
// 靜默跳過，使重跑回測/聚合 job 冪等（不報錯、不產生重複列）。source 為
// 空字串會回傳錯誤（對齊 schema 的 NOT NULL 語意，空字串不視為合法來源）。
//
// net_forward_return / cost_rate / regime 由未來的聚合 job（PR 1c）補寫；
// 本層只落 symbol/trigger_date/source/forward_return/hit/created_at。
func RecordOutcomes(ctx context.Context, db *sql.DB, outcomes []SignalOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}
	if err := ensureSignalOutcomeSchema(db); err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("stockpicker: begin signal outcome tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO stock_signal_outcomes
			(symbol, trigger_date, source, forward_return, hit, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("stockpicker: prepare signal outcome insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	createdAt := time.Now().UTC().Format(time.RFC3339)
	for _, o := range outcomes {
		if o.Source == "" {
			return fmt.Errorf("stockpicker: signal outcome has empty source (symbol=%q trigger_date=%q)", o.Symbol, o.TriggerDate)
		}
		if o.Symbol == "" {
			return fmt.Errorf("stockpicker: signal outcome has empty symbol (source=%q)", o.Source)
		}
		if o.TriggerDate == "" {
			return fmt.Errorf("stockpicker: signal outcome has empty trigger_date (symbol=%q source=%q)", o.Symbol, o.Source)
		}

		if _, err := stmt.ExecContext(ctx,
			o.Symbol, o.TriggerDate, o.Source, o.ForwardReturn, boolToInt(o.Hit), createdAt); err != nil {
			return fmt.Errorf("stockpicker: insert signal outcome symbol=%s date=%s source=%s: %w",
				o.Symbol, o.TriggerDate, o.Source, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("stockpicker: commit signal outcomes: %w", err)
	}
	return nil
}

// LoadOutcomes 依 (symbol, source) 讀取 raw signal outcomes。
//
// symbol 或 source 為空字串表示「不限定」該維度（呼叫端至少給其一以縮小
// 範圍）。window 為滾動窗口標籤（如 "120d"），非空時只回傳 trigger_date 落在
// 「UTC 現在往前 N 天」內的列；空字串表示不設日期下界。精確的 as-of 語意由
// 未來的聚合 job（PR 1c）定義，此處僅提供儲存層的滾動讀取。
func LoadOutcomes(ctx context.Context, db *sql.DB, symbol, source, window string) ([]SignalOutcome, error) {
	if err := ensureSignalOutcomeSchema(db); err != nil {
		return nil, err
	}

	query := `SELECT symbol, trigger_date, source, forward_return, hit FROM stock_signal_outcomes`
	var conds []string
	var args []any
	if symbol != "" {
		conds = append(conds, "symbol = ?")
		args = append(args, symbol)
	}
	if source != "" {
		conds = append(conds, "source = ?")
		args = append(args, source)
	}
	if window != "" {
		cutoff, err := rollingWindowCutoff(window)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "trigger_date >= ?")
		args = append(args, cutoff)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY trigger_date, source"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("stockpicker: query signal outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SignalOutcome, 0)
	for rows.Next() {
		var (
			o             SignalOutcome
			forwardReturn float64
			hit           int
		)
		if err := rows.Scan(&o.Symbol, &o.TriggerDate, &o.Source, &forwardReturn, &hit); err != nil {
			return nil, fmt.Errorf("stockpicker: scan signal outcome: %w", err)
		}
		o.ForwardReturn = forwardReturn
		o.Hit = hit == 1
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stockpicker: iterate signal outcomes: %w", err)
	}
	return out, nil
}

// rollingWindowCutoff 把滾動窗口標籤（"60d"/"120d"）轉成該窗口最早包含的
// trigger_date（YYYY-MM-DD）。空字串回傳 ""（無下界）。
func rollingWindowCutoff(window string) (string, error) {
	if window == "" {
		return "", nil
	}
	if !strings.HasSuffix(window, "d") {
		return "", fmt.Errorf("stockpicker: invalid window %q (want Nd, e.g. 120d)", window)
	}
	days, err := strconv.Atoi(strings.TrimSuffix(window, "d"))
	if err != nil || days <= 0 {
		return "", fmt.Errorf("stockpicker: invalid window %q (want Nd, e.g. 120d)", window)
	}
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02"), nil
}

// boolToInt 把 bool 轉成 SQLite INTEGER 欄位用的 0/1。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
