package monitoring

import (
	"context"
	"errors"
	"io/fs"
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

	t.Run("registry stale while the snapshot is fresh (the shape gap B was about)", func(t *testing.T) {
		// The world the registry gauge exists for: the SAME run writes two files,
		// one of them lands and the other does not. It is arranged by occupying the
		// registry's TEMPORARY path with a directory, so WriteUniverseRegistry
		// fails at its first write while SaveUniverseSnapshot (a different path,
		// and a different temp file) succeeds.
		//
		// Why not occupy the registry path itself: WriteUniverseRegistry moves an
		// existing path aside with `os.Rename(path, path+".bak")` first, and POSIX
		// renames a directory just as happily as a file — so the write would
		// SUCCEED and the test would assert the wrong world (measured, not
		// assumed: the first version of this subtest passed for that reason and
		// then failed on the fixture-drift assertion).
		//
		// Before the registry gauge, this state produced one WARN line and no
		// signal a rule could read: two consumers of one run, reading two
		// different mother universes, and every verdict said healthy.
		workDir := tempDir(t)
		registryPath := UniverseRegistryPath(workDir)
		registryTmp := registryPath + ".tmp"
		if err := os.MkdirAll(registryTmp, 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		logs := captureLogs(t)

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
		// Guard the fixture: the run must stay healthy, otherwise the two gauges
		// would read 0/0 and the assertion could pass for the wrong reason.
		if len(ranked) == 0 || !result.RankedTrustworthy {
			t.Fatalf("fixture did not stay healthy: ranked=%d trustworthy=%v", len(ranked), result.RankedTrustworthy)
		}

		body := scrapeUniverseMetrics(t, collector)
		// The snapshot landed...
		wantMetricLine(t, body, `atlas_universe_last_run_snapshot_persisted{stage="daily"} 1.000000`)
		// ...and the registry did not.
		wantMetricLine(t, body, `atlas_universe_last_run_registry_persisted{stage="daily"} 0.000000`)
		// The other stage is untouched: a daily failure must not claim anything
		// about the weekly artifact.
		wantMetricLine(t, body, `atlas_universe_last_run_registry_persisted{stage="weekly"} 0.000000`)

		if _, err := os.Stat(UniverseSnapshotPath(workDir)); err != nil {
			t.Errorf("the snapshot should have landed: %v", err)
		}
		if _, err := os.Stat(registryPath); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("fixture should have blocked the registry write: %s exists (err=%v)", registryPath, err)
		}
		// The failure must also be readable in the log stream: the WARN that used
		// to be the ONLY trace, plus the new "not persisted" line that names the
		// artifact and whether the write reported an error.
		for _, want := range []string{"universe_registry_write_error", "universe_registry_not_persisted"} {
			if !strings.Contains(logs.String(), want) {
				t.Errorf("log is missing %q: %s", want, logs.String())
			}
		}
		// Disk is the second witness for both readings, exactly as for the
		// snapshot gauge: stat must agree with the pair of gauges.
		if !SnapshotPersistedOnDisk(workDir, result.Timestamp) {
			t.Error("snapshot gauge says 1 but the filesystem says the artifact is not this run's")
		}
		if RegistryPersistedOnDisk(workDir, result.Timestamp) {
			t.Error("registry gauge says 0 but the filesystem says the registry is this run's")
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
