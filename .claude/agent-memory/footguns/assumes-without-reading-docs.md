# Agent Assumes Without Reading Existing Code or Docs

- **Discovered**: 2026-07-16
- **Related incident**: Agent asked whether Fubon broker integration was required for production, despite the integration being fully implemented and documented
- **Prevention**: `atlas-pre-change-protocol` mandates MCP/gitnexus lookup before project-specific questions; deny-dangerous hook blocks blind modifications

## Symptom

An agent asked the user basic questions about systems that were already implemented and documented (e.g., "Is Fubon broker integration required for production launch?"). The Fubon proxy, certificate handling, Docker build, and `docs/environment.md` all already existed. The question revealed that the agent had not read the relevant files before asking.

This pattern generalizes to many other incidents: agents modify code based on assumptions, create duplicate implementations, or propose changes that contradict existing architecture.

## Root Cause

1. Agents optimize for response speed over verification.
2. There was no enforced "check first" gate before asking or editing.
3. The cost of asking is lower than the cost of reading, so agents prefer to ask.

## Prevention

1. **Mandatory lookup before project-specific questions**: `atlas-pre-change-protocol` requires querying MCP tools (`atlas-mcp` or `gitnexus`) and reading relevant docs before asking the user.
2. **Use MCP tools to answer factual questions**: If the answer exists in code or docs, the agent should retrieve it via tools rather than asking.
3. **Footgun check**: Before proposing a change, search for existing implementations of the same concept (`gitnexus_query`, `codebase-memory_search_graph`).
4. **Deny blind edits**: The deny-dangerous hook blocks actions that look like unverified assumptions (e.g., modifying `.env.example` without updating docs).

## Evidence

- `docs/environment.md` § Fubon SDK explains the integration in detail.
- `services/fubon-proxy/` contains the Python microservice.
- `internal/fubonproxy/` manages the proxy lifecycle.
- `atlas-pre-change-protocol` Step 0 (overlap detection) prevents duplicate implementations.
