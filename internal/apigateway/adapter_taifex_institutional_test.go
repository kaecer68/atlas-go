package apigateway

import (
	"context"
	"strings"
	"testing"
)

func TestTaifexInstitutionalAdapter_Metadata(t *testing.T) {
	a := NewTaifexInstitutionalAdapter()
	m := a.Metadata()
	if m.ChannelID != "taifex_institutional" {
		t.Errorf("channel id=%s, want taifex_institutional", m.ChannelID)
	}
	if !m.HasLimiter {
		t.Error("expected HasLimiter=true")
	}
	if !strings.Contains(m.Path, "MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate") {
		t.Errorf("path=%s, missing endpoint name", m.Path)
	}
}

func TestTaifexInstitutionalAdapter_RateLimit(t *testing.T) {
	a := NewTaifexInstitutionalAdapter()
	if a.RateLimit() == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

// TestTaifexInstitutionalAdapter_Fetch_ContextCancelled — post-BK-10 the
// limiter wait is decoupled from the caller ctx, so a canceled ctx must
// either error OR return a stale/fallback result, never a fresh one.
func TestTaifexInstitutionalAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewTaifexInstitutionalAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := a.Fetch(ctx)
	if err == nil && res != nil && !res.Stale && !res.Fallback {
		t.Error("Fetch with cancelled context must not return a fresh result")
	}
}
