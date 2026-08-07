#!/usr/bin/env python3
"""Validate that a1-channels.json stays synchronized with the Go channel registry.

Checks:
1. canonical_channel_count matches len(channelIDs()) in internal/apigateway/gateway.go.
2. Every id in a1-channels.json registered_channels is in channelIDs().
3. Every channel registered at runtime in internal/apigateway/register_adapters.go
   is in channelIDs() (prevents orphan runtime channels without circuit breaker/health slot).
4. No duplicate ids in a1-channels.json.

Exit code: 0 = synchronized, 1 = drift detected.
"""

import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
GATEWAY_GO = REPO_ROOT / "internal" / "apigateway" / "gateway.go"
REGISTER_ADAPTERS_GO = REPO_ROOT / "internal" / "apigateway" / "register_adapters.go"
CHANNEL_INDEX_JSON = (
    REPO_ROOT
    / "docs"
    / "contracts"
    / "channel-index.json"
)


def extract_channel_ids() -> list[str]:
    text = GATEWAY_GO.read_text(encoding="utf-8")
    # Find the channelIDs function and extract the string literal return slice.
    match = re.search(
        r"func\s+channelIDs\s*\(\)\s*\[\]string\s*\{([^}]+)\}", text, re.DOTALL
    )
    if not match:
        print(f"FAIL: could not locate channelIDs() return slice in {GATEWAY_GO}", file=sys.stderr)
        sys.exit(1)

    body = match.group(1)
    ids = re.findall(r'"([^"]+)"', body)
    if not ids:
        print(f"FAIL: no channel IDs found in channelIDs() body", file=sys.stderr)
        sys.exit(1)
    return ids


def extract_runtime_registrations() -> set[str]:
    text = REGISTER_ADAPTERS_GO.read_text(encoding="utf-8")
    # Match g.registry.Register("id", ...) or g.registry.Register("id" , NewXxx(...))
    ids = set(re.findall(r'g\.registry\.Register\s*\(\s*"([^"]+)"', text))
    return ids


def load_index() -> dict:
    with CHANNEL_INDEX_JSON.open(encoding="utf-8") as f:
        return json.load(f)


def main() -> int:
    gateway_ids = extract_channel_ids()
    runtime_ids = extract_runtime_registrations()
    index = load_index()

    registered = [ch["id"] for ch in index.get("registered_channels", [])]
    registered_set = set(registered)

    errors: list[str] = []

    # 1. canonical count
    if index.get("canonical_channel_count") != len(gateway_ids):
        errors.append(
            f"canonical_channel_count is {index.get('canonical_channel_count')} "
            f"but gateway.go channelIDs() has {len(gateway_ids)} channels."
        )

    # 2. registered_channels must match channelIDs exactly
    missing_in_json = set(gateway_ids) - registered_set
    extra_in_json = registered_set - set(gateway_ids)
    if missing_in_json:
        errors.append(
            f"channel IDs in gateway.go but missing from a1-channels.json: {sorted(missing_in_json)}"
        )
    if extra_in_json:
        errors.append(
            f"channel IDs in a1-channels.json but not in gateway.go: {sorted(extra_in_json)}"
        )

    # 3. duplicates in a1-channels.json
    if len(registered) != len(registered_set):
        seen = set()
        duplicates = [cid for cid in registered if cid in seen or seen.add(cid)]  # type: ignore[func-returns-value]
        errors.append(f"duplicate ids in a1-channels.json registered_channels: {duplicates}")

    # 4. runtime registrations must be subset of canonical channelIDs
    orphan_runtime = runtime_ids - set(gateway_ids)
    if orphan_runtime:
        errors.append(
            f"runtime-registered channels not in gateway.go channelIDs(): {sorted(orphan_runtime)}"
        )

    if errors:
        print("Channel index drift detected:", file=sys.stderr)
        for err in errors:
            print(f"  - {err}", file=sys.stderr)
        print(
            "\nFix: update internal/apigateway/gateway.go channelIDs(), "
            "internal/apigateway/register_adapters.go, and a1-channels.json/a1-channels.md together.",
            file=sys.stderr,
        )
        return 1

    print(
        f"PASS: a1-channels.json synchronized with {len(gateway_ids)} canonical channel IDs "
        f"and {len(runtime_ids)} runtime registrations."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
