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
	flag.Parse()

	res, err := config.ValidateCalibration(*path, *maxAge)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(2)
	}

	if *format == "json" {
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("OK=%v segments=%d L1=%d L2=%d updated_at=%s mtime=%s stale_by=%s\n",
			res.OK, res.SegmentsCount, res.L1Count, res.L2Count,
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
