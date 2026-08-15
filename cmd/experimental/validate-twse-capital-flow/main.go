package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: validate-twse-capital-flow [--help]")
		fmt.Println("Fetches the latest TWSE capital-flow snapshot and validates non-zero data.")
		os.Exit(0)
	}

	// TODO(low): Migrate to Gateway when experimental tooling is unified.
	// TWSECapitalFlowProvider is a local file reader (data/state/capital_flow),
	// not a network client. Non-production path — (see .omo/audit/ for historical records).
	provider := marketdata.NewTWSECapitalFlowProvider(constants.StateCapitalFlow)
	ctx := context.Background()

	fmt.Println("=== TWSE Capital Flow Validation ===")
	fmt.Println("Provider:", provider.Name())

	snap, err := provider.FetchSnapshot(ctx)
	if err != nil {
		fmt.Println("Error fetching snapshot:", err)
		os.Exit(1)
	}

	fmt.Println("Fetched snapshot successfully")
	out := map[string]any{
		"foreign_investor_net": snap.ForeignInvestorNet.Value,
		"domestic_fund_net":    snap.DomesticFundNet.Value,
		"dealer_net":           snap.DealerNet.Value,
		"recorded_at":          snap.RecordedAt,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))

	if snap.ForeignInvestorNet.Value == 0 && snap.DomesticFundNet.Value == 0 && snap.DealerNet.Value == 0 {
		fmt.Println("\nWARNING: all capital flow values are zero. This may indicate a non-trading day or API issue.")
		os.Exit(2)
	}

	fmt.Println("\nValidation passed: non-zero capital flow data retrieved.")
}
