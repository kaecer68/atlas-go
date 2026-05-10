package tax

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestHandleTaxSnapshot_WithRealFinMindProvider(t *testing.T) {
	apiKey := os.Getenv("FINMIND_API_KEY")
	if apiKey == "" {
		t.Skip("FINMIND_API_KEY not set, skipping integration test")
	}

	client := marketdata.NewFinMindClient(apiKey)
	provider := marketdata.NewFinMindDividendProvider(client, "../../../testdata/dividend-cache")

	h := NewHandlers("./testdata", provider)

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
	t.Logf("Note: %s", note)

	totalDividendTax, _ := data["total_dividend_tax"].(float64)
	t.Logf("Total Dividend Tax: %.2f", totalDividendTax)

	snapshots, ok := data["snapshots"].([]domain.TaxSnapshot)
	if ok && len(snapshots) > 0 {
		for _, snap := range snapshots {
			t.Logf("  %s: dividend_tax=%.2f, transaction_tax=%.2f, total_tax=%.2f",
				snap.Symbol, snap.DividendTax, snap.TransactionTax, snap.TotalTax)
		}
	}
}
