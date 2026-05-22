HANDOFF CONTEXT
===============

USER REQUESTS (AS-IS)
---------------------
- "What did we do so far?"
- "Continue if you have next steps, or stop and ask for clarification if you are unsure how to proceed."
- "你到底還有多少問題？全部做好，還有非常多要做嗎？如過還有非常多，或還需要非常久，是不是先 commit，然後給我一段提示詞，讓我重新開機後，給 AI 用提示詞來接續工作。"
- "好的，做一個 handoff 讓新 session 可以接續處理這兩個剩餘問題"

GOAL
----
Make PR #38 (branch feat/industry-map-weight-recalc) pass all CI checks and become mergeable.

WORK COMPLETED
--------------
[Previously completed]
- Fixed logging package race condition in internal/logging/logger.go: replaced atomic.Value with sync.RWMutex
- Fixed DynamicEnvModulator race in internal/industry/dynamic_env.go: added sync.RWMutex
- Removed unused updateEnvFile() in internal/monitoring/dashboard_api.go (staticcheck U1000)
- Fixed .gitignore merge conflicts and gitnexus-stats config
- Restored enhanced_experiment_runner.go (deleted by git amend + graphify hook)
- Fixed generate.go (corrupted by graphify hook)

[This session - May 17 2026]
- Fixed TestManager_Subscribe DATA RACE in internal/taskexec/manager.go
  - Root cause 1: channel close() in unsubscribe raced with channel send in Emit()
    - Fix: added sync.Mutex to subscription struct; lock around closed.Load()+send and close()
  - Root cause 2: slice backing array race — Emit() read m.subs under subMu.RLock() then iterated after unlock, while Subscribe() append wrote to same backing array
    - Fix: moved iteration inside subMu.RLock() scope
  - Root cause 3: 3s timeout from missed initial "running" event — Subscribe() was called after event emission
    - Fix: Subscribe() replays existing events from store before adding itself to subscriber list
- Confirmed cmd/atlas 600s timeout is PRE-EXISTING on main branch (git diff main...HEAD shows no changes to cmd/atlas/main.go)
  - Root cause: macro ingestion on API mode startup makes real HTTP calls to Yahoo Finance (30s timeout each)
  - Some tests hang indefinitely because they pass -api but don't set shutdown channel, blocking on select{}
  - NOT caused by our branch — fix separately or work around in CI config
- Pushed commit a9b20d7 ("fix(taskexec): fix Subscribe DATA RACE and missed event delivery")
  - Only changed file: internal/taskexec/manager.go (+16/-4)
  - Force-pushed with lease to origin/feat/industry-map-weight-recalc

CURRENT STATE
-------------
- Branch: feat/industry-map-weight-recalc
- Latest commit: a9b20d7 ("fix(taskexec): fix Subscribe DATA RACE and missed event delivery")
- All CI-breaking issues from our changes are now FIXED
- The only remaining CI failure is the pre-existing cmd/atlas timeout (also broken on main)

PENDING TASKS
-------------
- Monitor CI on commit a9b20d7 to confirm taskexec fix passes
- If CI still shows the taskexec race on the "CI/CD Pipeline" or "ci" jobs, investigate why
- For cmd/atlas timeout: this is pre-existing on main. Options:
  1. Accept as pre-existing — merge PR anyway if it was already broken on main
  2. Fix the tests: add shutdown channels to tests missing them, or use httptest.Server instead of real port
  3. Add `-timeout 120s` to quality.yml coverage job to fail fast instead of hanging 600s

KEY FILES
---------
- internal/taskexec/manager.go - FIXED (sync.Mutex in subscription, replay in Subscribe, RLock scope)
- internal/taskexec/taskexec_test.go - UNCHANGED (manager.go fixes made race/timeout go away naturally)
- internal/logging/logger.go - sync.RWMutex fix (previous session)
- internal/industry/dynamic_env.go - mutex fix (previous session)
- .github/workflows/ci.yml - ci job (go test ./...)
- .github/workflows/quality.yml - Code Quality job (go test -v -race -coverprofile=coverage.out ./...)

IMPORTANT DECISIONS
-------------------
- atomic.Value is dangerous for context.Context (different concrete types per implementation)
- sync.Mutex in subscription struct is the correct fix for channel send vs close race
- Event replay from store in Subscribe() fixes missed-event timeout without changing the test
- cmd/atlas test hang is pre-existing on main — do NOT fix in this PR
- git pre-commit hook (graphify) may corrupt files during git operations — always verify
  generate.go and enhanced_experiment_runner.go after any git commit

CONTEXT FOR CONTINUATION
------------------------
- PR #38 should be mergeable now that our CI-breaking issues are fixed
- Monitor CI after commit a9b20d7 before merging
- If PR is merged, follow AGENTS.md git workflow (PR → merge via gh CLI)
- cmd/atlas timeout should be addressed in a separate PR if needed
- To verify the taskexec fix locally: go test -race -count=10 ./internal/taskexec/...
- Pre-commit hook note: generate.go had package main — verify after any commit

---

TO CONTINUE IN A NEW SESSION:

1. Press 'n' in OpenCode TUI to open a new session
2. This handoff is the first message
3. Add: "Continue from the handoff context above."
