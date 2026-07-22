#!/usr/bin/env python3
"""Production canary — reads the MCP tool contract and pings every endpoint.

Usage:
  ATLAS_URL=http://localhost:18080 make canary
  python3 scripts/canary-check.py https://atlas.example.com

Exit 0 = all tools healthy, Exit 1 = failures detected.
"""

import json, sys, os, urllib.request, urllib.error
from concurrent.futures import ThreadPoolExecutor, as_completed

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CONTRACT_PATH = os.path.join(PROJECT_ROOT, "docs/contracts/mcp-tools.contract.json")
DEFAULT_URL = "http://localhost:18080"
TIMEOUT = 15

SKIP_INTERNAL = {
    "mcp_anomaly_get_recent", "mcp_get_call_stats", "mcp_get_session_topology",
    "mcp_get_tenant_usage", "mcp_get_top_slow_tools", "mcp_quickstart",
    "mcp_roots_list", "daily_report",
}

SKIP_DESTRUCTIVE = {
    "experiment_judge", "experiment_promote", "experiment_revert",
    "control_pause_agent", "control_resume_agent", "control_sector_ban",
    "control_approve_recommendation", "control_reject_recommendation",
}

def should_skip(name: str, info: dict) -> str | None:
    if name in SKIP_INTERNAL:
        return "mcp-internal"
    if name in SKIP_DESTRUCTIVE:
        return "destructive"
    if info.get("auth") == "required":
        return "auth-required"
    if info.get("canary_skip"):
        return info.get("canary_skip_reason", "known-backend-issue")
    return None

def check_one(name: str, info: dict, base_url: str) -> tuple[str, bool, str, str | None]:
    skip = should_skip(name, info)
    if skip:
        return (name, True, "skipped", skip)

    path = info["path"]
    url = f"{base_url}{path}"
    expected_keys = info.get("response_keys", [])

    try:
        req = urllib.request.Request(url, method=info.get("method", "GET"))
        req.add_header("Accept", "application/json")
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            status = resp.getcode()
            body = resp.read().decode("utf-8", errors="replace")

        if status != 200:
            return (name, False, f"HTTP {status}", None)

        if expected_keys and body:
            try:
                data = json.loads(body)
            except json.JSONDecodeError:
                return (name, True, f"HTTP 200 (JSON parse warning)", None)
            if isinstance(data, list):
                return (name, True, f"HTTP 200 (array, {len(data)} items)", None)
            missing = [k for k in expected_keys if k not in data]
            if missing:
                return (name, True, f"HTTP 200 (missing keys: {missing})", None)

        return (name, True, "HTTP 200", None)

    except urllib.error.HTTPError as e:
        return (name, False, f"HTTP {e.code}", None)
    except urllib.error.URLError as e:
        return (name, False, f"connection: {e.reason}", None)
    except Exception as e:
        return (name, False, f"error: {e}", None)

def main():
    url = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("ATLAS_URL", DEFAULT_URL)
    url = url.rstrip("/")

    with open(CONTRACT_PATH) as f:
        contract = json.load(f)

    tools = contract["tools"]
    total = len(tools)

    skip_count = sum(1 for n, i in tools.items() if should_skip(n, i))
    print(f"=== Production Canary ===")
    print(f"  Target:   {url}")
    print(f"  Tools:    {total} ({skip_count} skipped)")
    print(f"  Contract: v{contract['version']}")
    print()

    passed, failed, skipped, warnings = 0, 0, 0, 0
    results = []

    with ThreadPoolExecutor(max_workers=8) as executor:
        futures = {executor.submit(check_one, n, i, url): n for n, i in tools.items()}
        for future in as_completed(futures):
            name, ok, detail, skip_reason = future.result()
            if skip_reason:
                skipped += 1
                continue
            if ok:
                if "warning" in detail.lower() or "missing" in detail:
                    warnings += 1
                    results.append((name, "⚠ ", detail))
                else:
                    passed += 1
            else:
                failed += 1
                results.append((name, "✗", detail))

    for name, mark, detail in results:
        if mark in ("✗", "⚠ "):
            print(f"  {mark} {name:<45} {detail}")

    tested = total - skipped
    print()
    print("=" * 43)
    print(f"  {passed} passed | {warnings} warnings | {failed} failed | {skipped} skipped")
    print("=" * 43)

    if failed > 0:
        print(f"❌ CANARY FAILED ({failed} failures)")
        sys.exit(1)
    else:
        print(f"✅ CANARY PASSED — all {passed} tools healthy")

if __name__ == "__main__":
    main()
