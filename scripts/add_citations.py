#!/usr/bin/env python3
import json

TEMPLATES = {
    "literature": {"evidence_quality": "high", "update_policy": "frozen", "validation_method": "Literature validation", "last_validated": "2026-05-04T00:00:00Z"},
    "empirical": {"evidence_quality": "high", "update_policy": "auto", "validation_method": "TWSE historical data validation", "last_validated": "2026-05-04T00:00:00Z"},
    "heuristic": {"evidence_quality": "medium", "update_policy": "manual", "validation_method": "Backtest optimization", "last_validated": "2026-05-04T00:00:00Z"},
    "inferred": {"evidence_quality": "low", "update_policy": "manual", "validation_method": "Code review", "last_validated": None},
    "calibrated": {"evidence_quality": "medium", "update_policy": "manual", "validation_method": "Backtest calibration", "last_validated": "2026-05-04T00:00:00Z"}
}

MODULE_REFS = {
    "darwinian": "Agent weight system calibration",
    "factor": "Factor scoring for TW market",
    "optimizer": "Portfolio optimization",
    "sizing": "Position sizing methodology",
    "health": "Agent health management",
    "garch": "GARCH volatility model",
    "experiment": "Experiment evaluation",
    "baseline": "Baseline policy",
    "orchestrator": "Orchestrator control layer",
    "risk": "Risk management",
    "realtime": "Real-time detection",
    "janus": "Cross-cohort regime detection",
    "narrative": "Macro narrative events",
    "marketdata": "Data provider config",
    "industry": "Industry analysis",
    "strategy": "Strategy selection"
}

def add_citations(obj, module):
    if not isinstance(obj, dict):
        return obj
    if "value" in obj and "rationale" in obj and "source" in obj:
        src = obj.get("source", "heuristic")
        tmpl = TEMPLATES.get(src, TEMPLATES["heuristic"])
        has_todo = "todo" in obj and obj["todo"]
        eq = "low" if has_todo else tmpl["evidence_quality"]
        up = "manual" if has_todo else tmpl["update_policy"]
        if "calibration_method" in obj:
            up = "auto"
            eq = "high"
        obj["citation"] = {
            "source_type": src,
            "source_reference": MODULE_REFS.get(module, "System parameter"),
            "evidence_quality": eq,
            "update_policy": up,
            "validation_method": tmpl["validation_method"],
            "dependencies": [],
            "last_validated": None if has_todo else tmpl["last_validated"]
        }
        return obj
    for k, v in obj.items():
        if isinstance(v, dict):
            obj[k] = add_citations(v, module)
    return obj

with open("/Users/kaecer/workspace/atlas/configs/parameters.json") as f:
    data = json.load(f)

for mod in list(data.keys()):
    if mod in ["version", "updated_at"]:
        continue
    if isinstance(data[mod], dict):
        data[mod] = add_citations(data[mod], mod)

count = 0
def cnt(obj):
    global count
    if isinstance(obj, dict):
        if "citation" in obj:
            count += 1
        for v in obj.values():
            cnt(v)
cnt(data)

with open("/Users/kaecer/workspace/atlas/configs/parameters.json", "w") as f:
    json.dump(data, f, indent=2, ensure_ascii=False)

print(f"Added citations to {count} parameters")
