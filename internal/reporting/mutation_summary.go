package reporting

import (
	"fmt"
	"strings"
)

// MutationStats holds aggregate counts for mutation survival analysis.
type MutationStats struct {
	Total      int
	Kept       int
	Reverted   int
	Pending    int
	SurvivalRate float64
}

// RenderMutationSummary generates a Markdown paragraph with mutation statistics.
func RenderMutationSummary(stats MutationStats) string {
	if stats.Total == 0 {
		return "_No mutation experiments recorded in this window._\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- **Total Experiments:** %d\n", stats.Total))
	sb.WriteString(fmt.Sprintf("- **Kept / Promoted:** %d\n", stats.Kept))
	sb.WriteString(fmt.Sprintf("- **Reverted:** %d\n", stats.Reverted))
	sb.WriteString(fmt.Sprintf("- **Pending / In-Flight:** %d\n", stats.Pending))
	sb.WriteString(fmt.Sprintf("- **Survival Rate:** %.1f%%\n", stats.SurvivalRate*100))
	return sb.String()
}
