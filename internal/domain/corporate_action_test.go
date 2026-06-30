package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestCorporateAction_ZeroValue ensures the zero value is a usable, non-panicking
// struct — required because Go callers may declare it via `var x CorporateAction`
// before populating fields.
func TestCorporateAction_ZeroValue(t *testing.T) {
	var a domain.CorporateAction
	if a.Symbol != "" {
		t.Fatalf("expected empty Symbol, got %q", a.Symbol)
	}
	if !a.ExDate.IsZero() {
		t.Fatalf("expected zero ExDate, got %v", a.ExDate)
	}
	if a.CashDividend != 0 || a.StockDividend != 0 {
		t.Fatalf("expected zero dividend fields, got cash=%v stock=%v",
			a.CashDividend, a.StockDividend)
	}
	if a.CapitalReductionRatio != 0 || a.ReferencePrice != 0 {
		t.Fatalf("expected zero ratio and reference price, got ratio=%v ref=%v",
			a.CapitalReductionRatio, a.ReferencePrice)
	}
	if a.Source != "" {
		t.Fatalf("expected empty Source, got %q", a.Source)
	}
}

// TestCorporateAction_JSONTags verifies the JSON serialization shape is exactly
// the snake_case contract required by shared_web/static/js/shared/field_types.ts and
// downstream API consumers. Adding a new field requires updating this test.
func TestCorporateAction_JSONTags(t *testing.T) {
	a := domain.CorporateAction{
		Symbol:                "2330",
		ExDate:                time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		CashDividend:          12.0,
		StockDividend:         0.0,
		CapitalReductionRatio: 0.0,
		ReferencePrice:        938.0,
		Source:                "twse_calendar",
	}

	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	got := string(b)

	wantKeys := []string{
		`"symbol":"2330"`,
		`"ex_date":"2026-06-15T00:00:00Z"`,
		`"cash_dividend":12`,
		`"stock_dividend":0`,
		`"capital_reduction_ratio":0`,
		`"reference_price":938`,
		`"source":"twse_calendar"`,
	}
	for _, want := range wantKeys {
		if !strings.Contains(got, want) {
			t.Errorf("expected JSON to contain %s, got %s", want, got)
		}
	}

	// Explicitly guard against PascalCase leakage (json tag omission bug).
	for _, bad := range []string{
		`"Symbol":`,
		`"ExDate":`,
		`"CashDividend":`,
		`"StockDividend":`,
		`"CapitalReductionRatio":`,
		`"ReferencePrice":`,
		`"Source":`,
	} {
		if strings.Contains(got, bad) {
			t.Errorf("JSON leaked PascalCase field %s — missing snake_case tag: %s", bad, got)
		}
	}
}

// TestCorporateAction_RoundTrip ensures encode → decode preserves all fields.
func TestCorporateAction_RoundTrip(t *testing.T) {
	orig := domain.CorporateAction{
		Symbol:                "2454",
		ExDate:                time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
		CashDividend:          20.5,
		StockDividend:         1.5,
		CapitalReductionRatio: 0.0,
		ReferencePrice:        1180.0,
		Source:                "finmind",
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got domain.CorporateAction
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Symbol != orig.Symbol {
		t.Errorf("symbol: got %q want %q", got.Symbol, orig.Symbol)
	}
	if !got.ExDate.Equal(orig.ExDate) {
		t.Errorf("ex_date: got %v want %v", got.ExDate, orig.ExDate)
	}
	if got.CashDividend != orig.CashDividend {
		t.Errorf("cash_dividend: got %v want %v", got.CashDividend, orig.CashDividend)
	}
	if got.StockDividend != orig.StockDividend {
		t.Errorf("stock_dividend: got %v want %v", got.StockDividend, orig.StockDividend)
	}
	if got.ReferencePrice != orig.ReferencePrice {
		t.Errorf("reference_price: got %v want %v", got.ReferencePrice, orig.ReferencePrice)
	}
	if got.Source != orig.Source {
		t.Errorf("source: got %q want %q", got.Source, orig.Source)
	}
}
