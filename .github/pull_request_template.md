## Summary

<!-- 1-3 句話描述這個 PR 做了什麼、為什麼。 -->

## Type of change

<!-- 勾選所有適用項 -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Refactor (no functional change, code structure improvement)
- [ ] Documentation only
- [ ] Chore (build, CI, tooling)

## Breaking changes

<!-- 若勾選 Breaking change,必須填寫。否則刪除整節。 -->

**API / JSON field renames:**

| Before | After | Migration |
|--------|-------|-----------|
| `<field>` | `<new_field>` | See `docs/migration-<version>.md` |

**New required fields / parameters:**

- `<field>` is now required. Default value: `<value>`.

**Removed fields / endpoints:**

- `<old_endpoint>` removed. Replaced by `<new_endpoint>`.

## Test plan

<!-- 勾選已執行的驗證 -->

- [ ] `gofmt -l .` clean
- [ ] `go vet ./...` clean
- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] New tests added for changed code paths
- [ ] Coverage maintained at ≥40% (per `internal/MATURITY.md`)

## Migration guide

<!-- 若有 breaking change,連結到對應的 migration doc;否則刪除 -->

- [ ] `docs/migration-<version>.md` updated / created

## Risk check

- [ ] No real trading behavior introduced (live broker remains gated)
- [ ] Simulation assumptions documented (if model changed)
- [ ] Provider behavior updated if market data path changed
- [ ] Frontend read path updated for any JSON field renames
- [ ] No `http.Error` calls in `internal/monitoring/` production code (use `shared.WriteJSONErrorEx`)

## Constitution compliance

<!-- 參考 .omo/CONSTITUTION.md 與 internal/apigateway/CONSTITUTION.md -->

- [ ] Does not bypass `BackgroundTaskManager` for long-running ops
- [ ] Does not bypass `ParametersConfig` validation
- [ ] Does not bypass `marketdata.Provider` abstraction
- [ ] Schema changes registered in `configs/agents.json` (if applicable)
- [ ] Cross-module traps checked (see `docs/TRAPS.md`)

## Pre-merge checklist

- [ ] Branch is up-to-date with `main` (no merge conflicts)
- [ ] CI green: governance, operations, ci/coverage, ci/lint, ci/commitlint
- [ ] At least 1 approving review
- [ ] PR body links to related issues / PRs
