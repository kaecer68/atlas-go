package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

const finmindAPI = "https://api.finmindtrade.com/api/v4/data"

type finmindResp struct {
	Msg    string `json:"msg"`
	Status int    `json:"status"`
	Data   []struct {
		Date          string  `json:"date"`
		StockID       string  `json:"stock_id"`
		TradingVolume int64   `json:"Trading_Volume"`
		Open          float64 `json:"open"`
		Max           float64 `json:"max"`
		Min           float64 `json:"min"`
		Close         float64 `json:"close"`
	} `json:"data"`
}

func main() {
	csvPath := flag.String("csv", "data/replay/tw_extended_90days.csv", "target CSV path")
	start := flag.String("start", "2026-04-01", "backfill start date (YYYY-MM-DD)")
	end := flag.String("end", "2026-04-14", "backfill end date (YYYY-MM-DD)")
	dryRun := flag.Bool("dry-run", false, "print what would be added without writing")
	flag.Parse()

	stateDir := filepath.Join(filepath.Dir(filepath.Dir(*csvPath)), "state")
	if _, err := os.Stat(*csvPath); err != nil {
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "error", err.Error())
		fmt.Fprintf(os.Stderr, "csv path error: %v\n", err)
		os.Exit(1)
	}

	// Read existing CSV
	f, err := os.Open(*csvPath)
	if err != nil {
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "error", err.Error())
		fmt.Fprintf(os.Stderr, "open csv: %v\n", err)
		os.Exit(1)
	}
	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	f.Close()
	if err != nil {
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "error", err.Error())
		fmt.Fprintf(os.Stderr, "read csv: %v\n", err)
		os.Exit(1)
	}

	if len(rows) == 0 {
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "error", "csv is empty")
		fmt.Fprintf(os.Stderr, "csv is empty\n")
		os.Exit(1)
	}

	header := rows[0]
	_ = header
	existing := make(map[string]struct{}) // key: "date|code"
	nameMap := make(map[string]string)
	codesSet := make(map[string]struct{})
	latestDateByCode := make(map[string]string)
	prevCloseByCode := make(map[string]float64)

	for _, row := range rows[1:] {
		if len(row) < 8 {
			continue
		}
		date, code, name := row[0], row[1], row[2]
		key := date + "|" + code
		existing[key] = struct{}{}
		nameMap[code] = name
		codesSet[code] = struct{}{}
		closeVal, _ := strconv.ParseFloat(row[7], 64)
		if d, ok := latestDateByCode[code]; !ok || date > d {
			latestDateByCode[code] = date
			prevCloseByCode[code] = closeVal
		}
	}

	codes := make([]string, 0, len(codesSet))
	for c := range codesSet {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	fmt.Printf("Existing CSV: %d rows, %d unique symbols\n", len(rows)-1, len(codes))
	fmt.Printf("Backfill range: %s to %s\n", *start, *end)

	var newRows [][]string
	client := &http.Client{Timeout: 30 * time.Second}

	for _, code := range codes {
		fd, err := fetchFinMindWithRetry(client, code, *start, *end)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] failed after retries: %v\n", code, err)
			continue
		}

		added := 0
		for _, d := range fd.Data {
			key := d.Date + "|" + d.StockID
			if _, ok := existing[key]; ok {
				continue
			}
			name := nameMap[d.StockID]
			if name == "" {
				name = d.StockID
			}
			newRow := []string{
				d.Date,
				d.StockID,
				name,
				fmt.Sprintf("%d", d.TradingVolume),
				fmt.Sprintf("%.2f", d.Open),
				fmt.Sprintf("%.2f", d.Max),
				fmt.Sprintf("%.2f", d.Min),
				fmt.Sprintf("%.2f", d.Close),
			}
			validateRow(newRow, prevCloseByCode)
			newRows = append(newRows, newRow)
			existing[key] = struct{}{}
			prevCloseByCode[d.StockID] = d.Close
			added++
		}
		if added > 0 {
			fmt.Printf("[%s] added %d rows\n", code, added)
		} else {
			fmt.Printf("[%s] no new rows\n", code)
		}
		time.Sleep(300 * time.Millisecond) // be polite to FinMind
	}

	if len(newRows) == 0 {
		fmt.Println("No new data to backfill.")
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "ok", "")
		return
	}

	fmt.Printf("\nTotal new rows to append: %d\n", len(newRows))

	if *dryRun {
		for _, r := range newRows {
			fmt.Println(strings.Join(r, ","))
		}
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "ok", "")
		return
	}

	// Append to CSV
	fout, err := os.OpenFile(*csvPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "error", err.Error())
		fmt.Fprintf(os.Stderr, "open csv for append: %v\n", err)
		os.Exit(1)
	}
	defer fout.Close()

	writer := csv.NewWriter(fout)
	for _, r := range newRows {
		if err := writer.Write(r); err != nil {
			fmt.Fprintf(os.Stderr, "write row: %v\n", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		monitoring.RecordChannelFetch(stateDir, "twse_replay", "error", err.Error())
		fmt.Fprintf(os.Stderr, "flush writer: %v\n", err)
		os.Exit(1)
	}

	monitoring.RecordChannelFetch(stateDir, "twse_replay", "ok", "")
	fmt.Printf("Successfully appended %d rows to %s\n", len(newRows), *csvPath)
}

func validateRow(row []string, prevCloseByCode map[string]float64) {
	if len(row) < 8 {
		return
	}
	code := row[1]
	date := row[0]
	open, _ := strconv.ParseFloat(row[4], 64)
	high, _ := strconv.ParseFloat(row[5], 64)
	low, _ := strconv.ParseFloat(row[6], 64)
	closeVal, _ := strconv.ParseFloat(row[7], 64)
	volume, _ := strconv.ParseInt(row[3], 10, 64)

	if high < open || high < closeVal || high < low {
		log.Printf("[WARN] %s on %s has invalid High (%.2f) relative to O=%.2f C=%.2f L=%.2f", code, date, high, open, closeVal, low)
	}
	if low > open || low > closeVal || low > high {
		log.Printf("[WARN] %s on %s has invalid Low (%.2f) relative to O=%.2f C=%.2f H=%.2f", code, date, low, open, closeVal, high)
	}
	if volume <= 0 {
		log.Printf("[WARN] %s on %s has non-positive volume (%d)", code, date, volume)
	}
	if prevClose, ok := prevCloseByCode[code]; ok && prevClose > 0 {
		changePct := (closeVal - prevClose) / prevClose * 100
		if changePct == 0 {
			log.Printf("[WARN] %s on %s has zero price change from previous close (%.2f -> %.2f)", code, date, prevClose, closeVal)
		} else if changePct > 20 || changePct < -20 {
			log.Printf("[WARN] %s on %s has extreme price change (%.2f%%) from previous close (%.2f -> %.2f)", code, date, changePct, prevClose, closeVal)
		}
	}
}

func fetchFinMindWithRetry(client *http.Client, code, start, end string) (*finmindResp, error) {
	url := fmt.Sprintf("%s?dataset=TaiwanStockPrice&data_id=%s&start_date=%s&end_date=%s", finmindAPI, code, start, end)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			time.Sleep(backoff)
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
		}
		var fd finmindResp
		if err := json.Unmarshal(body, &fd); err != nil {
			return nil, fmt.Errorf("json decode: %w (body:%s)", err, string(body)[:200])
		}
		if fd.Status != 200 {
			return nil, fmt.Errorf("api status: %d (%s)", fd.Status, fd.Msg)
		}
		return &fd, nil
	}
	return nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}
