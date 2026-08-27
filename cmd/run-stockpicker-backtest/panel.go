// Panel data sources for run-stockpicker-backtest.
//
// realPanel is the production PanelSource: bars from the SQLite quotes table
// and per-symbol T86 flows from data/state/stock_flows/<symbol>.json. Flow
// files are the production backfill target ("production 回填後補"); missing
// files mean the flow condition simply produces no triggers.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// newRealPanel reads bars from the SQLite quotes table and per-symbol T86
// flows from data/state/stock_flows/<symbol>.json. Flow files are the
// production backfill target ("production 回填後補"); missing files mean the
// flow condition simply produces no triggers.
func newRealPanel(ctx context.Context, db *sql.DB, workDir string) (*realPanel, error) {
	symbols, err := quoteSymbols(ctx, db)
	if err != nil {
		return nil, err
	}
	flowsDir := filepath.Join(workDir, "data", "state", "stock_flows")
	return &realPanel{db: db, symbols: symbols, flowsDir: flowsDir}, nil
}

// panelSymbols returns the symbols to scan: from -universe when set, else
// whatever the panel carries (quotes symbols).
func panelSymbols(panel stockpicker.PanelSource, universeFlag string) []string {
	if rp, ok := panel.(*realPanel); ok {
		if universeFlag != "" {
			return splitUniverse(universeFlag)
		}
		return rp.symbols
	}
	if universeFlag != "" {
		return splitUniverse(universeFlag)
	}
	return []string{"fixture"}
}

func splitUniverse(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// quoteSymbols lists distinct symbols present in the quotes table.
func quoteSymbols(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT symbol FROM quotes ORDER BY symbol`)
	if err != nil {
		return nil, fmt.Errorf("query quote symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan quote symbol: %w", err)
		}
		out = append(out, normalizeSymbol(s))
	}
	return out, rows.Err()
}

// normalizeSymbol strips the exchange suffix (.TW/.TWO) so bars and T86 flows
// share the bare TWSE code used as the outcome symbol.
func normalizeSymbol(s string) string {
	return strings.TrimSuffix(strings.TrimSuffix(s, ".TW"), ".TWO")
}

// realPanel is the production PanelSource: bars from SQLite quotes, flows
// from per-symbol JSON files.
type realPanel struct {
	db       *sql.DB
	symbols  []string
	flowsDir string
}

func (p *realPanel) Bars(ctx context.Context, symbol string) ([]stockpicker.HistoricalBar, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT date, close, volume FROM quotes
		WHERE symbol = ? OR symbol = ? OR symbol = ?
		ORDER BY date ASC`, symbol, symbol+".TW", symbol+".TWO")
	if err != nil {
		return nil, fmt.Errorf("query bars %s: %w", symbol, err)
	}
	defer func() { _ = rows.Close() }()
	var bars []stockpicker.HistoricalBar
	for rows.Next() {
		var dateStr string
		var b stockpicker.HistoricalBar
		if err := rows.Scan(&dateStr, &b.Close, &b.Volume); err != nil {
			return nil, fmt.Errorf("scan bar %s: %w", symbol, err)
		}
		b.Date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("parse bar date %q: %w", dateStr, err)
		}
		bars = append(bars, b)
	}
	return bars, rows.Err()
}

// flowFile is the on-disk shape of data/state/stock_flows/<symbol>.json.
type flowFile struct {
	Symbol string                  `json:"symbol"`
	Flows  []stockpicker.FlowPoint `json:"flows"`
}

func (p *realPanel) Flows(ctx context.Context, symbol string) ([]stockpicker.FlowPoint, error) {
	data, err := os.ReadFile(filepath.Join(p.flowsDir, symbol+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			// No per-symbol flow history yet (production backfill pending):
			// the flow condition stays silent rather than fabricating data.
			return nil, nil
		}
		return nil, fmt.Errorf("read flows %s: %w", symbol, err)
	}
	var f flowFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse flows %s: %w", symbol, err)
	}
	return f.Flows, nil
}
