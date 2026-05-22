package risk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	riskhandler "github.com/kaecer68/atlas-go/internal/monitoring/api/risk"
)

func TestHandleCorrelationMatrix(t *testing.T) {
	h := riskhandler.NewHandlers("testdata/ledger")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/correlation-matrix", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp riskhandler.CorrelationMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Symbols) == 0 {
		t.Fatal("expected non-empty symbols")
	}
	if len(resp.Symbols) != len(resp.Labels) {
		t.Fatalf("symbols and labels length mismatch: %d vs %d", len(resp.Symbols), len(resp.Labels))
	}

	n := len(resp.Symbols)
	if len(resp.Matrix) != n {
		t.Fatalf("matrix rows mismatch: expected %d, got %d", n, len(resp.Matrix))
	}
	for i, row := range resp.Matrix {
		if len(row) != n {
			t.Fatalf("matrix row %d length mismatch: expected %d, got %d", i, n, len(row))
		}
		if row[i] != 1.0 {
			t.Errorf("diagonal %d should be 1.0, got %f", i, row[i])
		}
	}
}

func TestHandleCorrelationMatrix_CustomMatrix(t *testing.T) {
	cm := industry.NewCorrelationMatrix(30)
	cm.UpdateCorrelation("A", "B", 0.75)
	cm.UpdateCorrelation("A", "C", -0.30)

	h := riskhandler.NewHandlers("testdata/ledger").WithCorrelationMatrix(cm)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/correlation-matrix", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp riskhandler.CorrelationMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(resp.Symbols))
	}

	findIdx := func(sym string) int {
		for i, s := range resp.Symbols {
			if s == sym {
				return i
			}
		}
		return -1
	}

	aIdx := findIdx("A")
	bIdx := findIdx("B")
	if aIdx < 0 || bIdx < 0 {
		t.Fatalf("symbols not found: A=%d, B=%d", aIdx, bIdx)
	}

	if resp.Matrix[aIdx][bIdx] != 0.75 {
		t.Errorf("expected A-B correlation 0.75, got %f", resp.Matrix[aIdx][bIdx])
	}
	if resp.Matrix[bIdx][aIdx] != 0.75 {
		t.Errorf("expected B-A correlation 0.75, got %f", resp.Matrix[bIdx][aIdx])
	}
}

func TestHandleCorrelationMatrix_MethodNotAllowed(t *testing.T) {
	h := riskhandler.NewHandlers("testdata/ledger")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/correlation-matrix", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleCorrelationMatrix_Symmetric(t *testing.T) {
	h := riskhandler.NewHandlers("testdata/ledger")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/correlation-matrix", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp riskhandler.CorrelationMatrixResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	for i := 0; i < len(resp.Matrix); i++ {
		for j := 0; j < len(resp.Matrix); j++ {
			got := resp.Matrix[i][j]
			want := resp.Matrix[j][i]
			if got != want {
				t.Errorf("matrix not symmetric at [%d][%d]: %f vs %f", i, j, got, want)
			}
		}
	}
}
