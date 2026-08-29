package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestEquityCurvePoint_JSONMarshaling(t *testing.T) {
	point := EquityCurvePoint{
		Label:         "session-20260413-daily",
		Value:         1000000.0,
		Currency:      "TWD",
		AfterTaxValue: 995000.0,
		TaxPaid:       5000.0,
	}

	bytes, err := json.Marshal(point)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	for _, key := range []string{"label", "value", "currency", "after_tax_value", "tax_paid"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("expected JSON key %q not found in %v", key, decoded)
		}
	}

	if decoded["label"] != "session-20260413-daily" {
		t.Errorf("label = %v, want session-20260413-daily", decoded["label"])
	}
	if decoded["value"] != 1000000.0 {
		t.Errorf("value = %v, want 1000000.0", decoded["value"])
	}
	if decoded["currency"] != "TWD" {
		t.Errorf("currency = %v, want TWD", decoded["currency"])
	}
	if decoded["after_tax_value"] != 995000.0 {
		t.Errorf("after_tax_value = %v, want 995000.0", decoded["after_tax_value"])
	}
	if decoded["tax_paid"] != 5000.0 {
		t.Errorf("tax_paid = %v, want 5000.0", decoded["tax_paid"])
	}
}

func TestEquityCurvePoint_Omitempty(t *testing.T) {
	point := EquityCurvePoint{
		Label: "session-20260413-daily",
		Value: 1000000.0,
	}

	bytes, err := json.Marshal(point)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if _, ok := decoded["currency"]; ok {
		t.Error("currency should be omitted but was present")
	}
	if _, ok := decoded["after_tax_value"]; ok {
		t.Error("after_tax_value should be omitted but was present")
	}
	if _, ok := decoded["tax_paid"]; ok {
		t.Error("tax_paid should be omitted but was present")
	}
}

func TestEquityCurvePoint_BackwardCompatibility(t *testing.T) {
	oldJSON := `{"label":"session-20260413-daily","value":1000000.0}`

	var point EquityCurvePoint
	if err := json.Unmarshal([]byte(oldJSON), &point); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if point.Label != "session-20260413-daily" {
		t.Errorf("Label = %v, want session-20260413-daily", point.Label)
	}
	if point.Value != 1000000.0 {
		t.Errorf("Value = %v, want 1000000.0", point.Value)
	}
	if point.Currency != "" {
		t.Errorf("Currency = %v, want empty string", point.Currency)
	}
	if point.AfterTaxValue != 0 {
		t.Errorf("AfterTaxValue = %v, want 0", point.AfterTaxValue)
	}
	if point.TaxPaid != 0 {
		t.Errorf("TaxPaid = %v, want 0", point.TaxPaid)
	}
}

func TestBuildEquityCurve_TaxFieldsPopulated(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	session1 := domain.SessionSummary{
		SessionID:      "session-20260413-daily",
		PortfolioValue: 1000000.0,
		TotalTaxPaid:   5000.0,
		Regime:         domain.RegimeNeutral,
	}
	session2 := domain.SessionSummary{
		SessionID:      "session-20260414-daily",
		PortfolioValue: 1020000.0,
		TotalTaxPaid:   6000.0,
		Regime:         domain.RegimeNeutral,
	}

	summary1Path := filepath.Join(sessionsDir, "session-20260413-daily")
	if err := os.MkdirAll(summary1Path, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes1, _ := json.Marshal(session1)
	if err := os.WriteFile(filepath.Join(summary1Path, "summary.json"), bytes1, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	summary2Path := filepath.Join(sessionsDir, "session-20260414-daily")
	if err := os.MkdirAll(summary2Path, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes2, _ := json.Marshal(session2)
	if err := os.WriteFile(filepath.Join(summary2Path, "summary.json"), bytes2, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	svc := NewLiveService(tmpDir, tmpDir)
	curve := svc.buildEquityCurve()

	if len(curve) != 2 {
		t.Fatalf("len(curve) = %d, want 2", len(curve))
	}

	if curve[0].Label != "session-20260413-daily" {
		t.Errorf("curve[0].Label = %v, want session-20260413-daily", curve[0].Label)
	}
	if curve[0].Value != 1000000.0 {
		t.Errorf("curve[0].Value = %v, want 1000000.0", curve[0].Value)
	}
	if curve[0].Currency != "TWD" {
		t.Errorf("curve[0].Currency = %v, want TWD", curve[0].Currency)
	}
	if curve[0].AfterTaxValue != 995000.0 {
		t.Errorf("curve[0].AfterTaxValue = %v, want 995000.0", curve[0].AfterTaxValue)
	}
	if curve[0].TaxPaid != 5000.0 {
		t.Errorf("curve[0].TaxPaid = %v, want 5000.0", curve[0].TaxPaid)
	}

	if curve[1].Label != "session-20260414-daily" {
		t.Errorf("curve[1].Label = %v, want session-20260414-daily", curve[1].Label)
	}
	if curve[1].Value != 1020000.0 {
		t.Errorf("curve[1].Value = %v, want 1020000.0", curve[1].Value)
	}
	if curve[1].AfterTaxValue != 1014000.0 {
		t.Errorf("curve[1].AfterTaxValue = %v, want 1014000.0", curve[1].AfterTaxValue)
	}
	if curve[1].TaxPaid != 6000.0 {
		t.Errorf("curve[1].TaxPaid = %v, want 6000.0", curve[1].TaxPaid)
	}
}

func TestBuildEquityCurve_ZeroTaxPaid(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	session := domain.SessionSummary{
		SessionID:      "session-20260415-daily",
		PortfolioValue: 1000000.0,
		TotalTaxPaid:   0.0,
		Regime:         domain.RegimeNeutral,
	}

	summaryPath := filepath.Join(sessionsDir, "session-20260415-daily")
	if err := os.MkdirAll(summaryPath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes, _ := json.Marshal(session)
	if err := os.WriteFile(filepath.Join(summaryPath, "summary.json"), bytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	svc := NewLiveService(tmpDir, tmpDir)
	curve := svc.buildEquityCurve()

	if len(curve) != 1 {
		t.Fatalf("len(curve) = %d, want 1", len(curve))
	}
	if curve[0].AfterTaxValue != 1000000.0 {
		t.Errorf("curve[0].AfterTaxValue = %v, want 1000000.0", curve[0].AfterTaxValue)
	}
	if curve[0].TaxPaid != 0.0 {
		t.Errorf("curve[0].TaxPaid = %v, want 0.0", curve[0].TaxPaid)
	}
}

func TestBuildEquityCurve_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	svc := NewLiveService(tmpDir, tmpDir)
	curve := svc.buildEquityCurve()

	if curve != nil {
		t.Errorf("buildEquityCurve() = %v, want nil for empty directory", curve)
	}
}

func TestEquityCurvePoint_NewFields(t *testing.T) {
	point := EquityCurvePoint{
		Label:         "test-session",
		Value:         500000.0,
		Currency:      "TWD",
		AfterTaxValue: 495000.0,
		TaxPaid:       5000.0,
	}

	if point.Currency != "TWD" {
		t.Errorf("Currency = %v, want TWD", point.Currency)
	}
	if point.AfterTaxValue != 495000.0 {
		t.Errorf("AfterTaxValue = %v, want 495000.0", point.AfterTaxValue)
	}
	if point.TaxPaid != 5000.0 {
		t.Errorf("TaxPaid = %v, want 5000.0", point.TaxPaid)
	}
}

func TestResolveSymbolName_KnownSymbol(t *testing.T) {
	name := resolveSymbolName("2330.TW")
	if name != "台積電" {
		t.Errorf("expected 台積電, got %q", name)
	}
}

func TestResolveSymbolName_KnownSymbolWithoutTW(t *testing.T) {
	name := resolveSymbolName("2330")
	if name != "台積電" {
		t.Errorf("expected 台積電, got %q", name)
	}
}

func TestResolveSymbolName_UnknownSymbol(t *testing.T) {
	name := resolveSymbolName("9999.TW")
	if name != "9999.TW" {
		t.Errorf("expected 9999.TW (unchanged), got %q", name)
	}
}

func TestResolveSymbolName_UnknownWithoutTW(t *testing.T) {
	name := resolveSymbolName("XXXX")
	if name != "XXXX" {
		t.Errorf("expected XXXX (unchanged), got %q", name)
	}
}

func TestNewLiveService(t *testing.T) {
	svc := NewLiveService("/workdir", "/ledger")
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.WorkDir != "/workdir" {
		t.Errorf("expected WorkDir /workdir, got %q", svc.WorkDir)
	}
	if svc.LedgerDir != "/ledger" {
		t.Errorf("expected LedgerDir /ledger, got %q", svc.LedgerDir)
	}
}

func TestLiveService_LoadLiveStatus(t *testing.T) {
	svc := NewLiveService(t.TempDir(), t.TempDir())
	status := svc.LoadLiveStatus()
	if status.CircuitBreaker.State == "" {
		t.Error("expected CircuitBreaker state to be set")
	}
	_ = status.Timestamp
}

func TestBuildSymbolSectorMap_NilClassifier(t *testing.T) {
	svc := NewLiveService(t.TempDir(), t.TempDir())
	m := svc.buildSymbolSectorMap()
	if m == nil {
		t.Error("expected non-nil map")
	}
	if len(m) != 0 {
		t.Errorf("expected 0 entries with nil classifier, got %d", len(m))
	}
}

func TestGetSectorForSymbol(t *testing.T) {
	m := map[string]string{"2330": "semiconductor", "2317": "semiconductor"}
	if got := getSectorForSymbol("2330", m); got != "semiconductor" {
		t.Errorf("expected semiconductor, got %q", got)
	}
	if got := getSectorForSymbol("unknown", m); got != "other" {
		t.Errorf("expected other, got %q", got)
	}
}

func TestCalculateMaxDrawdownFromEquityCurve(t *testing.T) {
	tests := []struct {
		name     string
		curve    []EquityCurvePoint
		want     float64
		wantZero bool
	}{
		{
			name:     "empty curve",
			curve:    []EquityCurvePoint{},
			wantZero: true,
		},
		{
			name:     "monotonically increasing",
			curve:    []EquityCurvePoint{{Value: 100}, {Value: 110}, {Value: 120}},
			wantZero: true,
		},
		{
			name:  "single peak then trough",
			curve: []EquityCurvePoint{{Value: 100}, {Value: 120}, {Value: 90}},
			want:  0.25, // (120 - 90) / 120
		},
		{
			name:  "new peak after first drawdown",
			curve: []EquityCurvePoint{{Value: 100}, {Value: 80}, {Value: 130}, {Value: 100}},
			want:  30.0 / 130.0, // second decline (130-100)/130 is larger than first drawdown
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMaxDrawdownFromEquityCurve(tt.curve)
			if tt.wantZero {
				if got != 0 {
					t.Errorf("calculateMaxDrawdownFromEquityCurve() = %v, want 0", got)
				}
				return
			}
			const eps = 1e-9
			if diff := got - tt.want; diff < -eps || diff > eps {
				t.Errorf("calculateMaxDrawdownFromEquityCurve() = %v, want %v", got, tt.want)
			}
		})
	}
}
