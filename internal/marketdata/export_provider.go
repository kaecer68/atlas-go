package marketdata

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CustomsExportImport holds a single row of customs export/import statistics.
type CustomsExportImport struct {
	Year         int     `json:"year"`          // ROC year (e.g., 115 = 2026)
	Month        int     `json:"month"`         // 1-12
	ExportTotal  float64 `json:"export_total"`  // 千美元 → /1000 = 百萬美元 (million USD)
	ImportTotal  float64 `json:"import_total"`  // 千美元 → /1000 = 百萬美元 (million USD)
	TradeBalance float64 `json:"trade_balance"` // 出超 (千美元 → /1000 = 百萬美元)
	DownloadedAt int64   `json:"downloaded_at"` // unix timestamp
}

// ExportStatsSaver persists customs export/import statistics rows to durable
// storage. *repository.DualWriteRepository satisfies this interface
// structurally; providers must treat it as optional (nil-safe).
// All numeric values are in million USD (百萬美元) — i.e. already divided by
// 1000 from the raw 千美元 source (see parseCustomsCSV).
type ExportStatsSaver interface {
	SaveExportStats(ctx context.Context, year, month int, exportTotal, importTotal, tradeBalance float64) error
}

// ExportStatisticsProvider fetches Taiwan export/import statistics from
// the government open data portal (data.gov.tw dataset 6053).
// CSV: https://opendata.customs.gov.tw/data/6053/csv.csv
type ExportStatisticsProvider struct {
	client     *http.Client
	storageDir string
	baseURL    string
	limiter    *rate.Limiter
	statsSaver ExportStatsSaver // optional: persists monthly rows to PostgreSQL
}

// NewExportStatisticsProvider creates a new export statistics provider.
func NewExportStatisticsProvider(storageDir string) *ExportStatisticsProvider {
	return &ExportStatisticsProvider{
		client:     httpclient.NewFactory().NewClient(30 * time.Second),
		storageDir: storageDir,
		baseURL:    "https://opendata.customs.gov.tw/data/6053/csv.csv",
		limiter:    rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Name returns the provider name.
func (e *ExportStatisticsProvider) Name() string {
	return "export_statistics"
}

// SetExportStatsSaver injects an optional export-statistics saver. When set,
// each successful JSON write in FetchSnapshot is mirrored to the saver
// (best-effort: saver failures are logged as WARN and never break the fetch).
func (e *ExportStatisticsProvider) SetExportStatsSaver(saver ExportStatsSaver) {
	e.statsSaver = saver
}

// FetchSnapshot fetches the latest customs export/import data and returns a MacroDataSnapshot.
func (e *ExportStatisticsProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	latest, prev, err := e.fetchLatestTwoMonths(ctx)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("export_statistics fetch: %w", err)
	}

	exportValue := latest.ExportTotal / 1e6 // convert from million USD → billion USD

	var changePct float64
	if prev.ExportTotal > 0 {
		changePct = ((latest.ExportTotal - prev.ExportTotal) / prev.ExportTotal) * 100
	}

	// Convert ROC year/month to unix timestamp (use 1st of month noon CST)
	ts := time.Date(latest.Year+1911, time.Month(latest.Month), 1, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60)).Unix()

	if err := e.saveExport(latest); err != nil {
		logging.Warn("export_statistics_provider", "save_export_latest_warning", logging.Err(err))
	} else {
		e.persistStats(ctx, latest)
	}
	if err := e.saveExport(prev); err != nil {
		logging.Warn("export_statistics_provider", "save_export_prev_warning", logging.Err(err))
	} else {
		e.persistStats(ctx, prev)
	}

	return MacroDataSnapshot{
		RecordedAt: latest.DownloadedAt,
		ExportElectronics: MacroDataPoint{
			Symbol:    "TW_EXPORT_ELECTRONICS",
			Value:     exportValue,
			ChangePct: changePct,
			Timestamp: ts,
		},
	}, nil
}

// fetchLatestTwoMonths returns the two most recent monthly records.
func (e *ExportStatisticsProvider) fetchLatestTwoMonths(ctx context.Context) (CustomsExportImport, CustomsExportImport, error) {
	if err := e.limiter.Wait(ctx); err != nil {
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("rate limit: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL, nil)
	if err != nil {
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/csv")

	resp, err := e.client.Do(req)
	if err != nil {
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("export statistics HTTP request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// P1-10: bounded upstream error body (FinMind 512B pattern) so the
		// channel LastError shows why the customs portal rejected the call.
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr == "" {
			bodyStr = "(empty body)"
		}
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("export statistics HTTP %d, body: %s", resp.StatusCode, bodyStr)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("export statistics read body: %w", err)
	}

	records, err := parseCustomsCSV(body)
	if err != nil {
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("export statistics CSV parse: %w", err)
	}

	if len(records) < 2 {
		return CustomsExportImport{}, CustomsExportImport{}, fmt.Errorf("export statistics: expected at least 2 rows, got %d", len(records))
	}

	// CSV is newest-first; records[0] = current month, records[1] = previous month
	return records[0], records[1], nil
}

// parseCustomsCSV parses the customs CSV body into CustomsExportImport slice (newest-first).
func parseCustomsCSV(body []byte) ([]CustomsExportImport, error) {
	// Strip UTF-8 BOM if present.
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		body = body[3:]
	}
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.FieldsPerRecord = -1 // allow variable fields
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	var results []CustomsExportImport
	now := time.Now().Unix()

	// Skip header row (records[0]), iterate data rows
	for _, row := range records[1:] {
		if len(row) < 8 {
			continue
		}
		year, err := strconv.Atoi(row[0])
		if err != nil {
			continue
		}
		month, err := strconv.Atoi(row[1])
		if err != nil || month < 1 || month > 12 {
			continue
		}

		exportTotal := parseTWDVolume(row[2])
		importTotal := parseTWDVolume(row[3])
		tradeBalance := parseTWDVolume(row[8])

		// Convert from thousand USD to million USD (same unit as other MacroDataPoint values)
		results = append(results, CustomsExportImport{
			Year:         year,
			Month:        month,
			ExportTotal:  exportTotal / 1000, // 千美元 → 百萬美元
			ImportTotal:  importTotal / 1000,
			TradeBalance: tradeBalance / 1000,
			DownloadedAt: now,
		})
	}

	return results, nil
}

func (e *ExportStatisticsProvider) saveExport(data CustomsExportImport) error {
	if err := os.MkdirAll(e.storageDir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(e.storageDir, fmt.Sprintf("%03d%02d_export.json", data.Year, data.Month))
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

// persistStats mirrors a successfully fetched monthly row to the optional
// saver (PostgreSQL export_statistics). Best-effort: failures are logged as
// WARN and never abort the fetch pipeline.
func (e *ExportStatisticsProvider) persistStats(ctx context.Context, data CustomsExportImport) {
	if e.statsSaver == nil {
		return
	}
	if err := e.statsSaver.SaveExportStats(ctx, data.Year, data.Month, data.ExportTotal, data.ImportTotal, data.TradeBalance); err != nil {
		logging.Warn("export_statistics_provider", "save_export_stats_warning",
			"year", data.Year, "month", data.Month, logging.Err(err))
	}
}

// ExportStatisticsProviderWithClient creates a provider with custom HTTP client (for testing).
func ExportStatisticsProviderWithClient(client *http.Client, storageDir string) *ExportStatisticsProvider {
	return &ExportStatisticsProvider{
		client:     client,
		storageDir: storageDir,
		baseURL:    "https://opendata.customs.gov.tw/data/6053/csv.csv",
		limiter:    rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}
