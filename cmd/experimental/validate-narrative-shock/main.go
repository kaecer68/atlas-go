package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: validate-narrative-shock [--help]")
		fmt.Println("Validates narrative shock detection against replay data for 2026-03-26.")
		os.Exit(0)
	}

	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
	}

	ds, err := replay.LoadTWSEOpenDataCSV(cfg.ReplayDataPath)
	if err != nil {
		fmt.Println("Error loading replay data:", err)
		os.Exit(1)
	}

	date := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)

	registry, _ := orchestrator.LoadRegistry(cfg.AgentRegistryPath)
	if len(registry.Agents) == 0 {
		registry = orchestrator.SeedRegistry()
	}
	symbols := orchestrator.RegistrySymbols(registry)
	baseQuotes := ds.QuotesForDate(date, symbols)

	policy, err := baseline.Load(cfg.BaselinePolicyPath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}

	nEngine := narrative.NewNarrativeEngine()

	// Baseline run
	baseRegimeBase, baseRaw, _, _ := orchestrator.ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry, baseQuotes, policy.PromptOverrides, policy.ExecutionPolicy)
	baseEvents := nEngine.DetectEvents(quotesToData(baseQuotes))
	baseRegime := orchestrator.AdjustRegimeFromNarrative(baseRegimeBase, baseEvents)
	baseRecs := attachNarrative(baseRaw, baseEvents, registry, nEngine)

	// Shock run: inject a US10Y spike quote
	shockQuotes := append([]domain.Quote{}, baseQuotes...)
	shockQuotes = append(shockQuotes, domain.Quote{
		Symbol:     "^TNX",
		Last:       15.0,
		Open:       4.5,
		High:       15.5,
		Low:        4.4,
		Volume:     100000,
		Market:     "US",
		AsOf:       date,
		IsTradable: true,
		Source:     "mock",
	})
	shockRegimeBase, shockRaw, _, _ := orchestrator.ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry, shockQuotes, policy.PromptOverrides, policy.ExecutionPolicy)
	shockEvents := nEngine.DetectEvents(quotesToData(shockQuotes))
	shockRegime := orchestrator.AdjustRegimeFromNarrative(shockRegimeBase, shockEvents)
	shockRecs := attachNarrative(shockRaw, shockEvents, registry, nEngine)

	fmt.Println("=== Narrative Shock Validation ===")
	fmt.Printf("Date: %s\n\n", date.Format("2006-01-02"))

	fmt.Println("Baseline:")
	printRun(baseRegime, baseEvents, baseRecs)
	fmt.Println()
	fmt.Println("Shock (US10Y spike to 15.0):")
	printRun(shockRegime, shockEvents, shockRecs)
	fmt.Println()

	// Diff summary
	fmt.Println("--- Diff Summary ---")
	if baseRegime != shockRegime {
		fmt.Printf("Regime changed: %s -> %s\n", baseRegime, shockRegime)
	} else {
		fmt.Printf("Regime unchanged: %s\n", baseRegime)
	}
	fmt.Printf("Events delta: +%d\n", len(shockEvents)-len(baseEvents))
	if len(baseEvents) != len(shockEvents) {
		fmt.Println("New events triggered:")
		for _, e := range shockEvents {
			found := false
			for _, b := range baseEvents {
				if b.Theme == e.Theme {
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("  - %s\n", e.Theme)
			}
		}
	}
}

func quotesToData(quotes []domain.Quote) narrative.MarketNarrativeData {
	data := narrative.MarketNarrativeData{}
	for _, q := range quotes {
		switch q.Symbol {
		case "DXY", "^DXY":
			data.DXYChangePct = (q.Last - q.Open) / q.Open * 100
		case "US10Y", "^TNX":
			data.US10YChangeBps = q.Last
		case "VIX", "^VIX":
			data.VIXLevel = q.Last
		case "OIL", "CL=F":
			data.OilChangePct = (q.Last - q.Open) / q.Open * 100
		case "GOLD", "GC=F":
			data.GoldChangePct = (q.Last - q.Open) / q.Open * 100
		case "JPY=X", "USDJPY=X":
			data.JPY_ChangePct = (q.Last - q.Open) / q.Open * 100
		}
	}
	return data
}

func attachNarrative(recs []domain.Recommendation, events []narrative.NarrativeEvent, registry domain.AgentRegistry, engine *narrative.NarrativeEngine) []domain.Recommendation {
	if len(events) == 0 {
		return recs
	}
	chains := engine.MatchChains(events)
	enriched := make([]domain.Recommendation, len(recs))
	copy(enriched, recs)
	for i := range enriched {
		var agentLayer string
		for _, a := range registry.Agents {
			if a.ID == enriched[i].Agent {
				agentLayer = string(a.Layer)
				break
			}
		}
		if agentLayer == "context" || agentLayer == "superinvestor" {
			enriched[i].SupportingEvents = make([]string, len(events))
			for j, e := range events {
				enriched[i].SupportingEvents[j] = e.ID
			}
			enriched[i].ReasoningChain = []string{}
			for _, e := range events {
				enriched[i].ReasoningChain = append(enriched[i].ReasoningChain, fmt.Sprintf("%s (%s, confidence %.2f)", e.Theme, e.Region, e.Confidence))
			}
			for _, c := range chains {
				if len(c.Steps) > 0 {
					enriched[i].ReasoningChain = append(enriched[i].ReasoningChain, fmt.Sprintf("Chain %s: %s", c.TemplateID, c.Steps[0].Description))
				}
			}
			if enriched[i].Reason != "" {
				enriched[i].Reason = fmt.Sprintf("%s | Narrative: %d event(s)", enriched[i].Reason, len(events))
			}
		}
	}
	return enriched
}

func printRun(regime domain.Regime, events []narrative.NarrativeEvent, recs []domain.Recommendation) {
	fmt.Printf("  Regime: %s\n", regime)
	fmt.Printf("  Events: %d\n", len(events))
	for _, e := range events {
		fmt.Printf("    - %s (confidence %.2f)\n", e.Theme, e.Confidence)
	}
	fmt.Printf("  Recommendations: %d\n", len(recs))
	var sum int
	for _, r := range recs {
		sum += r.Conviction
	}
	if len(recs) > 0 {
		fmt.Printf("  Avg Conviction: %.1f\n", float64(sum)/float64(len(recs)))
	}
	for _, r := range recs {
		if r.Agent == "taiwan-macro-01" {
			fmt.Printf("  taiwan-macro-01 reason: %s\n", r.Reason)
		}
	}
}
