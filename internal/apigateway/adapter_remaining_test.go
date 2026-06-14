package apigateway

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// BDI Adapter
// ---------------------------------------------------------------------------

func TestBDIChannelAdapter_Metadata(t *testing.T) {
	a := &BDIChannelAdapter{limiter: rate.NewLimiter(rate.Every(5*time.Second), 1)}
	m := a.Metadata()
	if m.ChannelID != "bdi" {
		t.Errorf("ChannelID = %q, want bdi", m.ChannelID)
	}
	if m.Country != "全球" {
		t.Errorf("Country = %q, want 全球", m.Country)
	}
}

func TestBDIChannelAdapter_RateLimit(t *testing.T) {
	a := NewBDIChannelAdapter(nil)
	if a.limiter == nil {
		t.Fatal("limiter is nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit returned nil")
	}
	if limiter != a.limiter {
		t.Error("RateLimit did not return the adapter's limiter")
	}
}

// ---------------------------------------------------------------------------
// DRAM Spot Price Adapter
// ---------------------------------------------------------------------------

func TestDRAMSpotPriceChannelAdapter_Metadata(t *testing.T) {
	a := &DRAMSpotPriceChannelAdapter{limiter: rate.NewLimiter(rate.Every(5*time.Second), 1)}
	m := a.Metadata()
	if m.ChannelID != "dram_spot_price" {
		t.Errorf("ChannelID = %q, want dram_spot_price", m.ChannelID)
	}
	if m.Country != "美國" {
		t.Errorf("Country = %q, want 美國", m.Country)
	}
}

func TestDRAMSpotPriceChannelAdapter_RateLimit(t *testing.T) {
	a := NewDRAMSpotPriceChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// Taifex Adapter
// ---------------------------------------------------------------------------

func TestTaifexChannelAdapter_Metadata(t *testing.T) {
	a := &TaifexChannelAdapter{limiter: rate.NewLimiter(rate.Every(5*time.Second), 1)}
	m := a.Metadata()
	if m.ChannelID != "taifex_daily" {
		t.Errorf("ChannelID = %q, want taifex_daily", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
}

func TestTaifexChannelAdapter_RateLimit(t *testing.T) {
	a := NewTaifexChannelAdapter()
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// TSM ADR Adapter
// ---------------------------------------------------------------------------

func TestTSMADRChannelAdapter_Constructor(t *testing.T) {
	a := NewTSMADRChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTSMADRChannelAdapter returned nil")
	}
}

func TestTSMADRChannelAdapter_Metadata(t *testing.T) {
	a := NewTSMADRChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "tsm_adr" {
		t.Errorf("ChannelID = %q, want tsm_adr", m.ChannelID)
	}
}

func TestTSMADRChannelAdapter_RateLimit(t *testing.T) {
	a := NewTSMADRChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// TWSE ETF Adapter
// ---------------------------------------------------------------------------

func TestTWSEETFChannelAdapter_Metadata(t *testing.T) {
	a := &TWSEETFChannelAdapter{limiter: rate.NewLimiter(rate.Every(1*time.Second), 1)}
	m := a.Metadata()
	if m.ChannelID != "twse_etf" {
		t.Errorf("ChannelID = %q, want twse_etf", m.ChannelID)
	}
}

func TestTWSEETFChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSEETFChannelAdapter()
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// TWSE Odd Lot Adapter
// ---------------------------------------------------------------------------

func TestTWSEOddLotChannelAdapter_Metadata(t *testing.T) {
	a := &TWSEOddLotChannelAdapter{limiter: rate.NewLimiter(rate.Every(1*time.Second), 1)}
	m := a.Metadata()
	if m.ChannelID != "twse_oddlot" {
		t.Errorf("ChannelID = %q, want twse_oddlot", m.ChannelID)
	}
}

func TestTWSEOddLotChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSEOddLotChannelAdapter()
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// TWSE Sector Index Adapter
// ---------------------------------------------------------------------------

func TestTWSESectorIndexChannelAdapter_Metadata(t *testing.T) {
	a := NewTWSESectorIndexChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "twse_sector_index" {
		t.Errorf("ChannelID = %q, want twse_sector_index", m.ChannelID)
	}
}

func TestTWSESectorIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSESectorIndexChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// US Index Adapters (S&P 500, Nasdaq, Dow Jones)
// ---------------------------------------------------------------------------

func TestUSSPXIndexChannelAdapter_Constructor(t *testing.T) {
	a := NewUSSPXIndexChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSSPXIndexChannelAdapter returned nil")
	}
}

func TestUSSPXIndexChannelAdapter_Metadata(t *testing.T) {
	a := NewUSSPXIndexChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "us_spx" {
		t.Errorf("ChannelID = %q, want us_spx", m.ChannelID)
	}
}

func TestUSSPXIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSSPXIndexChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

func TestUSNDXIndexChannelAdapter_Constructor(t *testing.T) {
	a := NewUSNDXIndexChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSNDXIndexChannelAdapter returned nil")
	}
}

func TestUSNDXIndexChannelAdapter_Metadata(t *testing.T) {
	a := NewUSNDXIndexChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "us_ndx" {
		t.Errorf("ChannelID = %q, want us_ndx", m.ChannelID)
	}
}

func TestUSNDXIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSNDXIndexChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

func TestUSDJIIndexChannelAdapter_Constructor(t *testing.T) {
	a := NewUSDJIIndexChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSDJIIndexChannelAdapter returned nil")
	}
}

func TestUSDJIIndexChannelAdapter_Metadata(t *testing.T) {
	a := NewUSDJIIndexChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "us_dji" {
		t.Errorf("ChannelID = %q, want us_dji", m.ChannelID)
	}
}

func TestUSDJIIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSDJIIndexChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// US Tech Adapters (NVDA, AAPL, MSFT)
// ---------------------------------------------------------------------------

func TestUSNVDAChannelAdapter_Constructor(t *testing.T) {
	a := NewUSNVDAChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSNVDAChannelAdapter returned nil")
	}
}

func TestUSNVDAChannelAdapter_Metadata(t *testing.T) {
	a := NewUSNVDAChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "us_nvda" {
		t.Errorf("ChannelID = %q, want us_nvda", m.ChannelID)
	}
}

func TestUSNVDAChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSNVDAChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

func TestUSAAPLChannelAdapter_Constructor(t *testing.T) {
	a := NewUSAAPLChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSAAPLChannelAdapter returned nil")
	}
}

func TestUSAAPLChannelAdapter_Metadata(t *testing.T) {
	a := NewUSAAPLChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "us_aapl" {
		t.Errorf("ChannelID = %q, want us_aapl", m.ChannelID)
	}
}

func TestUSAAPLChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSAAPLChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

func TestUSMSFTChannelAdapter_Constructor(t *testing.T) {
	a := NewUSMSFTChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSMSFTChannelAdapter returned nil")
	}
}

func TestUSMSFTChannelAdapter_Metadata(t *testing.T) {
	a := NewUSMSFTChannelAdapter(nil)
	m := a.Metadata()
	if m.ChannelID != "us_msft" {
		t.Errorf("ChannelID = %q, want us_msft", m.ChannelID)
	}
}

func TestUSMSFTChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSMSFTChannelAdapter(nil)
	l := a.RateLimit()
	if l == nil {
		t.Fatal("RateLimit returned nil")
	}
}

// ---------------------------------------------------------------------------
// Context-cancelled Fetch tests (cover limiter.Wait error path)
// ---------------------------------------------------------------------------

func TestDRAMSpotPriceChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewDRAMSpotPriceChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestTaifexChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewTaifexChannelAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestTWSEETFChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewTWSEETFChannelAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestTWSEOddLotChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewTWSEOddLotChannelAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestTWSESectorIndexChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewTWSESectorIndexChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestTSMADRChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewTSMADRChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestUSSPXIndexChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewUSSPXIndexChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestUSNDXIndexChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewUSNDXIndexChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestUSDJIIndexChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewUSDJIIndexChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestUSNVDAChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewUSNVDAChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestUSAAPLChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewUSAAPLChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestUSMSFTChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewUSMSFTChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}

func TestExchangeRateChannelAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewExchangeRateChannelAdapter(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx)
	if err == nil {
		t.Error("Fetch with cancelled context should return error")
	}
}
