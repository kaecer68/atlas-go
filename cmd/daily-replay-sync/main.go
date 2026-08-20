package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// stockNameMap mirrors frontend mapping for CSV output
var stockNameMap = map[string]string{
	"0050":   "元大台灣50",
	"0056":   "元大高股息",
	"00878":  "國泰永續高股息",
	"006208": "富邦台50",
	"00692":  "富邦公司治理",
	"00713":  "元大高股息低波動",
	"00881":  "國泰台灣5G+",
	"00891":  "中信關鍵半導體",
	"00919":  "群益台灣精選高息",
	"00929":  "復華台灣科技優息",
	"00940":  "元大台灣價值高息",
	"1301":   "台塑",
	"1303":   "南亞",
	"1326":   "台化",
	"2303":   "聯電",
	"2308":   "台達電",
	"2317":   "鴻海",
	"2330":   "台積電",
	"2382":   "廣達",
	"2454":   "聯發科",
	"2603":   "長榮",
	"2609":   "陽明",
	"2615":   "萬海",
	"2881":   "富邦金",
	"2882":   "國泰金",
	"2886":   "兆豐金",
	"2891":   "中信金",
	"2892":   "第一金",
	"3008":   "大立光",
	"3034":   "聯詠",
	"3037":   "欣興",
	"6669":   "緯穎",
}

type csvRecord struct {
	Date        string
	Code        string
	Name        string
	TradeVolume int64
	Open        float64
	High        float64
	Low         float64
	Close       float64
}

func main() {
	csvPath := flag.String("csv", constants.ReplayCSVPath, "target CSV path")
	backfillStart := flag.String("backfill-start", "", "backfill start date (YYYY-MM-DD)")
	backfillEnd := flag.String("backfill-end", "", "backfill end date (YYYY-MM-DD)")
	flag.Parse()

	stateDir := filepath.Join(filepath.Dir(filepath.Dir(*csvPath)), "state")
	var pool *pgxpool.Pool
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		migrationsPath := filepath.Join(filepath.Dir(stateDir), "sql/migrations")
		if _, err := os.Stat(migrationsPath); err == nil {
			var dbErr error
			pool, dbErr = db.Init(context.Background(), dsn, migrationsPath)
			if dbErr != nil {
				log.Printf("[DB] failed to initialize database: %v", dbErr)
			} else {
				log.Printf("[DB] connected and migrations applied")
				defer pool.Close()
			}
		}
	}

	if *backfillStart != "" && *backfillEnd != "" {
		if err := runBackfill(*csvPath, *backfillStart, *backfillEnd); err != nil {
			log.Fatalf("backfill failed: %v", err)
		}
		return
	}

	if err := runDailySync(*csvPath, pool); err != nil {
		log.Fatalf("daily sync failed: %v", err)
	}
}

func runDailySync(csvPath string, pool *pgxpool.Pool) error {
	stateDir := filepath.Join(filepath.Dir(filepath.Dir(csvPath)), "state")
	client := marketdata.GetSharedTWSEClient()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("[DailySync] Fetching today's quotes from TWSE OpenAPI...")
	quotes, err := client.GetQuotes(ctx)
	if err != nil {
		monitoring.RecordChannelFetchWithPool(stateDir, "twse_replay_sync", "error", err.Error(), pool)
		return fmt.Errorf("fetch quotes: %w", err)
	}

	targetSymbols := make(map[string]bool)
	for _, s := range orchestrator.DefaultSymbols() {
		targetSymbols[stripSuffix(s)] = true
	}

	dateStr := time.Now().Format("2006-01-02")
	var records []csvRecord
	for _, q := range quotes {
		code := stripSuffix(q.Symbol)
		if !targetSymbols[code] {
			continue
		}
		records = append(records, csvRecord{
			Date:        dateStr,
			Code:        code,
			Name:        stockNameMap[code],
			TradeVolume: q.Volume,
			Open:        q.Open,
			High:        q.High,
			Low:         q.Low,
			Close:       q.Last,
		})
	}

	if err := appendRecords(csvPath, records); err != nil {
		monitoring.RecordChannelFetchWithPool(stateDir, "twse_replay_sync", "error", err.Error(), pool)
		return err
	}
	monitoring.RecordChannelFetchWithPool(stateDir, "twse_replay_sync", "ok", "", pool)
	log.Printf("[DailySync] Appended %d records for %s", len(records), dateStr)
	return nil
}

func runBackfill(csvPath, startStr, endStr string) error {
	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return fmt.Errorf("parse start date: %w", err)
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return fmt.Errorf("parse end date: %w", err)
	}
	if end.Before(start) {
		return fmt.Errorf("end date before start date")
	}

	client := marketdata.GetSharedTWSEClient()
	ctx := context.Background()
	symbols := orchestrator.DefaultSymbols()

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		apiDateStr := d.Format("20060102") // TWSE API expects YYYYMMDD format
		log.Printf("[Backfill] Processing %s...", dateStr)

		var records []csvRecord
		for _, sym := range symbols {
			code := stripSuffix(sym)
			quote, err := client.GetDailyQuote(ctx, apiDateStr, code)
			if err != nil {
				log.Printf("[Backfill]   skip %s on %s: %v", code, dateStr, err)
				continue
			}
			records = append(records, csvRecord{
				Date:        dateStr,
				Code:        code,
				Name:        stockNameMap[code],
				TradeVolume: quote.Volume,
				Open:        quote.Open,
				High:        quote.High,
				Low:         quote.Low,
				Close:       quote.Last,
			})
		}
		if len(records) > 0 {
			if err := appendRecords(csvPath, records); err != nil {
				return fmt.Errorf("append %s: %w", dateStr, err)
			}
			log.Printf("[Backfill] Appended %d records for %s", len(records), dateStr)
		} else {
			log.Printf("[Backfill] No data available for %s", dateStr)
		}
	}
	return nil
}

func validateRecord(r csvRecord, prevCloseByCode map[string]float64) {
	if r.High < r.Open || r.High < r.Close || r.High < r.Low {
		log.Printf("[WARN] %s on %s has invalid High (%.2f) relative to O=%.2f C=%.2f L=%.2f", r.Code, r.Date, r.High, r.Open, r.Close, r.Low)
	}
	if r.Low > r.Open || r.Low > r.Close || r.Low > r.High {
		log.Printf("[WARN] %s on %s has invalid Low (%.2f) relative to O=%.2f C=%.2f H=%.2f", r.Code, r.Date, r.Low, r.Open, r.Close, r.High)
	}
	if r.TradeVolume <= 0 {
		log.Printf("[WARN] %s on %s has non-positive volume (%d)", r.Code, r.Date, r.TradeVolume)
	}
	if prevClose, ok := prevCloseByCode[r.Code]; ok && prevClose > 0 {
		changePct := (r.Close - prevClose) / prevClose * 100
		if changePct == 0 {
			log.Printf("[WARN] %s on %s has zero price change from previous close (%.2f -> %.2f)", r.Code, r.Date, prevClose, r.Close)
		} else if changePct > 20 || changePct < -20 {
			log.Printf("[WARN] %s on %s has extreme price change (%.2f%%) from previous close (%.2f -> %.2f)", r.Code, r.Date, changePct, prevClose, r.Close)
		}
	}
}

func buildPrevCloseByCode(existing []csvRecord) map[string]float64 {
	latest := make(map[string]string) // code -> latest date
	prev := make(map[string]float64)  // code -> latest close
	for _, r := range existing {
		if d, ok := latest[r.Code]; !ok || r.Date > d {
			latest[r.Code] = r.Date
			prev[r.Code] = r.Close
		}
	}
	return prev
}

func appendRecords(csvPath string, records []csvRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Load existing data for deduplication
	existing, err := loadCSV(csvPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load csv: %w", err)
	}

	// Create dedup key set
	seen := make(map[string]bool)
	for _, r := range existing {
		key := r.Date + "," + r.Code
		seen[key] = true
	}

	prevCloseByCode := buildPrevCloseByCode(existing)

	// Append new records, skipping duplicates
	f, err := os.OpenFile(csvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer func() { _ = f.Close() }()

	writer := csv.NewWriter(f)
	// Write header if new file
	if stat, _ := f.Stat(); stat.Size() == 0 {
		_ = writer.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	}

	for _, r := range records {
		key := r.Date + "," + r.Code
		if seen[key] {
			continue
		}
		validateRecord(r, prevCloseByCode)
		_ = writer.Write([]string{
			r.Date,
			r.Code,
			r.Name,
			strconv.FormatInt(r.TradeVolume, 10),
			fmt.Sprintf("%.2f", r.Open),
			fmt.Sprintf("%.2f", r.High),
			fmt.Sprintf("%.2f", r.Low),
			fmt.Sprintf("%.2f", r.Close),
		})
		seen[key] = true
		prevCloseByCode[r.Code] = r.Close
	}
	writer.Flush()
	return writer.Error()
}

func loadCSV(path string) ([]csvRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	lines, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	var records []csvRecord
	for i, line := range lines {
		if i == 0 && line[0] == "Date" {
			continue
		}
		if len(line) < 8 {
			continue
		}
		vol, _ := strconv.ParseInt(line[3], 10, 64)
		open, _ := strconv.ParseFloat(line[4], 64)
		high, _ := strconv.ParseFloat(line[5], 64)
		low, _ := strconv.ParseFloat(line[6], 64)
		close_, _ := strconv.ParseFloat(line[7], 64)
		records = append(records, csvRecord{
			Date:        line[0],
			Code:        line[1],
			Name:        line[2],
			TradeVolume: vol,
			Open:        open,
			High:        high,
			Low:         low,
			Close:       close_,
		})
	}
	return records, nil
}

func stripSuffix(s string) string {
	return strings.TrimSuffix(s, ".TW")
}
