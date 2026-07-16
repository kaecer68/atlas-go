# Agent Memory

Persistent memory for AI agents working on atlas-go.

This directory accumulates organizational knowledge that agents tend to forget across sessions: footguns, lessons, and architecture decisions. It is repo-owned and git-tracked so every future agent starts with the benefit of past mistakes.

## Layout

```
.claude/agent-memory/
├── footguns/     # Known traps that agents repeatedly fall into
├── lessons/      # Session-derived learnings (one file per incident)
└── decisions/    # Architecture / process decisions and their rationale
```

## When to Add an Entry

| Trigger | Add to |
|---------|--------|
| Agent makes a preventable mistake that cost >10 min | `footguns/` |
| A debugging session reveals non-obvious system behavior | `lessons/` |
| A design choice is made and should not be quietly reversed | `decisions/` |

## Entry Format

Each file is a short Markdown document with this structure:

```markdown
# Title

- **Discovered**: YYYY-MM-DD
- **Related incident**: link to manifest / PR / issue
- **Prevention**: hook / skill / rule that now blocks it

## Symptom
What happened.

## Root Cause
Why the agent (or human) made the mistake.

## Prevention
How to avoid it next time.

## Evidence
Commands, logs, or file paths that prove the pattern.
```

## For Agents

Before starting work on atlas-go, skim the `footguns/` directory. If you encounter a situation matching a footgun, stop and follow the prevention steps instead of repeating the mistake.
