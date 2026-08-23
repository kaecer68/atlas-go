// Command check-channel-contracts validates the gateway's per-channel data
// contracts (ChannelContract, internal/apigateway/channel_contract.go).
//
// It verifies:
//   - coverage: every channel in channelIDs() has an explicit contract
//   - no extras: no contract references a channel outside channelIDs()
//   - alias hygiene: aliases are non-empty, distinct, and do not collide
//     with canonical IDs or other channels' aliases
//   - semantic consistency: HealthSource/SuccessCriteria combinations that
//     contradict a channel's nature are flagged (e.g. live_ping claiming
//     file_exists, file_state claiming only data_present — the
//     government_broker ok 假象 pattern)
//
// Usage:
//
//	go run ./cmd/check-channel-contracts          # text output
//	go run ./cmd/check-channel-contracts --json   # JSON output for CI
//
// Exit code: 0 = all contracts valid, 1 = violations found.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

func main() {
	jsonMode := len(os.Args) > 1 && os.Args[1] == "--json"

	registry := apigateway.ChannelContracts()
	known := apigateway.KnownChannelIDs()
	violations := registry.Validate()

	if jsonMode {
		out := struct {
			KnownChannels   int                            `json:"known_channels"`
			CoveredChannels int                            `json:"covered_channels"`
			Violations      []apigateway.ContractViolation `json:"violations"`
		}{
			KnownChannels:   len(known),
			CoveredChannels: registry.Size(),
			Violations:      violations,
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "json marshal: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
	} else {
		printText(registry, known, violations)
	}

	if len(violations) > 0 {
		os.Exit(1)
	}
}

func printText(registry *apigateway.ChannelContractRegistry, known []string, violations []apigateway.ContractViolation) {
	fmt.Printf("[Channel Contract 檢查]\n")
	fmt.Printf("  已知通道: %d   已定義契約: %d\n", len(known), registry.Size())

	contracts := registry.All()
	ids := make([]string, 0, len(contracts))
	for id := range contracts {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("\n契約表（%d 通道）:\n", len(ids))
	fmt.Printf("  %-24s %-16s %-14s %-9s %-6s\n", "channel", "health_source", "criteria", "refresh", "degraded")
	for _, id := range ids {
		c := contracts[id]
		refresh := "default(24h)"
		if c.ExpectedRefresh != 0 {
			refresh = fmtDuration(c.ExpectedRefresh)
		}
		fmt.Printf("  %-24s %-16s %-14s %-9s %-6v\n", id, c.HealthSource, c.SuccessCriteria, refresh, c.DegradedOnEmpty)
	}

	if len(violations) == 0 {
		fmt.Printf("\n✅ 全部契約定義完整且語意一致\n")
		return
	}

	fmt.Printf("\n❌ %d 個契約違規:\n", len(violations))
	for _, v := range violations {
		fmt.Printf("  - [%s] %s: %s\n", v.Check, v.ChannelID, v.Detail)
	}
}

// fmtDuration renders a duration compactly (24h instead of 24h0m0s).
func fmtDuration(d time.Duration) string {
	s := d.String()
	s = strings.TrimSuffix(s, "0s") // 24h0m0s -> 24h0m, 1h30m0s -> 1h30m
	s = strings.TrimSuffix(s, "0m") // 24h0m -> 24h
	return s
}
