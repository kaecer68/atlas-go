package tax

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type mockDividendProvider struct {
	dividend float64
}

func (m *mockDividendProvider) GetLatestDividend(ctx context.Context, symbol string) (*domain.DividendRecord, error) {
	return &domain.DividendRecord{
		Symbol:       symbol,
		CashDividend: m.dividend,
	}, nil
}

func TestHandleTaxSnapshot_WithDividendProvider(t *testing.T) {
	h := NewHandlers("./testdata", &mockDividendProvider{dividend: 2.5})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/tax-snapshot", nil)
	status, body := h.HandleTaxSnapshot(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	data, ok := body.(map[string]any)
	if !ok {
		t.Fatal("expected map body")
	}

	note, _ := data["note"].(string)
	if note == "" {
		t.Fatal("expected note")
	}

	t.Logf("Note: %s", note)
	t.Logf("Response: %+v", data)
}

func TestHandleTaxSnapshot_WithoutDividendProvider(t *testing.T) {
	h := NewHandlers("./testdata", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/tax-snapshot", nil)
	status, body := h.HandleTaxSnapshot(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	data, ok := body.(map[string]any)
	if !ok {
		t.Fatal("expected map body")
	}

	note, _ := data["note"].(string)
	expectedNote := "tax snapshots computed from live positions using TaiwanTaxCalculator; dividend tax is 0 because dividend data provider is not configured"
	if note != expectedNote {
		t.Fatalf("expected note %q, got %q", expectedNote, note)
	}
}
