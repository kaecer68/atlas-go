package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kaecer68/atlas-go/internal/calibration"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "calibrate-parameters:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("calibrate-parameters", flag.ContinueOnError)
	module := fs.String("module", "all", "Module: garch, var, darwinian, factor, all")
	dryRun := fs.Bool("dry-run", false, "Preview changes without saving")
	dataPath := fs.String("data", "", "Replay path (CSV or JSONL). Defaults to ATLAS_REPLAY_DATA_PATH")
	verbose := fs.Bool("v", false, "Verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	workDir, _ := os.Getwd()
	report, err := calibration.Run(workDir, *module, *dataPath, *dryRun, *verbose)
	if err != nil {
		return err
	}
	fmt.Print(report)
	return nil
}
