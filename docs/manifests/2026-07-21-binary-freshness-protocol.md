# Audit Manifest: Binary Freshness Protocol

> **Audit source**: Post-A04/A05/B01-B03/D01-D03 closure observed multi-session pattern of "fixed code but docker still runs old binary" causing repeated debugging cycles. User 2026-07-21 audit identified: cron images had Commit=unknown (Dockerfile.cron hardcoding), atlas-atlas image had `atlas-mcp` / `daily-replay-sync` / `backfill-replay` / `calibrate-seasonal` built without ldflags, host `bin/atlas-mcp` built Jul 21 00:05 predating D02, and no systematic check that binaries track HEAD at closing time.
>
> **Goal**: (1) One-shot align all current binaries (Docker + host) with current HEAD. (2) Build a closing-time check + Makefile rebuild entry so future code changes automatically trigger binary rebuild.
>
> **Created**: 2026-07-21
> **Status**: in-progress

## Background

PR cycle #1244 → #1248 (A04 → D03-A → D01-D08) showed a recurring pattern:
- Source code was modified across multiple merges (#1246, #1247, #1248)
- Docker images were partially rebuilt (main atlas-atlas rebuilt once, cron images never rebuilt since 2026-07-20)
- Host `bin/atlas-mcp` was never rebuilt after PR #1245
- No automated check tied "announced done" to "binaries match HEAD"
- User's audit drill surfaced that the live `geopolitical_history` table and `/api/regime/history` response were serving stale `Commit=unknown` rows

Concrete symptoms from this session:
- 90 cron image rows from 2026-07-20T15:53 (all `Commit=unknown`)
- host `bin/atlas-mcp` from Jul 21 00:05 (predates D02, pre-D08 work)
- `bin/execute-experiment` from Apr 7 (x86_64 Mach-O on M1 Mac, legacy orphan, can't even run)
- atlas-atlas image: `atlas-go` had correct Commit, but 4 sibling binaries (`atlas-mcp`, `daily-replay-sync`, `backfill-replay`, `calibrate-seasonal`) had Commit=unknown

---

## Invariant Tracker

| ID | Problem | Root-cause hypothesis | Files | Acceptance | Status | Evidence |
|----|---------|----------------------|-------|------------|--------|----------|
| **D01** | Cron images' binaries all show `Commit=unknown` after build | `Dockerfile.cron` hardcodes `Commit=unknown` in ldflags block (F4 fix series left it as TODO) | `Dockerfile.cron`, `docker-compose.yml` | Fresh cron image's `buildinfo.Commit` matches `git rev-parse HEAD` | done | m00545 commit `f871ef2b`; live cron container's `/app/macro-ingest` strings shows `Commit=02530b3461404e4dc3e19dae69f260b47e32cba0` |
| **D02** | atlas-atlas image's `atlas-mcp` / `daily-replay-sync` / `backfill-replay` / `calibrate-seasonal` all show `Commit=unknown` | Main `Dockerfile` lines 64/68/76 build those binaries without applying the same ldflags block used for atlas-go (line 51) | `Dockerfile`, `Dockerfile.atlas.local`, `Dockerfile.cron.local` | All 5 binaries in atlas-atlas image have `buildinfo.Commit` matching HEAD | done | m00545 commit `7b6175ee`; live verified all 5 binaries in `atlas-atlas:local` carry Commit=02530b3461404e4dc3e19dae69f260b47e32cba0 |
| **D03** | No systematic check that binaries track HEAD at closing time | No script, no SOP, no CI gate; tools exist (`internal/buildinfo` package ldflags) but not wired into workflow | `scripts/check-binary-freshness.sh`, `Makefile`, `.gitignore` | `make check-binaries` exits 0 against HEAD after every code change; same gate referenced from AGENTS.md + atlas-audit-manifest-protocol | done | m00545 commit `06a0ecd3` + `3cdec237`; `make check-binaries` reports ✓ ALL BINARIES FRESH |
| **D04** | User instruction: "把當前所有的 unknown 一次處理完畢 + 建立收尾時記得更新、同步的機制" | One-shot alignment: cron container image currently serving stale code | `bin/` (host binaries), docker images | All deployed binaries (Docker + host) carry `buildinfo.Commit` matching HEAD after one rebuild session | done | m00545; this PR's session: rebuilt atlas-atlas image (digest 4b4e00dd4e61 with ELF binaries), rebuilt atlas-cron-rebuilt:local with ELF binaries (10 cron containers force-recreated), rebuilt host `bin/atlas-mcp` (backup `bin/atlas-mcp.bak-pre-freshness-2026-07-21`), quarantined `bin/execute-experiment` as `bin/execute-experiment.bak-x86_64-legacy-2026-07-21` (x86_64 binary can't run on Apple Silicon) |

---

## Phase A — Audit (read-only, completed 2026-07-21)

| Task | Status | Evidence |
|------|--------|----------|
| Catalog all cron Docker images | done | 10 cron containers via `docker ps`, all using 1 shared `Dockerfile.cron` image → 1 rebuild aligns all 10 |
| Verify atlas-atlas image binaries | done | 5 binaries total — atlas-go has buildinfo, other 4 missing |
| Inspect host `bin/` | done | atlas-mcp (Jul 21 00:05, no buildinfo ldflag), execute-experiment (Apr 7, x86_64 Mach-O legacy orphan) |
| Locate cron-entrypoint.sh | done | `scripts/cron-entrypoint.sh` (selected via `CRON_COMMAND` env var) |
| Confirm cron-entrypoint.sh runs single binary from image | done | Image contains all 10 binaries; per-container selection via env var |

---

## Phase B — Plan (completed)

| Task | Status |
|------|--------|
| Map each ID to files | done — see Invariant Tracker |
| Choose between Dockerfile fix vs Symfony-level fix | done — chose Dockerfile fix (canonical build path) |
| Choose CI gate vs closing-time gate | done — closing-time gate per user instruction (CI per PR too expensive) |
| Decide sandbox workaround for proxy.golang.org unreachable | done — `Dockerfile.cron.local` and `Dockerfile.atlas.local` for sandbox envs |

---

## Phase C — Implement (completed)

| Task | ID | Status | Commit |
|------|----|--------|--------|
| Add `ARG GIT_COMMIT` + replace 10x `Commit=unknown` in Dockerfile.cron | D01 | done | `f871ef2b` |
| Add `args: GIT_COMMIT` block to 10 cron services in docker-compose.yml | D01 | done | `f871ef2b` |
| Add ldflags to atlas-mcp / daily-replay-sync / backfill-replay / calibrate-seasonal builds in Dockerfile | D02 | done | `7b6175ee` |
| Add Dockerfile.atlas.local (sandbox variant) | D02 | done | `7b6175ee` |
| Add Dockerfile.cron.local (sandbox variant) | D02 | done | `7b6175ee` |
| One-shot rebuild all binaries with HEAD=02530b34 | D04 | done | (live rebuild, not in PR) |
| Delete legacy x86_64 `bin/execute-experiment` (quarantined with .bak suffix) | D04 | done | (filesystem op, not in PR) |
| Add scripts/check-binary-freshness.sh | D03 | done | `06a0ecd3` |
| Add Makefile targets: rebuild-all, rebuild-cron, rebuild-atlas, rebuild-host-bin, check-binaries | D03 | done | `06a0ecd3` |
| Add .build-*, Dockerfile.{atlas,cron}.local to .gitignore | D03 | done | `06a0ecd3` |
| Update AGENTS.md workspace-close SOP with binary freshness gate | D03 (cross-cutting) | done | global config edit (not in PR) |
| Update atlas-audit-manifest-protocol SKILL.md Session-End Checklist | D03 (cross-cutting) | done | `3cdec237` |

---

## Phase D — Close Out (in progress)

### Verification Report

#### Live container image_id alignment (after this PR rebuild)
```
✓ atlas-go (atlas-atlas:local): Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
✓ atlas-mcp (atlas-atlas:local): Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
✓ daily-replay-sync (atlas-atlas:local): Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
✓ backfill-replay (atlas-atlas:local): Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
✓ calibrate-seasonal (atlas-atlas:local): Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
✓ atlas-cron-rebuilt:local (/app/macro-ingest): Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
✓ all 10 cron containers using image_id sha256:f77a66943891 (consistent)
✓ host bin/atlas-mcp: Commit=02530b3461404e4dc3e19dae69f260b47e32cba0
```

#### 11/11 integration regression (post-rebuild, atlas-go running new ELF binary)
```
BASELINE 1: /api/regime/history?limit=5           sessions=5, current_regime=RISK_OFF  ✓
BASELINE 2: /api/regime/history?days=5            sessions=5 (≤5)                      ✓
BASELINE 3: /api/narrative/stress-index/history?days=90  history=90 zero-epoch=0/90  ✓
BASELINE 4: /api/geopolitical/history             history=1                            ✓
C-CHECK 1:  stress history date field              rows with date: 5/5                  ✓
C-CHECK 2:  stress history source field            rows with source: 5/5                ✓
C-CHECK 3:  /api/narrative/regime-mapping         PASS                                  ✓
D-CHECK 1:  /api/regime/history source field      with_source: 5/5                     ✓
D-CHECK 2:  regime_history.source column          90 synthetic / 0 macro_ingest        ✓
D-CHECK 3:  stress history normalized vocab       normalized=90/90 stress=0/90         ✓
```

#### Known caveats (out of scope for this PR, noted for future)
- `atlas-cron-c07-collect` and `atlas-cron-c07-evaluate` have double-prefixed image names `atlas-atlas-cron-c07-{collect,evaluate}` (docker compose project naming artifact). Already in cron image rebuild path.
- `bin/atlas-mcp.bak-pre-freshness-2026-07-21` and `bin/execute-experiment.bak-x86_64-legacy-2026-07-21` retained as rollback (1 week retention, manual cleanup).
- Backend rebuild via `docker compose build atlas` still needs to reach `proxy.golang.org` — sandbox rebuild must use `Dockerfile.atlas.local` + `make rebuild-atlas-bins`. Documented in Makefile comments.

---

## Backlog

| ID | 內容 | Priority |
|----|------|----------|
| A01 | Consolidate 10 cron services in docker-compose.yml into 1 image with shared entrypoint (currently each cron service has its own `image:atlas-cron-X:latest` tag) | low |
| A02 | Add a sandbox-build backstop to `Makefile check-binaries` — if check-binary-freshness.sh reports missing binaries because they're outside worktree, advise user to run from main worktree | low |
| A03 | Consider auto-invoking `make check-binaries` via a git pre-push hook (would block any push that doesn't pass — gated on check-binary-freshness.sh greenlight). Could be opt-in. | low |

---

## Commit Discipline

Per manifest protocol, 1 commit per ID where state allows:

| Commit | ID | Description |
|--------|----|-------------|
| `f871ef2b` | D01 | Dockerfile.cron ARG GIT_COMMIT + docker-compose.yml args blocks |
| `7b6175ee` | D02 | Dockerfile ldflags for all binaries + Dockerfile.{atlas,cron}.local |
| `06a0ecd3` | D03 | scripts/check-binary-freshness.sh + Makefile targets + .gitignore |
| `3cdec237` | D03 cross-cutting | atlas-audit-manifest-protocol SKILL.md update |

Phase C/D live rebuild actions (D04) are filesystem operations, not commits — captured in §Verification Report.

---

## Follow-up (Operational, Out of Scope)

- After this PR merges, run `make rebuild-all && make check-binaries` once on the CI runner before promoting any subsequent release.
- AGENTS.md update is an in-place edit to user config (`~/.config/opencode/AGENTS.md`). Other agents / users need to either receive this update out-of-band or sync via shared config distribution.
- sandbox atlas-mcp: for users without `proxy.golang.org` access, run `make rebuild-host-bin` after every `cmd/atlas-mcp/` change.

---

## Session-End State

- **Done this session**:
  - One-shot aligned all binaries with current code (Docker atlas-atlas image, Docker 10 cron containers, host bin/atlas-mcp)
  - Installed binary-freshness protocol (script + Makefile + AGENTS.md + skill)
  - PR #1249 prepared (4 commits)
- **Remaining**: merge PR, run final make rebuild-all + make check-binaries on post-merge state
- **Next action**: wait for PR #1249 CI, merge, run `make rebuild-all && make check-binaries` to confirm protocol works on production images
- **Branch / PR**: `fix/binary-freshness-protocol` / PR #1249 (open, awaiting CI)
- **Backlog**: A01 / A02 / A03 (low priority, see Backlog section)

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.0 | Initial manifest with D01 / D02 / D03 / D04 from user binary-freshness instruction; opens PR #1249 with 4 commits (D01/D02/D03/skill) | Sisyphus |
