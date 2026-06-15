package tax

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestCalculateDividendTax(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	tests := []struct {
		name   string
		amount float64
		want   float64
	}{
		{"zero", 0, 0},
		{"negative", -1000, 0},
		{"typical", 10000, 2800},
		{"large", 1000000, 280000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.CalculateDividendTax(tt.amount)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("CalculateDividendTax(%v) = %v, want %v", tt.amount, got, tt.want)
			}
		})
	}
}

// TestCalculateDividendTax_IncludeNHIGate verifies that TaxConfig.IncludeNHI
// actually gates the NHI surcharge out of the dividend tax calculation.
// IncludeNHI=true (default) uses the full DividendTaxRate (0.28).
// IncludeNHI=false subtracts NHISurchargeRate (0.02), yielding an effective
// rate of 0.26.
func TestCalculateDividendTax_IncludeNHIGate(t *testing.T) {
	const dividend = 10000.0

	cfgWithNHI := domain.TaxConfig{
		DividendTaxRate:    0.28,
		TransactionTaxRate: 0.003,
		IncludeNHI:         true,
	}
	cfgWithoutNHI := domain.TaxConfig{
		DividendTaxRate:    0.28,
		TransactionTaxRate: 0.003,
		IncludeNHI:         false,
	}

	withNHI := NewTaiwanTaxCalculator(cfgWithNHI).CalculateDividendTax(dividend)
	withoutNHI := NewTaiwanTaxCalculator(cfgWithoutNHI).CalculateDividendTax(dividend)

	wantWithNHI := 0.28 * dividend
	wantWithoutNHI := (0.28 - NHISurchargeRate) * dividend

	if math.Abs(withNHI-wantWithNHI) > 0.01 {
		t.Errorf("IncludeNHI=true: got %v, want %v", withNHI, wantWithNHI)
	}
	if math.Abs(withoutNHI-wantWithoutNHI) > 0.01 {
		t.Errorf("IncludeNHI=false: got %v, want %v", withoutNHI, wantWithoutNHI)
	}
	if withoutNHI >= withNHI {
		t.Errorf("IncludeNHI=false should produce strictly less tax than IncludeNHI=true (got %v vs %v)", withoutNHI, withNHI)
	}
}

// TestCalculatePositionTax_IncludeNHI verifies the gate is wired through the
// full position-level snapshot (DividendTax amount AND reported DividendTaxRate).
func TestCalculatePositionTax_IncludeNHI(t *testing.T) {
	pos := domain.Position{Symbol: "2330", Quantity: 1000, AverageCost: 500}
	sellPrice := 600.0
	dividend := 15000.0

	cfgWithoutNHI := domain.TaxConfig{
		DividendTaxRate:    0.28,
		TransactionTaxRate: 0.003,
		IncludeNHI:         false,
	}
	calc := NewTaiwanTaxCalculator(cfgWithoutNHI)

	snap := calc.CalculatePositionTax(pos, sellPrice, dividend)

	wantDivTax := (0.28 - NHISurchargeRate) * dividend
	if math.Abs(snap.DividendTax-wantDivTax) > 0.01 {
		t.Errorf("DividendTax = %v, want %v (NHI excluded)", snap.DividendTax, wantDivTax)
	}
	wantRate := 0.28 - NHISurchargeRate
	if math.Abs(snap.DividendTaxRate-wantRate) > 0.0001 {
		t.Errorf("DividendTaxRate = %v, want %v (snapshot should reflect effective rate)", snap.DividendTaxRate, wantRate)
	}
}

func TestCalculateTransactionTax(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	tests := []struct {
		name     string
		notional float64
		want     float64
	}{
		{"zero", 0, 0},
		{"negative", -50000, 0},
		{"typical", 100000, 300},
		{"large", 1000000, 3000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.CalculateTransactionTax(tt.notional)
			if got != tt.want {
				t.Errorf("CalculateTransactionTax(%v) = %v, want %v", tt.notional, got, tt.want)
			}
		})
	}
}

func TestCalculatePositionTax(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	pos := domain.Position{
		Symbol:      "2330",
		Quantity:    1000,
		AverageCost: 500,
	}

	t.Run("basic sell with dividend", func(t *testing.T) {
		sellPrice := 600.0
		dividend := 15000.0

		snap := calc.CalculatePositionTax(pos, sellPrice, dividend)

		if snap.Symbol != "2330" {
			t.Errorf("Symbol = %v, want 2330", snap.Symbol)
		}
		if snap.DividendTaxRate != 0.28 {
			t.Errorf("DividendTaxRate = %v, want 0.28", snap.DividendTaxRate)
		}
		if snap.TransactionTaxRate != 0.003 {
			t.Errorf("TransactionTaxRate = %v, want 0.003", snap.TransactionTaxRate)
		}

		wantDivTax := 15000.0 * 0.28
		if snap.DividendTax != wantDivTax {
			t.Errorf("DividendTax = %v, want %v", snap.DividendTax, wantDivTax)
		}

		sellNotional := 1000.0 * 600.0
		wantTxnTax := sellNotional * 0.003
		if snap.TransactionTax != wantTxnTax {
			t.Errorf("TransactionTax = %v, want %v", snap.TransactionTax, wantTxnTax)
		}

		wantTotal := wantDivTax + wantTxnTax
		if snap.TotalTax != wantTotal {
			t.Errorf("TotalTax = %v, want %v", snap.TotalTax, wantTotal)
		}

		unrealizedPnL := 1000.0 * (600.0 - 500.0)
		wantAfterTax := unrealizedPnL - wantTotal
		if snap.AfterTaxPnL != wantAfterTax {
			t.Errorf("AfterTaxPnL = %v, want %v", snap.AfterTaxPnL, wantAfterTax)
		}
	})

	t.Run("zero quantity returns empty snapshot", func(t *testing.T) {
		zeroPos := domain.Position{Symbol: "0000", Quantity: 0, AverageCost: 100}
		snap := calc.CalculatePositionTax(zeroPos, 200, 0)
		if snap.TotalTax != 0 {
			t.Errorf("TotalTax = %v, want 0", snap.TotalTax)
		}
	})

	t.Run("zero sell price returns empty snapshot", func(t *testing.T) {
		snap := calc.CalculatePositionTax(pos, 0, 0)
		if snap.TotalTax != 0 {
			t.Errorf("TotalTax = %v, want 0", snap.TotalTax)
		}
	})

	t.Run("no dividend", func(t *testing.T) {
		snap := calc.CalculatePositionTax(pos, 600, 0)
		if snap.DividendTax != 0 {
			t.Errorf("DividendTax = %v, want 0", snap.DividendTax)
		}
		if snap.TransactionTax == 0 {
			t.Error("TransactionTax should be non-zero for a sell")
		}
	})
}

func TestCalculatePortfolioTax(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	positions := []domain.Position{
		{Symbol: "2330", Quantity: 1000, AverageCost: 500, CurrentPrice: 550},
		{Symbol: "2454", Quantity: 500, AverageCost: 800, CurrentPrice: 850},
	}

	sellPrices := map[string]float64{
		"2330": 600,
		"2454": 900,
	}
	dividends := map[string]float64{
		"2330": 15000,
		"2454": 8000,
	}

	snapshots := calc.CalculatePortfolioTax(positions, sellPrices, dividends)

	if len(snapshots) != 2 {
		t.Fatalf("len(snapshots) = %d, want 2", len(snapshots))
	}

	snapBySymbol := make(map[string]domain.TaxSnapshot)
	for _, s := range snapshots {
		snapBySymbol[s.Symbol] = s
	}

	t.Run("aggregates correctly", func(t *testing.T) {
		var totalTax float64
		for _, s := range snapshots {
			totalTax += s.TotalTax
		}

		want2330DivTax := 15000.0 * 0.28
		want2330TxnTax := 1000.0 * 600.0 * 0.003
		want2454DivTax := 8000.0 * 0.28
		want2454TxnTax := 500.0 * 900.0 * 0.003
		wantTotal := want2330DivTax + want2330TxnTax + want2454DivTax + want2454TxnTax

		if math.Abs(totalTax-wantTotal) > 0.01 {
			t.Errorf("total portfolio tax = %v, want %v", totalTax, wantTotal)
		}
	})

	t.Run("falls back to CurrentPrice when sellPrice missing", func(t *testing.T) {
		partialPrices := map[string]float64{"2330": 600}
		snapshots := calc.CalculatePortfolioTax(positions, partialPrices, dividends)

		snap2454 := snapshots[1]
		wantTxnTax := 500.0 * 850.0 * 0.003
		if math.Abs(snap2454.TransactionTax-wantTxnTax) > 0.01 {
			t.Errorf("2454 TransactionTax = %v, want %v (should use CurrentPrice 850)",
				snap2454.TransactionTax, wantTxnTax)
		}
	})

	t.Run("empty positions returns empty slice", func(t *testing.T) {
		snapshots := calc.CalculatePortfolioTax(nil, sellPrices, dividends)
		if len(snapshots) != 0 {
			t.Errorf("expected empty slice, got %d items", len(snapshots))
		}
	})
}

func TestCalculatePositionTaxStrict_NormalCase(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	pos := domain.Position{
		Symbol:      "2330",
		Quantity:    1000,
		AverageCost: 50,
	}

	snap, err := calc.CalculatePositionTaxStrict(pos, 60, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// DividendTax = 1000 * 0.28 = 280
	// TransactionTax = 1000 * 60 * 0.003 = 180
	// TotalTax = 280 + 180 = 460
	// AfterTaxPnL = 1000*(60-50) - 460 = 10000 - 460 = 9540
	if snap.TotalTax != 460 {
		t.Errorf("TotalTax = %v, want 460", snap.TotalTax)
	}
	if snap.AfterTaxPnL != 9540 {
		t.Errorf("AfterTaxPnL = %v, want 9540", snap.AfterTaxPnL)
	}
	if snap.DividendTax != 280 {
		t.Errorf("DividendTax = %v, want 280", snap.DividendTax)
	}
	if snap.TransactionTax != 180 {
		t.Errorf("TransactionTax = %v, want 180", snap.TransactionTax)
	}
}

func TestCalculatePositionTaxStrict_ZeroQuantity(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	pos := domain.Position{Symbol: "2330", Quantity: 0, AverageCost: 50}
	snap, err := calc.CalculatePositionTaxStrict(pos, 60, 0)

	if err == nil {
		t.Fatal("expected error for zero quantity, got nil")
	}
	if !errors.Is(err, ErrInvalidPositionTaxInput) {
		t.Errorf("error should wrap ErrInvalidPositionTaxInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "quantity") {
		t.Errorf("error message should contain 'quantity', got %q", err.Error())
	}
	if snap != (domain.TaxSnapshot{}) {
		t.Errorf("expected zero TaxSnapshot on error, got %+v", snap)
	}
}

func TestCalculatePositionTaxStrict_ZeroPrice(t *testing.T) {
	calc := NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	pos := domain.Position{Symbol: "2330", Quantity: 1000, AverageCost: 50}
	snap, err := calc.CalculatePositionTaxStrict(pos, 0, 0)

	if err == nil {
		t.Fatal("expected error for zero sell price, got nil")
	}
	if !errors.Is(err, ErrInvalidPositionTaxInput) {
		t.Errorf("error should wrap ErrInvalidPositionTaxInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "price") {
		t.Errorf("error message should contain 'price', got %q", err.Error())
	}
	if snap != (domain.TaxSnapshot{}) {
		t.Errorf("expected zero TaxSnapshot on error, got %+v", snap)
	}
}

func TestConfig(t *testing.T) {
	cfg := domain.TaxConfig{
		DividendTaxRate:    0.20,
		TransactionTaxRate: 0.001,
		IncludeNHI:         false,
	}
	calc := NewTaiwanTaxCalculator(cfg)

	got := calc.Config()
	if got.DividendTaxRate != 0.20 {
		t.Errorf("Config DividendTaxRate = %v, want 0.20", got.DividendTaxRate)
	}
	if got.TransactionTaxRate != 0.001 {
		t.Errorf("Config TransactionTaxRate = %v, want 0.001", got.TransactionTaxRate)
	}
}
