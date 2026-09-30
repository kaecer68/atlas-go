# jev-docs: https://docs.typesafe.ai/primitives  (spec layer never calls the API)
"""stock_layer — Stage 2（個股／錢潮層）的 Jev 影子評估 spec。

【為什麼需要它】Stage 1（canonical L1 產業層）與 Stage 3（事件層）都已做過；個股／錢潮層
**從來沒有被量測過**。本 spec 是那個問題的唯一入口。

【有界取樣（per-stock × days 是乘法 ⇒ 全展開不可行）】
- symbols：依 `industry_id` **分層**，每層取 `symbols_per_industry` 檔（預設 2），
  以 `sha256(symbol|seed)` 排序取前 K（**決定性且不偏向低代號**），總數上限 `max_symbols`。
- dates：panel 日期範圍內**等距**取 `max_dates` 個（stride 決定性）。
- 成本模型：**1 request = 1 個日期**（同日各檔 fan-out）⇒ requests = len(dates)；
  **計價以 tokens 為準** —— 實測 2026-09-30：39 檔／request ⇒ **16,326 tokens/request**（$0.042/Mtok
  ⇒ $0.000686/request，為 Stage 1 request-based 基準的 2.7×）⇒ 本 spec 以
  408 tokens/檔 × 檔數 × requests 估價並**以 USD 判上限**（request-based 的 cap 守不住）。
  成本上限常數 `COST_CAP_USD`（預設 0.02）：估算超過即 **raise**（寧可少跑，不可超支）。

【兩階段（先篩再確認）】
- `stage=screen`：預設 `max_symbols=20`、`max_dates=20` ⇒ 400 cases ≈ ≤20 requests ≈ $0.005
- `stage=confirm`：預設 `max_symbols=60`、`max_dates=40` ⇒ 2,400 cases ≈ 40 requests ≈ $0.010
（明示兩階段是為了避免「一次跑滿才發現沒訊號」的浪費；兩階段共用同一 spec 與同一判準。）

【輸入契約（由操作者自生產 SQLite 匯出；spec 只讀檔，永不呼叫 API）】
`sympanel.jsonl` 每列至少：
`{"date": "YYYY-MM-DD", "symbol": "2330.TW", "industry_id": "semiconductor",
  "forward": {"ret": 0.012, "hit": true}, "backward": {"backward_hit": true},
  "pit": {...}, "cost_rate": 0.00585, "hold_days": 5}`
（`forward.hit` 已扣 round-trip 成本 ⇒ 標籤與 Stage 1 同義；`backward_hit` 供洩漏探針對照。）

【結構紀律（沿用 spec.py 的五條）】state 不得含 `gt`／`baselines`；標籤一律由程式重算；
模型只看 `CaseView`；閾值在 calibration split 上校準；結論一律附樣本數與成本。
"""
from __future__ import annotations

import hashlib
import json
import os
from typing import Any, Dict, List, Mapping, Optional, Sequence

from ..spec import Case, Request, SpecBuild, TaskSpec

#: 成本上限（USD，**token-aware**）。修前版本以「request 數」估算 ⇒ 根本守不住：
#: Stage 2 每個 request 要把該日**所有取樣個股的 pit ＋ baseline** 一起送出，實測
#: （2026-09-30 真跑，run manifest）為 **16,326 tokens/request**（39 檔／request）
#: ⇒ 實價 $0.000686/request，是 Stage 1 換算值（$0.000253）的 **2.7×**。
#: 因此上限改以「估算 tokens × 實價」判定，並在 meta 印出預估 tokens／USD。
#: 上限重新校準（2026-09-30 實測後）：screen（20 檔／20 requests）≈$0.0069–$0.0137、
#: confirm（39–60 檔／40 requests）≈$0.0267–$0.0411 ⇒ 舊值 $0.02 **低於 confirm 的真實成本** ✗
#: ⇒ 改為 $0.05（留 headroom），並在 meta 印出預估 tokens／USD 供事前核可。
COST_CAP_USD = 0.05
TOKEN_PRICE_PER_MTOK = 0.042          # 實測（run manifest 的 price_per_mtok）
TOKENS_PER_SYMBOL_REQUEST = 408       # 實測：16,326 tokens / 39 檔 ≈ 419；保守取 408（含少量固定開銷）
USD_PER_REQUEST = 0.0456 / 180.0      # 保留僅供對照（Stage 1 的 request-based 舊基準）

STAGE_DEFAULTS = {
    "screen": {"max_symbols": "20", "max_dates": "20"},
    "confirm": {"max_symbols": "60", "max_dates": "40"},
}


def _read_jsonl(path: str) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    with open(path, "r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    return rows


def _rank(symbol: str, seed: str) -> str:
    """決定性且與代號大小無關的排序鍵（避免只抽到低代號個股）。"""
    return hashlib.sha256(f"{seed}|{symbol}".encode("utf-8")).hexdigest()


def sample_symbols(rows: Sequence[Mapping[str, Any]], *, per_industry: int, max_symbols: int, seed: str) -> List[str]:
    """依 industry_id 分層抽樣：每層按 _rank 排序取前 per_industry 檔，再整體截斷。"""
    by_industry: Dict[str, List[str]] = {}
    for row in rows:
        by_industry.setdefault(str(row.get("industry_id") or "unknown"), []).append(str(row["symbol"]))
    picked: List[str] = []
    for iid in sorted(by_industry):
        uniq = sorted(set(by_industry[iid]), key=lambda s: _rank(s, seed))
        picked.extend(uniq[: max(1, per_industry)])
    picked = sorted(set(picked), key=lambda s: _rank(s, seed))
    return picked[: max_symbols] if max_symbols else picked


def sample_dates(dates: Sequence[str], max_dates: int) -> List[str]:
    """等距取樣：stride 決定性，末點保留（避免只覆蓋早期樣本）。"""
    ordered = sorted(set(dates))
    if not max_dates or len(ordered) <= max_dates:
        return ordered
    stride = len(ordered) / float(max_dates)
    out = [ordered[min(len(ordered) - 1, int(i * stride))] for i in range(max_dates)]
    if ordered[-1] not in out:
        out[-1] = ordered[-1]
    return sorted(set(out))


def _judge_question(row: Mapping[str, Any], date: str, hold_days: int, cost_pct: float) -> Dict[str, Any]:
    name = row.get("symbol_name") or row["symbol"]
    return {
        "type": "noul",
        "instructions": (
            f"Stock: {name} ({row['symbol']}), Taiwan equities, industry {row.get('industry_id')}. "
            f"Judge the NEXT {hold_days} trading sessions from the close of {date}: will this stock's "
            f"return exceed the {cost_pct}% round-trip transaction cost? Answer with a probability "
            "near 0.5 if the evidence in the state is weak."
        ),
        "criteria": {
            "true": f"the stock rose by more than {cost_pct}% over the next {hold_days} trading sessions",
            "false": f"the stock rose by {cost_pct}% or less (including any fall) over those sessions",
        },
    }


def _leakage_question(row: Mapping[str, Any], date: str, hold_days: int, cost_pct: float) -> Dict[str, Any]:
    start = (row.get("backward") or {}).get("backward_date") or ""
    name = row.get("symbol_name") or row["symbol"]
    return {
        "type": "noul",
        "instructions": (
            f"Stock: {name} ({row['symbol']}), Taiwan equities. No market data and no event details are "
            f"provided in this state. From your own knowledge only: over the {hold_days} trading sessions "
            f"ENDING at the close of {date} (from the close of {start} to the close of {date}), was this "
            f"stock's return > 0 after subtracting the {cost_pct}% round-trip cost? If you do not know the "
            "historical outcome, answer with a probability near 0.5 instead of guessing."
        ),
        "criteria": {
            "true": f"the stock rose by more than {cost_pct}% over those {hold_days} trading sessions",
            "false": f"the stock rose by {cost_pct}% or less (including any fall) over those sessions",
        },
    }


def _state(date: str, rows: Sequence[Mapping[str, Any]], mode: str, hold_days: int, cost_rate: float) -> Dict[str, Any]:
    base = {
        "as_of": date,
        "holding_sessions": hold_days,
        "round_trip_cost_pct": round(cost_rate * 100, 3),
        "symbols": [dict(r) for r in rows],
    }
    if mode == "leakage":
        base["task"] = "historical recall check (no market data and no event details are provided for this state)"
    else:
        base["task"] = (
            f"Taiwan equities: judge the next {hold_days} trading sessions for each listed symbol from the "
            "close of the as-of date."
        )
    return base


def _state_row(row: Mapping[str, Any], mode: str) -> Dict[str, Any]:
    """模型看得到的欄位（絕不含 forward/backward 的標籤）。"""
    out = {
        "symbol": row["symbol"],
        "symbol_name": row.get("symbol_name", ""),
        "industry_id": row.get("industry_id", ""),
    }
    pit = row.get("pit") or {}
    if mode == "judge":
        out["pit"] = dict(pit)
    return out


class StockLayerSpec(TaskSpec):
    name = "stock_layer"
    version = "1.0.0"
    description = (
        "Stage 2 (symbol / capital-flow layer): does a Jev judgment carry information about the NEXT "
        "hold_days of individual Taiwan stocks, and does it add value over the platform signal?"
    )
    defaults = {
        "stage": "confirm",
        "mode": "judge",
        "probe_step": "5",
        "symbols_per_industry": "2",
        "max_symbols": "60",
        "max_dates": "40",
        "seed": "20260930",
        "baselines": "momentum_20d,net_buy_3d",
        "cost_rate": "0.00585",
        "hold_days": "5",
    }

    def build(self, args: Mapping[str, Any], *, user_args: Optional[Mapping[str, Any]] = None) -> SpecBuild:
        cfg = self.merged_args(args, self.defaults)
        # 使用者真正給的鍵（CLI 會先把 merged 結果餵進來 ⇒ 必須靠 user_args 才分辨得出來）
        supplied = user_args if user_args is not None else args
        panel_path = str(cfg.get("panel") or "")
        if not panel_path:
            raise ValueError("stock_layer spec requires --spec-arg panel=<sympanel.jsonl>")
        mode = str(cfg.get("mode") or "judge").lower()
        if mode not in ("judge", "leakage"):
            raise ValueError(f"unknown mode {mode!r} (expected judge|leakage)")
        stage = str(cfg.get("stage") or "confirm").lower()
        # 優先序必須是：**顯式 CLI 參數 > stage 預設 > self.defaults**。
        #
        # 修前 bug：先 `merged_args(args, defaults)` 會用 self.defaults 把 max_symbols/max_dates
        # 填好，之後的 `cfg.setdefault(stage_defaults)` 因此**永不生效** ⇒ `stage=screen` 靜默
        # 變成 confirm 形狀（實測 dates=40/symbols=39，意圖是 20×20 ✗）。
        # 修法：以「呼叫者是否真的給了」為判準（key 存在且值非 None ⇒ 使用者給的），
        # 沒給才讓 stage 預設**覆寫** self.defaults 帶進來的值。
        stage_defaults = STAGE_DEFAULTS.get(stage)
        if stage_defaults:
            for k, v in stage_defaults.items():
                user_gave = k in supplied and supplied[k] is not None
                if not user_gave:
                    cfg[k] = v

        rows = _read_jsonl(panel_path)
        if not rows:
            raise ValueError(f"panel {panel_path} has no rows")
        per_industry = max(1, int(cfg.get("symbols_per_industry") or 2))
        max_symbols = int(cfg.get("max_symbols") or 60)
        max_dates = int(cfg.get("max_dates") or 40)
        seed = str(cfg.get("seed") or "20260930")
        probe_step = max(1, int(cfg.get("probe_step") or 5))
        baselines = [b.strip() for b in str(cfg.get("baselines") or "").split(",") if b.strip()]
        cost_rate = float(rows[0].get("cost_rate", cfg.get("cost_rate") or 0.00585))
        hold_days = int(rows[0].get("hold_days", cfg.get("hold_days") or 5))

        symbols = set(sample_symbols(rows, per_industry=per_industry, max_symbols=max_symbols, seed=seed))
        by_date: Dict[str, List[Mapping[str, Any]]] = {}
        for row in rows:
            if str(row["symbol"]) in symbols:
                by_date.setdefault(str(row["date"]), []).append(row)
        dates = sample_dates(sorted(by_date), max_dates)
        if mode == "leakage":
            dates = dates[::probe_step]
        if not dates:
            raise ValueError(f"panel {panel_path} yielded no sampled dates")

        requests: List[Request] = []
        cases: List[Case] = []
        for date in dates:
            day_rows = sorted(by_date.get(date, []), key=lambda r: str(r["symbol"]))
            questions: Dict[str, Any] = {}
            state_rows: List[Dict[str, Any]] = []
            day_cases: List[Case] = []
            for row in day_rows:
                sym = str(row["symbol"])
                if mode == "judge":
                    questions[sym] = _judge_question(row, date, hold_days, cost_rate * 100)
                    gt = bool((row.get("forward") or {}).get("hit"))
                    base_vals = {
                        b: (row.get("baselines") or {}).get(b) for b in baselines if (row.get("baselines") or {}).get(b) is not None
                    }
                else:
                    questions[sym] = _leakage_question(row, date, hold_days, cost_rate * 100)
                    gt = bool((row.get("backward") or {}).get("backward_hit"))
                    base_vals = {}
                state_rows.append(_state_row(row, mode))
                day_cases.append(
                    Case(
                        case_id=f"{date}:{sym}",
                        request_id=f"{mode}:{date}",
                        group_id=date,
                        gt=gt,
                        baselines=base_vals,
                        meta={
                            "question_id": sym,
                            "symbol": sym,
                            "industry_id": row.get("industry_id", ""),
                            "date": date,
                            "mode": mode,
                            "pit": row.get("pit", {}),
                            "forward": row.get("forward", {}),
                            "backward": row.get("backward", {}),
                        },
                    )
                )
            if not day_cases:
                continue
            requests.append(
                Request(
                    request_id=f"{mode}:{date}",
                    state=_state(date, state_rows, mode, hold_days, cost_rate),
                    questions=questions,
                    case_ids=[c.case_id for c in day_cases],
                )
            )
            cases.extend(day_cases)

        # token-aware 估算：requests × 每 request 檔數 × 每檔 tokens
        per_request_symbols = (len(cases) / len(requests)) if requests else 0.0
        est_tokens = int(round(len(requests) * per_request_symbols * TOKENS_PER_SYMBOL_REQUEST))
        est_cost = round(est_tokens / 1_000_000 * TOKEN_PRICE_PER_MTOK, 6)
        if est_cost > COST_CAP_USD:
            raise ValueError(
                f"estimated cost ${est_cost} (≈{est_tokens} tokens) exceeds COST_CAP_USD=${COST_CAP_USD} — "
                f"narrow the sampling (stage=screen, lower max_dates/max_symbols) or accept a higher cap explicitly"
            )

        meta = {
            "mode": mode,
            "stage": stage,
            "panel": os.path.abspath(panel_path),
            "panel_rows": len(rows),
            "symbols_sampled": len(symbols),
            "dates": len(dates),
            "date_start": dates[0],
            "date_end": dates[-1],
            "requests": len(requests),
            "cases": len(cases),
            "estimated_cost_usd": est_cost,
            "estimated_tokens": est_tokens,
            "cost_cap_usd": COST_CAP_USD,
            "token_price_per_mtok": TOKEN_PRICE_PER_MTOK,
            "tokens_per_symbol_request": TOKENS_PER_SYMBOL_REQUEST,
            # 舊基準僅供對照（request-based；Stage 2 低估 2.7× ⇒ 不作為判準）
            "usd_per_request_stage1_baseline": round(USD_PER_REQUEST, 8),
            "seed": seed,
            "probe_step": probe_step if mode == "leakage" else None,
            "baselines": baselines,
            "hold_days": hold_days,
            "cost_rate": cost_rate,
        }
        return SpecBuild(requests=requests, cases=cases, meta=meta)
