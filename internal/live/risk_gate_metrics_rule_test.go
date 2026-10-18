package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/reporoot"
)

// TestRiskGateHaltMetricHasARuleAndAFixture is the anti-rot guard for the
// FU-20261016-32 condition-3 pair (exposure + alert path).
//
// The metric name lives in this package (internal/live cannot import
// internal/monitoring, which already imports internal/live/store), while the
// alert rule and its promtool fixture are plain YAML that nothing type-checks
// against Go. Renaming the constant would otherwise leave the rule pointing at a
// series nobody emits — a rule that silently never fires, which is the shape this
// repo treats as worse than no rule at all (#1972/#1975, FU-20261016-10).
//
// Both halves are asserted: the rule file must name the series, and the promtool
// fixture must exist and name it too (a rule without a fixture has no proof that
// it can fire).
func TestRiskGateHaltMetricHasARuleAndAFixture(t *testing.T) {
	root, err := reporoot.FromCwd()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	cases := []struct {
		name string
		rel  string
	}{
		{"alert rule", filepath.Join("monitoring", "rules", "atlas_live_risk_gate_alerts.yml")},
		{"promtool fixture", filepath.Join("monitoring", "tests", "atlas_live_risk_gate_test.yml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.rel)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			if !strings.Contains(string(data), MetricRiskGateHaltTrading) {
				t.Fatalf("%s does not name %q: the %s and the metric have drifted apart",
					path, MetricRiskGateHaltTrading, tc.name)
			}
		})
	}
}
