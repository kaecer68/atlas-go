---
name: atlas-pre-change-protocol
description: "MUST use before ANY code modification in atlas-go. Mandates overlap detection (Step 0), GitNexus blast radius for changes, GitNexus architecture check, data source tracing, and design intent verification. Triggers: any edit, refactor, fix, add, delete, or code change request. Prevents shallow patch fixes, dead code misidentification, and parallel duplicate implementations. (Read-only investigations are exempt — this skill only triggers on write-intent tasks.)"
---

# Atlas Pre-Change Protocol

**IRON LAW: No code change without completing Steps 1-7 first.**
This protocol overrides the default AI impulse to edit immediately. Every step exists because atlas-go has 數萬個 symbols 和 relationships（請執行 `npx gitnexus status` 取得 live 計數）,300+ execution flows, and hidden dependencies that grep alone cannot see.

---

## Session Start — Lock Scope Before Any Edit

Before running Steps 1-7, record the session boundary. This prevents agents from drifting into other CLI sessions, editing on `main`, or losing track of the original task.

```
1. Record current state:
   □ Mode:          Plan / Audit / Execute
   □ Branch:        git branch --show-current
   □ Worktree:      pwd && git worktree list
   □ Manifest:      <path to docs/manifests/YYYY-MM-DD-*.md or "none">
   □ In-scope IDs:  <issue IDs from the manifest or user request>
   □ ATLAS_ENV:     development / staging / production

2. Environment Isolation Checkpoint (must run):
   echo "ATLAS_ENV=${ATLAS_ENV:-development}"
   □ If ATLAS_ENV=production:
     → Verify worktree is isolated (not the dev worktree and not on branch main).
     → Verify no dev/experiment commands will be issued this session.
     → If uncertain, STOP and ask the user before proceeding.
   □ If ATLAS_ENV is unset or development:
     → Safe for dev/experiment commands ONLY in the current dev worktree.
     → Do not switch ATLAS_ENV to production within the same session.

3. If branch is main AND this is an implementation task (not a read-only investigation):
   → STOP. Load using-git-worktrees skill.
   → git worktree add -b feat/<short-name> ../atlas-<short-name> main
   → Continue this protocol inside the new worktree.

4. If asked to modify files outside the recorded in-scope IDs:
   → WARN the user.
   → Either update the manifest scope or stop before touching unrelated files.
   → Never silently poach work from another CLI session or manifest.

5. Run git stash list and record any pre-existing stashes with their messages.
   → New stashes created this session must be labeled with the task/ID.
```

**Why this matters**: `main` is not a workspace. Multi-CLI parallelism is only safe when each CLI is bound to its own branch/worktree and manifest.

---

## Agent Safety Hooks — Hard Boundaries

Before executing ANY shell command that modifies state, reads secrets, or touches production, run the deny-dangerous hook:

```bash
./agent-guard --check "<command>"
# or
.agent-hooks/deny-dangerous.sh --check "<command>"
```

If the hook blocks the command, do not bypass it unless the user explicitly approves. If bypassing, document the reason in the manifest.

MUST check with the hook:
- `git push` (any branch)
- `rm`, `rm -rf`
- Commands reading `.env`, `*.p12`, `*secret*`, `*password*`, `*credential*`
- `eval`, `bash -c` with piped downloads
- Commands with `ATLAS_ENV=production`
- Commands enabling live broker (`-allow-live-broker`)
- Destructive SQL (`DROP TABLE`, `TRUNCATE`)

Install hooks once per worktree:
```bash
bash .agent-hooks/install.sh
```

### Read Agent Memory First

Before starting work, skim `.claude/agent-memory/footguns/`. If your task matches a known footgun, follow the prevention steps instead of repeating the mistake.

**Why this matters**: skills and prompts are suggestions that models sometimes ignore. Hooks are deterministic guards that block dangerous actions before they run. Agent memory prevents the same mistake from recurring across sessions.

---

## When to Use

Load this skill when the user requests ANY of these:

| Trigger | Example |
|---------|---------|
| Code modification | "fix X", "add Y", "change Z", "refactor W" |
| Code removal | "delete this", "remove dead code", "clean up" |
| Bug reports | "this is broken", "X returns wrong result" |
| File changes | Any edit in `internal/` or `cmd/` |
| "Simple" fixes | "just add a field", "quick rename", "one-line change" |

**Three execution modes:**

| Mode | Purpose | Can Edit Code? | Required Binding |
|------|---------|---------------|------------------|
| **Plan Mode** | Design / architecture / write implementation plan | ❌ NO | `docs/manifests/YYYY-MM-DD-*.md` or `.omo/plans/` |
| **Audit Mode** | Read-only debugging / investigation | ❌ NO | Symptom / scope statement |
| **Execute Mode** | Implement approved plan | ✅ YES, scoped | Manifest file + in-scope IDs + feature branch/worktree |

**Mode rules:**
- You cannot edit `internal/`, `cmd/`, `configs/`, frontend code, or any tracked production file in Plan or Audit mode.
- In Plan/Audit mode you may only: read code, write to `.omo/plans/`, `docs/manifests/`, `.claude/agent-memory/`, and update hypothesis/evidence tables.
- Switching to Execute mode requires a clear verbal transition: "I am switching from plan to execute mode for IDs X-Y on branch feat/Z."
- If the user asks you to "look into", "audit", "investigate", or "plan", start in Audit/Plan mode.
- If the user asks you to "fix", "implement", "add", or "change", you must be in Execute mode with a manifest binding.

**If the request involves `internal/` or `cmd/` directories, this protocol is MANDATORY.**

---

## Pre-Change Protocol (8 Mandatory Steps)

Perform each step BEFORE editing. Skip none. Report findings to user AT the step level if risk is HIGH/CRITICAL.

### Step 0: OVERLAP DETECTION（新增前重疊檢查）← NEW

**Before adding ANY new code, verify it doesn't duplicate existing functionality.** This is the #1 failure mode for AI agents working on multi-module systems: generating a new function/type/module that does what another module already does.

```
MANDATORY for ALL of these:
□ Adding a new function/method → gitnexus_query({query: "<describe your intent>"})
  → Do ANY existing execution flows already cover this?
  → gitnexus_context({name: "<similar sounding symbols>"}) to verify scope

□ Adding a new type/struct → codebase-memory_search_graph({query: "<concept>"})
  → codebase-memory_search_graph({semantic_query: ["<key concept terms>"]})
  → Check for semantically similar types across ALL modules

□ Adding a new module/package → codebase-memory_get_architecture()
  → Which Louvain cluster would this belong to? Is there already a cluster for this domain?
  → Read internal/MATURITY.md — is there already a module with overlapping responsibility?

□ Adding validation/business logic → gitnexus_query({query: "<rule type> validation"})
  → Does risk/, apigateway/, or config/ already enforce this rule?
  → Check docs/reference/traps.md for "同一件事不可有三種算法" violations

□ The search returns EMPTY: proceed (new ground — document intention in commit message)
□ The search returns HITS: STOP. Read the overlapping code FIRST. If intentional overlap
  (e.g., live/ has different constraints than sim/), document WHY in the new code's comment.
```
```
Red flags — STOP and re-evaluate:
→ "I'll just add a helper function here" → Check with gitnexus_query first
→ "This looks like something the system needs" → Check if it already exists
→ "Let me create a new validator/checker" → Risk of parallel duplicate implementation
→ "I'll add validation in this module" → Check if another module already validates this
```

### Step 1: BLAST RADIUS

Before touching ANY symbol, run impact analysis:

```
gitnexus_impact({target: "<symbol>", direction: "upstream"})
→ Report d=1 items (WILL BREAK) to user BEFORE making changes
→ If HIGH/CRITICAL risk: stop, report blast radius, ask for explicit confirmation
→ NEVER skip this for non-trivial changes (changes touching >3 symbols)
```

**Risk thresholds**: `<5 d=1 symbols = LOW` | `5-15 = MEDIUM` | `>15 or critical path = HIGH` | `auth/security path = CRITICAL`

### Step 1.5: EXPLORE（codebase-memory FORK-EXCLUSIVE）

對**中低風險改動** — blast-radius 同時還需要 source code 看 caller 怎麼用時：

```
codebase-memory_explore({query: "<symbol 或概念>"})
→ 一次拿回 blast-radius（callers + fan-in flags）+ nearby neighbors（callees + 同檔 sibling）+ 逐行 source code
```

**Complements（不取代）`gitnexus_impact`：**
- ✅ 取代：後續手動 `Read` 拿 caller source code 的多個 round-trip（節省 token）
- ✅ 補充：給 `gitnexus_impact` 補上「callers 的 source 內容」資訊
- ❌ **不取代** `gitnexus_impact` 的風險等級（LOW/MEDIUM/HIGH/CRITICAL）
- ❌ **不取代** `gitnexus_impact` 的「受影響 Process 流」分析（atlas 的核心抽象）
- ❌ **不取代** `gitnexus_impact` 的 d=1/d=2/d=3 depth-grouped blast radius

**Routing 規則：**
- 用 `gitnexus_impact` 當你需要風險等級 + 受影響 Process 流 + d=1 直接破壞者清單（特別是 HIGH/CRITICAL 必須用）
- 用 `codebase-memory_explore` 當你需要看 caller 的 source code 才能規劃改動（中低風險、單檔或跨檔小範圍）
- 兩者可串接：先 `gitnexus_impact` 拿風險等級與 Process 流 → 再 `codebase-memory_explore` 拿 source code 細節

### Step 2: MODULE PITFALLS

Read the target module's local AGENTS.md and maturity tier:

```
1. Check if internal/<module>/AGENTS.md exists
   → If yes: read it, note all 陷阱 entries
   → If no: note the gap, proceed with extra caution
2. Check internal/MATURITY.md for module stability tier:
   → S-tier (stable): breaking change needs migration plan
   → E-tier (evolving): API may adjust, PR review required
   → X-tier (experimental): no other module should depend on it
   → U-tier (utility): CLI/tool only, not runtime
3. If creating a new internal/ module:
   → Add doc.go with Maturity tag → Update internal/MATURITY.md → Run check_maturity.sh
```

### Step 3: DATA SOURCE TRACING

Before adding a new data-fetching method, claiming data is insufficient,
or modifying data paths:

```
1. Check internal/marketdata/ for existing providers (Fubon, FinMind, Fugle, TWSE, Hybrid)
2. Verify data availability in priority order:
   TWSE OpenAPI (primary) → Fubon (broker-direct) → FinMind (historical) → Fugle (paid)
3. CAPABILITY COMPARISON (NEW): Before adding a new data-fetching method to any provider,
   check whether a higher-priority provider already offers an equivalent capability:
   □ Does Fubon already have this? → If yes, use Fubon.
   □ Does FinMind already have this? → If yes, use FinMind.
   □ Only if neither has it → use Fugle (or add new method there).
   □ Document the decision in the method's doc comment (which providers were checked).
4. NEVER claim "data insufficient" without checking ALL providers in priority order
5. NEVER create a bare HTTP client — use gateway.Fetch(channelID) or marketdata.Provider
6. If touching data fetching: read internal/apigateway/CONSTITUTION.md first
```

### Step 4: CONSTITUTION CHECK

Verify no constitutional violations before writing code:

| If touching... | Must read... | Key rules |
|---------------|--------------|-----------|
| Data fetching / API calls | `internal/apigateway/CONSTITUTION.md` | Art.1: registered channels only. Art.4: BackgroundTaskManager only. Art.5: ParametersConfig only. |
| Portfolio / optimizer | `docs/reference/guidelines-index.md` + module AGENTS.md | Matrix ops required (Ledoit-Wolf), NOT linear weighting. Asset-universal code. |
| FactorType changes | `AGENTS.md` §高危陷阱 #22 | Must update 8 locations. Verify with `verify_factor_integrity.sh`. |

### Step 5: PATTERN MATCHING

Verify alignment with existing conventions BEFORE coding:

```
1. Read AGENTS.md 高危陷阱 table — does your change violate any?
   → Darwinian clipping [0.3, 2.5]? Reused mutable slices? Missing baseline?
   → GuardOutcomes ID override? OutcomeCount from global file?
2. Match repository design principles:
   → Small focused interfaces (Supports() + one operate method)
   → Early return over deep nesting
   → fmt.Errorf("context: %w", err) wrapping
   → Standard library → external → internal import order
3. Verify: "同一件事不可有三種算法" — single source of truth for filtering/counting
```

### Step 6: GITNEXUS ARCHITECTURE CHECK (DO NOT SKIP)

GitNexus indexes the full call graph and execution flows. Use it to see connections that grep and IDE search miss:

```
1. Use gitnexus_query({query: "your topic"}) to find relevant execution flows
2. Use gitnexus_context({name: "symbol"}) for 360° view (callers + callees + processes)
3. Identify which functional communities your target belongs to — cross-community changes amplify risk
4. If the index is stale, run: npx gitnexus analyze --skip-agents-md
```

### Step 7: EXISTING CODE INTENT

Before modifying or removing code, understand WHY it exists:

```
1. git log --oneline -5 -- <file> — trace the design intent
2. The code may serve a purpose that's still relevant even if not obviously called:
   → Interface satisfaction (implements without direct invocation)
   → Plugin registry / dynamic dispatch (executors.go factory patterns)
   → Configuration-driven loading (agents.json, parameters.json references)
   → Runtime-generated call chains (reflect, init() registration)
3. Ask: "Is this truly dead, or just not obviously connected?"
4. For "dead code" claims: verify with gitnexus_impact (upstream), NOT just grep
```

---

## Code Removal Checklist

**DO NOT delete any code without completing ALL of these:**

```
□ gitnexus_impact({target: "<symbol>", direction: "upstream"}) — any callers?
□ git log --oneline -5 -- <file> — what was the original design intent?
□ grep -r "symbol_name" --include="*.go" internal/ — any dynamic/reflective references?
□ Check configs/agents.json — referenced by agent configuration?
□ Check prompts/agents/ — referenced by prompt templates?
□ Check if symbol satisfies an interface in the same package
□ If removing: document removal reason in the module's AGENTS.md
```

---

## Red Flags

**STOP — you are about to make a mistake if you catch yourself thinking:**

```
□ "This is just a simple fix"          → Run impact analysis first. Always.
□ "The data must be insufficient"      → Check all 4 providers before claiming this.
□ "This code is obviously dead"        → Complete the removal checklist above.
□ "I'll just add a quick workaround"   → Match existing patterns. No shortcuts.
□ "I don't need to read AGENTS.md"     → Always check module pitfalls.
□ "Let me just change this one line"   → Run gitnexus_impact. One-liners can cascade.
□ "The type system will catch errors"  → Types don't catch logic bugs or data gaps.
□ "I understand the system well"       → atlas-go has 300 execution flows. Check GitNexus.
□ "I'll add a helper/validator here"    → Run Step 0 overlap detection. Another module may already do this.
□ "No callers = dead code, delete it"   → Classify first: incomplete? superseded? accidentally disconnected? config-driven?
□ "This is new, can't overlap"          → codebase-memory semantic_query before declaring novelty.
□ "I'll fix it the simple way"         → Simple ≠ correct. Match the architecture.
□ "I'm on main but it's just a small edit" → STOP. Open a worktree + feature branch.
□ "This file looks related, let me touch it too" → Is it in the manifest scope? If not, ask first.
□ "Another CLI was working on this, I'll finish it" → No poaching. Stay bound to your branch/manifest.
□ "I'm in Plan/Audit mode but this fix is tiny" → STOP. Switch to Execute mode or add it to Backlog.
□ "The user said 'look into it' but I see the fix" → You are in Audit mode. Document hypothesis and evidence first.
□ "ATLAS_ENV doesn't matter for this command" → It does. Verify env before state-changing commands.
□ "I'll just switch ATLAS_ENV to production to test" → STOP. Production env must use an isolated worktree.
```

---

## Architecture Quick Reference

**Truth-source hierarchy** (when docs conflict):
1. `docs/reference/guidelines-index.md` — final arbiter
2. `internal/apigateway/CONSTITUTION.md` — mandatory rules (CI-enforced)
3. `AGENTS.md` + local `internal/*/AGENTS.md` — module boundaries and pitfalls
4. Source code — ultimate truth

**Key entry points for understanding the system:**

| Resource | Purpose |
|----------|---------|
| `AGENTS.md` | Project constitution, 22 高危陷阱, git workflow |
| `docs/reference/guidelines-index.md` | Authority hierarchy, use-case routing |
| `docs/environment.md` | Verified external dependency versions and setup notes |
| `.claude/SKILLS-MAP.md` | Full skill inventory (38+ skills) |
| `docs/architecture.md` | System architecture and data flow |
| `gitnexus://repo/atlas-go/clusters` | All functional communities detected by GitNexus |

**Tool quick reference:**

| Tool | Use for |
|------|---------|
| `gitnexus_impact` | Blast radius — ALWAYS before changes |
| `gitnexus_context` | 360° symbol view (callers + callees + processes) |
| `gitnexus_query` | Find execution flows by concept; overlap detection (Step 0) |
| `gitnexus_detect_changes` | Pre-commit change impact check |
| `codebase-memory_search_graph` | BM25 + semantic vector search; find similar implementations |
| `codebase-memory_get_architecture` | Louvain cluster detection; check module domain boundaries |
| `codebase-memory_query_graph` | Cypher analytics; hot-path complexity scan |
| `codebase-memory_explore` | **FORK-EXCLUSIVE (codebase-memory-mcp-pro 分支版)** — One call: blast-radius（callers + fan-in flags）+ nearby neighbors（callees + 同檔 sibling）+ 逐行 source code。Complements `gitnexus_impact` 用於中低風險、需要 source 的改動（見 Step 1.5）。 |
| `explore` (subagent) | oh-my-opencode subagent 的 contextual codebase pattern search。**NOT the same as `codebase-memory_explore` MCP tool 上一行** — 兩者完全不同的東西，避免誤用。 |
| `librarian` agent | External docs/libraries research |

---

## Investigation Mode (read-only inquiries)

For research, investigation, and audit tasks (no code modification expected), use this lighter protocol:

### Step I-1: GitNexus First
```
Use gitnexus_query({query: "your topic"}) → find relevant execution flows and symbols
This is the FASTEST way to understand system structure before diving into code.
→ Map the topic to a functional community (e.g., "market data providers" → marketdata community)
→ Check connected communities for cross-cutting concerns
→ If the index is stale, run: npx gitnexus analyze --skip-agents-md
```

### Step I-2: GitNexus Concept Search + Overlap Signal
```
gitnexus_query({query: "<concept>"}) → find related execution flows
→ Empty results are STRONG SIGNAL (concept may not exist in codebase — new ground)
→ Non-empty results → read process traces for data flow understanding
→ ALSO: codebase-memory_search_graph({semantic_query: ["<key terms>"]})
  → Catches implementations with different names but same semantics
  → Zero semantic matches + zero gitnexus hits = confidently new territory
```

### Step I-3: Data Source Tracing
```
For ANY question about data availability:
→ Check all providers in priority order: TWSE → Fubon → FinMind → Fugle
→ Check if the data type exists in MacroDataSnapshot or domain types
→ Check if a provider fills the field (trace from interface to implementation)
→ NEVER claim "data insufficient" without checking ALL providers
```

### Step I-4: Module Pitfalls
```
Read relevant internal/<module>/AGENTS.md for module-specific traps
→ Check internal/MATURITY.md for stability tier
→ Note any pitfalls that match your investigation area
```

**Investigation mode is EXEMPT from Steps 4, 5, 7 (constitution check, pattern matching, code intent) — these only apply to code modifications.**

## Strategy Techniques 觸及檢查（Step 2.5 增補）

當變更觸及 `internal/strategy_techniques/` 模組時，**額外**執行以下檢查：

1. **5 層框架一致性**：新增/修改的 StrategyFrame.Layer 須落在 L1~L5 之一，不可自創
2. **4 核心指標相容性**：Condition.Field 須使用 dot notation（`ForeignInvestorNet.Value`、`TSMADR.ChangePct` 等），與 `atlas-taiwan-leading-indicators` skill 列出的 4 個 MacroDataSnapshot 欄位一致
3. **3 個 enum 對齊**：Direction、Risk、Status 須使用 `strategy_techniques.Direction/Risk/Status` 型別，不可自建
4. **9 條 production seeds 不破壞**：新增/修改的代碼須確保既有 `data/seeds/strategy_techniques.json` 仍可載入
5. **自我修正路徑保留**：修改 `Validate` 或 `AttributionMode` 邏輯時，須保留 `rule_based` + `llm_annotated` 雙路徑

## Code Removal Checklist 增補：刪除 S-tier 模組時

當執行大規模模組清理（如未來刪除 `internal/strategy_techniques/` 等已退役 S-tier）時：

```
□ gitnexus_impact({target: "<module>", direction: "upstream"}) — 確認 0 呼叫者
□ grep -r "internal/<module>" --include="*.go" . — 確認僅剩 410 Gone handler
□ grep -r "<core-type>\b" --include="*.go" . — 確認無殘留
□ 確認 cmd/atlas/main.go 的相關 import 不依賴
□ 確認 internal/monitoring/dashboard_api.go 移除 handlers 欄位
□ 更新 internal/MATURITY.md（刪除 S-tier 條目）
□ 更新 internal/AGENTS.md（移除路由、保留替代品）
□ 跑 `go test ./...` 確認無破壞
□ 跑 `staticcheck ./...` 確認 0 issues
```

**歷史範例**：Wave 5 清理中刪除 `internal/eventlogic/` 的 7 步流程（已記錄於 git log）

---

## Mid-Session Re-focus Checkpoint

Long implementation sessions drift. Use this checkpoint after every unexpected error, debug detour, or roughly every 5 turns of active coding.

```
1. Re-read the active manifest / TODO list / plan file.
2. Ask:
   □ What is the current issue ID?
   □ Does this edit serve that ID?
   □ Am I still on the recorded branch/worktree?
   □ Have I touched files outside the manifest scope?
3. If scope has drifted:
   □ STOP the current edit.
   □ Add the new finding to the manifest Backlog.
   □ Ask the user whether to expand scope or return to the original ID.
4. Only resume after the user confirms the next action.
```

**Why this matters**: debugging is a trap. Agents fix the immediate compiler/test error and forget the original architectural goal, producing half-finished or unrelated changes.

---

## Session-End Cleanup

Before ending any implementation session:

```
□ All changed files are committed or explicitly reverted
□ No uncommitted implementation code remains in the working tree
□ git stash list reviewed:
  ├─ Stashes created this session: apply + commit + push, OR drop with user confirmation
  ├─ Pre-existing stashes: leave untouched unless user asked to clean them
  └─ All remaining stashes have descriptive messages (no "WIP" or unnamed stashes)
□ Manifest/TODO status updated if one exists
□ If work is incomplete: document next action before pausing
```

**Why this matters**: unnamed or abandoned stashes create ambiguity about what is "done" and pollute future sessions.

---

## Post-Merge Cleanup（Exit Criteria）

**AI 在每次 PR merge 後必須自動執行以下 4 步**（不要等使用者指示）：

```
1. git fetch origin main && git checkout main && git merge --ff-only origin/main
2. git branch -d <merged-branch>                       # -d 因為本地 main 已 ff
3. git push origin --delete <merged-branch>
4. 若使用獨立 worktree：git worktree remove <path>     # 需先切到其他 worktree
```

完整 SOP 見 `docs/quickstart.md` § Git 工作流 § 4。批次清理（>5 個
stale branch）見 `docs/branch-hygiene/`。multi-cli-protocol.md 的
「Post-merge cleanup checklist」段有對應摘要。

**為什麼加在這裡**：本 skill 在每次「修改程式碼前」必載入（description
MUST use），AI 進入下一個任務前會看到此段 → 在下一次 PR merge 動作時
觸發執行。

**歷史背景**：Wave 9 完成後遺留 16 個 stale branch（落後 main 30-60
commits），雖 multi-cli-protocol.md 早已明列「PR merge → branch 自動
刪除」SOP，但 AI 未主動執行 — 文件有寫但 AI 沒 follow。本段為直接
觸發點。清理見 PR #748。
