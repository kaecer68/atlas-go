package main

import (
	"flag"
	"log"

	"github.com/kaecer68/atlas-go/internal/importer"
)

func main() {
	source := flag.String("source", "samples/replay/twse_stock_day_all_sample.csv", "source TWSE/TPEX open data CSV")
	target := flag.String("target", "data/replay/tw_open_data.jsonl", "target normalized replay JSONL")
	flag.Parse()

	if err := importer.ImportTWOpenDataCSVToJSONL(*source, *target); err != nil {
		log.Fatalf("import replay data: %v", err)
	}
}
