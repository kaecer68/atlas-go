# jev-docs: https://docs.typesafe.ai/primitives  https://docs.typesafe.ai/noul
#   Question shape follows the official "one narrow judgment, with a refusal/low
#   confidence escape" guidance; the threshold is calibrated from our own data.
"""Stage-1 E0 task spec: the canonical L1 industry layer.

Target      : over the fixed 5-trading-session holding period, will a canonical
              L1 industry index return be POSITIVE after the canonical 0.585%
              round-trip cost (docs/specs/industry-hitrate-metric-spec.md §1)?
Judgment    : Jev `noul` per (date, industry) — one question per industry, all
              industries of a date in ONE request (fan-out).
State       : point-in-time only — trailing 5/20/60-session returns, 20-session
              realised volatility, distance from the 60-session high, the day's
              own return, plus the calendar as-of date. Built from the `pit`
              block of the panel emitted by cmd/experimental/jev-eval-panel.
Ground truth: the `forward` block of the same panel, produced with the canonical
              caliber (stockpicker.NetHit → IndustryWinRate: Wilson 95% CI,
              min_samples gate, coverage). Regenerable by one command:
                go run ./cmd/experimental/jev-eval-panel -start ... -end ... -out panel.jsonl
              The GT is written to the run's cases.jsonl *after* the call; the
              state builder receives a CaseView that cannot see it.
Baselines   : industry momentum (5- and 20-session) on the same canonical
              substrate, plus the platform's own industry-layer signal —
              the sectorallocation industry hit-rate tilt
              (internal/sectorallocation/industry_hitrate_consume.go, #1959,
              tilt = WilsonLower − 0.5, eligible rows only) — which is only
              available for windows where PIT hit-rate rows exist.

Two modes:
  judge   — the E0 measurement itself.
  leakage — memorisation probe: the same industries and windows, but asked
            about the window ENDING at the as-of date with a state that carries
            no price data at all. It can only be answered from prior knowledge,
            so its accuracy bounds how much of the `judge` result could be
            recall of a past series rather than a forecast.
"""
from __future__ import annotations

import json
import os
from typing import Any, Dict, List, Mapping, Sequence, Tuple

from ..spec import Case, Request, SpecBuild, TaskSpec

PROVENANCE = {
    "industry_momentum_20d": (
        "20-session trailing return of the canonical L1 industry index, sign > 0 = up. "
        "Reconstructed from the same substrate the platform reads "
        "(internal/marketdata/sector_index_reader.go); it is the persistence baseline, "
        "not a wired production decision."
    ),
    "industry_momentum_5d": (
        "5-session trailing return of the same series, sign > 0 = up "
        "(the holding-period-matched persistence baseline)."
    ),
    "platform_industry_hitrate_tilt": (
        "The platform's own industry-layer signal: sectorallocation consumes the canonical "
        "industry hit-rate row (WilsonLower) and tilts by (WilsonLower - 0.5) "
        "(internal/sectorallocation/industry_hitrate_consume.go + "
        "industry_hitrate_assessment_decorator.go, #1959), eligible (min_samples 30) L1 rows "
        "only. Needs PIT industry hit-rate rows supplied via --spec-arg hitrate_rows=<jsonl>; "
        "without them the baseline is reported unavailable rather than approximated."
    ),
}


class IndustryL1Spec(TaskSpec):
    name = "industry_l1"
    version = "1.0.0"
    description = (
        "Canonical L1 industry layer: does Jev add information about the 5-session, "
        "cost-adjusted forward direction of a canonical industry index?"
    )
    defaults: Mapping[str, Any] = {
        "panel": "",
        "mode": "judge",
        "probe_step": "5",
        "hitrate_rows": "",
        "baselines": "industry_momentum_20d,industry_momentum_5d,platform_industry_hitrate_tilt",
        "max_dates": "0",
    }

    def build(self, args: Mapping[str, Any]) -> SpecBuild:
        cfg = self.merged_args(args, self.defaults)
        panel_path = str(cfg.get("panel") or "")
        if not panel_path:
            raise ValueError("industry_l1 spec requires --spec-arg panel=<panel.jsonl>")
        mode = str(cfg.get("mode") or "judge").lower()
        if mode not in ("judge", "leakage"):
            raise ValueError(f"unknown mode {mode!r} (expected judge|leakage)")
        probe_step = max(1, int(cfg.get("probe_step") or 5))
        max_dates = max(0, int(cfg.get("max_dates") or 0))
        baselines = [b.strip() for b in str(cfg.get("baselines") or "").split(",") if b.strip()]
        hitrate_path = str(cfg.get("hitrate_rows") or "")

        rows = _read_panel(panel_path)
        by_date: Dict[str, List[Dict[str, Any]]] = {}
        for row in rows:
            by_date.setdefault(row["date"], []).append(row)
        dates = sorted(by_date)
        if mode == "leakage":
            dates = dates[::probe_step]
        if max_dates:
            dates = dates[:max_dates]
        if not dates:
            raise ValueError(f"panel {panel_path} has no usable rows")

        hitrate = _read_hitrate_rows(hitrate_path) if hitrate_path else {}
        cost_rate = float(rows[0].get("cost_rate", 0.00585))
        hold_days = int(rows[0].get("hold_days", 5))

        requests: List[Request] = []
        cases: List[Case] = []
        for date in dates:
            industries = sorted(by_date[date], key=lambda r: r["industry_id"])
            questions: Dict[str, Any] = {}
            case_list: List[Case] = []
            state_rows: List[Dict[str, Any]] = []
            for row in industries:
                iid = row["industry_id"]
                qid = iid
                if mode == "judge":
                    questions[qid] = _judge_question(row, date, hold_days, cost_rate)
                    gt = bool(row["forward"]["hit"])
                    baselines_values = _baseline_values(row, date, baselines, hitrate)
                else:
                    questions[qid] = _leakage_question(row, date, hold_days, cost_rate)
                    gt = bool(row["backward"]["backward_hit"])
                    baselines_values = {}
                state_rows.append(_state_row(row, mode))
                case_list.append(
                    Case(
                        case_id=f"{date}:{iid}",
                        request_id=f"{mode}:{date}",
                        group_id=date,
                        gt=gt,
                        baselines=baselines_values,
                        meta={
                            "question_id": qid,
                            "industry_id": iid,
                            "date": date,
                            "mode": mode,
                            "pit": row.get("pit", {}),
                            "forward": row.get("forward", {}),
                            "backward": row.get("backward", {}),
                        },
                    )
                )
            state = _state(date, state_rows, mode, hold_days, cost_rate)
            requests.append(
                Request(
                    request_id=f"{mode}:{date}",
                    state=state,
                    questions=questions,
                    case_ids=[c.case_id for c in case_list],
                )
            )
            cases.extend(case_list)

        meta = {
            "mode": mode,
            "panel": os.path.abspath(panel_path),
            "panel_rows": len(rows),
            "dates": len(dates),
            "date_start": dates[0],
            "date_end": dates[-1],
            "industries_per_date": sorted({str(len(by_date[d])) for d in dates}),
            "hold_days": hold_days,
            "cost_rate": cost_rate,
            "baselines": baselines,
            "baseline_provenance": {b: PROVENANCE.get(b, "unknown baseline") for b in baselines},
            "platform_baseline_available": bool(hitrate),
            "probe_step": probe_step if mode == "leakage" else None,
            "questions_per_request": sorted({len(r.questions) for r in requests}),
        }
        return SpecBuild(requests=requests, cases=cases, meta=meta)


def _read_panel(path: str) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    with open(path, "r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    for row in rows:
        for key in ("date", "industry_id", "pit", "forward", "backward"):
            if key not in row:
                raise ValueError(f"panel row missing {key!r}: {row}")
    return rows


def _read_hitrate_rows(path: str) -> Dict[Tuple[str, str], float]:
    """PIT industry hit-rate rows: {date, industry_id, wilson_lower, calibration_status?}.

    A row is only usable when its calibration status is `eligible` (the #1959
    consumption rule); anything else is treated as "no view" (abstain).
    """
    out: Dict[Tuple[str, str], float] = {}
    with open(path, "r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            row = json.loads(line)
            status = row.get("calibration_status", "eligible")
            if status != "eligible":
                continue
            out[(row["date"], row["industry_id"])] = float(row["wilson_lower"]) - 0.5
    return out


def _baseline_values(
    row: Mapping[str, Any], date: str, baselines: Sequence[str], hitrate: Mapping[Tuple[str, str], float]
) -> Dict[str, Any]:
    pit = row.get("pit", {})
    iid = row["industry_id"]
    values: Dict[str, Any] = {}
    for name in baselines:
        if name == "industry_momentum_20d":
            values[name] = pit.get("ret_20d_pct")
        elif name == "industry_momentum_5d":
            values[name] = pit.get("ret_5d_pct")
        elif name == "platform_industry_hitrate_tilt":
            values[name] = hitrate.get((date, iid))
        else:
            raise ValueError(f"unknown baseline {name!r}")
    return values


def _state_row(row: Mapping[str, Any], mode: str) -> Dict[str, Any]:
    iid = row["industry_id"]
    if mode == "leakage":
        return {"industry_id": iid, "name_zh": row.get("industry_name_zh", "")}
    pit = row.get("pit", {})
    return {
        "industry_id": iid,
        "name_zh": row.get("industry_name_zh", ""),
        "trailing_return_5_sessions_pct": _round(pit.get("ret_5d_pct")),
        "trailing_return_20_sessions_pct": _round(pit.get("ret_20d_pct")),
        "trailing_return_60_sessions_pct": _round(pit.get("ret_60d_pct")),
        "realised_vol_20_sessions_pct": _round(pit.get("vol_20d_pct")),
        "below_60_session_high_pct": _round(pit.get("dist_high_60d_pct")),
        "session_return_pct": _round(pit.get("daily_return_pct")),
    }


def _state(date: str, rows: Sequence[Mapping[str, Any]], mode: str, hold_days: int, cost_rate: float) -> Dict[str, Any]:
    if mode == "leakage":
        return {
            "task": "historical recall check (no market data is provided for this state)",
            "as_of": date,
            "holding_sessions": hold_days,
            "round_trip_cost_pct": round(cost_rate * 100, 3),
            "industries": list(rows),
        }
    return {
        "task": (
            "Taiwan equity canonical L1 industry indices: judge the next "
            f"{hold_days} trading sessions from the close of the as-of date."
        ),
        "as_of": date,
        "holding_sessions": hold_days,
        "round_trip_cost_pct": round(cost_rate * 100, 3),
        "industries": list(rows),
    }


def _judge_question(row: Mapping[str, Any], date: str, hold_days: int, cost_rate: float) -> Dict[str, Any]:
    name = row.get("industry_name_zh") or row["industry_id"]
    cost_pct = round(cost_rate * 100, 3)
    return {
        "type": "noul",
        "instructions": (
            f"Industry: {name} (canonical L1 id {row['industry_id']}), Taiwan equities. "
            f"All trailing figures in the state are measured up to and including the close of {date}. "
            f"Judge this single question: over the NEXT {hold_days} trading sessions (close of {date} "
            f"through the close {hold_days} sessions later), will this industry index return be "
            f"> 0 after subtracting the {cost_pct}% round-trip transaction cost? "
            "Base the judgment on the state alone."
        ),
        "criteria": {
            "true": f"the industry index is expected to rise by more than {cost_pct}% over the next {hold_days} trading sessions",
            "false": f"the industry index is expected to rise by {cost_pct}% or less (including any fall) over the next {hold_days} trading sessions",
        },
    }


def _leakage_question(row: Mapping[str, Any], date: str, hold_days: int, cost_rate: float) -> Dict[str, Any]:
    name = row.get("industry_name_zh") or row["industry_id"]
    cost_pct = round(cost_rate * 100, 3)
    start = row.get("backward", {}).get("backward_date", "")
    return {
        "type": "noul",
        "instructions": (
            f"Industry: {name} (canonical L1 id {row['industry_id']}), Taiwan equities. "
            f"No market data is provided in this state. From your own knowledge only, judge this single "
            f"question: over the {hold_days} trading sessions ENDING at the close of {date} "
            f"(from the close of {start} to the close of {date}), was this industry index return > 0 "
            f"after subtracting the {cost_pct}% round-trip transaction cost? "
            "If you do not know the historical outcome, answer with low confidence instead of guessing."
        ),
        "criteria": {
            "true": f"the industry index rose by more than {cost_pct}% over those {hold_days} trading sessions",
            "false": f"the industry index rose by {cost_pct}% or less (including any fall) over those {hold_days} trading sessions",
        },
    }


def _round(value: Any, digits: int = 3) -> Any:
    if value is None:
        return None
    try:
        return round(float(value), digits)
    except (TypeError, ValueError):
        return None
