# jev-docs: https://docs.typesafe.ai/primitives  (spec layer never calls the API;
#   it only builds states/questions for jevkit — see scripts/jev_eval/runner.py)
"""jev_eval — reusable shadow-evaluation framework for Jev (TypeSafe System One).

The framework answers one question shape, repeatedly:

    "Does a Jev judgment carry predictive information for <target>, and does it
     add value on top of the baseline signal <target> already has?"

It is built so that a NEW target (industry layer, symbol layer, event layer) is
a new *task spec* — not a new script. A spec supplies:
  - how to build the point-in-time state and the questions (per request),
  - the ground truth per case (regenerable by code, never a hand-written label),
  - the baselines the judgment must beat,
  - the evaluation window and grouping (cluster) key.

Discipline this package enforces structurally (docs/jev/JEV-USAGE-CONTRACT.md §4):

  1. The model NEVER sees the ground truth. States are built from Case inputs
     only; `Case.gt` lives on the case record written to disk after the call.
  2. Every request goes through `scripts/jevkit.py` (fail-open, UA, timeout,
     retry, pinned model id). No other HTTP path exists in this package.
  3. Every judgment is logged to JSONL (score, tokens, latency, model, error),
     and metrics are recomputable offline from that JSONL — no re-spending.
  4. Thresholds are calibrated on a *calibration* split of the data, never on
     the evaluation split and never copied from a cookbook (§3).
  5. Conclusions are graded (已驗證 / 已更正 / 未驗證) and always carry the
     sample count and the cost (§4.5).
"""
from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Dict, Iterable, List, Mapping, Optional, Sequence, Tuple


@dataclass(frozen=True)
class Case:
    """One evaluated unit: a single judgment plus its truth and baselines.

    `gt` and `baselines` are recorded for scoring only. They are written to the
    run's cases.jsonl and are never placed into `Request.state`.
    """

    case_id: str
    request_id: str
    group_id: str
    gt: bool
    baselines: Mapping[str, Optional[float]] = field(default_factory=dict)
    meta: Mapping[str, Any] = field(default_factory=dict)

    def to_json(self) -> Dict[str, Any]:
        return {
            "case_id": self.case_id,
            "request_id": self.request_id,
            "group_id": self.group_id,
            "gt": bool(self.gt),
            "baselines": {k: (None if v is None else float(v)) for k, v in self.baselines.items()},
            "meta": dict(self.meta),
        }


@dataclass(frozen=True)
class Request:
    """One API call: a state plus one or more questions (fan-out).

    Fan-out is deliberate: the official/measured cost advantage of batching is
    large, and questions about the same state belong in one request. All cases
    answered by this request must share `group_id` so the CI clustering stays
    honest.
    """

    request_id: str
    state: Any
    questions: Mapping[str, Any]
    case_ids: Sequence[str]

    def fingerprint(self, model: str) -> str:
        payload = json.dumps(
            {"state": self.state, "questions": self.questions, "model": model},
            ensure_ascii=False,
            sort_keys=True,
            default=str,
        )
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:16]


@dataclass
class SpecBuild:
    """The deterministic output of a task spec for one window/configuration."""

    requests: List[Request]
    cases: List[Case]
    meta: Dict[str, Any] = field(default_factory=dict)

    def cases_by_id(self) -> Dict[str, Case]:
        return {c.case_id: c for c in self.cases}


@dataclass(frozen=True)
class CaseView:
    """The only view of a case a state builder may use.

    Deliberately excludes `gt` and `baselines`: a state builder that cannot see
    the truth cannot leak it. This is a structural guarantee, not a convention.
    """

    case_id: str
    group_id: str
    meta: Mapping[str, Any]


class TaskSpec:
    """Base class for evaluation targets.

    Subclasses implement `build()` and declare `name`, `version`, `defaults`
    and `question_ids`. `build()` must be deterministic: same inputs → same
    requests (same order, same fingerprints).
    """

    name: str = ""
    version: str = ""
    #: default spec arguments (CLI `--spec-arg k=v` overrides)
    defaults: Mapping[str, Any] = {}
    #: human description of the target and the baseline set
    description: str = ""

    def build(self, args: Mapping[str, Any]) -> SpecBuild:  # pragma: no cover - interface
        raise NotImplementedError

    # ---- helpers shared by specs -------------------------------------------
    @staticmethod
    def view(case: Case) -> CaseView:
        return CaseView(case_id=case.case_id, group_id=case.group_id, meta=case.meta)

    @staticmethod
    def merged_args(args: Mapping[str, Any], defaults: Mapping[str, Any]) -> Dict[str, Any]:
        merged = dict(defaults)
        merged.update({k: v for k, v in args.items() if v is not None})
        return merged


def parse_spec_args(pairs: Iterable[str]) -> Dict[str, str]:
    """Parse `--spec-arg k=v` values into a dict (values stay strings)."""
    out: Dict[str, str] = {}
    for pair in pairs:
        if "=" not in pair:
            raise ValueError(f"--spec-arg expects k=v, got {pair!r}")
        key, value = pair.split("=", 1)
        out[key.strip()] = value
    return out
