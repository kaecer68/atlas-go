#!/usr/bin/env python3
"""Phase 3.5 M4 — Forecast Bridge Sortino Evaluation.

Compares portfolio Sortino ratio with and without forecast-bridge signals
over a 7-day replay window.

Usage:
   python3 scripts/eval/forecast_bridge_sortino.py \\
       --workdir data/replay/sessions/ \\
       --window 7

Output: delta-sortino metric (bridge_sortino - baseline_sortino).
Positive delta = forecast bridge improves risk-adjusted return.
"""

import argparse
import json
import math
import os
import sys
from pathlib import Path
from statistics import mean


def load_daily_returns(sessions_dir: Path, window_days: int) -> list[float]:
    """Load sorted daily portfolio returns from session outcome files."""
    returns = []
    session_dirs = sorted(sessions_dir.glob("session_*"), reverse=True)[:window_days]
    for sd in session_dirs:
        outcome_file = sd / "recommendation_outcomes.jsonl"
        if not outcome_file.exists():
            continue
        daily_ret = 0.0
        with open(outcome_file) as f:
            for line in f:
                if not line.strip():
                    continue
                rec = json.loads(line)
                daily_ret += float(rec.get("pnl", 0) or rec.get("realized_pnl", 0))
        returns.append(daily_ret)
    return returns


def sortino_ratio(returns: list[float], mar: float = 0.0) -> float:
    """Calculate Sortino ratio: excess return / downside deviation."""
    if len(returns) < 2:
        return 0.0
    avg = mean(returns)
    downside = [min(r - mar, 0) ** 2 for r in returns]
    if sum(downside) == 0:
        return 0.0
    return (avg - mar) / math.sqrt(sum(downside) / len(returns))


def main():
    parser = argparse.ArgumentParser(description="M4 Forecast Bridge Sortino Evaluation")
    parser.add_argument("--workdir", default="data/replay/sessions/", help="session data directory")
    parser.add_argument("--window", type=int, default=7, help="replay window in days")
    parser.add_argument("--baseline", help="path to baseline run sessions")
    parser.add_argument("--bridge", help="path to bridge-enabled run sessions")
    args = parser.parse_args()

    baseline_dir = Path(args.baseline) if args.baseline else Path(args.workdir) / "baseline"
    bridge_dir = Path(args.bridge) if args.bridge else Path(args.workdir) / "bridge_enabled"

    if not baseline_dir.exists() or not bridge_dir.exists():
        print(f"ERROR: session directories not found.\n  baseline: {baseline_dir}\n  bridge:   {bridge_dir}")
        print("Run two replay sessions first: one without forecast bridge, one with.")
        sys.exit(1)

    baseline_returns = load_daily_returns(baseline_dir, args.window)
    bridge_returns = load_daily_returns(bridge_dir, args.window)

    if len(baseline_returns) < 2 or len(bridge_returns) < 2:
        print(f"WARNING: insufficient data ({len(baseline_returns)} / {len(bridge_returns)} days)")
        sys.exit(1)

    baseline_sortino = sortino_ratio(baseline_returns)
    bridge_sortino = sortino_ratio(bridge_returns)
    delta = bridge_sortino - baseline_sortino

    print(json.dumps({
        "window_days": args.window,
        "baseline_sortino": round(baseline_sortino, 4),
        "bridge_sortino": round(bridge_sortino, 4),
        "delta_sortino": round(delta, 4),
        "improvement_pct": round(delta / max(abs(baseline_sortino), 0.001) * 100, 2) if baseline_sortino != 0 else None,
    }, indent=2))

    if delta > 0:
        print(f"✅ Forecast bridge improved Sortino by {delta:+.4f}")
    else:
        print(f"❌ Forecast bridge did not improve Sortino ({delta:+.4f})")


if __name__ == "__main__":
    main()
