// Panel data sources for the stockpicker daily win-rate update.
//
// RealPanel is the production PanelSource: bars from a backend-aware
// ledger.QuoteStore and per-symbol T86 flows from
// data/state/stock_flows/<symbol>.json. Flow files are the production
// backfill target ("production 回填後補"); missing files mean the flow
// condition simply produces no triggers.
//
// Moved here from cmd/run-stockpicker-backtest/panel.go (PR 2e) so the
// scheduled daily-update task and the CLI share the exact same panel —
// no copy-pasted panel construction between the two entry points.
package stockpicker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// NewRealPanel reads bars from a QuoteStore and per-symbol T86 flows from
// data/state/stock_flows/<symbol>.json. Flow files are the production
// backfill target; missing files mean the flow condition stays silent.
func NewRealPanel(ctx context.Context, quoteStore ledger.QuoteStore, workDir string) (*RealPanel, error) {
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
	return &RealPanel{quoteStore: quoteStore, symbols: symbols, flowsDir: flowsDir}, nil
}

// PanelSymbols returns the symbols to scan: from universeFlag when set, else
// whatever the panel carries (quotes symbols).
func PanelSymbols(panel PanelSource, universeFlag string) []string {
	if rp, ok := panel.(*RealPanel); ok {
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

// RealPanel is the production PanelSource: bars from a QuoteStore, flows
// from per-symbol JSON files.
type RealPanel struct {
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

func (p *RealPanel) Bars(ctx context.Context, symbol string) ([]HistoricalBar, error) {
	variants := []string{symbol + ".TW", symbol, symbol + ".TWO"}
	for _, sym := range variants {
		quotes, err := p.quoteStore.LoadQuotes(sym, barWindowStart, barWindowEnd)
		if err != nil {
			return nil, fmt.Errorf("load quotes %s: %w", sym, err)
		}
		if len(quotes) > 0 {
			bars := make([]HistoricalBar, len(quotes))
			for i, q := range quotes {
				bars[i] = HistoricalBar{
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
	Symbol string      `json:"symbol"`
	Flows  []FlowPoint `json:"flows"`
}

func (p *RealPanel) Flows(ctx context.Context, symbol string) ([]FlowPoint, error) {
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
