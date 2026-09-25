package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
)

func main() {
	path := flag.String("path", constants.ParametersFile, "path to params.json to validate")
	maxAge := flag.Duration("max-age", 48*time.Hour, "max age for params.json before it's considered stale")
	format := flag.String("format", "text", "output format: text|json")
	policyPath := flag.String("policy", "", "path to a validation policy JSON (scope + accepted findings). "+
		"Empty = fail-closed: full freshness scope, no accepted findings, every finding fails the run")
	flag.Parse()

	opts := config.CalibrationValidationOptions{MaxAge: *maxAge}
	if *policyPath != "" {
		policy, err := config.LoadCalibrationValidationPolicy(*policyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			os.Exit(2)
		}
		opts.Policy = policy
	}

	res, err := config.ValidateCalibrationWithOptions(*path, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(2)
	}

	if *format == "json" {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	} else {
		// Issue #1944 Batch 4 (I31): the header states which policy the verdict
		// was produced under, so "OK=false" alone can no longer be misread as
		// "the tree regressed" when it only means "this checkout is stale".
		fmt.Printf("OK=%v scope=%s freshness_enforced=%v errors=%d observations=%d status=%s\n",
			res.OK, res.Scope, res.FreshnessEnforced, res.ErrorCount, res.ObservationCount, status(res))
		fmt.Printf("segments=%d L1=%d L2=%d updated_at=%s mtime=%s stale_by=%s\n",
			res.SegmentsCount, res.L1Count, res.L2Count,
			res.UpdatedAt.Format(time.RFC3339),
			res.FileMTime.Format(time.RFC3339),
			res.StaleBy.Truncate(time.Minute))
		// Issue #1944 Batch 3 (I31): findings carry stable codes so callers (and
		// the operator reading the artifact) can classify a failure without
		// parsing message text. Falls back to the legacy message list when a
		// caller supplied issues without findings.
		switch {
		case len(res.Findings) > 0:
			for _, f := range res.Findings {
				if f.Segment != "" {
					fmt.Printf("  - [%s][%s] segment=%s %s\n", f.Code, f.Severity, f.Segment, f.Message)
					continue
				}
				fmt.Printf("  - [%s][%s] %s\n", f.Code, f.Severity, f.Message)
			}
		default:
			for _, iss := range res.Issues {
				fmt.Printf("  - %s\n", iss)
			}
		}
	}

	if !res.OK {
		os.Exit(1)
	}
}

// status renders the machine-readable verdict class, so the nightly job (and its
// annotations) never confuse "structure regressed" with "checkout is stale".
func status(res *config.CalibrationValidationResult) string {
	switch {
	case res.OK && res.ObservationCount > 0:
		return "passed_with_observations"
	case res.OK:
		return "passed"
	case res.FreshnessEnforced:
		return "failed"
	default:
		return "failed_structure"
	}
}
