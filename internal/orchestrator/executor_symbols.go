package orchestrator

import (
	"encoding/csv"
	"os"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func DefaultSymbols() []string {
	base := []string{
		"2330.TW",
		"2317.TW",
		"2382.TW",
		"2345.TW",
		"2412.TW",
		"2454.TW",
		"2303.TW",
		"2308.TW",
		"3008.TW",
		"3034.TW",
		"3037.TW",
		"3711.TW",
		"6669.TW",
		"2603.TW",
		"2609.TW",
		"2615.TW",
		"2881.TW",
		"2882.TW",
		"2884.TW",
		"2885.TW",
		"2886.TW",
		"2887.TW",
		"2891.TW",
		"2892.TW",
		"1301.TW",
		"1303.TW",
		"1326.TW",
		"0050.TW",
		"0051.TW",
		"0052.TW",
		"0053.TW",
		"0056.TW",
		"00878.TW",
		"006208.TW",
		"00692.TW",
		"00713.TW",
		"00881.TW",
		"00891.TW",
		"00919.TW",
		"00929.TW",
		"00940.TW",
	}

	// Auto-sync: merge symbols from replay CSV if available
	return mergeReplaySymbols(base, loadSymbolsFromCSV(constants.ReplayCSVPath))
}

// mergeReplaySymbols appends CSV symbols missing from base, normalizing
// bare codes (e.g. "2330") to the ".TW"-suffixed form used across the
// universe (same convention as replay.LoadTWSEOpenDataCSV) so the CSV
// merge genuinely expands the symbol set instead of double-listing codes.
func mergeReplaySymbols(base, csvSymbols []string) []string {
	if len(csvSymbols) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(csvSymbols))
	for _, s := range base {
		seen[s] = true
	}
	for _, s := range csvSymbols {
		if !strings.HasSuffix(s, ".TW") {
			s += ".TW"
		}
		if !seen[s] {
			base = append(base, s)
			seen[s] = true
		}
	}
	return base
}

func loadSymbolsFromCSV(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil
	}
	// Find the symbol column: daily-replay-sync writes "Code", older
	// replay CSVs used "symbol". Accepting both keeps DefaultSymbols
	// expanding from the actual replay artifact (H4 remediation).
	symIdx := -1
	for i, col := range header {
		c := strings.TrimSpace(col)
		if strings.EqualFold(c, "symbol") || strings.EqualFold(c, "code") {
			symIdx = i
			break
		}
	}
	if symIdx < 0 {
		return nil
	}
	seen := make(map[string]bool)
	for {
		record, err := r.Read()
		if err != nil {
			break
		}
		if symIdx < len(record) {
			sym := strings.TrimSpace(record[symIdx])
			if sym != "" {
				seen[sym] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// ExpandUniverse reads the replay CSV and returns all symbols matching
// the given prefix patterns. When prefixes is nil or empty, all CSV symbols
// are returned. This allows agent universes to auto-expand as new symbols
// are added to the replay data.
func ExpandUniverse(csvPath string, prefixes []string) []string {
	csvSymbols := loadSymbolsFromCSV(csvPath)
	if len(csvSymbols) == 0 {
		return nil
	}
	if len(prefixes) == 0 {
		return csvSymbols
	}
	var result []string
	for _, sym := range csvSymbols {
		for _, prefix := range prefixes {
			if strings.HasPrefix(sym, prefix) {
				result = append(result, sym)
				break
			}
		}
	}
	return result
}

func RegistrySymbols(registry domain.AgentRegistry) []string {
	seen := make(map[string]struct{})
	symbols := make([]string, 0)

	for _, symbol := range DefaultSymbols() {
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		for _, symbol := range agent.Universe {
			if _, ok := seen[symbol]; ok {
				continue
			}
			seen[symbol] = struct{}{}
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

func SymbolsForSkill(registry domain.AgentRegistry, skill string) []string {
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Skill != skill {
			continue
		}
		if len(agent.Universe) > 0 {
			return agent.Universe
		}
		break
	}
	return DefaultSymbols()
}

func symbolIterator(symbols []string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for _, symbol := range symbols {
			if !yield(symbol) {
				return
			}
		}
	}
}
