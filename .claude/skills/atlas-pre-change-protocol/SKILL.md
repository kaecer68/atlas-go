---
name: atlas-pre-change-protocol
description: "MUST use before ANY code modification in atlas-go. Mandates GitNexus blast radius for changes, GitNexus architecture check, data source tracing, and design intent verification. Triggers: any edit, refactor, fix, add, delete, or code change request. Prevents shallow patch fixes and dead code misidentification. (Read-only investigations are exempt — this skill only triggers on write-intent tasks.)"
---

# Atlas Pre-Change Protocol

**IRON LAW: No code change without completing Steps 1-7 first.**
This protocol overrides the default AI impulse to edit immediately. Every step exists because atlas-go has 52,662 symbols, 165,265 relationships, 300 execution flows, and hidden dependencies that grep alone cannot see.

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

**Two modes based on task type:**
- **Write mode** (modifications): Full 7-step protocol → Steps 1-7
- **Investigation mode** (read-only inquiries): Lightweight protocol → Steps 1, 2, 3, 6

**If the request involves `internal/` or `cmd/` directories, this protocol is MANDATORY.**

---

## Pre-Change Protocol (7 Mandatory Steps)

Perform each step BEFORE editing. Skip none. Report findings to user AT the step level if risk is HIGH/CRITICAL.

### Step 1: BLAST RADIUS

Before touching ANY symbol, run impact analysis:

```
gitnexus_impact({target: "<symbol>", direction: "upstream"})
→ Report d=1 items (WILL BREAK) to user BEFORE making changes
→ If HIGH/CRITICAL risk: stop, report blast radius, ask for explicit confirmation
→ NEVER skip this for non-trivial changes (changes touching >3 symbols)
```

**Risk thresholds**: `<5 d=1 symbols = LOW` | `5-15 = MEDIUM` | `>15 or critical path = HIGH` | `auth/security path = CRITICAL`

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

Before claiming "data is insufficient" or modifying data paths:

```
1. Check internal/marketdata/provider.go for registered providers
2. Verify data availability in priority order:
   TWSE OpenAPI (primary) → FinMind (historical) → Fubon (real-time) → Fugle (paid)
3. NEVER claim "data insufficient" without checking ALL providers in priority order
4. NEVER create a bare HTTP client — use gateway.Fetch(channelID) or marketdata.Provider
5. If touching data fetching: read internal/apigateway/CONSTITUTION.md first
```

### Step 4: CONSTITUTION CHECK

Verify no constitutional violations before writing code:

| If touching... | Must read... | Key rules |
|---------------|--------------|-----------|
| Data fetching / API calls | `internal/apigateway/CONSTITUTION.md` | Art.1: registered channels only. Art.4: BackgroundTaskManager only. Art.5: ParametersConfig only. |
| Portfolio / optimizer | `docs/GUIDELINES_INDEX.md` + module AGENTS.md | Matrix ops required (Ledoit-Wolf), NOT linear weighting. Asset-universal code. |
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
4. If the index is stale, run: npx gitnexus analyze
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
□ "I'll fix it the simple way"         → Simple ≠ correct. Match the architecture.
```

---

## Architecture Quick Reference

**Truth-source hierarchy** (when docs conflict):
1. `docs/GUIDELINES_INDEX.md` — final arbiter
2. `internal/apigateway/CONSTITUTION.md` — mandatory rules (CI-enforced)
3. `AGENTS.md` + local `internal/*/AGENTS.md` — module boundaries and pitfalls
4. Source code — ultimate truth

**Key entry points for understanding the system:**

| Resource | Purpose |
|----------|---------|
| `AGENTS.md` | Project constitution, 22 高危陷阱, git workflow |
| `docs/GUIDELINES_INDEX.md` | Authority hierarchy, use-case routing |
| `docs/ENVIRONMENT.md` | Verified external dependency versions and setup notes |
| `.claude/SKILLS-MAP.md` | Full skill inventory (38+ skills) |
| `docs/architecture.md` | System architecture and data flow |
| `gitnexus://repo/atlas-go/clusters` | All functional communities detected by GitNexus |

**Tool quick reference:**

| Tool | Use for |
|------|---------|
| `gitnexus_impact` | Blast radius — ALWAYS before changes |
| `gitnexus_context` | 360° symbol view (callers + callees + processes) |
| `gitnexus_query` | Find execution flows by concept |
| `gitnexus_detect_changes` | Pre-commit change impact check |
| `explore` agent | Contextual codebase pattern search |
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
→ If the index is stale, run: npx gitnexus analyze
```

### Step I-2: GitNexus Concept Search
```
gitnexus_query({query: "<concept>"}) → find related execution flows
→ Empty results are STRONG SIGNAL (concept may not exist in codebase)
→ Non-empty results → read process traces for data flow understanding
```

### Step I-3: Data Source Tracing
```
For ANY question about data availability:
→ Check all providers in priority order: TWSE → FinMind → Fubon → Fugle
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
