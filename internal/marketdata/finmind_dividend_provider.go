package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type FinMindDividendProvider struct {
	client   *FinMindClient
	cacheDir string
	cacheTTL time.Duration
}

func NewFinMindDividendProvider(client *FinMindClient, cacheDir string) *FinMindDividendProvider {
	return &FinMindDividendProvider{
		client:   client,
		cacheDir: cacheDir,
		cacheTTL: 24 * time.Hour,
	}
}

func (p *FinMindDividendProvider) GetDividends(ctx context.Context, symbol string, startDate, endDate string) ([]domain.DividendRecord, error) {
	if cached, ok := p.loadFromCache(symbol, startDate, endDate); ok {
		return cached, nil
	}

	data, err := p.client.fetchDataset(ctx, "TaiwanStockDividend", symbol, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("fetch dividends for %s: %w", symbol, err)
	}

	records := make([]domain.DividendRecord, 0, len(data))
	for _, item := range data {
		record := p.parseDividendRecord(item)
		if record != nil {
			records = append(records, *record)
		}
	}

	if err := p.saveToCache(symbol, startDate, endDate, records); err != nil {
		logging.Warn("dividend_provider", "cache_save_failed", "symbol", symbol, logging.Err(err))
	}

	return records, nil
}

func (p *FinMindDividendProvider) GetLatestDividend(ctx context.Context, symbol string) (*domain.DividendRecord, error) {
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")

	records, err := p.GetDividends(ctx, symbol, startDate, endDate)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no dividend data for %s", symbol)
	}

	latest := &records[len(records)-1]
	return latest, nil
}

func (p *FinMindDividendProvider) parseDividendRecord(data map[string]any) *domain.DividendRecord {
	symbol, ok := data["stock_id"].(string)
	if !ok || symbol == "" {
		return nil
	}

	record := &domain.DividendRecord{
		Symbol: symbol,
	}

	cashDiv := 0.0
	if v, ok := data["CashEarningsDistribution"].(float64); ok {
		cashDiv += v
	}
	if v, ok := data["CashStatutorySurplus"].(float64); ok {
		cashDiv += v
	}
	record.CashDividend = cashDiv

	stockDiv := 0.0
	if v, ok := data["StockEarningsDistribution"].(float64); ok {
		stockDiv += v
	}
	record.StockDividend = stockDiv

	if v, ok := data["CashExDividendTradingDate"].(string); ok && v != "" {
		record.ExDividendDate = v
	}
	if v, ok := data["CashDividendPaymentDate"].(string); ok && v != "" {
		record.PaymentDate = v
	}

	if v, ok := data["year"].(string); ok && v != "" {
		if year, err := strconv.Atoi(v); err == nil {
			record.Year = year
		}
	}

	return record
}

func (p *FinMindDividendProvider) cacheFilePath(symbol, startDate, endDate string) string {
	filename := fmt.Sprintf("%s_%s_%s.json", strings.ReplaceAll(symbol, ".", "_"), startDate, endDate)
	return filepath.Join(p.cacheDir, filename)
}

func (p *FinMindDividendProvider) loadFromCache(symbol, startDate, endDate string) ([]domain.DividendRecord, bool) {
	path := p.cacheFilePath(symbol, startDate, endDate)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	if time.Since(info.ModTime()) > p.cacheTTL {
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var records []domain.DividendRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, false
	}

	return records, true
}

func (p *FinMindDividendProvider) saveToCache(symbol, startDate, endDate string, records []domain.DividendRecord) error {
	if err := os.MkdirAll(p.cacheDir, 0755); err != nil {
		return err
	}

	path := p.cacheFilePath(symbol, startDate, endDate)
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
