# L2.4 CLI Flag Wiring — Implementation Plan

> **Status**: Investigation complete; ready for implementation.
> **Branch**: `feat/l2-4-cli-flag-wiring` (from main `f69b3551`)
> **Related**: Issue (to be filed); `docs/operations/l2-4-followup.md` §2; PR #821
> **Estimated effort**: 0.5 days implementation + testing

## Background & Motivation

L2.4 observation period is currently toggled via:
1. **Config file**: `configs/parameters.json` `orchestrator.use_llm_sector_agents.value`
2. **Env var**: `LLM_SECTOR_AGENTS_ENABLED=true` (required for `factory.go` to register the plugin)

Both require either editing a tracked file (config) or setting env vars in the deployment. For **dev/staging/canary** workflows, this is awkward:

- Dev wants to enable L2.4 for a quick test session without committing config changes
- Canary workflows want to toggle L2.4 per-cohort without env var coordination
- Ops wants an audit trail (CLI invocation log) for "who turned L2.4 on and when"

A CLI flag `--use-llm-sector-agents` (string, accepts `true|false|empty`) addresses all three. It follows the existing **broker-config override pattern** in `cmd/atlas/main.go` (lines 122-133, 192-202), which already has the "string flag → config override" idiom.

## Current State Investigation

### Existing CLI flag pattern (cmd/atlas/main.go)

- Line 116: `flags := flag.NewFlagSet("atlas", flag.ContinueOnError)`
- Lines 119-148: 25+ flags already defined using `flags.Bool()` and `flags.String()` and `flags.Int()` — all have explicit defaults (usually `false` or `""`)
- Line 150: `cfg := deps.loadConfig()` — flags are parsed BEFORE config is loaded
- Lines 192-202: `bootstrap.ApplyBrokerConfig(&cfg, bootstrap.BrokerOverrides{...})` — **string broker flags override config after load** (when non-empty)

### Existing `UseLLMSectorAgents` flow

- `internal/config/parameters.go:60-78` — defines `UseLLMSectorAgentsMetadata` + `GetUseLLMSectorAgents()` getter
- `internal/config/parameters.go:280` — `UseLLMSectorAgents` field in `OrchestratorParameters` (loaded from `configs/parameters.json`)
- The getter returns `cfg.Orchestrator.UseLLMSectorAgents.Value` (with fallback to metadata default)

### Existing env var (NOT touched by this work)

- `LLM_SECTOR_AGENTS_ENABLED` is consumed by `internal/orchestrator/system_plugins.go` (per the runbook §1). This is **separate from** the `use_llm_sector_agents` config flag — env var is required for plugin registration; config flag controls gate. CLI flag will override the config flag only; env var is independent (and still required for non-`apiMode` operation).

## Design

### Flag definition

```go
useLLMSectorAgents := flags.String("use-llm-sector-agents", "",
    "override L2.4 LLM-driven sector agent (true|false, empty=no-override)")
```

- **Type**: string (not bool) to distinguish "not set" from "set to false"
- **Default**: `""` (empty string = no override, use config file value)
- **Valid values**: `""`, `"true"`, `"false"`, `"1"`, `"0"`, `"True"`, `"False"` (case-insensitive, also accept 1/0 for shell scripting convenience)
- **Invalid values**: anything else → flag parse error (Go's flag package handles this for free if we use a custom flag.Value; otherwise we validate after Parse)

### Override application

After `cfg := deps.loadConfig()` (line 150), before any downstream code reads `cfg.Orchestrator.UseLLMSectorAgents`:

```go
// L2.4 CLI flag override (Issue TBD)
if v := strings.ToLower(strings.TrimSpace(*useLLMSectorAgents)); v != "" {
    switch v {
    case "true", "1":
        cfg.Orchestrator.UseLLMSectorAgents.Value = true
    case "false", "0":
        cfg.Orchestrator.UseLLMSectorAgents.Value = false
    default:
        return fmt.Errorf("--use-llm-sector-agents must be 'true', 'false', '1', or '0' (got %q)", *useLLMSectorAgents)
    }
    cfg.Orchestrator.UseLLMSectorAgents.Rationale = "CLI flag override at " + time.Now().UTC().Format(time.RFC3339)
}
```

### Print loop update

The existing print loop (lines 1439-1450) shows resolved config values. Add L2.4 row after the broker config block:

```go
fmt.Printf("use_llm_sector_agents: %v (source: %s)\n",
    cfg.Orchestrator.UseLLMSectorAgents.Value,
    cfg.Orchestrator.UseLLMSectorAgents.Source)
```

This gives operators a one-line confirmation of the **resolved** value and its governance source (`experimental` / `empirical` / `default`).

### Audit logging

When the CLI flag overrides the config, log to `slog` so the override is captured in the structured log:

```go
logging.Info("main", "l24_flag_override",
    "cli_value", *useLLMSectorAgents,
    "previous", cfg.Orchestrator.UseLLMSectorAgents.Value,
    "new", cfg.Orchestrator.UseLLMSectorAgents.Value,
    "at", cfg.Orchestrator.UseLLMSectorAgents.Rationale)
```

This satisfies the audit-trail use case (who turned L2.4 on, when, what was the config before).

## Files to Modify

| File | Change | Lines (approx) |
|------|--------|----------------|
| `cmd/atlas/main.go` | Add `--use-llm-sector-agents` flag parsing | +1 |
| `cmd/atlas/main.go` | Add override application after `loadConfig()` | +12 |
| `cmd/atlas/main.go` | Add print row in resolved-config section | +2 |
| `cmd/atlas/main.go` | Add slog audit log on override | +5 |
| `docs/QUICKSTART.md` | Add CLI flag example in "啟用觀察期" section | +6 |
| `docs/operations/l2-4-runbook.md` | Add note that CLI flag can be used as alternative to flag flip | +3 |

**Total**: 1 file with real code change (`cmd/atlas/main.go`, ~20 lines), 2 files with doc updates.

## Testing Approach

### Unit tests

Add to a new `cmd/atlas/main_test.go` (or extend existing if present):

```go
func TestUseLLMSectorAgentsOverride(t *testing.T) {
    tests := []struct{
        name string
        flagValue string
        configValue bool
        expectedValue bool
        expectError bool
    }{
        {"empty preserves config true", "", true, true, false},
        {"empty preserves config false", "", false, false, false},
        {"true overrides false", "true", false, true, false},
        {"false overrides true", "false", true, false, false},
        {"1 overrides", "1", false, true, false},
        {"0 overrides", "0", true, false, false},
        {"True case-insensitive", "True", false, true, false},
        {"invalid value errors", "maybe", false, false, true},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            cfg := minimalConfig()
            cfg.Orchestrator.UseLLMSectorAgents.Value = tc.configValue
            err := applyUseLLMSectorAgentsOverride(&cfg, tc.flagValue)
            // ... assertions
        })
    }
}
```

### Integration tests

- `go run ./cmd/atlas --use-llm-sector-agents=true` → startup log shows L2.4 enabled
- `go run ./cmd/atlas --use-llm-sector-agents=false` → startup log shows L2.4 disabled
- `go run ./cmd/atlas` (no flag) → existing behavior (config value used)
- `go run ./cmd/atlas --use-llm-sector-agents=invalid` → exit with error

### Manual smoke test

1. `cp configs/parameters.json /tmp/parameters.json.bak` (backup)
2. Edit `configs/parameters.json` set `use_llm_sector_agents.value: false`
3. `go run ./cmd/atlas --use-llm-sector-agents=true`
4. Check log: should show `use_llm_sector_agents: true (source: experimental)` AND `l24_flag_override` audit log
5. Kill process, restore config
6. `go run ./cmd/atlas` (no flag) → should show `use_llm_sector_agents: false (source: experimental)`

## Acceptance Criteria

1. **AC1**: `atlas --use-llm-sector-agents=true` enables L2.4 regardless of config file value
2. **AC2**: `atlas --use-llm-sector-agents=false` disables L2.4 regardless of config file value
3. **AC3**: `atlas` (no flag) uses config file value (current behavior preserved)
4. **AC4**: `atlas --use-llm-sector-agents=invalid` exits with non-zero status and clear error message
5. **AC5**: Override event is logged via `slog` with `cli_value`, `previous`, `new`, `at` fields
6. **AC6**: Print loop shows resolved value + source
7. **AC7**: `gofmt -l .` clean
8. **AC8**: `go test ./...` pass (no regression in existing tests)
9. **AC9**: `go vet ./...` clean
10. **AC10**: `pre-commit` hooks pass (commitlint, fmt, lint)

## Rollback Plan

If the CLI flag causes issues in production:
- **Phase 1**: Revert the PR (1 commit revert)
- **Phase 2**: Users can still use the config file + env var approach
- **Phase 3**: No data loss; L2.4 observation log continues (if it was running)

The CLI flag is purely additive — does not change default behavior.

## Risks

| Risk | Mitigation |
|------|------------|
| CLI flag typo breaks startup | Pre-flight validation in override function (return error on invalid value) |
| Override logged but not visible in `kubectl logs` | slog is JSON-format when `LOG_FORMAT=json`; ensure staging uses json |
| Conflicting flags (e.g. `--use-llm-sector-agents=true` + env `LLM_SECTOR_AGENTS_ENABLED=false`) | Document precedence: CLI flag > config > env var only registers plugin (independent of flag) |
| Performance: extra string parsing on startup | Negligible (one-time per process start) |

## Open Questions

1. Should the flag accept `yes`/`no`/`on`/`off` aliases? (Decision: NO — keep narrow, document only `true/false/1/0`)
2. Should we also expose `--use-llm-sector-agents-override-period-days` for time-bounded observation windows? (Decision: NO — current L2.4 plan has fixed 7-day period; CLI flag is for on/off only, period override is a separate future concern)
3. Should the flag also override the env var `LLM_SECTOR_AGENTS_ENABLED`? (Decision: NO — env var is for plugin registration, separate concern; flagged in §1 of plan as out of scope)

## Implementation Steps

1. **Step 1**: Add flag parsing line in `cmd/atlas/main.go` (line 121-130 area)
2. **Step 2**: Add `applyUseLLMSectorAgentsOverride(&cfg, *useLLMSectorAgents)` helper function (can be in same file or extracted to `cmd/atlas/cli.go`)
3. **Step 3**: Call the helper after `cfg := deps.loadConfig()` (after line 150)
4. **Step 4**: Add print row in resolved-config section (after line 1450)
5. **Step 5**: Add unit tests in `cmd/atlas/main_test.go` (or new `cmd/atlas/cli_test.go`)
6. **Step 6**: Update `docs/QUICKSTART.md` with CLI flag example
7. **Step 7**: Update `docs/operations/l2-4-runbook.md` §1 with CLI flag note
8. **Step 8**: Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `pre-commit` to verify
9. **Step 9**: Open PR with title `feat(cmd): add --use-llm-sector-agents CLI flag override`
10. **Step 10**: After merge, update `l2-4-followup.md` §2 to mark item done

## Pairing with Future Work

After this PR lands:
- `l2-4-followup.md` §2 (CLI Flag Wiring) marked done
- §1 (Auto-cron Scheduler) prerequisites include "CLI flag wiring shipped" (for canary flag-flip workflows without env coordination)
- §3a (Source upgrade) prerequisites include "CLI flag shipped" (for ops canary governance)

## References

- `docs/operations/l2-4-followup.md` §2 — original work report entry
- `docs/operations/l2-4-runbook.md` §1 — Pre-flight Checklist (where CLI flag is added as alternative)
- `docs/QUICKSTART.md` — 啟用觀察期 section
- `cmd/atlas/main.go:116-148` — existing flag definitions
- `cmd/atlas/main.go:192-202` — existing `bootstrap.ApplyBrokerConfig` pattern
- `internal/config/parameters.go:60-78` — `GetUseLLMSectorAgents()` getter
- PR #821 — L2.4 observation scheduling API + admin panel (commit `f69b3551`)
