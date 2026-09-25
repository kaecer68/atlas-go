"""Report renderer: metrics → markdown with an explicit graded conclusion.

The report is the artefact a reviewer reads, so it always states, in this order:
the grade, the sample count, the cost, the calibrated threshold and how it was
obtained, the discrimination and operating-point numbers, the baseline
comparison, the calibration table, and the limitations that were NOT resolved.
"""
from __future__ import annotations

import json
from typing import Any, Dict, List, Mapping, Optional


def _pct(x: Optional[float], digits: int = 2) -> str:
    return "n/a" if x is None else f"{x * 100:.{digits}f}%"


def _num(x: Optional[float], digits: int = 4) -> str:
    return "n/a" if x is None else f"{x:.{digits}f}"


def _ci(block: Optional[Mapping[str, Any]]) -> str:
    if not block:
        return "n/a"
    ci = block.get("ci") or [None, None]
    return f"{_num(block.get('point'))} [{_num(ci[0])}, {_num(ci[1])}]"


def render_markdown(
    *,
    title: str,
    spec_name: str,
    spec_version: str,
    manifest: Mapping[str, Any],
    metrics: Mapping[str, Any],
    verdict: Mapping[str, Any],
    extra_notes: Optional[List[str]] = None,
) -> str:
    ev = metrics.get("evaluation", {}) or {}
    jev = ev.get("jev", {}) or {}
    calib = metrics.get("threshold_calibration", {}) or {}
    lift = ev.get("lift", {}) or {}
    cost = metrics.get("cost", {}) or {}
    lines: List[str] = []
    lines.append(f"# {title}")
    lines.append("")
    lines.append(f"- spec: `{spec_name}` v{spec_version}")
    lines.append(f"- model: `{manifest.get('model', 'n/a')}` (repeat={manifest.get('repeat')})")
    lines.append(f"- window: {manifest.get('spec_meta', {}).get('date_start', '?')} .. {manifest.get('spec_meta', {}).get('date_end', '?')}")
    lines.append(f"- requests: {manifest.get('requests')} · cases: {manifest.get('cases')}")
    lines.append(f"- run dir: `{manifest.get('run_dir', '')}`")
    lines.append(f"- git: `{manifest.get('git_rev', '')}`")
    lines.append("")

    lines.append("## 分級結論 (§4.5)")
    lines.append("")
    lines.append(f"**{verdict.get('verdict')}** — {verdict.get('detail')}")
    lines.append("")
    lines.append(f"- 樣本數：held-out {verdict.get('held_out_cases')} cases（answered fraction {verdict.get('answered_fraction')}）")
    lines.append(f"- 成本：${cost.get('usd', 0):.4f}（{cost.get('tokens', 0)} input tokens @ ${manifest.get('price_per_mtok')}/Mtok）")
    lines.append(f"- 洩漏探針：{verdict.get('leakage_note') or 'not run'}")
    if verdict.get("reasons"):
        lines.append(f"- 未通過的閘門：{'; '.join(verdict['reasons'])}")
    lines.append("")

    lines.append("## 門檻校準 (§3：不得照抄 cookbook)")
    lines.append("")
    calib_block = metrics.get("calibration", {}) or {}
    lines.append(f"- 校準集：{calib.get('n')} cases（{len(calib_block.get('groups', []))} 個交易日）· base rate {_pct(calib_block.get('base_rate'))}")
    lines.append(f"- 目標 min_precision={calib.get('min_precision')} → threshold **{calib.get('used_threshold')}**")
    if calib.get("precision") is not None:
        lines.append(f"- 校準集上：precision {_num(calib.get('precision'))} · recall {_num(calib.get('recall'))} (tp={calib.get('tp')} fp={calib.get('fp')} fn={calib.get('fn')})")
    if calib.get("fallback"):
        lines.append(f"- ⚠ {calib['fallback']}")
    lines.append("")

    lines.append("## 判別力與操作點（held-out）")
    lines.append("")
    lines.append(f"- held-out：{ev.get('n')} cases（{len(ev.get('groups', []))} 個交易日）· base rate {_pct(ev.get('base_rate'))}")
    lines.append(f"- Jev AUC（pooled，主要指標）：**{_ci(lift.get('jev_auc'))}**　（0.5 = 無資訊）")
    within = lift.get("jev_auc_within_group", {}) or {}
    lines.append(f"- Jev AUC（同日產業間排名，次要指標）：**{_ci(within)}**（可用交易日 {within.get('groups_used')}）")
    lines.append(f"- Jev precision {_num(jev.get('precision'))} (CI {[round(x, 4) if x is not None else None for x in (jev.get('precision_ci') or [None, None])]}) · recall {_num(jev.get('recall'))} · coverage {_pct(jev.get('coverage'))} · f1 {_num(jev.get('f1'))}")
    if jev.get("operating_point_reliable") is False:
        lines.append(f"- ⚠ 操作點不可用：{jev.get('operating_point_note')}")
    lines.append(f"- Brier {_num(jev.get('brier'))} · ECE {_num(jev.get('ece'))}")
    lines.append(f"- 「一律看多」的 precision 上限參考 = base rate {_pct(ev.get('base_rate'))}")
    lines.append("")

    lines.append("## 相對 baseline 的 lift")
    lines.append("")
    lines.append("| baseline | n | precision | recall | coverage | AUC | AUC(同日) | ΔAUC [CI] | ΔAUC(同日) [CI] | Δprecision [CI] | filter lift [CI] (retained) |")
    lines.append("|---|---|---|---|---|---|---|---|---|---|---|")
    for name, entry in (ev.get("baselines") or {}).items():
        if not entry.get("available"):
            lines.append(f"| {name} | – | – | – | – | – | – | – | unavailable |")
            continue
        lift_entry = lift.get(name, {}) if isinstance(lift.get(name), dict) else {}
        filt = lift_entry.get("filter", {}) if isinstance(lift_entry, dict) else {}
        reliable = entry.get("operating_point_reliable", True)
        lines.append(
            "| {name} | {n} | {prec} | {rec} | {cov} | {auc} | {aucw} | {dauc} | {daucw} | {dprec} | {fl} ({ret}) |".format(
                name=name,
                n=entry.get("n"),
                prec=_num(entry.get("precision")) if reliable else "n/a*",
                rec=_num(entry.get("recall")) if reliable else "n/a*",
                cov=_pct(entry.get("coverage")),
                auc=_num(entry.get("auc")),
                aucw=_num(entry.get("auc_within_group")),
                dauc=_ci(lift_entry.get("delta_auc")),
                daucw=_ci(lift_entry.get("delta_auc_within_group")),
                dprec=_ci(lift_entry.get("delta_precision")) if jev.get("operating_point_reliable", True) else "n/a*",
                fl=_ci(filt.get("lift")) if jev.get("operating_point_reliable", True) else "n/a*",
                ret=_pct(filt.get("retained_coverage")) if filt else "n/a",
            )
        )
    lines.append("")
    lines.append("ΔAUC / Δprecision / filter lift 的信賴區間 = 以交易日為 cluster 的 paired bootstrap（1000 次）。")
    lines.append("`n/a*` = 該操作點的正向呼叫數 < 30，precision/recall/lift 不具解釋力（見上列 ⚠）。")
    lines.append("")
    lines.append("Baseline 出處：")
    for name, prov in ((manifest.get("spec_meta", {}) or {}).get("baseline_provenance", {}) or {}).items():
        lines.append(f"- `{name}`：{prov}")
    lines.append("")

    lines.append("## 校準表（reliability，held-out）")
    lines.append("")
    lines.append("| bin | n | mean score | accuracy | gap |")
    lines.append("|---|---|---|---|---|")
    for row in ev.get("reliability", []) or []:
        lines.append(f"| {row['bin']} | {row['n']} | {_num(row['mean_score'])} | {_num(row['accuracy'])} | {_num(row['gap'])} |")
    lines.append("")

    sweep = ev.get("sweep") or []
    if sweep:
        lines.append("## 門檻掃描（held-out）")
        lines.append("")
        lines.append("| threshold | up calls | coverage | precision | recall | f1 |")
        lines.append("|---|---|---|---|---|---|")
        for row in sweep:
            lines.append(
                f"| {row['threshold']} | {row['up_calls']} | {_pct(row['coverage'])} | {_num(row['precision'])} | {_num(row['recall'])} | {_num(row['f1'])} |"
            )
        lines.append("")

    if extra_notes:
        lines.append("## 限制與未解問題")
        lines.append("")
        for note in extra_notes:
            lines.append(f"- {note}")
        lines.append("")
    return "\n".join(lines)


def write_report(path: str, text: str) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)


def dump_json(path: str, payload: Any) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
