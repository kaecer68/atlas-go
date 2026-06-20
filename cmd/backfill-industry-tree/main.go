package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

var dryRun = flag.Bool("dry-run", false, "print computed weights without saving")

func main() {
	flag.Parse()

	cfg := config.GetParametersConfig()
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "config not initialized; run from project root with parameters.json available\n")
		os.Exit(1)
	}

	treeCfg := cfg.Industry.ClassificationTree.Value
	if len(treeCfg.Segments) == 0 {
		fmt.Fprintf(os.Stderr, "classification tree is empty\n")
		os.Exit(1)
	}

	fmt.Println("Fetching TWSE daily data...")
	client := marketdata.GetSharedTWSEClient()
	quotes, err := client.GetQuotes(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch TWSE data: %v\n", err)
		os.Exit(1)
	}

	tradeValue := make(map[string]float64)
	for _, q := range quotes {
		proxy := q.Last * float64(q.Volume)
		tradeValue[q.Symbol] = proxy
	}

	fmt.Printf("Fetched %d quotes from TWSE\n", len(quotes))

	newSegments := make([]config.IndustrySegmentConfig, len(treeCfg.Segments))
	copy(newSegments, treeCfg.Segments)

	l1Indices := make([]int, 0)
	l2ByParent := make(map[string][]int)
	for i, seg := range newSegments {
		if seg.Level == 1 {
			l1Indices = append(l1Indices, i)
		} else if seg.Level == 2 && seg.ParentID != "" {
			l2ByParent[seg.ParentID] = append(l2ByParent[seg.ParentID], i)
		}
	}

	totalL1Proxy := 0.0
	l1Proxies := make([]float64, len(l1Indices))
	for i, idx := range l1Indices {
		seg := newSegments[idx]
		proxy := 0.0
		for _, sym := range seg.RepresentativeStocks {
			if v, ok := tradeValue[sym]; ok {
				proxy += v
			}
		}
		l1Proxies[i] = proxy
		totalL1Proxy += proxy
	}

	if totalL1Proxy == 0 {
		fmt.Fprintf(os.Stderr, "warning: all L1 proxies are zero; weights unchanged\n")
	} else {
		for i, idx := range l1Indices {
			newSegments[idx].Weight = l1Proxies[i] / totalL1Proxy
		}
	}

	for parentID, indices := range l2ByParent {
		totalL2Proxy := 0.0
		l2Proxies := make([]float64, len(indices))
		for i, idx := range indices {
			seg := newSegments[idx]
			proxy := 0.0
			for _, sym := range seg.RepresentativeStocks {
				if v, ok := tradeValue[sym]; ok {
					proxy += v
				}
			}
			l2Proxies[i] = proxy
			totalL2Proxy += proxy
		}

		if totalL2Proxy == 0 {
			fmt.Fprintf(os.Stderr, "warning: L2 proxies for parent %s are all zero; weights unchanged\n", parentID)
			continue
		}
		for i, idx := range indices {
			newSegments[idx].Weight = l2Proxies[i] / totalL2Proxy
		}
	}

	fmt.Println("\n--- Computed Weights ---")
	for _, idx := range l1Indices {
		seg := newSegments[idx]
		fmt.Printf("L1 %-24s weight=%.4f  stocks=%v\n", seg.ID, seg.Weight, seg.RepresentativeStocks)
	}
	for parentID, indices := range l2ByParent {
		for _, idx := range indices {
			seg := newSegments[idx]
			fmt.Printf("L2 %-24s parent=%-15s weight=%.4f  stocks=%v\n", seg.ID, parentID, seg.Weight, seg.RepresentativeStocks)
		}
	}

	if *dryRun {
		fmt.Println("\nDry run: weights computed but not saved.")
		return
	}

	updated := cfg.Industry.ClassificationTree
	updated.Value.Segments = newSegments
	updated.Rationale = fmt.Sprintf("Auto-updated on %s from TWSE trade-value proxy. Previous rationale: %s", time.Now().Format("2006-01-02"), updated.Rationale)

	cfg.Industry.ClassificationTree = updated

	if err := cfg.TryLockedSaveWithRollback("configs/parameters.json", 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save config (skipping): %v\n", err)
		return
	}

	fmt.Println("\nConfig saved successfully.")
}
