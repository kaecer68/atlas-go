package apigateway

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// TSM ADR Adapter
// ---------------------------------------------------------------------------

func TestTSMADRChannelAdapter_Constructor(t *testing.T) {
	a := NewTSMADRChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTSMADRChannelAdapter returned nil")
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
