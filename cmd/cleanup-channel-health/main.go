package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func main() {
	workDir := flag.String("work-dir", "", "Atlas work directory (required)")
	maxAge := flag.String("max-age", "168h", "Maximum age of alerts to keep")
	flag.Parse()

	if *workDir == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --work-dir is required")
		flag.Usage()
		os.Exit(1)
	}

	dur, err := time.ParseDuration(*maxAge)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid --max-age: %v\n", err)
		os.Exit(1)
	}

	store, err := monitoring.NewAlertStore(filepath.Join(*workDir, "data/state/alerts"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: open alert store: %v\n", err)
		os.Exit(1)
	}

	cutoff := time.Now().Add(-dur)
	deleted, err := store.DeleteWhere(func(r *domain.AlertRecord) bool {
		return r.Timestamp.Before(cutoff)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: delete failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted %d stale alerts\n", deleted)
}
