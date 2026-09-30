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
from scripts.jev_eval.spec import Case, Request, SpecBuild, parse_spec_args
from scripts.jev_eval.specs.event_calendar import EventCalendarSpec
from scripts.jev_eval.specs import stock_layer
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


def _stock_spec():
    return stock_layer.StockLayerSpec()


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


class DecisionLog(unittest.TestCase):
    """每筆 case 的稽核列（分數/門檻/決定/延遲/tokens/成本）必須存在且一致。"""

    def test_decision_rows_carry_threshold_and_cost(self):
        from scripts.jev_eval.cli import decision_rows
        from scripts.jev_eval.spec import Case, Request, SpecBuild

        cases = [
            Case(case_id="d:a", request_id="judge:d", group_id="d", gt=True, baselines={}, meta={"question_id": "a"}),
            Case(case_id="d:b", request_id="judge:d", group_id="d", gt=False, baselines={}, meta={"question_id": "b"}),
        ]
        build = SpecBuild(
            requests=[Request(request_id="judge:d", state={}, questions={}, case_ids=["d:a", "d:b"])],
            cases=cases,
            meta={},
        )
        judgments = {
            "judge:d": {"ok": True, "model": "jev-1.13.0", "tokens": 1000, "latency_ms": 800,
                        "answers": {"a": {"type": "noul", "noul": 0.8}, "b": {"type": "noul", "noul": 0.2}}}
        }
        scores = runner.score_cases(build, judgments)
        rows = decision_rows(build, judgments, scores, 0.5, price_per_mtok=0.042)
        self.assertEqual(len(rows), 2)
        self.assertEqual(rows[0]["decision"], True)
        self.assertEqual(rows[1]["decision"], False)
        self.assertEqual(rows[0]["threshold"], 0.5)
        self.assertEqual(rows[0]["latency_ms"], 800)
        # cost is allocated per case (Jev bills per request, not per question)
        self.assertEqual(rows[0]["allocated_tokens"], 500)
        self.assertAlmostEqual(rows[0]["allocated_cost_usd"], 500 / 1_000_000 * 0.042, places=10)

    def test_decision_rows_mark_failed_request(self):
        from scripts.jev_eval.cli import decision_rows
        from scripts.jev_eval.spec import Case, Request, SpecBuild

        build = SpecBuild(
            requests=[Request(request_id="judge:d", state={}, questions={}, case_ids=["d:a"])],
            cases=[Case(case_id="d:a", request_id="judge:d", group_id="d", gt=True, baselines={}, meta={"question_id": "a"})],
            meta={},
        )
        rows = decision_rows(build, {"judge:d": {"ok": False, "error": "boom"}}, runner.score_cases(build, {"judge:d": {"ok": False}}), 0.5, price_per_mtok=0.042)
        self.assertFalse(rows[0]["ok"])
        self.assertIsNone(rows[0]["score"])
        self.assertIsNone(rows[0]["decision"])


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


    def test_paired_auc_margin_zero_for_identical_streams(self):
        """Same answers on both tasks ⇒ margin ~0 with a CI that includes 0 ⇒ recall not excluded."""
        pairs = [(f"c{i}", (i % 10) / 10.0, i % 2 == 0) for i in range(120)]
        m = metrics_mod.paired_auc_margin(pairs, pairs, iterations=300, seed=7)
        self.assertIsNotNone(m)
        self.assertEqual(m["n_shared"], 120)
        self.assertLessEqual(abs(m["point"]), 1e-9)
        self.assertLessEqual(m["ci"][0], 0.0)  # 下界 <= 0 ⇒ increment gate 會降級
        self.assertGreaterEqual(m["ci"][1], 0.0)

    def test_paired_auc_margin_positive_when_judge_beats_probe(self):
        """Judge correlates with the label, probe is label-blind ⇒ margin CI lower bound > 0."""
        labels = [i % 2 == 0 for i in range(200)]
        judge = [(f"c{i}", 0.9 if labels[i] else 0.1, labels[i]) for i in range(len(labels))]
        probe = [(f"c{i}", 0.5, labels[i]) for i in range(len(labels))]
        m = metrics_mod.paired_auc_margin(judge, probe, iterations=300, seed=11)
        self.assertIsNotNone(m)
        self.assertGreater(m["ci"][0], 0.0)
        # judge 完美（AUC 1.0）− probe 全平手（AUC 0.5）= 0.5
        self.assertGreaterEqual(m["point"], 0.5)

    def test_paired_auc_margin_none_when_too_few_shared(self):
        judge = [(f"c{i}", 0.7, i % 2 == 0) for i in range(10)]
        probe = [(f"c{i}", 0.4, i % 2 == 0) for i in range(10)]
        self.assertIsNone(metrics_mod.paired_auc_margin(judge, probe, iterations=100))

    def test_grade_increment_gate_downgrades_when_margin_ci_includes_zero(self):
        """絕對 AUC 過關但增量 margin 下界 <= 0 ⇒ 仍須降為未驗證（洩漏不可排除）。"""
        metrics = {
            "n_cases": 400,
            "n_answered": 400,
            "evaluation": {
                "n": 400,
                "base_rate": 0.5,
                "jev": {"precision": 0.6},
                "lift": {"jev_auc": {"point": 0.62, "ci": [0.56, 0.68]}, "baselines": {}},
            },
        }
        verdict = metrics_mod.grade(
            metrics,
            auc_margin={"point": 0.01, "ci": [-0.02, 0.04], "n_shared": 200},
        )
        self.assertEqual(verdict["verdict"], "未驗證")
        self.assertIn("increment gate", verdict["detail"])

class StockLayerChecks(unittest.TestCase):
    def _write_jsonl(self, rows):
        fd, path = tempfile.mkstemp(suffix=".jsonl")
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            for row in rows:
                fh.write(json.dumps(row) + "\n")
        self.addCleanup(lambda: os.path.exists(path) and os.remove(path))
        return path

    def test_stock_layer_sampling_is_seeded_and_stratified(self):
        """固定 seed ⇒ 同一組 symbols/dates；且每個 L1 至少 1 檔（分層成立）。"""
        rows = []
        for iid in ("semiconductor", "shipping", "financials"):
            for k in range(4):
                for d in ("2026-07-01", "2026-07-08", "2026-07-15", "2026-07-22"):
                    rows.append({
                        "date": d, "symbol": f"{iid[:3].upper()}{k}.TW", "industry_id": iid,
                        "forward": {"ret": 0.01, "hit": k % 2 == 0},
                        "backward": {"backward_hit": k % 2 == 0, "backward_date": "2026-06-24"},
                        "baselines": {"momentum_20d": 0.1},
                    })
        path = self._write_jsonl(rows)
        spec = _stock_spec()
        b1 = spec.build({"panel": path, "max_symbols": "12", "max_dates": "4", "seed": "s1"})
        b2 = spec.build({"panel": path, "max_symbols": "12", "max_dates": "4", "seed": "s1"})
        self.assertEqual([c.case_id for c in b1.cases], [c.case_id for c in b2.cases])
        self.assertEqual([r.request_id for r in b1.requests], [r.request_id for r in b2.requests])
        per_industry = {}
        for case in b1.cases:
            per_industry.setdefault(case.meta["industry_id"], set()).add(case.meta["symbol"])
        self.assertEqual(len(per_industry), 3)
        for iid, syms in per_industry.items():
            # 分層守門：預設 symbols_per_industry=2 ⇒ 每層應取到 2 檔（上限足夠時）
            self.assertGreaterEqual(len(syms), 2, f"industry {iid} lost its stratified quota")

    def test_stock_layer_cost_cap_rejects_oversized_run(self):
        """估算成本超過 COST_CAP_USD ⇒ 建構即 raise（寧可少跑，不可超支）。"""
        # 100 dates × 40 檔（20 個產業 × 2）⇒ 100×40×408 tokens ≈ $0.0686 > cap($0.05) ⇒ 必須 raise
        rows = [{
            "date": f"2026-{1 + (d // 28):02d}-{1 + (d % 28):02d}", "symbol": f"S{i}.TW",
            "industry_id": f"ind{i % 20}",
            "forward": {"ret": 0.01, "hit": True}, "backward": {"backward_hit": True},
        } for d in range(100) for i in range(40)]
        path = self._write_jsonl(rows)
        # 100 requests × 40 檔 × 408 tokens ≈ $0.0686 > cap($0.05) ⇒ 必須 raise（token-aware）
        with self.assertRaises(ValueError) as ctx:
            _stock_spec().build({"panel": path, "max_symbols": "40", "max_dates": "100", "stage": "confirm"})
        msg = str(ctx.exception)
        self.assertIn("COST_CAP_USD", msg)
        self.assertIn("tokens", msg)   # 訊息必須揭露 token 估算（修前只看 requests）
        ok = _stock_spec().build({"panel": path, "max_symbols": "40", "max_dates": "20"})
        self.assertLessEqual(ok.meta["estimated_cost_usd"], stock_layer.COST_CAP_USD)
        self.assertEqual(ok.meta["requests"], 20)

    def _stage_fixture(self, industries, per_industry, dates):
        rows = []
        for iid in range(industries):
            for k in range(per_industry):
                for d in range(dates):
                    rows.append({
                        "date": f"2026-{(1 + d // 28):02d}-{(1 + d % 28):02d}",
                        "symbol": f"I{iid}S{k}.TW",
                        "industry_id": f"ind{iid}",
                        "forward": {"ret": 0.01, "hit": (d + k) % 2 == 0},
                        "backward": {"backward_hit": True, "backward_date": "2026-01-02"},
                        "baselines": {"momentum_20d": 0.1},
                    })
        return self._write_jsonl(rows)

    def test_stock_layer_stage_screen_is_20x20(self):
        """★ 修 regression：stage=screen 必須真的 20×20（cases=400）。

        修前 `merged_args` 先用 self.defaults 填好 max_symbols/max_dates，之後的
        `setdefault(stage_defaults)` 永不生效 ⇒ screen 靜默變成 confirm 形狀（實測 40×39）。
        """
        path = self._stage_fixture(industries=12, per_industry=4, dates=25)
        b = _stock_spec().build({"panel": path, "stage": "screen"})
        self.assertEqual(b.meta["stage"], "screen")
        self.assertEqual(b.meta["dates"], 20)
        self.assertEqual(b.meta["symbols_sampled"], 20)
        self.assertEqual(len(b.cases), 400)
        self.assertEqual(len(b.requests), 20)

    def test_stock_layer_stage_confirm_is_60x40(self):
        """既有預期不變：stage=confirm ⇒ 40 dates × 60 symbols = 2,400 cases。"""
        path = self._stage_fixture(industries=30, per_industry=2, dates=45)
        b = _stock_spec().build({"panel": path, "stage": "confirm"})
        self.assertEqual(b.meta["stage"], "confirm")
        self.assertEqual(b.meta["dates"], 40)
        self.assertEqual(b.meta["symbols_sampled"], 60)
        self.assertEqual(len(b.cases), 2400)
        self.assertEqual(len(b.requests), 40)
        # 上限已於 2026-09-30 重新校準（token-aware）：confirm 的真實估價必須落在上限內
        self.assertLessEqual(b.meta["estimated_cost_usd"], stock_layer.COST_CAP_USD)


    def test_stock_layer_stage_screen_via_cli_path(self):
        """★ 走**CLI 的真實呼叫形狀**（cli.py:119-122）：先 merged_args ⇒ 再 build(user_args=…)。

        這是 2026-09-30 真跑時咬到的破口：`build(raw_args)` 的直呼測試會過 ✗，
        但 CLI 先把 merged 結果餵進來 ⇒ 「使用者是否真的給了」在 args 裡已消失 ⇒
        `stage=screen` 靜默變成 confirm 形狀（40 requests ✗）。修法＝CLI 傳 user_args。
        """
        path = self._stage_fixture(industries=12, per_industry=4, dates=25)
        raw = parse_spec_args(["panel=" + path, "stage=screen"])
        spec = _stock_spec()
        merged = spec.merged_args(raw, spec.defaults)
        build = spec.build(merged, user_args=raw)          # ← CLI 的呼叫形狀
        self.assertEqual(build.meta["dates"], 20)
        self.assertEqual(build.meta["symbols_sampled"], 20)
        self.assertEqual(len(build.requests), 20)
        # 對照：不傳 user_args（舊行為）會落到 confirm 形狀 ⇒ 證明這個參數是必要的
        legacy = spec.build(merged)
        self.assertGreater(len(legacy.requests), 20)

    def test_stock_layer_cost_estimate_is_token_aware(self):
        """成本上限改以 tokens 判定（request-based 低估 2.7× ⇒ 守不住）。"""
        path = self._stage_fixture(industries=12, per_industry=4, dates=25)
        b = _stock_spec().build({"panel": path, "stage": "screen"})
        m = b.meta
        self.assertIn("estimated_tokens", m)
        self.assertGreater(m["estimated_tokens"], 0)
        self.assertEqual(m["cost_cap_usd"], stock_layer.COST_CAP_USD)
        # 實測錨點：39 檔／request ⇒ 16,326 tokens ⇒ token 模型要落在同量級
        self.assertGreater(stock_layer.TOKENS_PER_SYMBOL_REQUEST, 300)
        self.assertLess(stock_layer.TOKENS_PER_SYMBOL_REQUEST, 600)

    def test_stock_layer_explicit_arg_beats_stage_default(self):
        """優先序：顯式 CLI 參數 > stage 預設 > self.defaults。"""
        path = self._stage_fixture(industries=12, per_industry=4, dates=25)
        b = _stock_spec().build({"panel": path, "stage": "screen", "max_dates": "5", "max_symbols": "8"})
        self.assertEqual(b.meta["dates"], 5)
        self.assertEqual(b.meta["symbols_sampled"], 8)

class MetricMathExtra(unittest.TestCase):
    pass


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


def _event_row(event_id, event_type, direction, anchor, kind="peak", affected=None, start=None, end=None):
    return {
        "event_id": event_id,
        "event_type": event_type,
        "name": event_type,
        "description": event_type,
        "direction": direction,
        "base_weight": 0.5,
        "decay_days": 3,
        "affected_industries": affected or [],
        "window_start": start or anchor,
        "peak_date": anchor,
        "window_end": end or anchor,
        "anchor_kind": kind,
        "nominal_anchor_date": anchor,
        "anchor_date": anchor,
        "anchor_shift_days": 0,
        "sessions_from_window_start": 0,
        "sessions_anchor_to_window_end": 0,
        "calendar_year": 2021,
    }


class EventLayerFixture(unittest.TestCase):
    """Stage-3 spec: the event skeleton must not leak the outcome, and the
    platform's own prior must abstain exactly where the platform does."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(prefix="jev-eval-events-selfcheck-")
        self.panel = os.path.join(self.dir, "panel.jsonl")
        self.events = os.path.join(self.dir, "events.jsonl")
        self.adjustments = os.path.join(self.dir, "adjustments.jsonl")
        panel_rows = []
        for d in range(1, 9):
            date = f"2021-06-{d + 9:02d}"
            panel_rows.append(_panel_row(date, "semiconductor", 1.0 + d, -1.0, fwd_hit=(d % 2 == 0), back_hit=(d % 3 == 0)))
            panel_rows.append(_panel_row(date, "financials", -1.0 - d, 2.0, fwd_hit=(d % 3 == 0), back_hit=(d % 2 == 0), name="金融保險"))
        with open(self.panel, "w", encoding="utf-8") as fh:
            for row in panel_rows:
                fh.write(json.dumps(row, ensure_ascii=False) + "\n")
        event_rows = [
            _event_row("msci_rebalance_2021_05", "msci_rebalance", "neutral", "2021-06-10", affected=["semiconductor"]),
            _event_row("ex_dividend_2021", "ex_dividend", "bullish", "2021-06-11", kind="start", affected=["financials"]),
        ]
        with open(self.events, "w", encoding="utf-8") as fh:
            for row in event_rows:
                fh.write(json.dumps(row, ensure_ascii=False) + "\n")
        with open(self.adjustments, "w", encoding="utf-8") as fh:
            for d in range(1, 9):
                date = f"2021-06-{d + 9:02d}"
                for iid in ("semiconductor", "financials"):
                    fh.write(json.dumps({"date": date, "industry_id": iid, "event_adjustment": 0.004}) + "\n")

    def _args(self, **over):
        args = {
            "events": self.events,
            "panel": self.panel,
            "adjustments": self.adjustments,
        }
        args.update(over)
        return args

    # A -------------------------------------------------------------------
    def test_event_state_hides_ground_truth(self):
        build = EventCalendarSpec().build(self._args())
        state_text = json.dumps(build.requests[0].state, ensure_ascii=False)
        for banned in ("forward", "backward", "hit", "net_return"):
            self.assertNotIn(banned, state_text)
        self.assertFalse(hasattr(EventCalendarSpec.view(build.cases[0]), "gt"))

    # B -------------------------------------------------------------------
    def test_event_build_is_deterministic(self):
        spec = EventCalendarSpec()
        a = spec.build(self._args())
        b = spec.build(self._args())
        self.assertEqual(
            [r.fingerprint("jev-1.13.0") for r in a.requests],
            [r.fingerprint("jev-1.13.0") for r in b.requests],
        )
        self.assertEqual([c.to_json() for c in a.cases], [c.to_json() for c in b.cases])

    # C -------------------------------------------------------------------
    def test_leakage_state_carries_no_prices_and_no_event_details(self):
        build = EventCalendarSpec().build(self._args(mode="leakage"))
        text = json.dumps(build.requests[0].state, ensure_ascii=False)
        for banned in ("ret_5d", "trailing_return", "vol", "session_return", "below_60", "event_type", "msci"):
            self.assertNotIn(banned, text)
        self.assertTrue(all(not c.baselines for c in build.cases))

    def test_leakage_ground_truth_is_the_backward_window(self):
        judge = EventCalendarSpec().build(self._args()).cases_by_id()
        probe = EventCalendarSpec().build(self._args(mode="leakage")).cases_by_id()
        shared = sorted(set(judge) & set(probe))
        self.assertTrue(shared)
        self.assertEqual(probe[shared[0]].gt, bool(judge[shared[0]].meta["backward"]["backward_hit"]))

    # D -------------------------------------------------------------------
    def test_prior_abstains_where_the_platform_declares_no_direction(self):
        cases = EventCalendarSpec().build(self._args()).cases
        neutral = [c for c in cases if c.meta["event_direction"] == "neutral"]
        bullish = [c for c in cases if c.meta["event_direction"] == "bullish"]
        self.assertTrue(neutral and bullish)
        for c in neutral:
            self.assertIsNone(c.baselines["event_direction_prior"])
        # affected industry -> full weight; everything else -> 0.3 spillover
        for c in bullish:
            expected = 1.0 if c.meta["industry_id"] == "financials" else 0.3
            self.assertAlmostEqual(c.baselines["event_direction_prior"], expected)

    def test_unconditional_baseline_is_constant(self):
        cases = EventCalendarSpec().build(self._args()).cases
        self.assertEqual({c.baselines["unconditional_always_up"] for c in cases}, {0.5})

    def test_unavailable_baseline_is_null_not_a_proxy(self):
        cases = EventCalendarSpec().build(self._args()).cases
        name = "platform_eventdriven_t1_capital_flow_direction"
        self.assertTrue(all(c.baselines[name] is None for c in cases))
        obs = [
            metrics_mod.Observation(c.case_id, c.group_id, c.gt, 0.6, dict(c.baselines)) for c in cases
        ]
        result = metrics_mod.evaluate(obs, baseline_names=(name,), bootstrap_iterations=20)
        self.assertFalse(result["evaluation"]["baselines"][name]["available"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
