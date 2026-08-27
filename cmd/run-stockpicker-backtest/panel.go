// Panel data sources for run-stockpicker-backtest.
//
// realPanel is the production PanelSource: bars from a backend-aware
// ledger.QuoteStore and per-symbol T86 flows from
// data/state/stock_flows/<symbol>.json. Flow files are the production
// backfill target ("production 回填後補"); missing files mean the flow
// condition simply produces no triggers.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// newRealPanel reads bars from a QuoteStore and per-symbol T86 flows from
// data/state/stock_flows/<symbol>.json. Flow files are the production
// backfill target; missing files mean the flow condition stays silent.
func newRealPanel(ctx context.Context, quoteStore ledger.QuoteStore, workDir string) (stockpicker.PanelSource, error) {
	lister, ok := quoteStore.(ledger.QuoteSymbolLister)
	var symbols []string
	var err error
	if ok {
		symbols, err = quoteSymbols(ctx, lister)
		if err != nil {
			return nil, err
		}
	}
	flowsDir := filepath.Join(workDir, "data", "state", "stock_flows")
	return &realPanel{quoteStore: quoteStore, symbols: symbols, flowsDir: flowsDir}, nil
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

// quoteSymbols lists distinct symbols from a QuoteSymbolLister and strips
// the exchange suffix so bars and T86 flows share the bare TWSE code used as
// the outcome symbol.
func quoteSymbols(ctx context.Context, lister ledger.QuoteSymbolLister) ([]string, error) {
	raw, err := lister.QuoteSymbols(ctx)
	if err != nil {
		return nil, fmt.Errorf("query quote symbols: %w", err)
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		sym := normalizeSymbol(s)
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		out = append(out, sym)
	}
	return out, nil
}

// normalizeSymbol strips the exchange suffix (.TW/.TWO) so bars and T86 flows
// share the bare TWSE code used as the outcome symbol.
func normalizeSymbol(s string) string {
	return strings.TrimSuffix(strings.TrimSuffix(s, ".TW"), ".TWO")
}

// realPanel is the production PanelSource: bars from a QuoteStore, flows
// from per-symbol JSON files.
type realPanel struct {
	quoteStore ledger.QuoteStore
	symbols    []string
	flowsDir   string
}

// barWindowStart is the earliest date loaded from the QuoteStore. Quotes are
// keyed by session date, so any date before the dataset is harmless.
var barWindowStart = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// barWindowEnd is the latest date loaded from the QuoteStore. Using now gives
// a stable upper bound without needing a schema query.
var barWindowEnd = time.Now()

func (p *realPanel) Bars(ctx context.Context, symbol string) ([]stockpicker.HistoricalBar, error) {
	variants := []string{symbol + ".TW", symbol, symbol + ".TWO"}
	for _, sym := range variants {
		quotes, err := p.quoteStore.LoadQuotes(sym, barWindowStart, barWindowEnd)
		if err != nil {
			return nil, fmt.Errorf("load quotes %s: %w", sym, err)
		}
		if len(quotes) > 0 {
			bars := make([]stockpicker.HistoricalBar, len(quotes))
			for i, q := range quotes {
				bars[i] = stockpicker.HistoricalBar{
					Date:   q.Date,
					Close:  q.Close,
					Volume: q.Volume,
				}
			}
			return bars, nil
		}
	}
	return nil, nil
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
