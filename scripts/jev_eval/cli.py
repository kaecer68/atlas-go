# jev-docs: https://docs.typesafe.ai/primitives  (this CLI only drives jevkit; no HTTP here)
"""CLI for the Jev shadow-evaluation framework.

    python3 scripts/jev_eval/cli.py specs
    python3 scripts/jev_eval/cli.py collect --spec industry_l1 --out-dir /tmp/run \
        --spec-arg panel=/tmp/panel.jsonl [--concurrency 4] [--offline]
    python3 scripts/jev_eval/cli.py score   --spec industry_l1 --out-dir /tmp/run \
        --spec-arg panel=/tmp/panel.jsonl [--leakage-run-dir /tmp/leak]
    python3 scripts/jev_eval/cli.py report  --spec industry_l1 --out-dir /tmp/run \
        --spec-arg panel=/tmp/panel.jsonl [--title "..."] [--leakage-run-dir /tmp/leak]
    python3 scripts/jev_eval/cli.py all     ... (collect + score + report)

`score` and `report` never call the API: they recompute from run_dir/requests.jsonl
plus a deterministic rebuild of the spec, so a metric can be revisited for free.

Shadow-only guarantee: nothing here is imported by a product path; the CLI writes
only inside --out-dir.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from typing import Any, Dict, List, Mapping, Optional

if __package__ in (None, ""):  # allow `python3 scripts/jev_eval/cli.py`
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))

from scripts.jev_eval import metrics as metrics_mod  # noqa: E402
from scripts.jev_eval import report as report_mod  # noqa: E402
from scripts.jev_eval import runner  # noqa: E402
from scripts.jev_eval.spec import TaskSpec, parse_spec_args  # noqa: E402
from scripts.jev_eval.specs.industry_l1 import IndustryL1Spec  # noqa: E402

SPECS: Dict[str, TaskSpec] = {IndustryL1Spec.name: IndustryL1Spec()}


def _load_spec(name: str) -> TaskSpec:
    if name not in SPECS:
        raise SystemExit(f"unknown spec {name!r}; known: {', '.join(sorted(SPECS))}")
    return SPECS[name]


def _observations(build, judgments, repeat_aggregate: str = "first") -> List[metrics_mod.Observation]:
    scores = runner.score_cases(build, judgments, repeat_aggregate=repeat_aggregate)
    out: List[metrics_mod.Observation] = []
    for case in build.cases:
        out.append(
            metrics_mod.Observation(
                case_id=case.case_id,
                group_id=case.group_id,
                gt=case.gt,
                score=scores.get(case.case_id),
                baselines=case.baselines,
            )
        )
    return out


def _metrics_path(run_dir: str) -> str:
    return os.path.join(run_dir, "metrics.json")


def _build_and_metrics(args, *, log=print) -> Dict[str, Any]:
    spec = _load_spec(args.spec)
    cfg = spec.merged_args(parse_spec_args(args.spec_arg), spec.defaults)
    build = spec.build(cfg)
    judgments, manifest = runner.load_judgments(args.out_dir)
    obs = _observations(build, judgments, repeat_aggregate=getattr(args, "repeat_aggregate", "first"))
    baseline_names = [b for b in str(cfg.get("baselines") or "").split(",") if b.strip()]
    payload = metrics_mod.evaluate(
        obs,
        baseline_names=baseline_names,
        calib_fraction=args.calib_fraction,
        min_precision=args.min_precision,
        bootstrap_iterations=args.bootstrap,
        seed=args.seed,
    )
    tokens = int(manifest.get("spec_meta", {}).get("tokens_estimate", 0) or 0)
    tokens = sum(int(row.get("tokens", 0) or 0) for row in judgments.values() if row.get("ok"))
    payload["cost"] = {
        "tokens": tokens,
        "price_per_mtok": manifest.get("price_per_mtok", runner.DEFAULT_PRICE_PER_MTOK),
        "usd": round(tokens / 1_000_000 * float(manifest.get("price_per_mtok", runner.DEFAULT_PRICE_PER_MTOK)), 6),
        "requests": len(judgments),
        "failed_requests": sum(1 for row in judgments.values() if not row.get("ok")),
    }
    payload["manifest_summary"] = {
        "spec": manifest.get("spec"),
        "spec_version": manifest.get("spec_version"),
        "model": manifest.get("model"),
        "repeat": manifest.get("repeat"),
        "requests": manifest.get("requests"),
        "cases": manifest.get("cases"),
        "spec_meta": manifest.get("spec_meta", {}),
        "requests_fingerprint": manifest.get("requests_fingerprint"),
        "created_at": manifest.get("created_at"),
        "git_rev": manifest.get("git_rev"),
    }
    return {"payload": payload, "manifest": manifest, "build": build, "spec": spec, "cfg": cfg}


def _leakage_metrics(path: Optional[str]) -> Optional[Mapping[str, Any]]:
    """Summarise a leakage-probe run into {auc:{point,ci}, base_rate, n}."""
    if not path:
        return None
    metrics_path = os.path.join(path, "metrics.json")
    if not os.path.exists(metrics_path):
        return None
    with open(metrics_path, "r", encoding="utf-8") as fh:
        probe = json.load(fh)
    ev = probe.get("evaluation", {}) or {}
    lift = (ev.get("lift") or {}).get("jev_auc", {}) or {}
    return {
        "auc": {"point": lift.get("point"), "ci": lift.get("ci")},
        "base_rate": ev.get("base_rate"),
        "n": ev.get("n"),
        "source": os.path.abspath(path),
    }


def cmd_repeats(args) -> int:
    """Quantify sampling non-determinism: per-repeat agreement and score spread.

    The judge is a probabilistic primitive, so two identical requests can answer
    differently. This command makes that visible instead of letting it hide
    inside a single headline number.
    """
    judgments, _manifest = runner.load_judgments(args.out_dir)
    requests_with_repeats = 0
    per_case: Dict[str, List[float]] = {}
    for row in judgments.values():
        group = [r for r in (row.get("repeats") or [row]) if r.get("ok")]
        if len(group) < 2:
            continue
        requests_with_repeats += 1
        for repeat_row in group:
            for qid, answer in (repeat_row.get("answers") or {}).items():
                if isinstance(answer, dict) and isinstance(answer.get("noul"), (int, float)):
                    per_case.setdefault(f"{row.get('request_id')}:{qid}", []).append(float(answer["noul"]))
    cases = [values for values in per_case.values() if len(values) >= 2]
    if not cases:
        print(json.dumps({"requests_with_repeats": requests_with_repeats, "cases_with_repeats": 0,
                          "note": "run with --repeat N > 1 to measure agreement"}, ensure_ascii=False, indent=2))
        return 0
    identical = sum(1 for values in cases if max(values) - min(values) <= 1e-9)
    spreads = sorted(max(values) - min(values) for values in cases)

    def _q(p: float) -> float:
        return spreads[min(len(spreads) - 1, int(p * len(spreads)))]

    print(
        json.dumps(
            {
                "requests_with_repeats": requests_with_repeats,
                "cases_with_repeats": len(cases),
                "identical_fraction": round(identical / len(cases), 4),
                "score_spread_mean": round(sum(spreads) / len(spreads), 4),
                "score_spread_p50": round(_q(0.5), 4),
                "score_spread_p95": round(_q(0.95), 4),
                "score_spread_max": round(spreads[-1], 4),
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0


def cmd_specs(args) -> int:
    for name, spec in sorted(SPECS.items()):
        print(f"{name} v{spec.version}: {spec.description}")
    return 0


def cmd_collect(args) -> int:
    spec = _load_spec(args.spec)
    cfg = spec.merged_args(parse_spec_args(args.spec_arg), spec.defaults)
    build = spec.build(cfg)
    stats = runner.collect(
        spec=spec,
        build=build,
        run_dir=args.out_dir,
        model=args.model,
        concurrency=args.concurrency,
        repeat=args.repeat,
        force=args.force,
        offline=args.offline,
        args=cfg,
        log=print,
    )
    print(json.dumps(stats.as_dict(), ensure_ascii=False))
    return 0


def cmd_score(args) -> int:
    data = _build_and_metrics(args)
    payload = data["payload"]
    verdict = metrics_mod.grade(
        payload,
        leakage=_leakage_metrics(args.leakage_run_dir),
        min_eval_cases=args.min_eval_cases,
    )
    payload["verdict"] = verdict
    report_mod.dump_json(_metrics_path(args.out_dir), payload)
    print(json.dumps(verdict, ensure_ascii=False, indent=2))
    return 0


def cmd_report(args) -> int:
    data = _build_and_metrics(args)
    payload = data["payload"]
    verdict = metrics_mod.grade(
        payload,
        leakage=_leakage_metrics(args.leakage_run_dir),
        min_eval_cases=args.min_eval_cases,
    )
    payload["verdict"] = verdict
    report_mod.dump_json(_metrics_path(args.out_dir), payload)
    text = report_mod.render_markdown(
        title=args.title,
        spec_name=data["spec"].name,
        spec_version=data["spec"].version,
        manifest=data["manifest"],
        metrics=payload,
        verdict=verdict,
        extra_notes=parse_notes(args.note),
    )
    path = os.path.join(args.out_dir, "report.md")
    report_mod.write_report(path, text)
    print(path)
    return 0


def cmd_all(args) -> int:
    rc = cmd_collect(args)
    if rc:
        return rc
    rc = cmd_score(args)
    if rc:
        return rc
    return cmd_report(args)


def parse_notes(notes: List[str]) -> List[str]:
    return list(notes or [])


def build_parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(description="Jev shadow-evaluation framework (reusable, offline-rescorable)")
    sub = ap.add_subparsers(dest="command", required=True)

    def common(p: argparse.ArgumentParser, *, need_run_dir: bool = True) -> None:
        p.add_argument("--spec", required=True, help="task spec name (see `specs`)")
        p.add_argument("--spec-arg", action="append", default=[], metavar="K=V", help="spec argument (repeatable)")
        if need_run_dir:
            p.add_argument("--out-dir", required=True, help="run directory")
        p.add_argument("--model", default=None, help="Jev model id (default: jevkit pinned id)")

    p = sub.add_parser("specs", help="list task specs")
    p.set_defaults(func=cmd_specs)

    p = sub.add_parser("repeats", help="agreement / score spread across self-consistency repeats")
    p.add_argument("--out-dir", required=True)
    p.set_defaults(func=cmd_repeats)

    p = sub.add_parser("collect", help="call Jev (shadow) and log judgments")
    common(p)
    p.add_argument("--concurrency", type=int, default=4)
    p.add_argument("--repeat", type=int, default=1, help="self-consistency repeats (each logged separately)")
    p.add_argument("--force", action="store_true", help="re-call even when a cached judgment exists")
    p.add_argument("--offline", action="store_true", help="never call the API (metrics-only reruns)")
    p.set_defaults(func=cmd_collect)

    for name, fn in (("score", cmd_score), ("report", cmd_report)):
        p = sub.add_parser(name, help="recompute metrics / render the report from logged judgments")
        common(p)
        p.add_argument("--calib-fraction", type=float, default=0.6, help="share of EARLY dates used for calibration")
        p.add_argument("--min-precision", type=float, default=0.55, help="calibration target precision")
        p.add_argument("--bootstrap", type=int, default=1000, help="clustered bootstrap iterations")
        p.add_argument("--seed", type=int, default=20260925)
        p.add_argument("--min-eval-cases", type=int, default=200)
        p.add_argument("--repeat-aggregate", choices=("first", "mean"), default="first",
                       help="how to combine self-consistency repeats (mean = average noul over repeats)")
        p.add_argument("--leakage-run-dir", default=None, help="run dir of the memorisation probe")
        if fn is cmd_report:
            p.add_argument("--title", default="Jev shadow evaluation")
            p.add_argument("--note", action="append", default=[], help="limitation note (repeatable)")
        p.set_defaults(func=fn)

    p = sub.add_parser("all", help="collect + score + report in one go")
    common(p)
    p.add_argument("--concurrency", type=int, default=4)
    p.add_argument("--repeat", type=int, default=1)
    p.add_argument("--force", action="store_true")
    p.add_argument("--offline", action="store_true")
    p.add_argument("--calib-fraction", type=float, default=0.6)
    p.add_argument("--min-precision", type=float, default=0.55)
    p.add_argument("--bootstrap", type=int, default=1000)
    p.add_argument("--seed", type=int, default=20260925)
    p.add_argument("--min-eval-cases", type=int, default=200)
    p.add_argument("--repeat-aggregate", choices=("first", "mean"), default="first")
    p.add_argument("--leakage-run-dir", default=None)
    p.add_argument("--title", default="Jev shadow evaluation")
    p.add_argument("--note", action="append", default=[])
    p.set_defaults(func=cmd_all)
    return ap


def main(argv: Optional[List[str]] = None) -> int:
    ap = build_parser()
    args = ap.parse_args(argv)
    from jevkit import DEFAULT_MODEL

    if getattr(args, "model", None) is None:
        args.model = DEFAULT_MODEL
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
