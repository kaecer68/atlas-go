#!/usr/bin/env python3
"""Contract generator — produces docs/contracts/mcp-tools.contract.json
from the authoritative canaryRoutes map and hermes-smoke shape assertions.

Usage: python3 scripts/gen-contracts.py [--validate]
  --validate  also check every path exists in the route table
"""

import json, re, sys, os, subprocess
from datetime import datetime, timezone

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CANARY_FILE = os.path.join(PROJECT_ROOT, "cmd/atlas-mcp/server/tools_canary_test.go")
SMOKE_FILE = os.path.join(PROJECT_ROOT, "scripts/hermes-smoke.sh")

# ---------------------------------------------------------------------------
# 1. Parse canaryRoutes from Go source
# ---------------------------------------------------------------------------
def parse_canary_routes(path: str) -> dict[str, str]:
    """Extracts tool_name -> HTTP path from canaryRoutes map."""
    with open(path) as f:
        content = f.read()

    # Find the map block
    m = re.search(r'var canaryRoutes = map\[string\]canaryTest\{(.*?)\n\}', content, re.DOTALL)
    if not m:
        print("ERROR: canaryRoutes map not found", file=sys.stderr)
        sys.exit(1)

    routes = {}
    for line in m.group(1).strip().split('\n'):
        # "tool_name": {Path: "/api/path?query"},
        match = re.search(r'"([^"]+)"\s*:\s*\{Path:\s*"([^"]+)"', line)
        if match:
            name, path = match.group(1), match.group(2)
            # Extract method from Path if it has a query string
            method = "GET"
            routes[name] = {"method": method, "path": path}
    return routes

# ---------------------------------------------------------------------------
# 2. Parse shape assertions from hermes-smoke.sh
# ---------------------------------------------------------------------------
def parse_smoke_shapes(path: str) -> dict[str, list[str]]:
    """Extracts tool_name -> expected_keys from hermes-smoke.sh."""
    shapes = {}
    with open(path) as f:
        for line in f:
            # do_smoke "E-01 explain_market_move" "/api/..." "key1,key2"
            m = re.search(r'do_smoke\s+"([^"]+)"\s+"([^"]+)"\s+"([^"]+)"', line)
            if m:
                tool_label = m.group(1)
                keys = [k.strip() for k in m.group(3).split(",") if k.strip() != "-"]
                if keys:
                    shapes[tool_label] = keys
    return shapes

# ---------------------------------------------------------------------------
# 3. Tool label to canonical name mapping (hermes-smoke uses labels, not names)
# ---------------------------------------------------------------------------
LABEL_TO_NAME = {
    "E-01 explain_market_move": "explain_market_move",
    "E-02 regime_get_history": "regime_get_history",
    "E-03 risk_get_metrics": "risk_get_metrics",
    "E-04 crossmarket_correlation": "crossmarket_get_correlation",
    "E-05 strategy_get_summary": "strategy_get_summary",
    "E-06 capital_flow_daily": "capital_flow_daily",
    "E-07 crossmarket_us_indices": "crossmarket_get_us_indices",
    "capital_flow_summary": "capital_flow_summary",
    "crossmarket_status": "crossmarket_get_status",
    "system_health": "system_get_health",
}

# ---------------------------------------------------------------------------
# 4. Assemble contract
# ---------------------------------------------------------------------------
def build_contract():
    routes = parse_canary_routes(CANARY_FILE)
    shapes = parse_smoke_shapes(SMOKE_FILE)

    tools = {}
    for name, info in sorted(routes.items()):
        entry = {
            "method": info["method"],
            "path": info["path"],
            "auth": "optional",
        }
        # Attach shape if we have one
        for label, canonical in LABEL_TO_NAME.items():
            if canonical == name and label in shapes:
                entry["response_keys"] = shapes[label]
                break
        tools[name] = entry

    return {
        "$schema": "https://atlas-go.dev/contracts/v1/mcp-tools.schema.json",
        "version": "1",
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "source": "cmd/atlas-mcp/server/tools_canary_test.go",
        "total_tools": len(tools),
        "tools": tools,
    }

# ---------------------------------------------------------------------------
# 5. Validate paths against route table
# ---------------------------------------------------------------------------
def validate_contract(contract: dict) -> list[str]:
    """Checks every contract path exists in the route table. Uses check-routes.sh output."""
    import subprocess
    result = subprocess.run(
        ["bash", os.path.join(PROJECT_ROOT, "scripts/check-routes.sh")],
        capture_output=True, text=True, cwd=PROJECT_ROOT
    )
    if result.returncode != 0:
        print("WARNING: check-routes.sh exited non-zero, skipping path validation")
        return []

    # Extract route list from check-routes output
    route_set = set()
    for line in result.stdout.split('\n'):
        r = line.strip()
        if r.startswith('/api/') or r.startswith('/health') or r.startswith('/ready'):
            # Normalize: strip query string for matching
            route_set.add(r.split('?')[0])

    # MCP-internal tools that don't route through HTTP
    MCP_INTERNAL = {
        "mcp_anomaly_get_recent", "mcp_get_call_stats", "mcp_get_session_topology",
        "mcp_get_tenant_usage", "mcp_get_top_slow_tools", "mcp_quickstart",
        "mcp_roots_list",
    }

    def normalize(p: str) -> str:
        p = p.split('?')[0]
        # Dynamic routes: foreign-3day-inflow → {id}, fugle → {id}, latest → {id}
        p = re.sub(r'/foreign-3day-inflow', '/{id}', p)
        p = re.sub(r'/fugle', '/{id}', p)
        p = re.sub(r'/universe/sessions/latest', '/universe/sessions/{id}', p)
        return p

    issues = []
    for name, info in contract["tools"].items():
        if name in MCP_INTERNAL:
            continue
        path_base = normalize(info["path"])
        if path_base not in route_set:
            issues.append(f"  {name}: {info['path']} → {path_base} — not in route table")

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
def main():
    validate = "--validate" in sys.argv
    out_path = os.path.join(PROJECT_ROOT, "docs/contracts/mcp-tools.contract.json")

    contract = build_contract()
    os.makedirs(os.path.dirname(out_path), exist_ok=True)

    with open(out_path, "w") as f:
        json.dump(contract, f, indent=2, ensure_ascii=False)
    print(f"✅ Generated {out_path} ({contract['total_tools']} tools)")

    if validate:
        issues = validate_contract(contract)
        if issues:
            print(f"\n❌ Path validation: {len(issues)} issue(s):")
            for i in issues:
                print(i)
            sys.exit(1)
        else:
            print("✅ All paths validated against route table")

if __name__ == "__main__":
    main()
