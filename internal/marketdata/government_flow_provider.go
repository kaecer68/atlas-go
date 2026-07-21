package marketdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GovernmentFlowReading is a single daily reading of aggregated 官股行庫 net
// buy/sell in TWD. Source identifies the provenance per the methodology in
// docs/specs/government-force-proxy-spec.md.
type GovernmentFlowReading struct {
	Date     string `json:"date"`
	TotalNet int64  `json:"total_net"` // TWD, positive = net buy, negative = net sell
	Source   string `json:"source"`    // operator-imported | broker-aggregate | media-curated
	RawURL   string `json:"raw_url,omitempty"`
}

// GovernmentFlowProvider reads operator-imported daily readings from a flat
// directory of YYYYMMDD.json files. The data source itself (broker-branch
// aggregation or third-party) is backlog BK-13/14 — this provider is the
// honest file-driven seam that lights up once the data exists.
type GovernmentFlowProvider struct {
	dir string
}

// NewGovernmentFlowProvider creates a provider rooted at dir.
func NewGovernmentFlowProvider(dir string) *GovernmentFlowProvider {
	return &GovernmentFlowProvider{dir: dir}
}

// AllowedSources enumerates accepted source labels per the methodology doc.
var GovernmentFlowAllowedSources = map[string]bool{
	"operator-imported": true,
	"broker-aggregate":  true,
	"media-curated":     true,
}

// Latest returns the most recent reading or (zero, false, nil) if the
// directory is empty or unreadable. An invalid reading (missing date /
// unknown source) returns an error so the channel marks degraded rather
// than silently exposing garbage data.
func (p *GovernmentFlowProvider) Latest() (GovernmentFlowReading, bool, error) {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return GovernmentFlowReading{}, false, nil
		}
		return GovernmentFlowReading{}, false, fmt.Errorf("government_flow read dir: %w", err)
	}
	dates := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || len(name) != len("YYYYMMDD.json") {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if !isDateYYYYMMDD(date) {
			continue
		}
		dates = append(dates, date)
	}
	if len(dates) == 0 {
		return GovernmentFlowReading{}, false, nil
	}
	sort.Strings(dates)
	latest := dates[len(dates)-1]

	raw, err := os.ReadFile(filepath.Join(p.dir, latest+".json"))
	if err != nil {
		return GovernmentFlowReading{}, false, fmt.Errorf("government_flow read %s: %w", latest, err)
	}
	var r GovernmentFlowReading
	if err := json.Unmarshal(raw, &r); err != nil {
		return GovernmentFlowReading{}, false, fmt.Errorf("government_flow decode %s: %w", latest, err)
	}
	if r.Date == "" {
		r.Date = latest
	}
	if !GovernmentFlowAllowedSources[r.Source] {
		return GovernmentFlowReading{}, false, fmt.Errorf("government_flow %s: unknown source %q", latest, r.Source)
	}
	return r, true, nil
}

func isDateYYYYMMDD(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
