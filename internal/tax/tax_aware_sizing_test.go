package tax

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestTaxAwareSizerReducesSize(t *testing.T) {
	base := portfolio.NewSizer()
	cfg := domain.DefaultTaiwanTaxConfig()
	sizer := NewTaxAwareSizer(base, cfg)

	capital := 1000000.0
	price := 500.0

	taxAwareShares := sizer.SizePosition("2330", capital, price)

	// Base sizer without tax would allow: 1000000 / 500 = 2000 shares
	// Tax-aware: effectiveCapital = 1000000 / 1.003 ≈ 997009
	// shares = 997009 / 500 ≈ 1994, rounded to 1000-lot = 1000
	if taxAwareShares <= 0 {
		t.Fatal("expected positive share count")
	}

	// Verify tax-aware size is less than or equal to naive size
	naiveShares := int(capital/price/1000.0) * 1000
	if taxAwareShares > naiveShares {
		t.Errorf("tax-aware size %d should not exceed naive size %d", taxAwareShares, naiveShares)
	}
}

func TestTaxAwareSizerEdgeCases(t *testing.T) {
	base := portfolio.NewSizer()
	cfg := domain.DefaultTaiwanTaxConfig()
	sizer := NewTaxAwareSizer(base, cfg)

	t.Run("zero price returns zero", func(t *testing.T) {
		if got := sizer.SizePosition("2330", 1000000, 0); got != 0 {
			t.Errorf("expected 0 for zero price, got %d", got)
		}
	})

	t.Run("negative price returns zero", func(t *testing.T) {
		if got := sizer.SizePosition("2330", 1000000, -100); got != 0 {
			t.Errorf("expected 0 for negative price, got %d", got)
		}
	})

	t.Run("zero capital returns zero", func(t *testing.T) {
		if got := sizer.SizePosition("2330", 0, 500); got != 0 {
			t.Errorf("expected 0 for zero capital, got %d", got)
		}
	})

	t.Run("negative capital returns zero", func(t *testing.T) {
		if got := sizer.SizePosition("2330", -1000, 500); got != 0 {
			t.Errorf("expected 0 for negative capital, got %d", got)
		}
	})
}

func TestTaxAwareSizerLotRounding(t *testing.T) {
	base := portfolio.NewSizer()
	cfg := domain.DefaultTaiwanTaxConfig()
	sizer := NewTaxAwareSizer(base, cfg)

	// 500500 capital at 500 price: effectiveCapital = 500500/1.003 ≈ 499003
	// shares = 499003/500 ≈ 998, rounded to 1000-lot = 0
	shares := sizer.SizePosition("2330", 500500, 500)
	if shares%1000 != 0 {
		t.Errorf("shares %d should be a multiple of 1000", shares)
	}

	// 1003000 capital at 500 price: effectiveCapital = 1003000/1.003 = 1000000
	// shares = 1000000/500 = 2000, rounded to 1000-lot = 2000
	shares = sizer.SizePosition("2330", 1003000, 500)
	if shares != 2000 {
		t.Errorf("expected 2000 shares, got %d", shares)
	}
}

func TestTaxAwareSizerBaseSizer(t *testing.T) {
	base := portfolio.NewSizer()
	cfg := domain.DefaultTaiwanTaxConfig()
	sizer := NewTaxAwareSizer(base, cfg)

	if sizer.BaseSizer() != base {
		t.Error("BaseSizer should return the original base sizer")
	}
}

func TestTaxAwareSizerWithCustomTaxRate(t *testing.T) {
	base := portfolio.NewSizer()
	cfg := domain.TaxConfig{
		DividendTaxRate:    0.28,
		TransactionTaxRate: 0.006, // doubled rate
		IncludeNHI:         true,
	}
	sizer := NewTaxAwareSizer(base, cfg)

	capital := 1003000.0
	price := 500.0

	// effectiveCapital = 1003000 / 1.006 ≈ 997018
	// shares = 997018 / 500 ≈ 1994, rounded to 1000-lot = 1000
	shares := sizer.SizePosition("2330", capital, price)

	// With higher tax rate, should get fewer shares than default rate
	defaultSizer := NewTaxAwareSizer(base, domain.DefaultTaiwanTaxConfig())
	defaultShares := defaultSizer.SizePosition("2330", capital, price)

	if shares > defaultShares {
		t.Errorf("higher tax rate should produce fewer shares: got %d vs default %d", shares, defaultShares)
	}
}
