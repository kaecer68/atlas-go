package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
)

func TestExportStatisticsProvider_Name(t *testing.T) {
	p := NewExportStatisticsProvider(constants.StateExport)
	if p.Name() != "export_statistics" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestExportStatisticsProvider_FetchSnapshot_RealAPI(t *testing.T) {
	p := NewExportStatisticsProvider(constants.StateExport)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Logf("ExportStatisticsProvider.FetchSnapshot returned error (may be expected in CI): %v", err)
		return
	}

	if snap.ExportElectronics.Symbol != "TW_EXPORT_ELECTRONICS" {
		t.Fatalf("unexpected symbol: %s", snap.ExportElectronics.Symbol)
	}
	if snap.RecordedAt == 0 {
		t.Fatalf("RecordedAt should be set")
	}
}

func TestExportStatisticsProvider_FetchSnapshot_Decommissioned(t *testing.T) {
	p := ExportStatisticsProviderWithClient(http.DefaultClient, t.TempDir())
	p.baseURL = "https://fake-test-server"
	p.client.Timeout = 5 * time.Second

	ctx := context.Background()
	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error since FAS210 is decommissioned")
	}
}

func TestParseCustomsCSV_ValidData(t *testing.T) {
	input := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","01","50,000,000","45,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
"113","12","48,000,000","43,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`

	records, err := parseCustomsCSV([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Check first record (newest)
	if records[0].Year != 114 || records[0].Month != 1 {
		t.Errorf("expected 114/01, got %d/%d", records[0].Year, records[0].Month)
	}
	// 50,000,000 / 1000 = 50,000 (million USD)
	if records[0].ExportTotal != 50000 {
		t.Errorf("expected export total 50000, got %f", records[0].ExportTotal)
	}
	if records[0].ImportTotal != 45000 {
		t.Errorf("expected import total 45000, got %f", records[0].ImportTotal)
	}
	if records[0].TradeBalance != 5000 {
		t.Errorf("expected trade balance 5000, got %f", records[0].TradeBalance)
	}

	// Check second record
	if records[1].Year != 113 || records[1].Month != 12 {
		t.Errorf("expected 113/12, got %d/%d", records[1].Year, records[1].Month)
	}
}

func TestParseCustomsCSV_HeaderOnly(t *testing.T) {
	input := `年月,出口總值,進口總值,出超`
	records, err := parseCustomsCSV([]byte(input))
	if err == nil {
		t.Fatal("expected error for header-only CSV")
	}
	if records != nil {
		t.Errorf("expected nil records, got %v", records)
	}
}

func TestParseCustomsCSV_MalformedRows(t *testing.T) {
	input := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","01","50,000,000","45,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
bad_row
"113","invalid_month","48,000,000","43,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`

	records, err := parseCustomsCSV([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip bad rows and keep valid ones
	if len(records) != 1 {
		t.Errorf("expected 1 valid record, got %d", len(records))
	}
	if records[0].Year != 114 {
		t.Errorf("expected year 114, got %d", records[0].Year)
	}
}

func TestParseCustomsCSV_UTF8BOM(t *testing.T) {
	data := []byte{0xef, 0xbb, 0xbf} // UTF-8 BOM
	data = append(data, []byte(`年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","01","50,000,000","45,000,000","5,000,000","10.0","5.0","8.0","5,000,000"`)...)

	records, err := parseCustomsCSV(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Year != 114 {
		t.Errorf("expected year 114, got %d", records[0].Year)
	}
}

func TestExportStatisticsProvider_FetchSnapshot_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/csv" {
			t.Errorf("expected Accept: text/csv, got %s", r.Header.Get("Accept"))
		}
		csv := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","01","50,000,000","45,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
"113","12","48,000,000","43,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csv))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := ExportStatisticsProviderWithClient(server.Client(), tmpDir)
	provider.baseURL = server.URL

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.ExportElectronics.Symbol != "TW_EXPORT_ELECTRONICS" {
		t.Errorf("expected symbol TW_EXPORT_ELECTRONICS, got %s", snap.ExportElectronics.Symbol)
	}
	// 50,000,000 / 1000 / 1e6 = 0.05 (trillion USD)
	if snap.ExportElectronics.Value != 0.05 {
		t.Errorf("expected value 0.05, got %f", snap.ExportElectronics.Value)
	}
	// ((50M - 48M) / 48M) * 100 = 4.166...%
	expectedChange := ((50000000.0 - 48000000.0) / 48000000.0) * 100
	if math.Abs(snap.ExportElectronics.ChangePct-expectedChange) > 0.0001 {
		t.Errorf("expected change %f, got %f", expectedChange, snap.ExportElectronics.ChangePct)
	}
}

func TestExportStatisticsProvider_FetchSnapshot_InsufficientData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csv := `年月,出口總值,進口總值,出超
"114","01","50,000,000","45,000,000","5,000,000"`
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csv))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := ExportStatisticsProviderWithClient(server.Client(), tmpDir)
	provider.baseURL = server.URL

	_, err := provider.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error for insufficient data")
	}
	if !strings.Contains(err.Error(), "expected at least 2 rows") {
		t.Errorf("expected 'expected at least 2 rows' error, got: %v", err)
	}
}

func TestExportStatisticsProvider_SaveExport(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewExportStatisticsProvider(tmpDir)

	data := CustomsExportImport{
		Year:         114,
		Month:        1,
		ExportTotal:  50000,
		ImportTotal:  45000,
		TradeBalance: 5000,
		DownloadedAt: 1234567890,
	}

	err := provider.saveExport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify filename format (year padded to 3 digits)
	expectedPath := filepath.Join(tmpDir, "11401_export.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", expectedPath)
	}

	// Verify file content
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	var saved CustomsExportImport
	if err := json.Unmarshal(content, &saved); err != nil {
		t.Fatalf("failed to unmarshal saved file: %v", err)
	}

	if saved.Year != data.Year {
		t.Errorf("expected year %d, got %d", data.Year, saved.Year)
	}
	if saved.Month != data.Month {
		t.Errorf("expected month %d, got %d", data.Month, saved.Month)
	}
	if saved.ExportTotal != data.ExportTotal {
		t.Errorf("expected export total %f, got %f", data.ExportTotal, saved.ExportTotal)
	}
}

func TestExportStatisticsProvider_FetchSnapshot_SavesBothMonths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csv := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","03","52,000,000","47,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
"114","02","50,000,000","45,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csv))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := ExportStatisticsProviderWithClient(server.Client(), tmpDir)
	provider.baseURL = server.URL

	_, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	latestFile := filepath.Join(tmpDir, "11403_export.json")
	prevFile := filepath.Join(tmpDir, "11402_export.json")

	if _, err := os.Stat(latestFile); os.IsNotExist(err) {
		t.Errorf("expected latest file %s to exist", latestFile)
	}
	if _, err := os.Stat(prevFile); os.IsNotExist(err) {
		t.Errorf("expected previous file %s to exist", prevFile)
	}
}

type recordingExportStatsSaver struct {
	mu    sync.Mutex
	calls []CustomsExportImport
	fail  bool
}

func (s *recordingExportStatsSaver) SaveExportStats(ctx context.Context, year, month int, exportTotal, importTotal, tradeBalance float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return fmt.Errorf("boom")
	}
	s.calls = append(s.calls, CustomsExportImport{Year: year, Month: month, ExportTotal: exportTotal, ImportTotal: importTotal, TradeBalance: tradeBalance})
	return nil
}

func (s *recordingExportStatsSaver) snapshot() []CustomsExportImport {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CustomsExportImport, len(s.calls))
	copy(out, s.calls)
	return out
}

// TestExportStatisticsProvider_FetchSnapshot_PersistsStats verifies that after a
// successful JSON write, the optional saver (PostgreSQL pipeline) is invoked for
// both fetched months with the parsed row values.
func TestExportStatisticsProvider_FetchSnapshot_PersistsStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csv := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","03","52,000,000","47,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
"114","02","50,000,000","45,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csv))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := ExportStatisticsProviderWithClient(server.Client(), tmpDir)
	provider.baseURL = server.URL
	saver := &recordingExportStatsSaver{}
	provider.SetExportStatsSaver(saver)

	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.ExportElectronics.Symbol != "TW_EXPORT_ELECTRONICS" {
		t.Fatalf("unexpected symbol: %s", snap.ExportElectronics.Symbol)
	}

	calls := saver.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected 2 SaveExportStats calls (latest + prev), got %d", len(calls))
	}

	// Latest month (114/03): 52,000,000 / 1000 = 52,000 (million USD)
	got := calls[0]
	if got.Year != 114 || got.Month != 3 {
		t.Errorf("expected 114/03, got %d/%d", got.Year, got.Month)
	}
	if got.ExportTotal != 52000 || got.ImportTotal != 47000 || got.TradeBalance != 5000 {
		t.Errorf("unexpected row values: export=%v import=%v balance=%v", got.ExportTotal, got.ImportTotal, got.TradeBalance)
	}

	// Previous month (114/02): 50,000,000 / 1000 = 50,000 (million USD)
	got = calls[1]
	if got.Year != 114 || got.Month != 2 {
		t.Errorf("expected 114/02, got %d/%d", got.Year, got.Month)
	}
	if got.ExportTotal != 50000 || got.ImportTotal != 45000 || got.TradeBalance != 5000 {
		t.Errorf("unexpected row values: export=%v import=%v balance=%v", got.ExportTotal, got.ImportTotal, got.TradeBalance)
	}
}

// TestExportStatisticsProvider_FetchSnapshot_SaverFailureDoesNotBreakFetch
// verifies the best-effort contract: a saver failure is logged and swallowed,
// the fetch still succeeds and JSON files are still written.
func TestExportStatisticsProvider_FetchSnapshot_SaverFailureDoesNotBreakFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csv := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","03","52,000,000","47,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
"114","02","50,000,000","45,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csv))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := ExportStatisticsProviderWithClient(server.Client(), tmpDir)
	provider.baseURL = server.URL
	saver := &recordingExportStatsSaver{fail: true}
	provider.SetExportStatsSaver(saver)

	if _, err := provider.FetchSnapshot(context.Background()); err != nil {
		t.Fatalf("saver failure must not break the fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "11403_export.json")); err != nil {
		t.Errorf("expected latest JSON file to exist despite saver failure: %v", err)
	}
}

// TestExportStatisticsProvider_NilSaverIsNoop verifies the provider works when
// no saver is injected (backward compatible: JSON-only mode).
func TestExportStatisticsProvider_NilSaverIsNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csv := `年月,出口總值,進口總值,出超,出超(與上月比較),出超(與上年同月比較),出口總值(與上月比較),出口總值(與上年同月比較),出超值
"114","03","52,000,000","47,000,000","5,000,000","10.0","5.0","8.0","5,000,000"
"114","02","50,000,000","45,000,000","5,000,000","9.0","4.0","7.0","5,000,000"`
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte(csv))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := ExportStatisticsProviderWithClient(server.Client(), tmpDir)
	provider.baseURL = server.URL
	// No SetExportStatsSaver call — must not panic and must not save.

	if _, err := provider.FetchSnapshot(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
