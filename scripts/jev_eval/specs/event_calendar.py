# jev-docs: https://docs.typesafe.ai/primitives  https://docs.typesafe.ai/noul
#   Question shape follows the official "one narrow judgment, with a refusal/low
#   confidence escape" guidance; the threshold is calibrated from our own data
#   (scripts/jevkit.py::calibrate_threshold on the calibration split only).
"""Stage-3 E0 task spec: the scheduled-event layer.

Target      : over the fixed N-trading-session holding period starting at a
              scheduled market event's decision date, will a canonical L1
              industry index return be POSITIVE after the canonical 0.585%
              round-trip cost (docs/specs/industry-hitrate-metric-spec.md §1)?
Judgment    : Jev `noul` per (event occurrence, anchor, canonical L1 industry) —
              all industries of one event occurrence in ONE request (fan-out).
State       : point-in-time only. The event block is pure schedule information
              (the calendar is deterministic from the year alone:
              industry.EventCalendar.RefreshEvents -> defaultEventRules), and the
              industry block is the same PIT feature set Stage 1 used, so the
              test is "can Jev do better with the same information", not "can
              Jev guess the market".
Ground truth: the `forward` block of the canonical L1 panel emitted by
              cmd/experimental/jev-eval-panel (stockpicker.NetHit). Regenerable:
                go run ./cmd/experimental/jev-eval-panel -start ... -end ... -out panel.jsonl
              The GT is written to the run's cases.jsonl *after* the call; the
              state builder receives a CaseView that cannot see it.

Why the event skeleton comes from Go
------------------------------------
`internal/industry/event_calendar.go` owns the dates: the rules are deterministic
functions of the year, so the calendar is a *regenerable* ground truth rather
than a hand-written label. cmd/experimental/jev-eval-events exports it verbatim:

    go run ./cmd/experimental/jev-eval-events \
      -dir data/state/sector_index -start 2021-01-04 -end 2021-12-30 \
      -anchor peak -out events_peak.jsonl -adjustments-out adjustments.jsonl

Three anchor kinds are exported (`start` / `peak` / `end`) because a scheduled
event has three well-defined decision dates: the session its window opens, the
session of its peak, and the session it closes. The layer is also exported with
the platform's OWN event-layer decision variable
(industry.EventCalendar.GetEventAdjustment) so the baseline can be measured
against the number the platform already computes instead of a re-implementation.

Two modes:
  judge   — the E0 measurement itself.
  leakage — memorisation probe: the same industries and dates, but asked about
            the window ENDING at the anchor with a state that carries neither
            price data nor event details. It can only be answered from prior
            knowledge, so its accuracy bounds how much of `judge` could be
            recall of a past series rather than a forecast.
"""
from __future__ import annotations

import json
import os
from typing import Any, Dict, List, Mapping, Sequence, Tuple

from ..spec import Case, Request, SpecBuild, TaskSpec

#: Direction sign used by industry.EventCalendar.computeSentimentAdjustment.
#: `mixed` and `neutral` map to 0.0 there, i.e. the platform itself declares no
#: direction, so the prior abstains instead of inventing a call.
DIRECTION_SIGN = {"bullish": 1.0, "bearish": -1.0, "mixed": 0.0, "neutral": 0.0}

#: Cross-industry spillover weight from industry.EventCalendar.GetEventAdjustment:
#: an industry the event declares gets full weight, everything else 0.3.
SPILLOVER_WEIGHT = 0.3

#: A constant score. Under the framework's baseline decision rule (score > 0)
#: this is exactly the "always call up" strategy, so its precision IS the base
#: rate and its AUC is 0.5 by construction — the no-information reference every
#: lift claim has to beat.
ALWAYS_UP_SCORE = 0.5

PROVENANCE: Dict[str, str] = {
    "unconditional_always_up": (
        "Unconditional reference: a constant score (0.5). Under the framework's baseline "
        "decision rule (score > 0) this is the always-call-up strategy, whose precision is the "
        "base rate and whose AUC is 0.5 by construction. It answers 'what does the event layer "
        "tell us if we condition on nothing?'."
    ),
    "event_direction_prior": (
        "The platform's own naive event direction prior: CalendarEvent.Direction mapped through "
        "the sign table used by EventCalendar.computeSentimentAdjustment (bullish +1, bearish -1) "
        "and weighted by the relevance rule of EventCalendar.GetEventAdjustment (full weight for "
        "AffectedIndustries, 0.3 spillover otherwise). `mixed`/`neutral` yield the same 0.0 the "
        "platform computes, so the prior ABSTAINS (no value) rather than inventing a direction. "
        "internal/industry/event_calendar.go."
    ),
    "platform_event_adjustment": (
        "The platform's own event-layer decision variable, read through the platform's function "
        "instead of being recomputed: industry.EventCalendar.GetEventAdjustment(industryID, t) = "
        "mean over active events of BaseWeight * direction * linear decay from the peak, capped at "
        "+/-0.05. Exported offline by cmd/experimental/jev-eval-events -adjustments-out. It is zero "
        "outside each event's +/-decayDays peak window, so an anchor far from the peak gives this "
        "baseline no view (0), which is the platform's honest position rather than a missing value."
    ),
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
    "platform_eventdriven_t1_capital_flow_direction": (
        "The platform's event-driven module (internal/eventdriven) predicts the T+1 capital-flow "
        "direction and is scored by an H6 metric, NOT by a hit rate. No point-in-time historical "
        "series of those predictions exists locally for the 2021 window (data/state/"
        "event_flow_predictions.jsonl starts 2026-07), so this baseline is reported as "
        "unavailable and is NOT approximated by a proxy."
    ),
}


class EventCalendarSpec(TaskSpec):
    name = "event_calendar"
    version = "1.0.0"
    description = (
        "Scheduled-event layer: does Jev add information about the cost-adjusted forward "
        "direction of a canonical L1 industry index after a Taiwan market calendar event?"
    )
    defaults: Mapping[str, Any] = {
        "events": "",
        "panel": "",
        "adjustments": "",
        "mode": "judge",
        "anchor_kind": "",
        "probe_step": "5",
        "max_anchors": "0",
        "baselines": (
            "unconditional_always_up,event_direction_prior,platform_event_adjustment,"
            "industry_momentum_20d,industry_momentum_5d,"
            "platform_eventdriven_t1_capital_flow_direction"
        ),
    }

    def build(self, args: Mapping[str, Any]) -> SpecBuild:
        cfg = self.merged_args(args, self.defaults)
        panel_path = str(cfg.get("panel") or "")
        if not panel_path:
            raise ValueError("event_calendar spec requires --spec-arg panel=<panel.jsonl>")
        events_paths = [p.strip() for p in str(cfg.get("events") or "").split(",") if p.strip()]
        if not events_paths:
            raise ValueError("event_calendar spec requires --spec-arg events=<events.jsonl>[,<more>]")
        mode = str(cfg.get("mode") or "judge").lower()
        if mode not in ("judge", "leakage"):
            raise ValueError(f"unknown mode {mode!r} (expected judge|leakage)")
        anchor_filter = str(cfg.get("anchor_kind") or "").strip()
        probe_step = max(1, int(cfg.get("probe_step") or 5))
        max_anchors = max(0, int(cfg.get("max_anchors") or 0))
        baselines = [b.strip() for b in str(cfg.get("baselines") or "").split(",") if b.strip()]

        panel = _read_panel(panel_path)
        if not panel:
            raise ValueError(f"panel {panel_path} has no rows")
        panel_by_date: Dict[str, Dict[str, Dict[str, Any]]] = {}
        for row in panel:
            panel_by_date.setdefault(row["date"], {})[row["industry_id"]] = row
        cost_rate = float(panel[0].get("cost_rate", 0.00585))
        hold_days = int(panel[0].get("hold_days", 5))

        adjustment_path = str(cfg.get("adjustments") or "")
        adjustments = _read_adjustments(adjustment_path) if adjustment_path else {}

        events = _read_events(events_paths, anchor_filter)
        if not events:
            raise ValueError("no event occurrence survived the anchor/panel join")

        # ---- join the skeleton onto the canonical panel --------------------
        joined: List[Tuple[Dict[str, Any], Dict[str, Dict[str, Any]]]] = []
        drop_reasons: Dict[str, int] = {}
        for evt in events:
            anchor = evt["anchor_date"]
            industries = panel_by_date.get(anchor)
            if not industries:
                key = "anchor not covered by the panel window (insufficient trailing history or forward sessions)"
                drop_reasons[key] = drop_reasons.get(key, 0) + 1
                continue
            joined.append((evt, industries))
        if max_anchors:
            joined = joined[:max_anchors]
        if not joined:
            raise ValueError("event/panel join is empty: no event anchor has a panel row")

        if mode == "leakage":
            anchors = sorted({evt["anchor_date"] for evt, _ in joined})
            keep = set(anchors[::probe_step])
            joined = [(e, ind) for e, ind in joined if e["anchor_date"] in keep]

        requests: List[Request] = []
        cases: List[Case] = []
        for evt, industries in joined:
            anchor = evt["anchor_date"]
            event_id = evt["event_id"]
            anchor_kind = evt.get("anchor_kind", "")
            request_id = f"{mode}:{anchor_kind}:{event_id}"
            questions: Dict[str, Any] = {}
            case_list: List[Case] = []
            state_rows: List[Dict[str, Any]] = []
            for iid in sorted(industries):
                row = industries[iid]
                questions[iid] = (
                    _judge_question(evt, row, anchor, hold_days, cost_rate)
                    if mode == "judge"
                    else _leakage_question(row, anchor, hold_days, cost_rate)
                )
                if mode == "judge":
                    gt = bool(row["forward"]["hit"])
                    baselines_values = _baseline_values(evt, row, anchor, iid, baselines, adjustments)
                else:
                    gt = bool(row["backward"]["backward_hit"])
                    baselines_values = {}
                state_rows.append(_state_row(row, mode))
                case_list.append(
                    Case(
                        case_id=f"{anchor_kind}:{event_id}:{iid}",
                        request_id=request_id,
                        group_id=anchor,
                        gt=gt,
                        baselines=baselines_values,
                        meta={
                            "question_id": iid,
                            "industry_id": iid,
                            "date": anchor,
                            "mode": mode,
                            "anchor_kind": anchor_kind,
                            "event_id": event_id,
                            "event_type": evt["event_type"],
                            "event_direction": evt["direction"],
                            "pit": row.get("pit", {}),
                            "forward": row.get("forward", {}),
                            "backward": row.get("backward", {}),
                        },
                    )
                )
            state = _state(evt, anchor, state_rows, mode, hold_days, cost_rate)
            requests.append(
                Request(request_id=request_id, state=state, questions=questions,
                        case_ids=[c.case_id for c in case_list])
            )
            cases.extend(case_list)

        anchor_dates = sorted({c.group_id for c in cases})
        meta = {
            "mode": mode,
            "panel": os.path.abspath(panel_path),
            "panel_rows": len(panel),
            "events": [os.path.abspath(p) for p in events_paths],
            "events_rows": len(events) + sum(drop_reasons.values()),
            "adjustments": os.path.abspath(adjustment_path) if adjustment_path else None,
            "anchor_filter": anchor_filter or None,
            "anchor_kinds": sorted({c.meta["anchor_kind"] for c in cases}),
            "event_occurrences": len(cases) // max(1, len({c.meta["industry_id"] for c in cases})),
            "distinct_events": len({c.meta["event_id"] for c in cases}),
            "distinct_anchor_dates": len(anchor_dates),
            # The report renderer reads date_start/date_end; for this layer the
            # evaluated window IS the set of anchor dates.
            "date_start": anchor_dates[0],
            "date_end": anchor_dates[-1],
            "anchor_date_start": anchor_dates[0],
            "anchor_date_end": anchor_dates[-1],
            "industries_per_anchor": sorted({len(r.questions) for r in requests}),
            "requests": len(requests),
            "cases": len(cases),
            "hold_days": hold_days,
            "cost_rate": cost_rate,
            "dropped": drop_reasons,
            "baselines": baselines,
            "baseline_provenance": {b: PROVENANCE.get(b, "unknown baseline") for b in baselines},
            "probe_step": probe_step if mode == "leakage" else None,
            "questions_per_request": sorted({len(r.questions) for r in requests}),
        }
        return SpecBuild(requests=requests, cases=cases, meta=meta)


# --------------------------------------------------------------------- readers
def _read_jsonl(path: str) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    with open(path, "r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    return rows


def _read_panel(path: str) -> List[Dict[str, Any]]:
    rows = _read_jsonl(path)
    for row in rows:
        for key in ("date", "industry_id", "pit", "forward", "backward"):
            if key not in row:
                raise ValueError(f"panel row missing {key!r}: {row}")
    return rows


def _read_events(paths: Sequence[str], anchor_filter: str) -> List[Dict[str, Any]]:
    out: List[Dict[str, Any]] = []
    for path in paths:
        for row in _read_jsonl(path):
            for key in ("event_id", "event_type", "direction", "anchor_date", "anchor_kind"):
                if key not in row:
                    raise ValueError(f"event row missing {key!r}: {row}")
            if anchor_filter and row["anchor_kind"] != anchor_filter:
                continue
            out.append(row)
    return out


def _read_adjustments(path: str) -> Dict[Tuple[str, str], float]:
    """The platform's own event adjustment keyed by (date, industry_id)."""
    out: Dict[Tuple[str, str], float] = {}
    for row in _read_jsonl(path):
        out[(row["date"], row["industry_id"])] = float(row["event_adjustment"])
    return out


# ------------------------------------------------------------------- baselines
def _baseline_values(
    evt: Mapping[str, Any],
    row: Mapping[str, Any],
    anchor: str,
    iid: str,
    baselines: Sequence[str],
    adjustments: Mapping[Tuple[str, str], float],
) -> Dict[str, Any]:
    pit = row.get("pit", {})
    values: Dict[str, Any] = {}
    for name in baselines:
        if name == "unconditional_always_up":
            values[name] = ALWAYS_UP_SCORE
        elif name == "event_direction_prior":
            values[name] = _direction_prior(evt, iid)
        elif name == "platform_event_adjustment":
            values[name] = adjustments.get((anchor, iid))
        elif name == "industry_momentum_20d":
            values[name] = pit.get("ret_20d_pct")
        elif name == "industry_momentum_5d":
            values[name] = pit.get("ret_5d_pct")
        elif name == "platform_eventdriven_t1_capital_flow_direction":
            # Unavailable for 2021: no point-in-time prediction history exists.
            # Recorded as "no value" (null) so the metrics layer reports the
            # baseline as unavailable instead of silently dropping the row.
            values[name] = None
        else:
            raise ValueError(f"unknown baseline {name!r}")
    return values


def _direction_prior(evt: Mapping[str, Any], iid: str) -> Any:
    """The platform's own direction prior, or None when it declares none."""
    sign = DIRECTION_SIGN.get(str(evt.get("direction", "")))
    if sign is None:
        raise ValueError(f"unknown event direction {evt.get('direction')!r} for {evt.get('event_id')}")
    if sign == 0.0:
        return None
    affected = evt.get("affected_industries") or []
    if not affected or iid in affected:
        return sign
    return sign * SPILLOVER_WEIGHT


# --------------------------------------------------------------- state / questions
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


def _event_block(evt: Mapping[str, Any]) -> Dict[str, Any]:
    """Schedule facts only: every field is known before the anchor session."""
    return {
        "event_id": evt["event_id"],
        "event_type": evt["event_type"],
        "name": evt.get("name", ""),
        "description": evt.get("description", ""),
        "direction": evt.get("direction", ""),
        "base_weight": _round(evt.get("base_weight")),
        "decay_days": evt.get("decay_days"),
        "affected_industries": list(evt.get("affected_industries") or []),
        "window_start": evt.get("window_start"),
        "peak_date": evt.get("peak_date"),
        "window_end": evt.get("window_end"),
        "anchor_kind": evt.get("anchor_kind"),
        "nominal_anchor_date": evt.get("nominal_anchor_date"),
        "anchor_shift_days": evt.get("anchor_shift_days"),
        "sessions_from_window_start": evt.get("sessions_from_window_start"),
        "sessions_anchor_to_window_end": evt.get("sessions_anchor_to_window_end"),
    }


def _state(
    evt: Mapping[str, Any],
    anchor: str,
    rows: Sequence[Mapping[str, Any]],
    mode: str,
    hold_days: int,
    cost_rate: float,
) -> Dict[str, Any]:
    if mode == "leakage":
        return {
            "task": "historical recall check (no market data and no event details are provided for this state)",
            "as_of": anchor,
            "holding_sessions": hold_days,
            "round_trip_cost_pct": round(cost_rate * 100, 3),
            "industries": list(rows),
        }
    return {
        "task": (
            "Taiwan equities: a scheduled market-calendar event is in focus. Judge the next "
            f"{hold_days} trading sessions for each canonical L1 industry index from the close of "
            "the as-of date."
        ),
        "as_of": anchor,
        "holding_sessions": hold_days,
        "round_trip_cost_pct": round(cost_rate * 100, 3),
        "event": _event_block(evt),
        "industries": list(rows),
    }


def _judge_question(
    evt: Mapping[str, Any], row: Mapping[str, Any], anchor: str, hold_days: int, cost_rate: float
) -> Dict[str, Any]:
    name = row.get("industry_name_zh") or row["industry_id"]
    cost_pct = round(cost_rate * 100, 3)
    event_label = evt.get("name") or evt["event_type"]
    return {
        "type": "noul",
        "instructions": (
            f"Industry: {name} (canonical L1 id {row['industry_id']}), Taiwan equities. "
            f"Scheduled event in focus: {event_label} ({evt['event_type']}), "
            f"window {evt.get('window_start')} to {evt.get('window_end')}, "
            f"peak {evt.get('peak_date')}. The as-of date {anchor} is this event's "
            f"{evt.get('anchor_kind')} date. All trailing figures in the state are measured up to "
            f"and including the close of {anchor}. Judge this single question: over the NEXT "
            f"{hold_days} trading sessions (close of {anchor} through the close {hold_days} "
            f"sessions later), will this industry index return be > 0 after subtracting the "
            f"{cost_pct}% round-trip transaction cost? Base the judgment on the state alone."
        ),
        "criteria": {
            "true": (
                f"the industry index is expected to rise by more than {cost_pct}% over the next "
                f"{hold_days} trading sessions"
            ),
            "false": (
                f"the industry index is expected to rise by {cost_pct}% or less (including any "
                f"fall) over the next {hold_days} trading sessions"
            ),
        },
    }


def _leakage_question(row: Mapping[str, Any], anchor: str, hold_days: int, cost_rate: float) -> Dict[str, Any]:
    name = row.get("industry_name_zh") or row["industry_id"]
    cost_pct = round(cost_rate * 100, 3)
    start = row.get("backward", {}).get("backward_date", "")
    return {
        "type": "noul",
        "instructions": (
            f"Industry: {name} (canonical L1 id {row['industry_id']}), Taiwan equities. "
            f"No market data and no event details are provided in this state. From your own "
            f"knowledge only, judge this single question: over the {hold_days} trading sessions "
            f"ENDING at the close of {anchor} (from the close of {start} to the close of {anchor}), "
            f"was this industry index return > 0 after subtracting the {cost_pct}% round-trip "
            "transaction cost? If you do not know the historical outcome, answer with low "
            "confidence instead of guessing."
        ),
        "criteria": {
            "true": f"the industry index rose by more than {cost_pct}% over those {hold_days} trading sessions",
            "false": (
                f"the industry index rose by {cost_pct}% or less (including any fall) over those "
                f"{hold_days} trading sessions"
            ),
        },
    }


def _round(value: Any, digits: int = 3) -> Any:
    if value is None:
        return None
    try:
        return round(float(value), digits)
    except (TypeError, ValueError):
        return None
