# jev-docs: https://docs.typesafe.ai/primitives  https://docs.typesafe.ai/models
#   Why this file exists: every Jev call in this repo must go through
#   scripts/jevkit.py (User-Agent C1, timeout C2, fail-open C3, dict criteria C4,
#   pinned model id C7). This runner never builds an HTTP request itself.
"""Shadow runner: executes a TaskSpec against Jev and logs every judgment.

Design constraints (docs/jev/JEV-USAGE-CONTRACT.md §4/§5):

shadow only     — the runner is never imported by a product path; its output is
                  a run directory under `--out-dir`, nothing else.
fail-open       — any transport/API error is recorded as ok=false and the run
                  continues; a Jev outage cannot abort the evaluation.
log-first       — one JSONL line per request with score inputs, tokens, latency,
                  model, attempts and error; metrics are recomputed offline from
                  that file, so no API budget is spent twice.
reproducible    — requests carry a content fingerprint; an existing line with the
                  same fingerprint is reused instead of re-called (the JSONL is
                  the cache). Repeats (`--repeat N`) are opt-in and are recorded
                  as separate lines, so sampling noise stays visible instead of
                  hiding inside the metrics.
"""
from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Dict, List, Mapping, Optional, Sequence, Tuple

_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
if os.path.join(_REPO_ROOT, "scripts") not in sys.path:
    sys.path.insert(0, os.path.join(_REPO_ROOT, "scripts"))

from jevkit import DEFAULT_MODEL, ask_or_none  # noqa: E402  (path bootstrap above)

from .spec import Case, Request, SpecBuild, TaskSpec  # noqa: E402

#: Input-token price. Only input tokens are billed (output is free); the figure
#: is the measured $0.042/Mtok from experiments/008-jev-systemone (2026-09-23).
DEFAULT_PRICE_PER_MTOK = 0.042


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def git_rev(root: str) -> str:
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--short", "HEAD"], cwd=root, capture_output=True, text=True, timeout=10
        )
        return out.stdout.strip() if out.returncode == 0 else ""
    except Exception:  # noqa: BLE001 - provenance is best effort, never fatal
        return ""


@dataclass
class CollectStats:
    requests_total: int
    called: int
    cached: int
    failed: int
    tokens: int
    wall_seconds: float

    def as_dict(self) -> Dict[str, Any]:
        return {
            "requests_total": self.requests_total,
            "called": self.called,
            "cached": self.cached,
            "failed": self.failed,
            "tokens": self.tokens,
            "wall_seconds": round(self.wall_seconds, 2),
        }


def _read_jsonl(path: str) -> List[Dict[str, Any]]:
    if not os.path.exists(path):
        return []
    rows: List[Dict[str, Any]] = []
    with open(path, "r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            rows.append(json.loads(line))
    return rows


def write_cases(path: str, cases: Sequence[Case]) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        for case in cases:
            fh.write(json.dumps(case.to_json(), ensure_ascii=False) + "\n")


def read_cases(path: str) -> List[Case]:
    out: List[Case] = []
    for row in _read_jsonl(path):
        out.append(
            Case(
                case_id=row["case_id"],
                request_id=row["request_id"],
                group_id=row["group_id"],
                gt=bool(row["gt"]),
                baselines=row.get("baselines", {}) or {},
                meta=row.get("meta", {}) or {},
            )
        )
    return out


def build_manifest(
    *,
    spec: TaskSpec,
    build: SpecBuild,
    run_dir: str,
    model: str,
    args: Mapping[str, Any],
    price_per_mtok: float,
    repeat: int,
    uid_mode: str,
) -> Dict[str, Any]:
    fingerprints = [r.fingerprint(model) for r in build.requests]
    digest = hashlib.sha256("\n".join(fingerprints).encode("utf-8")).hexdigest()[:16]
    return {
        "spec": spec.name,
        "spec_version": spec.version,
        "model": model,
        "model_alias_policy": "pinned version id via jevkit.DEFAULT_MODEL (JEV_MODEL may override)",
        "price_per_mtok": price_per_mtok,
        "repeat": repeat,
        "uid_mode": uid_mode,
        "spec_args": {k: str(v) for k, v in sorted(args.items())},
        "requests": len(build.requests),
        "cases": len(build.cases),
        "requests_fingerprint": digest,
        "input_fingerprint": hashlib.sha256(
            json.dumps(build.meta, ensure_ascii=False, sort_keys=True, default=str).encode("utf-8")
        ).hexdigest()[:16],
        "spec_meta": json.loads(json.dumps(build.meta, ensure_ascii=False, default=str)),
        "created_at": utc_now(),
        "git_rev": git_rev(_REPO_ROOT),
        "run_dir": os.path.abspath(run_dir),
    }


def collect(
    *,
    spec: TaskSpec,
    build: SpecBuild,
    run_dir: str,
    model: str = DEFAULT_MODEL,
    price_per_mtok: float = DEFAULT_PRICE_PER_MTOK,
    concurrency: int = 4,
    repeat: int = 1,
    force: bool = False,
    offline: bool = False,
    args: Optional[Mapping[str, Any]] = None,
    requests_filter: Optional[Sequence[str]] = None,
    progress_every: int = 25,
    log: Any = None,
) -> CollectStats:
    """Run the spec's requests through jevkit and log every judgment to JSONL.

    Existing lines keyed by (request_id, repeat) fingerprint are reused unless
    `force`; `offline=True` never calls the API (metrics-only reruns).
    """
    os.makedirs(run_dir, exist_ok=True)
    requests_path = os.path.join(run_dir, "requests.jsonl")
    cases_path = os.path.join(run_dir, "cases.jsonl")
    manifest_path = os.path.join(run_dir, "manifest.json")

    write_cases(cases_path, build.cases)
    manifest = build_manifest(
        spec=spec,
        build=build,
        run_dir=run_dir,
        model=model,
        args=args or {},
        price_per_mtok=price_per_mtok,
        repeat=repeat,
        uid_mode="content-hash",
    )
    with open(manifest_path, "w", encoding="utf-8") as fh:
        json.dump(manifest, fh, ensure_ascii=False, indent=2)
        fh.write("\n")

    existing = _read_jsonl(requests_path)
    done: Dict[Tuple[str, int], Dict[str, Any]] = {}
    for row in existing:
        if row.get("ok"):
            done[(row["request_id"], int(row.get("repeat", 0)))] = row

    wanted = [r for r in build.requests if requests_filter is None or r.request_id in set(requests_filter)]
    todo: List[Tuple[Request, int]] = []
    for req in wanted:
        for rep in range(repeat):
            key = (req.request_id, rep)
            fp = req.fingerprint(model)
            if force or key not in done or done[key].get("fingerprint") != fp:
                todo.append((req, rep))

    t0 = time.time()
    tokens_total = sum(int(r.get("tokens", 0)) for r in done.values())
    cached = len(wanted) * repeat - len(todo)
    failed = 0
    called = 0

    if offline:
        todo = []

    def _one(item: Tuple[Request, int]) -> Dict[str, Any]:
        req, rep = item
        state = req.state
        if rep > 0:
            # Self-consistency repeats must not be served from a server cache:
            # a fresh uid goes into the state (official cookbook pattern), not
            # into the questions, so the judgment itself is unchanged.
            from jevkit import fresh_uid

            if isinstance(state, dict):
                state = dict(state)
                state["uid"] = fresh_uid(f"rep{rep}")
        res = ask_or_none(state, req.questions, model=model)
        if res is None:
            return {
                "request_id": req.request_id,
                "repeat": rep,
                "fingerprint": req.fingerprint(model),
                "ok": False,
                "error": "jevkit.ask_or_none returned None (fail-open: transport or API error)",
                "sampled_at": utc_now(),
            }
        return {
            "request_id": req.request_id,
            "repeat": rep,
            "fingerprint": req.fingerprint(model),
            "ok": True,
            "model": res.model,
            "tokens": res.tokens,
            "latency_ms": res.latency_ms,
            "attempts": res.attempts,
            "answers": res.answers,
            "sampled_at": utc_now(),
        }

    def _accumulate(row: Mapping[str, Any]) -> None:
        nonlocal tokens_total, failed, called
        called += 1
        if row.get("ok"):
            tokens_total += int(row.get("tokens", 0) or 0)
        else:
            failed += 1

    if todo:
        with open(requests_path, "a", encoding="utf-8") as fh:
            if concurrency <= 1:
                iterator = (_one(item) for item in todo)
                for i, row in enumerate(iterator, 1):
                    fh.write(json.dumps(row, ensure_ascii=False) + "\n")
                    fh.flush()
                    _accumulate(row)
                    if log and progress_every and i % progress_every == 0:
                        log(f"  collected {i}/{len(todo)} requests (tokens={tokens_total})")
            else:
                with ThreadPoolExecutor(max_workers=concurrency) as pool:
                    futures = {pool.submit(_one, item): item for item in todo}
                    for i, fut in enumerate(as_completed(futures), 1):
                        row = fut.result()
                        fh.write(json.dumps(row, ensure_ascii=False) + "\n")
                        fh.flush()
                        _accumulate(row)
                        if log and progress_every and i % progress_every == 0:
                            log(f"  collected {i}/{len(todo)} requests (tokens={tokens_total})")

    return CollectStats(
        requests_total=len(wanted) * repeat,
        called=called,
        cached=cached,
        failed=failed,
        tokens=tokens_total,
        wall_seconds=time.time() - t0,
    )


def load_judgments(run_dir: str) -> Tuple[Dict[str, Dict[str, Any]], Dict[str, Any]]:
    """Read requests.jsonl into {request_id: best row} plus the manifest.

    With `repeat > 1` the FIRST successful repeat is used as the headline and
    all repeats are returned inside the row as `repeats` so the metrics layer can
    report agreement instead of silently averaging sampling noise away.
    """
    manifest_path = os.path.join(run_dir, "manifest.json")
    manifest: Dict[str, Any] = {}
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as fh:
            manifest = json.load(fh)
    rows = _read_jsonl(os.path.join(run_dir, "requests.jsonl"))
    out: Dict[str, Dict[str, Any]] = {}
    for row in rows:
        rid = row.get("request_id", "")
        if not rid:
            continue
        if row.get("ok"):
            prev = out.get(rid)
            if prev is None or not prev.get("ok"):
                out[rid] = dict(row)
                out[rid]["repeats"] = [row]
            else:
                prev.setdefault("repeats", []).append(row)
        else:
            out.setdefault(rid, dict(row))
    return out, manifest


def score_cases(
    build: SpecBuild,
    judgments: Mapping[str, Mapping[str, Any]],
    *,
    repeat_aggregate: str = "first",
) -> Dict[str, Optional[float]]:
    """Extract one score per case from the logged answers (None = no judgment).

    The score is the `noul` probability of the case's question. A missing or
    malformed answer yields None rather than 0.0 — "no answer" and "answered
    zero" are different evidence and must not be conflated.
    """
    scores: Dict[str, Optional[float]] = {}
    by_request: Dict[str, List[Case]] = {}
    for case in build.cases:
        by_request.setdefault(case.request_id, []).append(case)
    for request in build.requests:
        row = judgments.get(request.request_id)
        cases = by_request.get(request.request_id, [])
        if not row or not row.get("ok"):
            for case in cases:
                scores[case.case_id] = None
            continue
        rows = [r for r in ([row] + list(row.get("repeats", []) or [])) if r.get("ok")] if repeat_aggregate != "first" else [row]
        if not rows:
            rows = [row]
        for case in cases:
            qid = case.meta.get("question_id")
            values: List[float] = []
            for candidate in rows:
                ans = (candidate.get("answers") or {}).get(qid) if qid else None
                if isinstance(ans, dict) and isinstance(ans.get("noul"), (int, float)):
                    values.append(float(ans["noul"]))
            if not values:
                scores[case.case_id] = None
                continue
            if repeat_aggregate == "first":
                scores[case.case_id] = values[0]
            elif repeat_aggregate == "mean":
                scores[case.case_id] = sum(values) / len(values)
            else:
                raise ValueError(f"unknown repeat_aggregate {repeat_aggregate!r} (expected first|mean)")
    return scores
