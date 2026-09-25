# jev-docs: https://docs.typesafe.ai/primitives  (threshold calibration follows the
#   official guidance to tune against your own data — never the cookbook values)
"""Metrics for the Jev shadow evaluation: calibration, discrimination, lift, grade.

All functions are pure and take already-collected judgments, so a run can be
re-scored offline as many times as wanted without spending API budget.

Definitions (kept explicit because the industry layer has a base rate well below
50%: a cost-adjusted 5-session "up" is the minority outcome):

  score        Jev's `noul` probability that the case outcome is positive (up).
  gt           verified ground truth (bool), regenerable by code.
  precision    P(gt=1 | we called "up") — the hit rate of the positive calls.
  recall       P(we called "up" | gt=1).
  coverage     share of cases on which a positive call was made.
  base_rate    P(gt=1) — the precision of the "always call up" strategy, i.e. the
               floor any lift claim has to beat.
  AUC          rank-based discrimination of `score` against `gt` (0.5 = no
               information). Threshold-free, so it survives the small-sample
               threshold problem.
  ΔAUC         AUC(jev) − AUC(baseline) — the "does Jev add" number.
  filter_lift  within the cases the baseline already called "up", the precision
               gain from keeping only those Jev also calls "up" (this is the
               realistic incremental-value question for a platform that already
               consumes the baseline), reported with the retained coverage.
  ECE          expected calibration error of the probabilities themselves.

Uncertainty is clustered by `group_id` (one trading date) on purpose: all
industries judged in one request share a market day, so treating those rows as
independent would understate the interval.
"""
from __future__ import annotations

import math
import random
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Mapping, Optional, Sequence, Tuple

_REPO_SCRIPTS = None
try:  # jevkit lives next to this package (scripts/jevkit.py)
    from jevkit import calibrate_threshold
except ImportError:  # `python3 scripts/jev_eval/cli.py` — put scripts/ on the path
    import os
    import sys

    _scripts = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    if _scripts not in sys.path:
        sys.path.insert(0, _scripts)
    from jevkit import calibrate_threshold


#: Fewer positive calls than this and the operating point is noise, not evidence.
MIN_UP_CALLS = 30


@dataclass(frozen=True)
class Observation:
    case_id: str
    group_id: str
    gt: bool
    score: Optional[float]
    baselines: Mapping[str, Optional[float]]


# --------------------------------------------------------------------- helpers
def wilson_interval(hits: int, n: int, z: float = 1.959963984540054) -> Tuple[float, float]:
    """Wilson score interval; (0, 0) for an empty sample (mirrors the Go helper)."""
    if n <= 0 or hits < 0 or hits > n:
        return 0.0, 0.0
    p = hits / n
    denom = 1 + z * z / n
    center = (p + z * z / (2 * n)) / denom
    margin = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / denom
    return max(0.0, center - margin), min(1.0, center + margin)


def auc(scores: Sequence[float], labels: Sequence[bool]) -> Optional[float]:
    """Rank-based AUC with average ranks for ties. None when one class is empty."""
    pairs = [(float(s), bool(y)) for s, y in zip(scores, labels)]
    if not pairs:
        return None
    pos = [p for p in pairs if p[1]]
    neg = [p for p in pairs if not p[1]]
    if not pos or not neg:
        return None
    ordered = sorted(pairs, key=lambda p: p[0])
    ranks: List[float] = [0.0] * len(ordered)
    i = 0
    while i < len(ordered):
        j = i
        while j + 1 < len(ordered) and ordered[j + 1][0] == ordered[i][0]:
            j += 1
        avg = (i + j) / 2 + 1  # 1-indexed average rank
        for k in range(i, j + 1):
            ranks[k] = avg
        i = j + 1
    rank_sum_pos = sum(r for r, p in zip(ranks, ordered) if p[1])
    n_pos, n_neg = len(pos), len(neg)
    return (rank_sum_pos - n_pos * (n_pos + 1) / 2) / (n_pos * n_neg)


def brier(scores: Sequence[float], labels: Sequence[bool]) -> Optional[float]:
    if not scores:
        return None
    return sum((s - (1.0 if y else 0.0)) ** 2 for s, y in zip(scores, labels)) / len(scores)


def reliability_table(scores: Sequence[float], labels: Sequence[bool], bins: int = 10) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    for b in range(bins):
        lo, hi = b / bins, (b + 1) / bins
        idx = [i for i, s in enumerate(scores) if (lo <= s < hi) or (b == bins - 1 and s == 1.0)]
        if not idx:
            rows.append({"bin": f"[{lo:.1f},{hi:.1f})", "n": 0, "mean_score": None, "accuracy": None, "gap": None})
            continue
        mean_score = sum(scores[i] for i in idx) / len(idx)
        acc = sum(1 for i in idx if labels[i]) / len(idx)
        rows.append(
            {
                "bin": f"[{lo:.1f},{hi:.1f})",
                "n": len(idx),
                "mean_score": round(mean_score, 4),
                "accuracy": round(acc, 4),
                "gap": round(mean_score - acc, 4),
            }
        )
    return rows


def ece(scores: Sequence[float], labels: Sequence[bool], bins: int = 10) -> Optional[float]:
    table = reliability_table(scores, labels, bins)
    total = sum(r["n"] for r in table)
    if total == 0:
        return None
    return sum(r["n"] * abs(r["gap"]) for r in table if r["n"]) / total


def cluster_bootstrap(
    observations: Sequence[Observation],
    statistic,
    *,
    iterations: int = 1000,
    seed: int = 20260925,
    alpha: float = 0.05,
) -> Tuple[Optional[float], Optional[float], Optional[float]]:
    """Percentile bootstrap over clusters (group_id), returning (point, lo, hi).

    `statistic` receives a list of Observations and returns a float or None.
    Clusters are resampled with replacement, so within-day correlation between
    industries does not fake significance.
    """
    point = statistic(list(observations))
    if point is None:
        return None, None, None
    by_group: Dict[str, List[Observation]] = {}
    for obs in observations:
        by_group.setdefault(obs.group_id, []).append(obs)
    groups = sorted(by_group)
    if len(groups) < 3:
        return point, None, None
    rng = random.Random(seed)
    samples: List[float] = []
    for _ in range(iterations):
        draw: List[Observation] = []
        for _ in range(len(groups)):
            g = groups[rng.randrange(len(groups))]
            draw.extend(by_group[g])
        value = statistic(draw)
        if value is not None:
            samples.append(value)
    if not samples:
        return point, None, None
    samples.sort()
    lo = samples[int(math.floor(alpha / 2 * len(samples)))]
    hi = samples[min(len(samples) - 1, int(math.ceil((1 - alpha / 2) * len(samples))) - 1)]
    return point, lo, hi


def _decision_stats(obs: Sequence[Observation], decisions: Sequence[Optional[bool]]) -> Dict[str, Any]:
    """Precision/recall/coverage for a set of binary calls (None = abstain)."""
    called = [(o, d) for o, d in zip(obs, decisions) if d is not None]
    positives = [o for o, _ in called if o.gt]
    tp = sum(1 for o, d in called if d and o.gt)
    fp = sum(1 for o, d in called if d and not o.gt)
    up_calls = tp + fp
    precision = tp / up_calls if up_calls else None
    recall = tp / len(positives) if positives else None
    f1 = None
    if precision is not None and recall is not None and (precision + recall) > 0:
        f1 = 2 * precision * recall / (precision + recall)
    lo, hi = wilson_interval(tp, up_calls)
    return {
        "n": len(called),
        "up_calls": up_calls,
        "coverage": (up_calls / len(called)) if called else None,
        "precision": precision,
        "precision_ci": [lo, hi] if up_calls else None,
        "recall": recall,
        "f1": f1,
        "true_positives": tp,
        "false_positives": fp,
    }


def within_group_auc(observations: Sequence[Observation], key: str = "jev", threshold_metric: Optional[str] = None) -> Optional[float]:
    """Mean AUC computed INSIDE each group (trading date), then averaged.

    The pooled AUC mixes two different questions: "does the model separate good
    days from bad days" and "does it rank the industries of one day correctly".
    At the industry layer the second one is the actionable one (rotation), and it
    is the only one that is unaffected by market-wide drift, so it is reported
    separately. Groups with a single class (every industry moved the same way)
    carry no ranking information and are skipped, and the count of usable groups
    is reported so the difference cannot hide.
    """
    by_group: Dict[str, List[Observation]] = {}
    for obs in observations:
        by_group.setdefault(obs.group_id, []).append(obs)
    values: List[float] = []
    for group in sorted(by_group):
        subset = by_group[group]
        if key == "jev":
            scored = [o for o in subset if o.score is not None]
            if len(scored) < 2:
                continue
            value = auc([o.score for o in scored], [o.gt for o in scored])
        else:
            scored = [o for o in subset if o.baselines.get(key) is not None]
            if len(scored) < 2:
                continue
            value = auc([o.baselines[key] for o in scored], [o.gt for o in scored])
        if value is not None:
            values.append(value)
    if not values:
        return None
    return sum(values) / len(values)


def usable_groups(observations: Sequence[Observation], key: str = "jev") -> int:
    count = 0
    by_group: Dict[str, List[Observation]] = {}
    for obs in observations:
        by_group.setdefault(obs.group_id, []).append(obs)
    for group in sorted(by_group):
        subset = by_group[group]
        if key == "jev":
            scored = [o for o in subset if o.score is not None]
        else:
            scored = [o for o in subset if o.baselines.get(key) is not None]
        if len(scored) >= 2 and auc(
            [o.score if key == "jev" else o.baselines[key] for o in scored], [o.gt for o in scored]
        ) is not None:
            count += 1
    return count


def threshold_sweep(obs: Sequence[Observation], thresholds: Iterable[float]) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    for th in thresholds:
        stats = _decision_stats(obs, [None if o.score is None else (o.score >= th) for o in obs])
        rows.append({"threshold": round(th, 3), **{k: stats[k] for k in ("n", "up_calls", "coverage", "precision", "recall", "f1")}})
    return rows


def _fmt(x: Optional[float], digits: int = 4) -> Optional[float]:
    return None if x is None else round(float(x), digits)


def evaluate(
    observations: Sequence[Observation],
    *,
    baseline_names: Sequence[str] = (),
    calib_fraction: float = 0.6,
    min_precision: float = 0.55,
    bootstrap_iterations: int = 1000,
    seed: int = 20260925,
    sweep_thresholds: Sequence[float] = (0.1, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.55, 0.6, 0.7),
) -> Dict[str, Any]:
    """Calibrate on the early split, evaluate on the held-out split, compare to baselines."""
    answered = [o for o in observations if o.score is not None]
    groups = sorted({o.group_id for o in observations})
    n_calib_groups = max(1, int(round(len(groups) * calib_fraction))) if groups else 0
    calib_groups = set(groups[:n_calib_groups])
    eval_groups = set(groups[n_calib_groups:])

    calib = [o for o in answered if o.group_id in calib_groups]
    holdout = [o for o in answered if o.group_id in eval_groups]

    result: Dict[str, Any] = {
        "n_cases": len(observations),
        "n_answered": len(answered),
        "n_unanswered": len(observations) - len(answered),
        "groups": len(groups),
        "calibration": {
            "groups": sorted(calib_groups),
            "n": len(calib),
            "base_rate": _fmt(sum(1 for o in calib if o.gt) / len(calib)) if calib else None,
        },
        "evaluation": {
            "groups": sorted(eval_groups),
            "n": len(holdout),
        },
    }

    # ---- threshold calibration on the calibration split only (§3) ----------
    calib_pairs = [(o.score, o.gt) for o in calib]
    threshold_info: Dict[str, Any] = {"min_precision": min_precision, "n": len(calib_pairs)}
    threshold: Optional[float] = None
    if calib_pairs:
        cal = calibrate_threshold(calib_pairs, min_precision=min_precision)
        threshold_info.update(cal)
        threshold = cal.get("threshold")
    if threshold is None:
        threshold = 0.5
        threshold_info["fallback"] = (
            "no calibration threshold reached min_precision on the calibration split; "
            "the fixed 0.5 default is used and the conclusion is graded accordingly"
        )
    threshold_info["used_threshold"] = threshold
    result["threshold_calibration"] = threshold_info

    ev = result["evaluation"]
    ev["base_rate"] = _fmt(sum(1 for o in holdout if o.gt) / len(holdout)) if holdout else None
    ev["always_up_precision"] = ev["base_rate"]

    # ---- Jev at the calibrated threshold -----------------------------------
    jev_decisions = [None if o.score is None else (o.score >= threshold) for o in holdout]
    ev["jev"] = _decision_stats(holdout, jev_decisions)

    # ---- discrimination + calibration --------------------------------------
    if holdout:
        scores = [o.score for o in holdout]
        labels = [o.gt for o in holdout]
        ev["jev"]["auc"] = _fmt(auc(scores, labels))
        ev["jev"]["brier"] = _fmt(brier(scores, labels))
        ev["jev"]["ece"] = _fmt(ece(scores, labels))
        ev["jev"]["auc_within_group"] = _fmt(within_group_auc(holdout, "jev"))
        ev["jev"]["groups_used_within"] = usable_groups(holdout, "jev")
        ev["reliability"] = reliability_table(scores, labels)
        ev["sweep"] = threshold_sweep(holdout, sweep_thresholds)
        # A 1-call operating point is not an operating point: flag it so no
        # reader (and no report template) presents its precision as evidence.
        up_calls = ev["jev"].get("up_calls") or 0
        ev["jev"]["operating_point_reliable"] = up_calls >= MIN_UP_CALLS
        if not ev["jev"]["operating_point_reliable"]:
            ev["jev"]["operating_point_note"] = (
                f"only {up_calls} positive call(s) at the calibrated threshold "
                f"({threshold}); precision/recall/lift on this point are not interpretable"
            )

    # ---- baselines ---------------------------------------------------------
    ev["baselines"] = {}
    for name in baseline_names:
        rows = [(o, o.baselines.get(name)) for o in holdout]
        rows = [(o, v) for o, v in rows if v is not None]
        if not rows:
            ev["baselines"][name] = {"available": False, "reason": "no baseline value for any holdout case"}
            continue
        obs_only = [o for o, _ in rows]
        values = [v for _, v in rows]
        decisions = [v > 0 for v in values]
        entry: Dict[str, Any] = {"available": True, "n": len(rows)}
        entry.update(_decision_stats(obs_only, decisions))
        entry["auc"] = _fmt(auc(values, [o.gt for o in obs_only]))
        entry["auc_within_group"] = _fmt(within_group_auc(holdout, name))
        entry["decision_rule"] = "score > 0"
        entry["operating_point_reliable"] = (entry.get("up_calls") or 0) >= MIN_UP_CALLS
        ev["baselines"][name] = entry

    # ---- lift vs the strongest available baseline --------------------------
    ev["lift"] = _lift_block(holdout, threshold, baseline_names, bootstrap_iterations, seed)

    result["cost"] = None  # filled by the caller (needs the manifest)
    return result


def _lift_block(
    holdout: Sequence[Observation],
    threshold: float,
    baseline_names: Sequence[str],
    iterations: int,
    seed: int,
) -> Dict[str, Any]:
    out: Dict[str, Any] = {}
    if not holdout:
        return out

    def _auc_stat(subset: Sequence[Observation], key: str):
        def stat(sample: Sequence[Observation]) -> Optional[float]:
            scored = [o for o in sample if o.score is not None] if key == "jev" else [
                o for o in sample if o.baselines.get(key) is not None
            ]
            if key == "jev":
                return auc([o.score for o in scored], [o.gt for o in scored])
            return auc([o.baselines[key] for o in scored], [o.gt for o in scored])

        return stat

    def _precision_stat(key: str):
        def stat(sample: Sequence[Observation]) -> Optional[float]:
            if key == "jev":
                decisions = [None if o.score is None else (o.score >= threshold) for o in sample]
            else:
                decisions = [None if o.baselines.get(key) is None else (o.baselines[key] > 0) for o in sample]
            return _decision_stats(sample, decisions)["precision"]

        return stat

    point, lo, hi = cluster_bootstrap(holdout, _auc_stat(holdout, "jev"), iterations=iterations, seed=seed)
    out["jev_auc"] = {"point": _fmt(point), "ci": [_fmt(lo), _fmt(hi)]}

    def _within_stat(key: str):
        def stat(sample: Sequence[Observation]) -> Optional[float]:
            return within_group_auc(sample, key)

        return stat

    w_point, w_lo, w_hi = cluster_bootstrap(holdout, _within_stat("jev"), iterations=iterations, seed=seed)
    out["jev_auc_within_group"] = {
        "point": _fmt(w_point),
        "ci": [_fmt(w_lo), _fmt(w_hi)],
        "groups_used": usable_groups(holdout, "jev"),
    }

    for name in baseline_names:
        if not any(o.baselines.get(name) is not None for o in holdout):
            out[name] = {"available": False}
            continue
        base_point, base_lo, base_hi = cluster_bootstrap(
            holdout, _auc_stat(holdout, name), iterations=iterations, seed=seed
        )
        entry: Dict[str, Any] = {
            "available": True,
            "auc": {"point": _fmt(base_point), "ci": [_fmt(base_lo), _fmt(base_hi)]},
        }

        def _delta_auc_stat(sample: Sequence[Observation], name=name) -> Optional[float]:
            a = _auc_stat(sample, "jev")(sample)
            b = _auc_stat(sample, name)(sample)
            if a is None or b is None:
                return None
            return a - b

        d_point, d_lo, d_hi = cluster_bootstrap(holdout, _delta_auc_stat, iterations=iterations, seed=seed)
        entry["delta_auc"] = {"point": _fmt(d_point), "ci": [_fmt(d_lo), _fmt(d_hi)]}

        def _delta_within_stat(sample: Sequence[Observation], name=name) -> Optional[float]:
            a = within_group_auc(sample, "jev")
            b = within_group_auc(sample, name)
            if a is None or b is None:
                return None
            return a - b

        wd_point, wd_lo, wd_hi = cluster_bootstrap(holdout, _delta_within_stat, iterations=iterations, seed=seed)
        entry["delta_auc_within_group"] = {"point": _fmt(wd_point), "ci": [_fmt(wd_lo), _fmt(wd_hi)]}
        bq_point, bq_lo, bq_hi = cluster_bootstrap(holdout, _within_stat(name), iterations=iterations, seed=seed)
        entry["baseline_auc_within_group"] = {"point": _fmt(bq_point), "ci": [_fmt(bq_lo), _fmt(bq_hi)]}

        def _delta_precision_stat(sample: Sequence[Observation], name=name) -> Optional[float]:
            ja = _precision_stat("jev")(sample)
            ba = _precision_stat(name)(sample)
            if ja is None or ba is None:
                return None
            return ja - ba

        p_point, p_lo, p_hi = cluster_bootstrap(holdout, _delta_precision_stat, iterations=iterations, seed=seed)
        entry["delta_precision"] = {"point": _fmt(p_point), "ci": [_fmt(p_lo), _fmt(p_hi)]}

        # Incremental value on top of the baseline: keep only the baseline-up
        # cases that Jev also calls up, and compare precision + coverage.
        base_up = [o for o in holdout if o.baselines.get(name) is not None and o.baselines[name] > 0]
        both_up = [o for o in base_up if o.score is not None and o.score >= threshold]
        base_prec = (sum(1 for o in base_up if o.gt) / len(base_up)) if base_up else None
        filt_prec = (sum(1 for o in both_up if o.gt) / len(both_up)) if both_up else None

        def _filter_lift_stat(sample: Sequence[Observation], name=name) -> Optional[float]:
            up = [o for o in sample if o.baselines.get(name) is not None and o.baselines[name] > 0]
            kept = [o for o in up if o.score is not None and o.score >= threshold]
            if not up or not kept:
                return None
            return sum(1 for o in kept if o.gt) / len(kept) - sum(1 for o in up if o.gt) / len(up)

        f_point, f_lo, f_hi = cluster_bootstrap(holdout, _filter_lift_stat, iterations=iterations, seed=seed)
        entry["filter"] = {
            "baseline_up_n": len(base_up),
            "retained_n": len(both_up),
            "retained_coverage": (len(both_up) / len(base_up)) if base_up else None,
            "baseline_precision": _fmt(base_prec),
            "filtered_precision": _fmt(filt_prec),
            "lift": {"point": _fmt(f_point), "ci": [_fmt(f_lo), _fmt(f_hi)]},
        }
        out[name] = entry
    return out


# ------------------------------------------------------------------- grading
def grade(
    metrics: Mapping[str, Any],
    *,
    leakage: Optional[Mapping[str, Any]] = None,
    min_eval_cases: int = 200,
    min_answered_fraction: float = 0.9,
) -> Dict[str, Any]:
    """Turn the metrics into the §4.5 graded verdict.

    Rules (evaluated in order):
      1. Coverage gate — too few held-out cases or too many failed calls → 未驗證.
      2. AUC gate — held-out AUC CI lower bound > 0.5 → Jev carries information.
         Negative information (CI upper < 0.5) is reported as 已更正, not as failure.
      3. Lift gate — ΔAUC CI lower bound > 0 → the information is *incremental*
         over the best baseline; otherwise the verdict says "no lift".
      4. Leakage cap — if the memorisation probe shows the model can recall the
         evaluated series, the verdict is downgraded (the measured numbers may be
         knowledge, not prediction).
    """
    ev = metrics.get("evaluation", {}) or {}
    lift = ev.get("lift", {}) or {}
    n_eval = int(ev.get("n", 0) or 0)
    n_cases = int(metrics.get("n_cases", 0) or 0)
    answered_fraction = (metrics.get("n_answered", 0) / n_cases) if n_cases else 0.0
    jev = ev.get("jev", {}) or {}
    auc_block = (lift.get("jev_auc", {}) or {})
    auc_ci = auc_block.get("ci") or [None, None]

    reasons: List[str] = []
    verdict = "未驗證"
    detail = ""

    if n_eval < min_eval_cases:
        reasons.append(f"held-out cases {n_eval} < {min_eval_cases}")
    if answered_fraction < min_answered_fraction:
        reasons.append(f"answered fraction {answered_fraction:.2f} < {min_answered_fraction}")
    if jev.get("auc") is None:
        reasons.append("no held-out AUC (single-class or empty holdout)")

    delta_blocks = [
        (name, blk.get("delta_auc", {}) or {})
        for name, blk in lift.items()
        if name != "jev_auc" and isinstance(blk, dict) and blk.get("available")
    ]

    if not reasons:
        lo, hi = auc_ci[0], auc_ci[1]
        if lo is not None and lo > 0.5:
            verdict = "已驗證"
            best_lift = None
            for name, d in delta_blocks:
                dlo = (d.get("ci") or [None, None])[0]
                if dlo is not None and d.get("point") is not None:
                    if best_lift is None or d["point"] > best_lift[1]:
                        best_lift = (name, d["point"], dlo, (d.get("ci") or [None, None])[1])
            if best_lift and best_lift[2] > 0:
                detail = (
                    f"Jev AUC {auc_block.get('point')} (CI {auc_ci}) carries information AND beats "
                    f"baseline {best_lift[0]} (ΔAUC {best_lift[1]}, CI lower {best_lift[2]})"
                )
            else:
                detail = (
                    f"Jev AUC {auc_block.get('point')} (CI {auc_ci}) carries information, but no baseline "
                    "was beaten with a CI excluding 0"
                )
        elif hi is not None and hi < 0.5:
            verdict = "已更正"
            detail = f"Jev AUC {auc_block.get('point')} (CI {auc_ci}) is significantly WORSE than chance"
        else:
            verdict = "未驗證"
            detail = f"held-out AUC CI {auc_ci} includes 0.5 — no statistically distinguishable signal"
    else:
        detail = "coverage gate failed: " + "; ".join(reasons)

    leakage_suspect = False
    leakage_note = ""
    if leakage:
        probe_auc = leakage.get("auc") or {}
        plo = (probe_auc.get("ci") or [None, None])[0]
        if plo is not None and plo > 0.5:
            leakage_suspect = True
            leakage_note = (
                f"memorisation probe AUC {probe_auc.get('point')} (CI {probe_auc.get('ci')}) — the model can "
                "recall the evaluated series, so the measured numbers are not evidence of forecasting skill"
            )
        else:
            leakage_note = (
                f"memorisation probe AUC {probe_auc.get('point')} (CI {probe_auc.get('ci')}) — no recall signal "
                "detected for this series"
            )
    if leakage_suspect:
        verdict = "未驗證"
        detail = (detail + "; " if detail else "") + "downgraded by leakage cap: " + leakage_note

    return {
        "verdict": verdict,
        "detail": detail,
        "reasons": reasons,
        "held_out_cases": n_eval,
        "answered_fraction": round(answered_fraction, 4),
        "leakage_probe": leakage,
        "leakage_note": leakage_note,
    }
