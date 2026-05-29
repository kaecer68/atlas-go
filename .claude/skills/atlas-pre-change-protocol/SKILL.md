---
name: atlas-pre-change-protocol
description: "MUST use before ANY code modification in atlas-go. Mandates blast radius analysis via GitNexus, data source tracing via graphify/constitutions, and design intent verification. Triggers: any edit, refactor, fix, add, delete, or code change request. Prevents shallow patch fixes and dead code misidentification."
---

# Atlas Pre-Change Protocol

**IRON LAW: No code change without completing Steps 1-7 first.**
This protocol overrides the default AI impulse to edit immediately. Every step exists because atlas-go has 8972 nodes, 151 communities, and hidden dependencies that grep alone cannot see.

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
| Portfolio / optimizer | `.omo/CONSTITUTION.md` | Art.1: matrix ops required (Ledoit-Wolf), NOT linear weighting. Art.3: asset-universal code. |
| FactorType changes | `AGENTS.md` §高危陷阱 #22 | Must update 7 locations. Verify with `verify_factor_integrity.sh`. |

### Step 5: PATTERN MATCHING

Verify alignment with existing conventions BEFORE coding:

```
1. Read AGENTS.md 高危陷阱 table (lines 224-249) — does your change violate any?
   → Darwinian clipping [0.3, 2.5]? Reused mutable slices? Missing baseline?
   → GuardOutcomes ID override? OutcomeCount from global file?
2. Match repository design principles:
   → Small focused interfaces (Supports() + one operate method)
   → Early return over deep nesting
   → fmt.Errorf("context: %w", err) wrapping
   → Standard library → external → internal import order
3. Verify: "同一件事不可有三種算法" — single source of truth for filtering/counting
```

### Step 6: GRAPHIFY ARCHITECTURE CHECK

Understand the system landscape before editing:

```
1. Read graphify-out/GRAPH_REPORT.md for community structure (151 communities, 8972 nodes)
2. Use gitnexus_query({query: "your topic"}) to find relevant execution flows
3. Use gitnexus_context({name: "symbol"}) for 360° view (callers + callees + processes)
4. Check which community your target belongs to — cross-community changes amplify risk
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
□ "I understand the system well"       → atlas-go has 151 communities. Check graphify.
□ "I'll fix it the simple way"         → Simple ≠ correct. Match the architecture.
```

---

## Architecture Quick Reference

**Truth-source hierarchy** (when docs conflict):
1. `docs/GUIDELINES_INDEX.md` — final arbiter
2. `internal/apigateway/CONSTITUTION.md` + `.omo/CONSTITUTION.md` — mandatory rules (CI-enforced)
3. `AGENTS.md` + local `internal/*/AGENTS.md` — module boundaries and pitfalls
4. Source code — ultimate truth

**Key entry points for understanding the system:**

| Resource | Purpose |
|----------|---------|
| `AGENTS.md` | Project constitution, 22 高危陷阱, git workflow |
| `docs/GUIDELINES_INDEX.md` | Authority hierarchy, use-case routing |
| `.claude/SKILLS-MAP.md` | Full skill inventory (38+ skills) |
| `.claude/skills/atlas-core-architecture/SKILL.md` | System architecture and data flow |
| `graphify-out/GRAPH_REPORT.md` | Knowledge graph (8972 nodes, 151 communities) |

**Tool quick reference:**

| Tool | Use for |
|------|---------|
| `gitnexus_impact` | Blast radius — ALWAYS before changes |
| `gitnexus_context` | 360° symbol view (callers + callees + processes) |
| `gitnexus_query` | Find execution flows by concept |
| `gitnexus_detect_changes` | Pre-commit change impact check |
| `explore` agent | Contextual codebase pattern search |
| `librarian` agent | External docs/libraries research |
