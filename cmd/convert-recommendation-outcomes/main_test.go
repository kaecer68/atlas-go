package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestConvertRootLevelFile(t *testing.T) {
	dir := t.TempDir()

	legacyLine := `{"AgentID":"agent-1","Skill":"growth_momentum","Layer":"style","Symbol":"2330.TW","Side":"BUY","Conviction":87,"TargetPrice":1050.5,"StopLossPrice":980.2,"Window":"1d","ForwardReturn":0.021,"BenchmarkDelta":0.011,"Hit":true,"Reason":"breakout","Price":1001.0,"PassedGuards":true,"GuardReason":"ok","RecordedAt":"2026-04-22T04:02:30.434394+08:00"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(legacyLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if strings.Contains(string(rewritten), `"AgentID"`) {
		t.Fatalf("expected PascalCase removed, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"agent_id"`) {
		t.Fatalf("expected canonical snake_case agent_id, got: %s", rewritten)
	}

	var outcome domain.RecommendationOutcome
	if err := json.Unmarshal(rewritten, &outcome); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if outcome.AgentID != "agent-1" {
		t.Fatalf("agent_id mismatch: got %q", outcome.AgentID)
	}
}

func TestConvertSessionLevelFile(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "session-20260101")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	legacyLine := `{"AgentID":"agent-2","Skill":"value_yield","Layer":"style","Symbol":"2317.TW","Side":"BUY","Conviction":72,"TargetPrice":155.5,"StopLossPrice":146.0,"Window":"5d","ForwardReturn":0.015,"BenchmarkDelta":0.004,"Hit":true,"Reason":"cheap","Price":150.0,"PassedGuards":true,"GuardReason":"ok","RecordedAt":"2026-04-22T04:02:30.434394+08:00"}` + "\n"
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(legacyLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if strings.Contains(string(rewritten), `"AgentID"`) {
		t.Fatalf("expected PascalCase removed from session file, got: %s", rewritten)
	}
	if !strings.Contains(string(rewritten), `"agent_id"`) {
		t.Fatalf("expected canonical snake_case in session file, got: %s", rewritten)
	}
}

func TestConvertPreservesLineCount(t *testing.T) {
	dir := t.TempDir()

	lines := []string{
		`{"agent_id":"a1","skill":"s1","layer":"sector","symbol":"2330.TW","side":"BUY","conviction":80,"target_price":600,"stop_loss_price":550,"window":"1d","forward_return":0.02,"benchmark_delta":0.01,"hit":true,"reason":"r1","price":580,"passed_guards":true,"guard_reason":"ok","recorded_at":"2026-01-01T00:00:00Z"}`,
		`{"agent_id":"a2","skill":"s2","layer":"style","symbol":"2317.TW","side":"SELL","conviction":60,"target_price":200,"stop_loss_price":190,"window":"5d","forward_return":-0.01,"benchmark_delta":0.00,"hit":false,"reason":"r2","price":195,"passed_guards":false,"guard_reason":"low_conviction","recorded_at":"2026-01-02T00:00:00Z"}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	rewrittenLines := strings.Split(strings.TrimSuffix(string(rewritten), "\n"), "\n")
	if len(rewrittenLines) != len(lines) {
		t.Fatalf("expected %d lines, got %d", len(lines), len(rewrittenLines))
	}
}

func TestConvertPreservesOrder(t *testing.T) {
	dir := t.TempDir()

	lines := []string{
		`{"agent_id":"first","skill":"s1","layer":"sector","symbol":"2330.TW","side":"BUY","conviction":80,"target_price":600,"stop_loss_price":550,"window":"1d","forward_return":0.02,"benchmark_delta":0.01,"hit":true,"reason":"r1","price":580,"passed_guards":true,"guard_reason":"ok","recorded_at":"2026-01-01T00:00:00Z"}`,
		`{"agent_id":"second","skill":"s2","layer":"style","symbol":"2317.TW","side":"SELL","conviction":60,"target_price":200,"stop_loss_price":190,"window":"5d","forward_return":-0.01,"benchmark_delta":0.00,"hit":false,"reason":"r2","price":195,"passed_guards":false,"guard_reason":"low_conviction","recorded_at":"2026-01-02T00:00:00Z"}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	rewrittenLines := strings.Split(strings.TrimSuffix(string(rewritten), "\n"), "\n")
	for i, line := range rewrittenLines {
		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(line), &outcome); err != nil {
			t.Fatalf("unmarshal line %d: %v", i, err)
		}
		switch i {
		case 0:
			if outcome.AgentID != "first" {
				t.Fatalf("line 0: expected first, got %s", outcome.AgentID)
			}
		case 1:
			if outcome.AgentID != "second" {
				t.Fatalf("line 1: expected second, got %s", outcome.AgentID)
			}
		}
	}
}

func TestConvertAlreadyCanonical(t *testing.T) {
	dir := t.TempDir()

	outcome := domain.RecommendationOutcome{
		AgentID:       "agent-3",
		Skill:         "semiconductor_desk",
		Layer:         domain.LayerSector,
		Symbol:        "2330.TW",
		Side:          domain.SideBuy,
		Conviction:    91,
		TargetPrice:   1100,
		StopLossPrice: 980,
		Window:        "1d",
		ForwardReturn: 0.031,
		Hit:           true,
		Reason:        "earnings",
		Price:         1020,
		PassedGuards:  true,
		GuardReason:   "ok",
		RecordedAt:    time.Now().UTC().Truncate(time.Second),
		FactorScores:  domain.FactorScores{Total: 0.93},
	}

	data, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	var decoded domain.RecommendationOutcome
	if err := json.Unmarshal(rewritten, &decoded); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if decoded.AgentID != outcome.AgentID {
		t.Fatalf("agent_id mismatch: got %q", decoded.AgentID)
	}
	if decoded.FactorScores.Total != 0.93 {
		t.Fatalf("factor_scores.total mismatch: got %v", decoded.FactorScores.Total)
	}
}

func TestConvertSkipsEmptyFile(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	stat, err := os.Stat(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if stat.Size() != 0 {
		t.Fatalf("expected empty file to remain empty, got size %d", stat.Size())
	}
}

func TestConvertRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(`not valid json`+"\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"-dir", dir}, &stdout)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "recommendation_outcomes.jsonl") {
		t.Fatalf("expected error to mention file path, got: %v", err)
	}
}

func TestConvertMultipleSessions(t *testing.T) {
	dir := t.TempDir()

	// Root file
	rootLine := `{"AgentID":"root-agent","Skill":"test","Layer":"sector","Symbol":"2330.TW","Side":"BUY","Conviction":80,"TargetPrice":600,"StopLossPrice":550,"Window":"1d","ForwardReturn":0.02,"BenchmarkDelta":0.01,"Hit":true,"Reason":"test","Price":580,"PassedGuards":true,"GuardReason":"ok","RecordedAt":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(rootLine), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}

	session1Dir := filepath.Join(dir, "sessions", "session-20260101")
	if err := os.MkdirAll(session1Dir, 0o755); err != nil {
		t.Fatalf("mkdir session1: %v", err)
	}
	session1Line := `{"AgentID":"s1-agent","Skill":"test","Layer":"style","Symbol":"2317.TW","Side":"SELL","Conviction":60,"TargetPrice":200,"StopLossPrice":190,"Window":"5d","ForwardReturn":-0.01,"BenchmarkDelta":0.00,"Hit":false,"Reason":"test","Price":195,"PassedGuards":false,"GuardReason":"low","RecordedAt":"2026-01-02T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(session1Dir, "recommendation_outcomes.jsonl"), []byte(session1Line), 0o644); err != nil {
		t.Fatalf("write session1 file: %v", err)
	}

	session2Dir := filepath.Join(dir, "sessions", "session-20260102")
	if err := os.MkdirAll(session2Dir, 0o755); err != nil {
		t.Fatalf("mkdir session2: %v", err)
	}
	session2Line := `{"AgentID":"s2-agent","Skill":"test","Layer":"control","Symbol":"2454.TW","Side":"BUY","Conviction":90,"TargetPrice":300,"StopLossPrice":280,"Window":"1d","ForwardReturn":0.03,"BenchmarkDelta":0.02,"Hit":true,"Reason":"test","Price":290,"PassedGuards":true,"GuardReason":"ok","RecordedAt":"2026-01-03T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(session2Dir, "recommendation_outcomes.jsonl"), []byte(session2Line), 0o644); err != nil {
		t.Fatalf("write session2 file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	for _, path := range []string{
		filepath.Join(dir, "recommendation_outcomes.jsonl"),
		filepath.Join(session1Dir, "recommendation_outcomes.jsonl"),
		filepath.Join(session2Dir, "recommendation_outcomes.jsonl"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), `"AgentID"`) {
			t.Fatalf("expected PascalCase removed in %s", path)
		}
		if !strings.Contains(string(data), `"agent_id"`) {
			t.Fatalf("expected canonical snake_case in %s", path)
		}
	}
}

func TestConvertNoFilesFound(t *testing.T) {
	dir := t.TempDir()

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No files") && !strings.Contains(output, "0 files") {
		t.Fatalf("expected output to indicate no files found, got: %s", output)
	}
}

func TestConvertHelp(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-help"}, &stdout); err != nil {
		t.Fatalf("run -help failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage") {
		t.Fatalf("expected help output to contain Usage, got:\n%s", output)
	}
}

func TestConvertLegacyWithFactorScores(t *testing.T) {
	dir := t.TempDir()

	legacyLine := `{"AgentID":"agent-1","Skill":"growth_momentum","Layer":"style","Symbol":"2330.TW","Side":"BUY","Conviction":87,"TargetPrice":1050.5,"StopLossPrice":980.2,"Window":"1d","ForwardReturn":0.021,"BenchmarkDelta":0.011,"Hit":true,"Reason":"breakout","Price":1001.0,"PassedGuards":true,"GuardReason":"ok","RecordedAt":"2026-04-22T04:02:30.434394+08:00","factor_scores":{"total":0.88},"conviction_breakdown":{"base":70,"floor":50,"final":87,"steps":[{"rule":"momentum","delta":17,"reason":"strong"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(legacyLine), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	rewritten, err := os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	var outcome domain.RecommendationOutcome
	if err := json.Unmarshal(rewritten, &outcome); err != nil {
		t.Fatalf("unmarshal rewritten: %v", err)
	}
	if outcome.FactorScores.Total != 0.88 {
		t.Fatalf("factor_scores.total: got %v", outcome.FactorScores.Total)
	}
	if outcome.ConvictionBreakdown == nil || outcome.ConvictionBreakdown.Final != 87 {
		t.Fatalf("conviction_breakdown.final: got %#v", outcome.ConvictionBreakdown)
	}
	if len(outcome.ConvictionBreakdown.Steps) != 1 || outcome.ConvictionBreakdown.Steps[0].Rule != "momentum" {
		t.Fatalf("conviction_breakdown.steps: got %#v", outcome.ConvictionBreakdown.Steps)
	}
}

func TestConvertAtomicWrite(t *testing.T) {
	dir := t.TempDir()

	line := `{"agent_id":"a1","skill":"s1","layer":"sector","symbol":"2330.TW","side":"BUY","conviction":80,"target_price":600,"stop_loss_price":550,"window":"1d","forward_return":0.02,"benchmark_delta":0.01,"hit":true,"reason":"r1","price":580,"passed_guards":true,"guard_reason":"ok","recorded_at":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"-dir", dir}, &stdout); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && strings.Contains(name, "tmp") {
			t.Fatalf("found leftover temp file: %s", name)
		}
	}
}
