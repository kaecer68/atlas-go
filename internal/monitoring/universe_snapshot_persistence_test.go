package monitoring

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// This file is the regression net for the gap the 2026-09-27 audit found: a run
// that COMPUTES a healthy verdict but whose output never reaches
// data/state/universe_snapshot.json.
//
// Why the existing nets could not see it:
//   - universe_metrics_emission_test.go pins the counters and the verdict
//     gauges. Every one of those is published in memory (from the single defer),
//     so a failed write leaves them all healthy — "the verdict says 150 ranked"
//     and "the artifact on disk says 150 ranked" are different statements.
//   - AtlasUniverseRunOverdue only covers "the scheduler never fired", and
//     AtlasUniverseRankedZero only covers "it fired and produced nothing".
//
// The assertions below are made on the /metrics TEXT (the artifact the alert
// rules actually read), and TestSnapshotPersistedOnDisk_* pins the measurement
// itself against the filesystem, including the negative control (an OLD file at
// the canonical path must NOT count as this run's output).

func TestSnapshotPersistedOnDisk(t *testing.T) {
	workDir := t.TempDir()
	runStart := time.Now()
	path := UniverseSnapshotPath(workDir)

	t.Run("missing artifact is false, not an error", func(t *testing.T) {
		if SnapshotPersistedOnDisk(workDir, runStart) {
			t.Fatal("a workDir with no snapshot reported the artifact as persisted")
		}
	})

	t.Run("artifact written by this run is true", func(t *testing.T) {
		if err := SaveUniverseSnapshot(workDir, &UniverseBuildResult{Timestamp: runStart}, nil); err != nil {
			t.Fatalf("SaveUniverseSnapshot: %v", err)
		}
		if !SnapshotPersistedOnDisk(workDir, runStart) {
			t.Fatalf("freshly written %s was not counted as this run's output", path)
		}
	})

	t.Run("artifact from an EARLIER run is false (the stale-output shape)", func(t *testing.T) {
		// A previous run's file, left in place because this run's write failed.
		old := runStart.Add(-48 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
		if SnapshotPersistedOnDisk(workDir, runStart) {
			t.Fatal("a 48h-old snapshot was reported as this run's output: the stale-output defect would be silent")
		}
	})
}

// TestBuildUniverse_PublishesSnapshotPersistedOnMetrics is the end-to-end half:
// a healthy run must publish the gauge as 1 for its own stage (and leave the
// other stage at its warm-up 0), through the REAL wiring (CollectorOnInc +
// GaugeSink + WarmUp) into the REAL /metrics text.
func TestBuildUniverse_PublishesSnapshotPersistedOnMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("healthy daily run", func(t *testing.T) {
		collector := NewMetricsCollector()
		um := metrics.NewUniverseMetrics()
		um.SetOnInc(CollectorOnInc(collector))
		um.SetRunVerdictSink(GaugeSink(collector))
		um.WarmUp()

		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)
		deps.UniverseMetrics = um

		if _, _, err := BuildUniverse(ctx, deps, false); err != nil {
			t.Fatalf("BuildUniverse: %v", err)
		}
		body := scrapeUniverseMetrics(t, collector)
		wantMetricLine(t, body, `atlas_universe_last_run_snapshot_persisted{stage="daily"} 1.000000`)
		wantMetricLine(t, body, `atlas_universe_last_run_snapshot_persisted{stage="weekly"} 0.000000`)

		// The reading must agree with the file system, not with the write call:
		// stat is the second, independent witness.
		if !SnapshotPersistedOnDisk(workDir, time.Now().Add(-1*time.Hour)) {
			t.Errorf("%s exists but the artifact check says it is not there", UniverseSnapshotPath(workDir))
		}
	})

	t.Run("the gauge tracks the artifact, not the intent", func(t *testing.T) {
		// Negative control / mutation half. The write cannot land (the workDir is
		// a regular file, so MkdirAll on <workDir>/data/state fails), while the
		// run itself is healthy: ranked > 0, trustworthy, counters emitted.
		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0o640); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		workDir := filepath.Join(blocker, "work")

		collector := NewMetricsCollector()
		um := metrics.NewUniverseMetrics()
		um.SetOnInc(CollectorOnInc(collector))
		um.SetRunVerdictSink(GaugeSink(collector))
		um.WarmUp()

		deps := buildDepsFixture(t, workDir)
		deps.UniverseMetrics = um

		result, ranked, err := BuildUniverse(ctx, deps, false)
		if err != nil {
			t.Fatalf("BuildUniverse: %v", err)
		}
		// Guard: the fixture must still be a HEALTHY run, otherwise the assertion
		// below would pass for the wrong reason (an unhealthy run publishes 0 too).
		if len(ranked) == 0 || !result.RankedTrustworthy {
			t.Fatalf("fixture did not stay healthy: ranked=%d trustworthy=%v", len(ranked), result.RankedTrustworthy)
		}

		body := scrapeUniverseMetrics(t, collector)
		// The two signals that made the 2026-09-25 failure invisible...
		wantMetricLine(t, body,
			"atlas_universe_last_run_symbols_ranked{stage=\"daily\"} "+strconv.Itoa(len(ranked))+".000000")
		wantMetricLine(t, body, `atlas_universe_last_run_valid{stage="daily"} 1.000000`)
		wantMetricLine(t, body, `atlas_universe_last_run_ranked_trustworthy{stage="daily"} 1.000000`)
		// ...and the one that sees it.
		wantMetricLine(t, body, `atlas_universe_last_run_snapshot_persisted{stage="daily"} 0.000000`)

		if _, err := os.Stat(UniverseSnapshotPath(workDir)); err == nil {
			t.Fatalf("%s exists although the write was supposed to fail", UniverseSnapshotPath(workDir))
		}
	})
}

// TestSnapshotPersisted_WarmUpIsZeroNotOne pins the warm-up value. If warm-up
// published 1, a process that has never written an artifact would look like a
// process whose artifact is healthy, and the alert would be dead from boot.
func TestSnapshotPersisted_WarmUpIsZeroNotOne(t *testing.T) {
	collector := NewMetricsCollector()
	um := metrics.NewUniverseMetrics()
	um.SetOnInc(CollectorOnInc(collector))
	um.SetRunVerdictSink(GaugeSink(collector))
	um.WarmUp()

	body := scrapeUniverseMetrics(t, collector)
	for _, stage := range []string{"daily", "weekly"} {
		wantMetricLine(t, body,
			`atlas_universe_last_run_snapshot_persisted{stage="`+stage+`"} 0.000000`)
	}
	if strings.Contains(body, `atlas_universe_last_run_snapshot_persisted{stage="daily"} 1.000000`) {
		t.Error("warm-up published 1 for a process that has not written any artifact")
	}
}
