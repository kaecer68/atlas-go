#!/usr/bin/env python3
# jev-docs: https://docs.typesafe.ai/primitives  (self-check for the evaluation layer)
"""Offline self-check for the Jev evaluation framework.  python3 scripts/jev_eval/selfcheck.py

No network, no API key, no repo data: everything is synthetic and deterministic,
so it can run inside CI. It exists because the framework's failure modes are not
loud — a leaked ground truth, a metrics layer that silently treats "no answer" as
"answered 0.0", or a calibration step that reads its own evaluation split would
all still produce a plausible-looking report.

Checks:
  A. PIT separation      — the state builder cannot see `gt`/baselines (structural).
  B. Determinism         — the same inputs produce identical request fingerprints.
  C. Leakage probe state — contains no price feature at all.
  D. Fail-open scoring   — a failed request yields "no score", never 0.0.
  E. Metric math         — Wilson / AUC (with ties) / ECE / bootstrap sanity.
  F. Threshold discipline— the threshold comes from the calibration split only.
  G. Grading             — 已驗證 / 已更正 / 未驗證 behave as documented.
"""
from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest

_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
if _REPO_ROOT not in sys.path:
    sys.path.insert(0, _REPO_ROOT)

from scripts.jev_eval import metrics as metrics_mod
from scripts.jev_eval import runner
from scripts.jev_eval.spec import Case, Request, SpecBuild
from scripts.jev_eval.specs.industry_l1 import IndustryL1Spec


def _panel_row(date, iid, ret5, ret20, fwd_hit, back_hit, name="半導體"):
    return {
        "date": date,
        "industry_id": iid,
        "industry_name_zh": name,
        "hold_days": 5,
        "cost_rate": 0.00585,
        "pit": {
            "ret_5d_pct": ret5,
            "ret_20d_pct": ret20,
            "ret_60d_pct": 1.0,
            "vol_20d_pct": 1.0,
            "dist_high_60d_pct": -2.0,
            "daily_return_pct": 0.1,
            "history_days": 60,
        },
        "backward": {
            "backward_date": "2021-01-01",
            "backward_return": back_hit and 0.02 or -0.02,
            "backward_net_return": back_hit and 0.014 or -0.026,
            "backward_hit": bool(back_hit),
        },
        "forward": {
            "forward_date": "2021-01-20",
            "forward_return": fwd_hit and 0.03 or -0.01,
            "forward_net_return": fwd_hit and 0.024 or -0.016,
            "hit": bool(fwd_hit),
            "forward_sessions": 5,
            "forward_span_calendar_days": 7,
        },
    }


class PanelFixture(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(prefix="jev-eval-selfcheck-")
        self.panel = os.path.join(self.dir, "panel.jsonl")
        rows = []
        for d in range(1, 13):
            date = f"2021-03-{d:02d}"
            rows.append(_panel_row(date, "semiconductor", 1.0 + d, -1.0, fwd_hit=(d % 2 == 0), back_hit=(d % 3 == 0)))
            rows.append(_panel_row(date, "financials", -1.0 - d, 2.0, fwd_hit=(d % 3 == 0), back_hit=(d % 2 == 0), name="金融保險"))
        with open(self.panel, "w", encoding="utf-8") as fh:
            for row in rows:
                fh.write(json.dumps(row, ensure_ascii=False) + "\n")

    # A -------------------------------------------------------------------
    def test_case_view_hides_ground_truth(self):
        build = IndustryL1Spec().build({"panel": self.panel})
        view = IndustryL1Spec.view(build.cases[0])
        self.assertFalse(hasattr(view, "gt"))
        self.assertFalse(hasattr(view, "baselines"))
        state_text = json.dumps(build.requests[0].state, ensure_ascii=False)
        self.assertNotIn("forward", state_text)
        self.assertNotIn("hit", state_text)

    # B -------------------------------------------------------------------
    def test_build_is_deterministic(self):
        spec = IndustryL1Spec()
        a = spec.build({"panel": self.panel})
        b = spec.build({"panel": self.panel})
        self.assertEqual(
            [r.fingerprint("jev-1.13.0") for r in a.requests],
            [r.fingerprint("jev-1.13.0") for r in b.requests],
        )
        self.assertEqual([c.to_json() for c in a.cases], [c.to_json() for c in b.cases])

    # C -------------------------------------------------------------------
    def test_leakage_state_carries_no_price_series(self):
        build = IndustryL1Spec().build({"panel": self.panel, "mode": "leakage"})
        text = json.dumps(build.requests[0].state, ensure_ascii=False)
        for banned in ("ret_5d", "trailing_return", "vol", "session_return", "below_60"):
            self.assertNotIn(banned, text)
        self.assertTrue(all(not c.baselines for c in build.cases))


class ScoringChecks(unittest.TestCase):
    def _build(self, n_dates=12, industries=("a", "b")):
        cases, requests = [], []
        for d in range(n_dates):
            date = f"2021-03-{d + 1:02d}"
            qs = {}
            ids = []
            for i, iid in enumerate(industries):
                qs[iid] = {"type": "noul", "instructions": "x"}
                cid = f"{date}:{iid}"
                ids.append(cid)
                cases.append(
                    Case(
                        case_id=cid,
                        request_id=f"judge:{date}",
                        group_id=date,
                        gt=(d + i) % 2 == 0,
                        baselines={"industry_momentum_20d": 1.0 if (d % 2 == 0) else -1.0},
                        meta={"question_id": iid},
                    )
                )
            requests.append(Request(request_id=f"judge:{date}", state={"as_of": date}, questions=qs, case_ids=ids))
        return SpecBuild(requests=requests, cases=cases, meta={})

    # D -------------------------------------------------------------------
    def test_failed_request_scores_none(self):
        build = self._build()
        judgments = {"judge:2021-03-01": {"ok": False, "error": "boom"}}
        scores = runner.score_cases(build, judgments)
        self.assertIsNone(scores["2021-03-01:a"])
        self.assertIsNone(scores["2021-03-02:a"])  # request absent entirely

    def test_missing_answer_key_scores_none(self):
        build = self._build()
        judgments = {"judge:2021-03-01": {"ok": True, "answers": {"a": {"type": "noul", "noul": 0.4}}}}
        scores = runner.score_cases(build, judgments)
        self.assertEqual(scores["2021-03-01:a"], 0.4)
        self.assertIsNone(scores["2021-03-01:b"])


class MetricMath(unittest.TestCase):
    # E -------------------------------------------------------------------
    def test_wilson_matches_go_reference(self):
        lo, hi = metrics_mod.wilson_interval(77, 180)
        self.assertAlmostEqual(lo, 0.3578, places=3)
        self.assertAlmostEqual(hi, 0.5008, places=3)
        self.assertEqual(metrics_mod.wilson_interval(0, 0), (0.0, 0.0))

    def test_auc_handles_ties_and_degenerate_inputs(self):
        self.assertAlmostEqual(metrics_mod.auc([0.1, 0.2, 0.3, 0.4], [False, False, True, True]), 1.0)
        self.assertAlmostEqual(metrics_mod.auc([0.4, 0.3, 0.2, 0.1], [False, False, True, True]), 0.0)
        self.assertAlmostEqual(metrics_mod.auc([0.5, 0.5, 0.5, 0.5], [True, False, True, False]), 0.5)
        self.assertIsNone(metrics_mod.auc([0.5], [True]))
        self.assertIsNone(metrics_mod.auc([], []))

    def test_ece_zero_for_perfectly_calibrated(self):
        # 100 cases at score 0.4, exactly 40% positive → ECE 0.
        scores = [0.4] * 100
        labels = [i < 40 for i in range(100)]
        self.assertAlmostEqual(metrics_mod.ece(scores, labels), 0.0, places=9)

    def test_cluster_bootstrap_is_seeded(self):
        obs = [
            metrics_mod.Observation(f"c{i}", f"2021-03-{i % 10 + 1:02d}", i % 2 == 0, 0.3 + 0.01 * i, {})
            for i in range(60)
        ]
        stat = lambda sample: metrics_mod.auc([o.score for o in sample], [o.gt for o in sample])  # noqa: E731
        a = metrics_mod.cluster_bootstrap(obs, stat, iterations=100, seed=7)
        b = metrics_mod.cluster_bootstrap(obs, stat, iterations=100, seed=7)
        self.assertEqual(a, b)
        self.assertIsNotNone(a[1])


class EvaluationFlow(unittest.TestCase):
    def _observations(self, n_dates=20, informed=True):
        obs = []
        for d in range(n_dates):
            date = f"2021-04-{d + 1:02d}"
            for i in range(6):
                gt = (d + i) % 2 == 0
                if informed:
                    score = 0.75 if gt else 0.25
                else:
                    score = 0.5
                baselines = {"industry_momentum_20d": 1.0 if (i % 2 == 0) else -1.0}
                obs.append(metrics_mod.Observation(f"{date}:{i}", date, gt, score, baselines))
        return obs

    # F -------------------------------------------------------------------
    def test_threshold_comes_from_calibration_split(self):
        obs = self._observations()
        result = metrics_mod.evaluate(obs, baseline_names=("industry_momentum_20d",), bootstrap_iterations=50, min_precision=0.6)
        calib_groups = result["calibration"]["groups"]
        eval_groups = result["evaluation"]["groups"]
        self.assertEqual(set(calib_groups) & set(eval_groups), set())
        # calibration must be the EARLY part of the window
        self.assertLess(max(calib_groups), min(eval_groups))

    # G -------------------------------------------------------------------
    def test_grade_positive_when_auc_clears_chance(self):
        obs = self._observations(informed=True)
        result = metrics_mod.evaluate(obs, baseline_names=("industry_momentum_20d",), bootstrap_iterations=200, min_precision=0.6)
        verdict = metrics_mod.grade(result, min_eval_cases=10)
        self.assertEqual(verdict["verdict"], "已驗證")

    def test_grade_unverified_when_scores_are_flat(self):
        obs = self._observations(informed=False)
        result = metrics_mod.evaluate(obs, baseline_names=("industry_momentum_20d",), bootstrap_iterations=200)
        verdict = metrics_mod.grade(result, min_eval_cases=10)
        self.assertEqual(verdict["verdict"], "未驗證")

    def test_grade_unverified_when_samples_too_small(self):
        obs = self._observations(n_dates=4)
        result = metrics_mod.evaluate(obs, baseline_names=("industry_momentum_20d",), bootstrap_iterations=50)
        verdict = metrics_mod.grade(result, min_eval_cases=200)
        self.assertEqual(verdict["verdict"], "未驗證")
        self.assertIn("held-out cases", verdict["detail"])

    def test_leakage_cap_downgrades_a_positive_result(self):
        obs = self._observations(informed=True)
        result = metrics_mod.evaluate(obs, baseline_names=("industry_momentum_20d",), bootstrap_iterations=200, min_precision=0.6)
        verdict = metrics_mod.grade(
            result,
            leakage={"auc": {"point": 0.9, "ci": [0.85, 0.95]}, "n": 300},
            min_eval_cases=10,
        )
        self.assertEqual(verdict["verdict"], "未驗證")
        self.assertIn("leakage cap", verdict["detail"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
