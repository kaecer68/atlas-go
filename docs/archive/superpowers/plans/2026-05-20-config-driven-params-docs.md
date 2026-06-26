# Config-Driven Parameters Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update AGENTS.md files to reflect that investment model parameters are now config-driven via ParametersConfig.

**Architecture:** Add a new section and update existing sections in `internal/portfolio/AGENTS.md` and `internal/orchestrator/AGENTS.md` to point to `internal/config/parameters.go` and `configs/parameters.json` as the source of truth.

**Tech Stack:** Go (for verification), Markdown.

---

### Task 1: Update internal/portfolio/AGENTS.md

**Files:**
- Modify: `internal/portfolio/AGENTS.md`

- [ ] **Step 1: Update FactorWeightEngine section to note config-driven nature**

Update the description at line 66-67 and add the source of truth note.

- [ ] **Step 2: Add note to hardcoded value tables in portfolio**

Add "These values are the defaults; override via parameters.json at runtime" to the table at line 69.

- [ ] **Step 3: Update Sector Rotator section to note config-driven macro/flow adjustments**

Update line 118-120.

- [ ] **Step 4: Commit changes to internal/portfolio/AGENTS.md**

```bash
git add internal/portfolio/AGENTS.md
git commit -m "docs(portfolio): mark parameters as config-driven in AGENTS.md"
```

### Task 2: Update internal/orchestrator/AGENTS.md

**Files:**
- Modify: `internal/orchestrator/AGENTS.md`

- [ ] **Step 1: Add a new section for Parameter Management**

Add a section detailing the migration to `ParametersConfig`.

- [ ] **Step 2: Update existing description to mention config-driven logic**

Briefly mention it in the opening or relevant section.

- [ ] **Step 3: Commit changes to internal/orchestrator/AGENTS.md**

```bash
git add internal/orchestrator/AGENTS.md
git commit -m "docs(orchestrator): document config-driven parameter management in AGENTS.md"
```

### Task 3: Verification

**Files:**
- Create: `.sisyphus/evidence/task-18-docs-verified.txt`

- [ ] **Step 1: Read the updated files to confirm coherence**

Run: `cat internal/portfolio/AGENTS.md internal/orchestrator/AGENTS.md`

- [ ] **Step 2: Save evidence**

```bash
echo "Documentation updated and verified:
- internal/portfolio/AGENTS.md updated for FactorWeightEngine and SectorRotator
- internal/orchestrator/AGENTS.md updated for NarrativeConviction and IndustryCycle
- Source of truth correctly referenced as parameters.go / parameters.json" > .sisyphus/evidence/task-18-docs-verified.txt
```

- [ ] **Step 3: Commit evidence**

```bash
git add .sisyphus/evidence/task-18-docs-verified.txt
git commit -m "docs: save evidence for parameter documentation update"
```
