package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

type bar struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("merge-replay", flag.ContinueOnError)
	output := fs.String("output", "data/replay/merged.csv", "output CSV path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	inputs := fs.Args()
	if len(inputs) == 0 {
		return fmt.Errorf("usage: merge-replay [-output path] <jsonl-file> [jsonl-file ...]")
	}

	seen := make(map[string]bool)
	var bars []csvRecord

	for _, input := range inputs {
		f, err := os.Open(input)
		if err != nil {
			return fmt.Errorf("open %s: %w", input, err)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var b bar
			if err := json.Unmarshal(line, &b); err != nil {
				log.Printf("skip %s:%d: %v", input, lineNum, err)
				continue
			}
			if b.Date == "" || b.Symbol == "" {
				continue
			}

			date := strings.TrimSuffix(b.Date, "T00:00:00Z")
			code := strings.TrimSuffix(b.Symbol, ".TW")
			key := date + ":" + code
			if seen[key] {
				continue
			}
			seen[key] = true

			bars = append(bars, csvRecord{
				Date:        date,
				Code:        code,
				Name:        b.Name,
				TradeVolume: b.Volume,
				Open:        b.Open,
				High:        b.High,
				Low:         b.Low,
				Close:       b.Close,
			})
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read %s: %w", input, err)
		}
	}

	sort.Slice(bars, func(i, j int) bool {
		if bars[i].Date != bars[j].Date {
			return bars[i].Date < bars[j].Date
		}
		return bars[i].Code < bars[j].Code
	})

	out, err := os.Create(*output)
	if err != nil {
		return fmt.Errorf("create %s: %w", *output, err)
	}
	defer out.Close()

	w := csv.NewWriter(out)
	w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})

	for _, r := range bars {
		w.Write([]string{
			r.Date,
			r.Code,
			r.Name,
			strconv.FormatInt(r.TradeVolume, 10),
			strconv.FormatFloat(r.Open, 'f', -1, 64),
			strconv.FormatFloat(r.High, 'f', -1, 64),
			strconv.FormatFloat(r.Low, 'f', -1, 64),
			strconv.FormatFloat(r.Close, 'f', -1, 64),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write CSV: %w", err)
	}

	fmt.Printf("merged %d unique bars from %d file(s) into %s\n", len(bars), len(inputs), *output)
	return nil
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
