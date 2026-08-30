// Command backfill-taifex-oi-finmind backfills 期貨三大法人買賣 (per-trader
// futures volume + open interest) history into data/state/taifex_oi/.
//
// Background (D07, #1740 Phase 1 Road 1): TAIFEX OpenAPI institutional data
// has no date parameter (latest session only) and the TAIFEX website CSV
// (futContractsDateDown) rejects dates <= 2023 with "日期時間錯誤 DateTime
// error" — so 2021 TAIFEX OI is unreachable through TAIFEX channels. FinMind's
// TaiwanFuturesInstitutionalInvestors dataset covers 2021 on the Free tier
// (期貨三大法人買賣) and Sponsor tier, and returns the full requested range in
// ONE API call. This command fetches per-contract ranges and writes one
// per-date JSON file under data/state/taifex_oi/.
//
// Usage:
//
//	backfill-taifex-oi-finmind -workdir . -start 2021-01-01 -end 2021-12-31
//	backfill-taifex-oi-finmind -workdir . -contract TX,MTX -dry-run
//
// Rate limit: the shared FinMindClient token bucket is the hard gate
// (Free 600/hr, Sponsor 6000/hr). A full-year range fetch is one request per
// contract, so even a 4-contract 2021 run is 4 requests — far below both
// ceilings. Output files are merged per date (idempotent re-runs; multiple
// contracts accumulate under "contracts").
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	defaultStart = "2021-01-01"
	defaultEnd   = "2021-12-31"
	defaultOut   = "data/state/taifex_oi"
	logFileName  = "backfill_log.jsonl"
	sourceTag    = "finmind:TaiwanFuturesInstitutionalInvestors"
)

// taifexOIDayFile is the per-date output artifact. Contracts maps FinMind
// futures_id → the standard InstitutionalFuturesDaily shape (same JSON tags
// the taifex_institutional adapter produces, so downstream macro consumers
// can reuse the struct).
type taifexOIDayFile struct {
	Date      string                                          `json:"date"`
	Source    string                                          `json:"source"`
	FetchedAt string                                          `json:"fetched_at"`
	Contracts map[string]marketdata.InstitutionalFuturesDaily `json:"contracts"`
}

// backfillLogEntry records one per-contract provenance row.
type backfillLogEntry struct {
	Contract  string `json:"contract"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Dates     int    `json:"dates"`
	Rows      int    `json:"rows"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	FetchedAt string `json:"fetched_at"`
}

func main() {
	var (
		workDir   = flag.String("workdir", ".", "atlas work directory (repo root)")
		startStr  = flag.String("start", defaultStart, "backfill start date YYYY-MM-DD (inclusive)")
		endStr    = flag.String("end", defaultEnd, "backfill end date YYYY-MM-DD (inclusive)")
		contracts = flag.String("contract", "TX", "comma-separated FinMind futures_id list (TX, MTX, EXF, FXF, GTF, TJF; TXF/MXF aliases translate)")
		outDir    = flag.String("out", defaultOut, "output directory under workdir")
		dryRun    = flag.Bool("dry-run", false, "plan only: print call count and date estimate, no HTTP")
	)
	flag.Parse()

	start, err := time.ParseInLocation("2006-01-02", *startStr, time.Local)
	if err != nil {
		log.Fatalf("parse -start %q: %v", *startStr, err)
	}
	end, err := time.ParseInLocation("2006-01-02", *endStr, time.Local)
	if err != nil {
		log.Fatalf("parse -end %q: %v", *endStr, err)
	}
	if end.Before(start) {
		log.Fatalf("-end %s before -start %s", *endStr, *startStr)
	}

	var ids []string
	for _, c := range strings.Split(*contracts, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			ids = append(ids, marketdata.NormalizeFinMindFuturesID(c))
		}
	}
	if len(ids) == 0 {
		log.Fatal("no contracts given (-contract)")
	}

	outRoot := filepath.Join(*workDir, *outDir)
	log.Printf("taifex-oi-finmind-backfill: %s..%s contracts=%v out=%s dry=%v",
		start.Format("2006-01-02"), end.Format("2006-01-02"), ids, outRoot, *dryRun)

	if *dryRun {
		fmt.Printf("plan: %d contract(s) x 1 range call each = %d FinMind call(s) (well below 6000/hr; free tier 600/hr)\n",
			len(ids), len(ids))
		return
	}

	cfg := config.Load()
	key := cfg.FinMindAPIKey
	if key == "" {
		log.Fatal("FINMIND_API_KEY not set (env or ~/.config/atlas-go/.env); required for backfill")
	}

	client := marketdata.GetSharedFinMindClient(key, *workDir)
	provider := marketdata.NewFinMindFuturesInstitutionalProvider(client)

	ctx := context.Background()
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", outRoot, err)
	}

	var totalDates int
	for _, id := range ids {
		entry := backfillLogEntry{
			Contract:  id,
			Start:     start.Format("2006-01-02"),
			End:       end.Format("2006-01-02"),
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		}
		rows, err := provider.FetchInstitutionalFuturesRange(ctx, id, entry.Start, entry.End)
		if err != nil {
			entry.Status = "error"
			entry.Error = err.Error()
			appendLog(outRoot, entry)
			log.Printf("  %s: ERROR: %v", id, err)
			continue
		}
		entry.Rows = len(rows)
		if len(rows) == 0 {
			entry.Status = "no_data"
			appendLog(outRoot, entry)
			log.Printf("  %s: no rows in range (holiday-only window or product not listed)", id)
			continue
		}
		if err := mergeRows(outRoot, id, rows); err != nil {
			entry.Status = "error"
			entry.Error = err.Error()
			appendLog(outRoot, entry)
			log.Printf("  %s: merge error: %v", id, err)
			continue
		}
		entry.Status = "ok"
		entry.Dates = len(rows)
		totalDates += len(rows)
		appendLog(outRoot, entry)
		log.Printf("  %s: %d dates merged (%s .. %s)", id, len(rows), rows[0].Date, rows[len(rows)-1].Date)
	}
	log.Printf("done: %d total date(s) across %d contract(s) -> %s", totalDates, len(ids), outRoot)
}

// mergeRows writes one per-date file under outRoot, merging the contract's
// rows into any existing file (idempotent, multi-contract safe).
func mergeRows(outRoot, futuresID string, rows []marketdata.InstitutionalFuturesDaily) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range rows {
		day := &rows[i]
		path := filepath.Join(outRoot, day.Date+".json")
		file := taifexOIDayFile{Contracts: map[string]marketdata.InstitutionalFuturesDaily{}}
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &file)
		}
		if file.Contracts == nil {
			file.Contracts = map[string]marketdata.InstitutionalFuturesDaily{}
		}
		file.Date = day.Date
		file.Source = sourceTag
		file.FetchedAt = now
		file.Contracts[futuresID] = *day
		b, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			return fmt.Errorf("%s: marshal: %w", path, err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return fmt.Errorf("%s: write: %w", path, err)
		}
	}
	return nil
}

func appendLog(outRoot string, entry backfillLogEntry) {
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(outRoot, logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(b, '\n'))
}
