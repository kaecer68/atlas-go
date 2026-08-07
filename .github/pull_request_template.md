## Summary

<!-- 1-3 句話描述這個 PR 做了什麼、為什麼。 -->

## Root Cause

<!-- 為什麼需要這個 PR（事實為本,不是猜測）。若有對應的 issue / discussion / 監控告警,link 到來源。Bug fix PR 必填（事實 + log/source code 佐證）。Feature / Refactor / Docs / Chore 可填「N/A」並簡述動機。詳見 docs/operations/pr-lifecycle.md §2.3。 -->

對應 issue / discussion:
<!-- 範例: Fixes #1457, Related #1460 -->

## Verification

<!-- 跑了什麼 test,結果是什麼。`make ci-full` 結果必填（見 docs/operations/pr-lifecycle.md §2.1.2）。Bug fix PR 必含「重現步驟 + 修後驗證」。Code PR 必含「make ci-gate 過 / make ci-full 過」證據。 -->

`make ci-gate` 結果:（過 / 失敗 — 附 log 截圖或最後 10 行）
`make ci-full` 結果:（過 / 失敗 — 附 log 截圖或最後 10 行 / KNOWN: 跳過,原因）

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
- [ ] Coverage maintained at ≥60% (per `internal/MATURITY.md`)

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

<!-- 參考 docs/reference/constitution.md 與 internal/apigateway/CONSTITUTION.md -->

- [ ] Does not bypass `BackgroundTaskManager` for long-running ops
- [ ] Does not bypass `ParametersConfig` validation
- [ ] Does not bypass `marketdata.Provider` abstraction
- [ ] Schema changes registered in `configs/agents.json` (if applicable)
- [ ] Cross-module traps checked (see `docs/reference/traps.md`)
- [ ] 方法論變更（methodology_rules.yaml / period config / MacroDataSnapshot 欄位 / capitalflow 評分）→ 已同步 `docs/ATLAS_CONSTITUTION_AUDIT.md` §附錄 F 追蹤表（X2）

## Pre-merge checklist

- [ ] Branch is up-to-date with `main` (no merge conflicts)
- [ ] CI green: governance, operations, ci/coverage, ci/lint, ci/commitlint
- [ ] At least 1 approving review
- [ ] PR body links to related issues / PRs
