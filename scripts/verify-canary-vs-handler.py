#!/usr/bin/env python3
"""Cross-validate canary test paths against handler code paths.

Extracts tool_name -> HTTP path from BOTH:
  - canaryRoutes in tools_canary_test.go  (test mapping)
  - countedAddTool + handler functions in tools_*.go (truth)

Reports mismatches. MCP-internal + destructive tools are skipped.
Exit 1 if real mismatches found.
"""

import re, sys, os, glob
from collections import defaultdict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HANDLER_DIR = os.path.join(ROOT, "cmd/atlas-mcp/server")
CANARY_FILE = os.path.join(HANDLER_DIR, "tools_canary_test.go")

# Tools with no HTTP endpoint (query internal state or use MCP-internal logic)
MCP_INTERNAL = {
    "mcp_anomaly_get_recent", "mcp_anomaly_ack", "mcp_get_call_stats",
    "mcp_get_session_topology", "mcp_get_top_slow_tools", "mcp_get_tenant_usage",
    "mcp_quickstart", "mcp_roots_list",
    "industry_sector_list", "industry_sector_lookup",
    "template_detector_status",
}

# Destructive tools deliberately excluded from canary test
SKIP_DESTRUCTIVE = {
    "control_get_audit_log", "control_get_active_overrides",
    "control_approve_recommendation", "control_reject_recommendation",
    "control_pause_agent", "control_resume_agent", "control_sector_ban",
    "experiment_promote", "experiment_revert",
    "task_get", "task_get_events",
    "channel_health",
}

# ---------------------------------------------------------------------------
def extract_tool_to_handler(files: list[str]) -> dict[str, str]:
    tool_to_handler = {}
    for path in files:
        with open(path) as f:
            content = f.read()
        lines = content.split('\n')
        current_name = None
        for line in lines:
            m = re.search(r'Name:\s*"([^"]+)"', line)
            if m:
                current_name = m.group(1)
                continue
            if current_name:
                m = re.search(r's\.handle(\w+)', line)
                if m:
                    tool_to_handler[current_name] = "handle" + m.group(1)
                    current_name = None
    return tool_to_handler

# ---------------------------------------------------------------------------
def extract_handler_paths(files: list[str]) -> dict[str, list[str]]:
    handler_paths = defaultdict(list)
    for path in files:
        with open(path) as f:
            content = f.read()
        for m in re.finditer(
            r'func\s+\(s\s+\*server\)\s+(handle\w+)\(.*?\)\s*\(.*?\)\s*\{(.*?)\n\}',
            content, re.DOTALL
        ):
            func_name = m.group(1)
            body = m.group(2)
            for p in re.findall(r'\.Get\(ctx,\s*"([^"]+)"', body):
                handler_paths[func_name].append(p)
            for p in re.findall(r'\.GetRaw\(ctx,\s*"([^"]+)"', body):
                handler_paths[func_name].append(p)
            for p in re.findall(r'\.PostJSON\(ctx,\s*"([^"]+)"', body):
                handler_paths[func_name].append(p)
            for p in re.findall(r'\.Get\(ctx,\s*"([^"]+)"\+in\.', body):
                handler_paths[func_name].append(p + "{id}")
    return dict(handler_paths)

# ---------------------------------------------------------------------------
def extract_canary_routes() -> dict[str, str]:
    with open(CANARY_FILE) as f:
        content = f.read()
    m = re.search(r'var canaryRoutes = map\[string\]canaryTest\{(.*?)\n\}', content, re.DOTALL)
    if not m:
        print("ERROR: canaryRoutes not found", file=sys.stderr)
        sys.exit(1)
    routes = {}
    for line in m.group(1).strip().split('\n'):
        match = re.search(r'"([^"]+)"\s*:\s*\{Path:\s*"([^"]+)"', line)
        if match:
            routes[match.group(1)] = match.group(2).split('?')[0]
    return routes

# ---------------------------------------------------------------------------
def main():
    handler_files = sorted(glob.glob(os.path.join(HANDLER_DIR, "tools_*.go")))
    handler_files = [f for f in handler_files if '_test.go' not in f]

    tool_to_handler = extract_tool_to_handler(handler_files)
    handler_paths = extract_handler_paths(handler_files)
    canary_routes = extract_canary_routes()

    mismatches = []
    for tool_name, handler_func in sorted(tool_to_handler.items()):
        if tool_name in MCP_INTERNAL or tool_name in SKIP_DESTRUCTIVE:
            continue
        canary_path = canary_routes.get(tool_name)
        handler_path_list = handler_paths.get(handler_func, [])

        if not canary_path:
            if tool_name not in SKIP_DESTRUCTIVE:
                mismatches.append(f"  {tool_name}: in handler registry but NOT in canaryRoutes")
            continue
        if not handler_path_list:
            mismatches.append(f"  {tool_name} → {canary_path}: handler has NO cli.Get path")
            continue

        matched = any(canary_path == hp or canary_path.startswith(hp) for hp in handler_path_list)
        if not matched:
            mismatches.append(f"  {tool_name}: canary={canary_path} ≠ handler={handler_path_list}")

    print(f"=== Canary vs Handler Cross-Validation ===")
    tools_checked = len([t for t in tool_to_handler if t not in MCP_INTERNAL and t not in SKIP_DESTRUCTIVE])
    print(f"  Tools checked: {tools_checked}")
    print()

    if mismatches:
        print(f"❌ {len(mismatches)} mismatch(es):")
        for m in mismatches:
            print(m)
        sys.exit(1)
    else:
        print(f"✅ All {tools_checked} tools: canary ↔ handler paths match")
        sys.exit(0)

if __name__ == "__main__":
    main()
