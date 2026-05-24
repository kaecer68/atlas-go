#!/usr/bin/env python3
"""
Slice the existing graphify-out/graph.json into per-module-group sub-graphs.

Usage:
    python3 scripts/slice-graph.py

Output:
    graphify-out/subgraphs/{core,analysis,research,infra}/graph.html
    graphify-out/subgraphs/index.html  (navigation hub)

No re-extraction needed — slices the already-built graph.
Run `graphify . --update` to refresh the master graph, then re-run this script
to refresh sub-graphs.
"""

import json
import sys
import os
from pathlib import Path
from collections import defaultdict

# ── graphify imports ──────────────────────────────────────────────────────
from graphify.build import build_from_json
from graphify.cluster import cluster, score_all
from graphify.export import to_html

# ── paths ─────────────────────────────────────────────────────────────────
REPO_ROOT = Path(__file__).resolve().parent.parent
GRAPH_JSON = REPO_ROOT / "graphify-out" / "graph.json"
SUBGRAPH_DIR = REPO_ROOT / "graphify-out" / "subgraphs"

# ── module groups ─────────────────────────────────────────────────────────
# Each group maps to the architecture layers defined in AGENTS.md
GROUPS = {
    "core": {
        "label": "Core Pipeline",
        "description": "Orchestrator → Sim → Ledger; market data, domain types, live & realtime, strategy",
        "modules": [
            "orchestrator", "sim", "domain", "marketdata", "ledger",
            "live", "realtime", "strategy", "reflexivity",
        ],
    },
    "analysis": {
        "label": "Analysis Layer",
        "description": "Portfolio (Darwinian weights, FactorEngine), Risk, Industry, Tax, Screener, Reporting",
        "modules": ["portfolio", "risk", "industry", "tax", "screener", "reporting"],
    },
    "research": {
        "label": "Research Pipeline",
        "description": "Experiment lifecycle, Narrative, Janus regime, Evolution, Baseline, Adversarial",
        "modules": [
            "experiment", "narrative", "janus", "evolution", "swarm",
            "autobacktest", "baseline", "adversarial", "spawning", "globalmarket",
        ],
    },
    "infra": {
        "label": "Infrastructure",
        "description": "Monitoring, Repository, TaskExec, EventBus, Bootstrap, Gateway, DB, Config",
        "modules": [
            "monitoring", "repository", "taskexec", "eventbus", "bootstrap",
            "apigateway", "db", "importer", "backtest", "config", "logging",
        ],
    },
}


def get_module(source_file: str) -> str | None:
    """Extract the internal/xxx module name from a source_file path."""
    if source_file.startswith("internal/"):
        parts = source_file.split("/")
        if len(parts) >= 2:
            return parts[1]
    return None


def slice_group(group_cfg: dict, nodes_data: list, links: list, group_name: str):
    """
    Build a sub-graph for one module group and write its HTML.
    Returns (node_count, edge_count, community_count) for reporting.
    """
    module_set = set(group_cfg["modules"])

    # ── 1. Collect nodes whose source_file belongs to this group ─────────
    group_node_ids: set = set()
    for n in nodes_data:
        sf = n.get("source_file", "")
        mod = get_module(sf)
        if mod and mod in module_set:
            group_node_ids.add(n["id"])

    if not group_node_ids:
        print(f"  ⚠  {group_name}: no nodes found")
        return (0, 0, 0)

    # ── 2. Filter edges where BOTH ends belong to the group ──────────────
    group_edges = [
        e for e in links
        if e.get("source") in group_node_ids
        and e.get("target") in group_node_ids
    ]

    # ── 3. Keep only nodes that actually participate in at least one edge ─
    connected_node_ids: set = set()
    for e in group_edges:
        connected_node_ids.add(e["source"])
        connected_node_ids.add(e["target"])

    # Also re-include any group node that has edges (in case it was filtered)
    group_nodes = [n for n in nodes_data if n["id"] in connected_node_ids]

    if not group_nodes:
        print(f"  ⚠  {group_name}: no connected nodes")
        return (0, 0, 0)

    # ── 4. Build extraction dict for graphify ────────────────────────────
    extraction = {
        "nodes": group_nodes,
        "edges": group_edges,
    }

    print(f"  Nodes: {len(group_nodes)}  Edges: {len(group_edges)}")

    # ── 5. Build networkx graph and re-cluster ───────────────────────────
    G = build_from_json(extraction)
    community_lists = cluster(G)  # {community_id: [node_id, ...]}
    # Reverse mapping: node_id -> community_id
    comm_map = {}
    for cid, members in community_lists.items():
        for nid in members:
            comm_map[nid] = cid
    cohesion = score_all(G, community_lists)

    n_comms = len(community_lists)
    print(f"  Communities: {n_comms}  (avg cohesion: {sum(cohesion.values())/max(len(cohesion),1):.2f})")

    # ── 6. Generate community labels ─────────────────────────────────────
    community_modules: dict[int, defaultdict] = defaultdict(lambda: defaultdict(int))
    for cid, members in community_lists.items():
        for nid in members:
            nd = G.nodes[nid]
            sf = nd.get("source_file", "")
            mod = get_module(sf) or "other"
            community_modules[cid][mod] += 1

    labels = {}
    for cid, mod_counts in community_modules.items():
        top = [m for m, _ in sorted(mod_counts.items(), key=lambda x: -x[1])[:3]]
        labels[cid] = " + ".join(top)

    # ── 7. Generate HTML ─────────────────────────────────────────────────
    out_dir = SUBGRAPH_DIR / group_name
    out_dir.mkdir(parents=True, exist_ok=True)
    html_path = out_dir / "graph.html"

    to_html(G, community_lists, str(html_path), community_labels=labels)

    return (len(group_nodes), len(group_edges), n_comms)


def build_index(results: dict, labels: dict):
    """Create an index.html that links to all sub-graphs."""
    html = """<!DOCTYPE html>
<html lang="zh-TW">
<head>
<meta charset="UTF-8">
<title>atlas-go — Graphify Subgraphs</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: #0f0f1a; color: #e0e0e0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
         display: flex; align-items: center; justify-content: center; min-height: 100vh; }
  .container { max-width: 720px; padding: 40px; }
  h1 { font-size: 24px; margin-bottom: 6px; }
  .subtitle { color: #888; font-size: 14px; margin-bottom: 32px; }
  .card { background: #1a1a2e; border: 1px solid #2a2a4e; border-radius: 10px; padding: 20px; margin-bottom: 16px;
           display: block; text-decoration: none; color: inherit; transition: border-color .2s; }
  .card:hover { border-color: #4E79A7; }
  .card h2 { font-size: 18px; margin-bottom: 4px; }
  .card p { font-size: 13px; color: #999; margin-bottom: 10px; }
  .card .stats { font-size: 12px; color: #666; display: flex; gap: 16px; }
  .card .legend { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 10px; }
  .card .legend-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
  .card .legend-item { display: inline-flex; align-items: center; gap: 4px; font-size: 11px; color: #888; }
  .full-link { display: block; text-align: center; margin-top: 24px; color: #4E79A7; font-size: 13px; text-decoration: none; }
  .full-link:hover { text-decoration: underline; }
</style>
</head>
<body>
<div class="container">
  <h1>atlas-go 知識圖譜</h1>
  <p class="subtitle">依架構層拆分 — 從原本 7,616 節點 × 100 社群降載</p>
"""
    for name, cfg in labels.items():
        name_label = cfg["label"]
        desc = cfg["description"]
        stats = results.get(name, (0, 0, 0))
        html += f"""
  <a class="card" href="{name}/graph.html">
    <h2>{name_label}</h2>
    <p>{desc}</p>
    <div class="stats">
      <span>📊 {stats[0]:,} 節點</span>
      <span>🔗 {stats[1]:,} 邊</span>
      <span>🏘️ {stats[2]} 社群</span>
    </div>
  </a>
"""

    html += """
  <a class="full-link" href="../graph.html">➜ 回到完整圖譜（觀察注意：7,616 節點，操作可能緩慢）</a>
</div>
</body>
</html>
"""
    index_path = SUBGRAPH_DIR / "index.html"
    index_path.write_text(html)
    print(f"\n📋  Index: {index_path}")


# ── main ──────────────────────────────────────────────────────────────────
def main():
    if not GRAPH_JSON.exists():
        print(f"❌  graph.json not found at {GRAPH_JSON}")
        print("    Run `graphify .` first.")
        sys.exit(1)

    print(f"📖  Reading {GRAPH_JSON} ...")
    data = json.loads(GRAPH_JSON.read_text())
    nodes = data.get("nodes", [])
    links = data.get("links", data.get("edges", []))
    print(f"    Master graph: {len(nodes)} nodes, {len(links)} edges")

    results = {}
    print()
    for name, cfg in GROUPS.items():
        print(f"━━━  {cfg['label']} ({name})  ━━━")
        n, e, c = slice_group(cfg, nodes, links, name)
        results[name] = (n, e, c)

    print(f"\n━━━  Summary  ━━━")
    total = sum(v[0] for v in results.values())
    print(f"    Total sub-graph nodes: {total} (master: {len(nodes)})")
    print(f"    Noise reduction: {(len(nodes) - total):,} nodes filtered")
    print(f"    Avg sub-graph size: {total // 4:,} nodes")

    build_index(results, GROUPS)

    print(f"\n✅  Done. Open graphify-out/subgraphs/index.html in a browser.")


if __name__ == "__main__":
    main()
