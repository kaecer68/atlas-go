package calibration

import (
	"fmt"
	"strings"
	"time"
)

func FormatReport(results []CalibrationResult, verbose bool) string {
	var b strings.Builder
	fmt.Fprintln(&b, "=== Parameter Calibration Report ===")
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().Format(time.RFC3339))

	totalParams := 0
	for _, res := range results {
		fmt.Fprintf(&b, "Module: %s\n", res.Module)
		fmt.Fprintln(&b, strings.Repeat("-", 60))

		if len(res.Errors) > 0 {
			fmt.Fprintf(&b, "  [ERRORS]\n")
			for _, e := range res.Errors {
				fmt.Fprintf(&b, "    - %s\n", e)
			}
			fmt.Fprintln(&b)
			continue
		}

		for _, p := range res.Parameters {
			totalParams++
			sigMark := ""
			if p.Significant {
				sigMark = " *"
			}
			fmt.Fprintf(&b, "  %-35s  before=%-10.6f  after=%-10.6f%s\n", p.Path, p.Before, p.After, sigMark)
			fmt.Fprintf(&b, "    method=%-20s confidence=%-5.2f  n=%d\n", p.Method, p.Confidence, p.SampleSize)
			if p.CalibrationNotes != "" {
				fmt.Fprintf(&b, "    notes: %s\n", p.CalibrationNotes)
			}
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintf(&b, "Total parameters calibrated: %d\n", totalParams)
	if verbose {
		fmt.Fprintln(&b, "\n[*] indicates statistically significant change (>10% relative)")
	}
	return b.String()
}
