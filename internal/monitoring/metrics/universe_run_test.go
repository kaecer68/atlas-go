package metrics

import (
	"testing"
	"time"
)

// recordingSink captures every gauge sample a report pushes.
type recordingSink struct {
	samples []gaugeSample
}

type gaugeSample struct {
	name   string
	value  float64
	labels map[string]string
}

func (s *recordingSink) sink() UniverseRunVerdictSink {
	return func(name string, value float64, labels map[string]string) {
		s.samples = append(s.samples, gaugeSample{name: name, value: value, labels: labels})
	}
}

func (s *recordingSink) last(name string, want map[string]string) (float64, bool) {
	for i := len(s.samples) - 1; i >= 0; i-- {
		sample := s.samples[i]
		if sample.name != name {
			continue
		}
		if len(sample.labels) != len(want) {
			continue
		}
		match := true
		for k, v := range want {
			if sample.labels[k] != v {
				match = false
				break
			}
		}
		if match {
			return sample.value, true
		}
	}
	return 0, false
}

// TestReportRun_PublishesVerdictWithoutCounterWiring is the regression test for
// the 2026-09-25 production shape: the counter path (CounterVec/OnInc) is
// broken — here it is simply never wired — and the verdict must still arrive,
// because it is the signal the alert rules use to decide whether the pipeline
// produced anything.
func TestReportRun_PublishesVerdictWithoutCounterWiring(t *testing.T) {
	sink := &recordingSink{}
	um := NewUniverseMetrics()
	um.SetRunVerdictSink(sink.sink())
	// Deliberately NO SetOnInc: the counter bridge is dead in this test.

	at := time.Date(2026, 9, 29, 6, 0, 30, 0, time.UTC)
	um.ReportRun(UniverseRunVerdict{
		Stage:           UniverseStageDaily,
		Outcome:         "ok",
		Gathered:        1599,
		Filtered:        1599,
		ScreenedPassed:  150,
		ScreenedFailed:  1449,
		Ranked:          150,
		QuotesRequested: 1599,
		QuotesReturned:  1581,
		Trustworthy:     true,
		// Both artifacts persisted. The two fields travel on the same verdict but
		// reach /metrics through separate series, so each has to be asserted: a
		// ReportRun that forgot one of them would leave that artifact's rule with
		// no input at all (and, because both are published from this one call,
		// nothing else in the suite would notice).
		SnapshotPersisted: true,
		RegistryPersisted: true,
		FinishedAt:        at,
	})

	for name, want := range map[string]float64{
		UniverseMetricLastRunValid:             1,
		UniverseMetricLastRunFinished:          float64(at.Unix()),
		UniverseMetricLastRunGathered:          1599,
		UniverseMetricLastRunFiltered:          1599,
		UniverseMetricLastRunScreenedPassed:    150,
		UniverseMetricLastRunScreenedFailed:    1449,
		UniverseMetricLastRunRanked:            150,
		UniverseMetricLastRunQuotesRequested:   1599,
		UniverseMetricLastRunQuotesReturned:    1581,
		UniverseMetricLastRunTrustworthy:       1,
		UniverseMetricLastRunSnapshotPersisted: 1,
		UniverseMetricLastRunRegistryPersisted: 1,
	} {
		got, ok := sink.last(name, map[string]string{UniverseStageLabel: UniverseStageDaily})
		if !ok {
			t.Errorf("%s{stage=daily} was never reported", name)
			continue
		}
		if got != want {
			t.Errorf("%s{stage=daily} = %v, want %v", name, got, want)
		}
	}

	outcome, ok := sink.last(UniverseMetricLastRunOutcome,
		map[string]string{UniverseStageLabel: UniverseStageDaily, "outcome": "ok"})
	if !ok || outcome != 1 {
		t.Errorf("outcome{daily,ok} = %v (present=%v), want 1", outcome, ok)
	}
}

// TestReportRun_OutcomeIsOneHot pins the append-only-sink contract: the previous
// outcome value of a stage is zeroed before the new one is published, so
// "the current outcome" is always the series with value 1.
func TestReportRun_OutcomeIsOneHot(t *testing.T) {
	sink := &recordingSink{}
	um := NewUniverseMetrics()
	um.SetRunVerdictSink(sink.sink())

	um.ReportRun(UniverseRunVerdict{Stage: UniverseStageDaily, Outcome: "quotes_mock"})
	um.ReportRun(UniverseRunVerdict{Stage: UniverseStageDaily, Outcome: "ok"})

	if v, ok := sink.last(UniverseMetricLastRunOutcome,
		map[string]string{UniverseStageLabel: UniverseStageDaily, "outcome": "quotes_mock"}); !ok || v != 0 {
		t.Errorf("previous outcome {daily,quotes_mock} = %v (present=%v), want 0", v, ok)
	}
	if v, ok := sink.last(UniverseMetricLastRunOutcome,
		map[string]string{UniverseStageLabel: UniverseStageDaily, "outcome": "ok"}); !ok || v != 1 {
		t.Errorf("current outcome {daily,ok} = %v (present=%v), want 1", v, ok)
	}
	// The other stage is untouched: one-hot per stage, not globally.
	um.ReportRun(UniverseRunVerdict{Stage: UniverseStageWeekly, Outcome: "ok"})
	if v, ok := sink.last(UniverseMetricLastRunOutcome,
		map[string]string{UniverseStageLabel: UniverseStageDaily, "outcome": "ok"}); !ok || v != 1 {
		t.Errorf("daily outcome changed by a weekly run: %v (present=%v)", v, ok)
	}
}

// TestWarmUpRunVerdicts_ValidZeroAndTimestampAtStart pins what a consumer may
// read before the first run: the family exists, valid is 0, and the finished
// timestamp is the warm-up instant (never 1970, which would make any
// "time() - finished" rule fire on a freshly started process).
func TestWarmUpRunVerdicts_ValidZeroAndTimestampAtStart(t *testing.T) {
	sink := &recordingSink{}
	um := NewUniverseMetrics()
	um.SetRunVerdictSink(sink.sink())

	start := time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)
	um.warmUpRunVerdicts(start)

	for _, stage := range []string{UniverseStageDaily, UniverseStageWeekly} {
		labels := map[string]string{UniverseStageLabel: stage}
		if v, ok := sink.last(UniverseMetricLastRunValid, labels); !ok || v != 0 {
			t.Errorf("valid{%s} = %v (present=%v), want 0", stage, v, ok)
		}
		if v, ok := sink.last(UniverseMetricLastRunFinished, labels); !ok || v != float64(start.Unix()) {
			t.Errorf("finished{%s} = %v (present=%v), want %v", stage, v, ok, start.Unix())
		}
		if v, ok := sink.last(UniverseMetricLastRunRanked, labels); !ok || v != 0 {
			t.Errorf("ranked{%s} = %v (present=%v), want 0", stage, v, ok)
		}
		// Both artifact gauges warm up to 0 for the same reason: at process start
		// neither file has been written BY THIS PROCESS, so claiming 1 would be a
		// claim about a file nobody checked. (The registry rule reads
		// snapshot_persisted == 1 AND registry_persisted == 0, so a wrong warm-up
		// value here would make it fire on every restart.)
		if v, ok := sink.last(UniverseMetricLastRunSnapshotPersisted, labels); !ok || v != 0 {
			t.Errorf("snapshot_persisted{%s} = %v (present=%v), want 0", stage, v, ok)
		}
		if v, ok := sink.last(UniverseMetricLastRunRegistryPersisted, labels); !ok || v != 0 {
			t.Errorf("registry_persisted{%s} = %v (present=%v), want 0", stage, v, ok)
		}
	}
}

// TestReportRun_SinkBehaviours covers the nil contracts the wiring relies on.
func TestReportRun_SinkBehaviours(t *testing.T) {
	// No sink: no panic, nothing recorded.
	um := NewUniverseMetrics()
	um.ReportRun(UniverseRunVerdict{Stage: UniverseStageDaily})
	um.ReportNextRun(time.Now())

	// Nil receiver: same.
	var nilUM *UniverseMetrics
	nilUM.SetRunVerdictSink(func(string, float64, map[string]string) {})
	nilUM.ReportRun(UniverseRunVerdict{Stage: UniverseStageDaily})

	// Zero "next run" is dropped instead of publishing 1970.
	sink := &recordingSink{}
	wired := NewUniverseMetrics()
	wired.SetRunVerdictSink(sink.sink())
	wired.ReportNextRun(time.Time{})
	if _, ok := sink.last(UniverseMetricNextRun, map[string]string{}); ok {
		t.Error("a zero next-run instant was published")
	}
	at := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	wired.ReportNextRun(at)
	if v, ok := sink.last(UniverseMetricNextRun, map[string]string{}); !ok || v != float64(at.Unix()) {
		t.Errorf("next_run = %v (present=%v), want %v", v, ok, at.Unix())
	}
}
