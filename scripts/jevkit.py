#!/usr/bin/env python3
# jev-docs: https://docs.typesafe.ai/primitives  https://docs.typesafe.ai/noul  https://docs.typesafe.ai/score  https://docs.typesafe.ai/choice
#   ⚠️ 官方未涵蓋（我們的工程實作）：User-Agent 強制（Cloudflare 403）、WAF 降級、
#       backoff 的 base/jitter/上限、state 截斷（官方只要求 429/529 指數退避 + honor Retry-After）
#   ⚠️ 已知缺口：noul() 尚未支援官方 `criteria:{true,false}`（見 docs/jev/JEV-USAGE-MAP.md）
"""jevkit — 本生態唯一被認可的 Jev (TypeSafe System One) 呼叫方式。

為什麼要有這支：2026-09 的實測反覆踩到同一批坑（詳見 docs/jev/JEV-USAGE-CONTRACT.md）。
把「正確用法」寫成程式庫，呼叫端就不必記得規則：

  1. 必須顯式設定 User-Agent —— TypeSafe 對 Python-urllib 預設 UA 回 403。
  2. `choice` 的 criteria 必須是「字典」{選項: 描述}；傳陣列會 422。
  3. 一次請求多題（fan-out）：官方 13 題一批比逐題便宜 12.2×、快 10×。
  4. 429/529 用指數退避並 honor Retry-After header（官方）；403 走 WAF 等義替換降級重送（實測工程政策）。
  4b. 預設釘住版本 ID（PINNED_MODEL），不用 jev-latest alias；JEV_MODEL 環境變數可覆寫（官方 models.md）。
  5. state 有上限（state + 最長問題 ≤ 32k tokens、整體 ≤ 64k）→ 這裡做保守的字元檢查。
  6. 只算 input token（output 免費）；題目愈多愈划算，state 重複送出則浪費。
  7. 門檻不得照抄 cookbook，必須用**自己的資料**校準（本檔提供 calibrate_threshold）。
  8. 失敗一律 fail-open（呼叫端不得因 Jev 故障而中斷）→ 提供 ask_or_none()。

用法：
    from jevkit import noul, score, choice, ask, ask_or_none, calibrate_threshold
    res = ask("訂單內容…", {"urgent": noul("是否急件?"),
                            "route": choice("該給誰處理?", {"sales": "業務", "support": "客服", "none": "都不適合"})})
    print(res.answers["route"]["choice"], res.tokens, res.latency_ms)

CLI:
    python3 scripts/jevkit.py --selftest
    python3 scripts/jevkit.py --calibrate pairs.json     # [{"score":0.2,"positive":true}, ...]
"""
from __future__ import annotations

import argparse
import json
import os
import random
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Dict, List, Mapping, MutableMapping, Optional, Sequence, Tuple

API_URL = "https://api.typesafe.ai/v1/systemone"

# ★ A7：已校準門檻的 caller 必須釘住版本 ID，不要用 alias。
#   官方（models.md）："If you have tuned confidence thresholds against a specific version,
#   pin that version's ID instead of the alias" —— alias 會隨官方更新漂移，讓已校準的門檻失效。
#   實測證據（2026-09-23，MacBook）：GET /v1/models 只列 alias（jev-latest / jev-preview,
#   release_date 2026-09-10）；以 model="jev-latest" 實際 POST /v1/systemone，
#   回應 model 欄位 = "jev-1.13.0"。升級版本 → 改 PINNED_MODEL + 重跑門檻校準（走排程）。
PINNED_MODEL = "jev-1.13.0"          # 版本 ID（非 alias）；查證方式見上方註解
DEFAULT_MODEL = os.environ.get("JEV_MODEL", PINNED_MODEL)  # 集中設定：JEV_MODEL 可覆寫（staging/實驗）
UA = "a2a-jevkit/1.0 (+a2a-dev; see docs/jev/JEV-USAGE-CONTRACT.md)"
KEY_PATH = os.path.expanduser("~/.config/typesafe/api_key")

# 保守上限（純文字；CJK 約 1 token ≈ 1-1.5 字元）。超過就截斷 state 並警告。
MAX_STATE_CHARS = 40_000
MAX_TOTAL_CHARS = 70_000


class JevError(RuntimeError):
    """Jev 呼叫失敗（HTTP 或傳輸）。呼叫端若不想處理，請用 ask_or_none()。"""


@dataclass
class JevResult:
    answers: Dict[str, Any]
    tokens: int = 0
    latency_ms: int = 0
    model: str = ""
    attempts: int = 0
    raw: Dict[str, Any] = field(default_factory=dict)


# ---------------------------------------------------------------- question builders
def noul(instructions: str, criteria: Optional[Mapping[str, str]] = None) -> Dict[str, Any]:
    """是非機率題（回 answers[id]['noul']）。

    criteria 可選：官方 noul.md 建議邊界微妙時給 {true: 描述, false: 描述}
    讓 yes/no 邊界明確（「Make the boundary between yes and no unambiguous」）。
    傳入時必須是含 'true'/'false' 鍵的 dict，會原樣送給 API。
    """
    q: Dict[str, Any] = {"type": "noul", "instructions": instructions}
    if criteria is not None:
        if not isinstance(criteria, Mapping) or not {"true", "false"} <= set(criteria):
            raise TypeError("noul criteria 必須是含 'true'/'false' 鍵的 dict {true: 描述, false: 描述}")
        q["criteria"] = dict(criteria)
    return q


def score(instructions: str, criteria: Sequence[str]) -> Dict[str, Any]:
    """有序等級題。criteria 為**有序**等級清單（低→高）；每級要能獨立成立。"""
    return {"type": "score", "instructions": instructions, "criteria": list(criteria)}


def choice(instructions: str, criteria: Mapping[str, str]) -> Dict[str, Any]:
    """多選一題。criteria **必須是 dict** {選項名: 描述}（傳 list 會 422）。
    建議永遠帶一個 `none` / `uncertain` 選項，讓模型能拒答而不是硬選。"""
    if isinstance(criteria, (list, tuple)):
        raise TypeError(
            "choice.criteria 必須是 dict {選項: 描述}；TypeSafe 對陣列回 422 `Input should be a valid dictionary`。"
            "若你只有一串標籤，請改傳 {標籤: 描述} 或改用 score()。")
    return {"type": "choice", "instructions": instructions, "criteria": dict(criteria)}

def fresh_uid(tag: str = "") -> str:
    """回傳 throwaway uid（官方 self-consistency cookbook 慣例：每次重複呼叫帶不同 uid 避開伺服器快取）。

    官方做法是把 uid 放進 state 當額外欄位（"as an extra field in the TypeSafe state"），
    state 本體（post + rubric）保持不變。本函數只產生唯一值，呼叫端自行併入 state：
        state = {"uid": fresh_uid("s2"), "task": ...}
    """
    core = f"{time.time_ns():x}-{random.getrandbits(32):08x}"
    return f"{tag}:{core}" if tag else core


# ★ Cloudflare WAF（實測 2026-09-23，二分定位）：state 含 `curl -s` 的請求會被 403。
#   觸發需要字串出現在足夠大的 payload 中（短文字不觸發）；含 `-H "Authorization: Bearer $TOKEN"`
#   這類真實指令的 SOP/程式碼片段最容易命中。
#   策略：先送原內容；若 403 才以「等義替換」重送（保留語意、不犧牲正確性）。
_WAF_REWRITES = (
    ("curl -s", "fetch"),
    ("curl -S", "fetch"),
    ("Authorization: Bearer", "Auth-token"),
)


def _sanitize_for_waf(value: Any) -> Any:
    """遞迴替換會被 Cloudflare WAF 擋下的字串（僅在 403 後使用）。"""
    if isinstance(value, str):
        out = value
        for a, b in _WAF_REWRITES:
            out = out.replace(a, b)
        return out
    if isinstance(value, dict):
        return {k: _sanitize_for_waf(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_sanitize_for_waf(v) for v in value]
    return value


def validate_questions(questions: Mapping[str, Any]) -> None:
    for qid, q in questions.items():
        if not isinstance(q, dict) or "type" not in q:
            raise ValueError(f"question '{qid}' 缺少 type")
        if q["type"] == "choice" and not isinstance(q.get("criteria"), dict):
            raise TypeError(f"question '{qid}': choice.criteria 必須是 dict")
        if q["type"] == "score" and not isinstance(q.get("criteria"), (list, tuple)):
            raise TypeError(f"question '{qid}': score.criteria 必須是有序清單")
        if q["type"] == "noul" and "criteria" in q and not isinstance(q["criteria"], Mapping):
            raise TypeError(f"question '{qid}': noul.criteria 必須是 dict {{true: 描述, false: 描述}}")
        if not str(q.get("instructions", "")).strip():
            raise ValueError(f"question '{qid}' 缺少 instructions（判斷條件要寫在這裡，不是題目 id）")


def estimate_chars(state: Any, questions: Mapping[str, Any]) -> int:
    return len(json.dumps(state, ensure_ascii=False)) + len(json.dumps(questions, ensure_ascii=False))


# ---------------------------------------------------------------- transport
def _api_key(explicit: Optional[str] = None) -> str:
    if explicit:
        return explicit
    env = os.environ.get("TYPESAFE_API_KEY", "").strip()
    if env:
        return env
    try:
        return open(KEY_PATH).read().strip()
    except OSError:
        return ""


def _truncate_state(state: Any, budget: int) -> Any:
    """把過大的 state 截斷到保守預算內（字串截尾；dict/list 遞迴處理字串值）。"""
    if isinstance(state, str):
        return state[:budget]
    if isinstance(state, dict):
        out = {}
        for k, v in state.items():
            out[k] = _truncate_state(v, max(200, budget // max(1, len(state))))
        return out
    if isinstance(state, list):
        return [_truncate_state(v, max(200, budget // max(1, len(state)))) for v in state]
    return state


# ★ A2：429/529 退避（官方 api.md / models.md）。官方要求「exponential backoff」+「honor the
#   retry-after header」；下列 base/jitter/上限為工程參數，官方未涵蓋。
BACKOFF_BASE_S = 1.0     # 未涵蓋：第一輪退避基數（秒）
BACKOFF_MAX_S = 30.0     # 未涵蓋：防止異常 Retry-After 值卡死呼叫端


def _retry_after_seconds(exc: urllib.error.HTTPError) -> Optional[float]:
    """讀 Retry-After header（秒）。回 None = 無 header 或非秒數格式（HTTP-date 未涵蓋）。"""
    headers = getattr(exc, "headers", None) or {}
    raw = (headers.get("Retry-After") or "").strip()
    if not raw:
        return None
    try:
        return max(0.0, min(BACKOFF_MAX_S, float(raw)))
    except ValueError:
        return None


def _backoff_seconds(attempt: int) -> float:
    """指數退避 base*2^(n-1) + jitter（equal jitter：一半固定 + 一半隨機）。

    指數退避本身為官方要求；base/jitter/上限均屬工程政策（官方未涵蓋）。
    attempt 為 1-indexed（第 1 次重試 → base、第 2 次 → 2*base、第 3 次 → 4*base…）。
    """
    delay = BACKOFF_BASE_S * (2 ** (attempt - 1))
    return min(BACKOFF_MAX_S, delay / 2 + random.uniform(0, delay / 2))


def ask(
    state: Any,
    questions: Mapping[str, Any],
    *,
    model: str = DEFAULT_MODEL,
    timeout: float = 15.0,
    retries: int = 2,
    api_key: Optional[str] = None,
    strict_size: bool = True,
) -> JevResult:
    """送出一次（可含多題）Jev 請求。失敗丟 JevError；不想處理請用 ask_or_none()。"""
    validate_questions(questions)
    key = _api_key(api_key)
    if not key:
        raise JevError("找不到 Jev API key（~/.config/typesafe/api_key 或 TYPESAFE_API_KEY）")

    if strict_size:
        total = estimate_chars(state, questions)
        if total > MAX_TOTAL_CHARS:
            state = _truncate_state(state, MAX_STATE_CHARS)
            if estimate_chars(state, questions) > MAX_TOTAL_CHARS:
                raise JevError(
                    f"state+questions 過大（{total} 字元 > {MAX_TOTAL_CHARS}）。請先裁切 state 或分批。"
                    "（Jev 上限：state + 最長問題 ≤ 32k tokens、整體 ≤ 64k）")

    body = json.dumps({"state": state, "model": model, "questions": questions}, ensure_ascii=False).encode("utf-8")
    last_exc: Optional[Exception] = None
    t0 = time.time()
    for attempt in range(1, retries + 2):
        req = urllib.request.Request(
            API_URL, data=body, method="POST",
            headers={
                "Authorization": f"Bearer {key}",
                "Content-Type": "application/json",
                # ★ 不設 UA 會被 Cloudflare 以 403 擋掉（Python-urllib 預設 UA）
                "User-Agent": UA,
            })
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                payload = json.loads(resp.read().decode("utf-8"))
            usage = payload.get("usage", {}) or {}
            return JevResult(answers=payload.get("answers", {}),
                             tokens=int(usage.get("input_tokens", 0)),
                             latency_ms=int((time.time() - t0) * 1000),
                             model=payload.get("model", ""), attempts=attempt, raw=payload)
        except urllib.error.HTTPError as exc:
            last_exc = exc
            if exc.code == 403 and attempt <= retries:
                # WAF 內容規則（實測：state 含 `curl -s`）→ 以等義替換重送，保留語意
                sanitized = _sanitize_for_waf(state)
                if sanitized != state:
                    state = sanitized
                    body = json.dumps({"state": state, "model": model, "questions": questions},
                                      ensure_ascii=False).encode("utf-8")
                    time.sleep(0.5)
                    continue
                time.sleep(1.5 * attempt)
                continue
            if exc.code in (429, 529) and attempt <= retries:
                # 官方（api.md / models.md）：429/529 用 exponential backoff；
                # 回應帶 Retry-After header 時優先採用。jitter/base/上限是工程參數（未涵蓋）。
                ra = _retry_after_seconds(exc)
                time.sleep(ra if ra is not None else _backoff_seconds(attempt))
                continue
            raise JevError(f"HTTP {exc.code}: {exc.reason}") from exc
        except Exception as exc:                    # noqa: BLE001
            last_exc = exc
            if attempt <= retries:
                time.sleep(1.0 * attempt)
                continue
            raise JevError(f"{type(exc).__name__}: {exc}") from exc
    raise JevError(str(last_exc))


def ask_or_none(state: Any, questions: Mapping[str, Any], **kw: Any) -> Optional[JevResult]:
    """Fail-open 版本：任何錯誤回 None（呼叫端不得因 Jev 故障而中斷）。"""
    try:
        return ask(state, questions, **kw)
    except Exception:  # noqa: BLE001
        return None


# ---------------------------------------------------------------- threshold calibration
def calibrate_threshold(
    pairs: Sequence[Tuple[float, bool]],
    *,
    min_precision: float = 0.95,
    min_recall: float = 0.0,
) -> Dict[str, Any]:
    """用**自己的資料**挑門檻（codify：「不要照抄 cookbook 預設值」）。

    pairs: [(分數或機率, 是否為正例)]。回傳可達 min_precision 的最低門檻（召回最大者）。
    """
    if not pairs:
        raise ValueError("pairs 不可為空")
    grid = sorted({round(s, 3) for s, _ in pairs})
    best: Optional[Dict[str, Any]] = None
    for th in grid:
        tp = sum(1 for s, p in pairs if s >= th and p)
        fp = sum(1 for s, p in pairs if s >= th and not p)
        fn = sum(1 for s, p in pairs if s < th and p)
        prec = tp / (tp + fp) if (tp + fp) else 1.0
        rec = tp / (tp + fn) if (tp + fn) else 0.0
        if prec < min_precision or rec < min_recall:
            continue
        cand = {"threshold": th, "precision": round(prec, 4), "recall": round(rec, 4),
                "tp": tp, "fp": fp, "fn": fn, "n": len(pairs)}
        if best is None or cand["recall"] > best["recall"]:
            best = cand
    if best is None:
        # 沒有任何門檻達標 → 回報最保守可行值，讓呼叫端自己決定
        tp = sum(1 for s, p in pairs if p)
        return {"threshold": None, "reason": "無門檻同時滿足條件",
                "n": len(pairs), "n_positive": tp,
                "suggest": "放寬 min_precision，或用更多資料重校準"}
    return best


def _selftest() -> int:
    print("jevkit self-test")
    try:
        validate_questions({"a": choice("x", {"none": "none"})})
    except Exception as exc:
        print("  ✗ choice dict 驗證異常:", exc)
        return 1
    try:
        validation_caught = False
        try:
            # 下面的 list criteria 是**故意**的：用來驗證 validate_questions 會擋下 422 陷阱
            validate_questions({"a": {"type": "choice", "instructions": "x", "criteria": ["a", "b"]}})  # jev-contract-allow C4
        except TypeError:
            validation_caught = True
        if not validation_caught:
            print("  ✗ 未擋下 choice criteria=list（422 陷阱）")
            return 1
    except Exception as exc:
        print("  ✗ 驗證測試異常:", exc)
        return 1
    print("  ✓ 題型驗證（choice 必須 dict）")
    cal = calibrate_threshold([(0.9, True), (0.8, True), (0.7, True), (0.2, False), (0.1, False)])
    print(f"  ✓ 門檻校準: {cal}")
    if not _api_key():
        print("  [SKIP] 無 API key → 略過實際呼叫")
        return 0
    res = ask_or_none("這是一段測試文字。", {"q": noul("這是測試嗎？")})
    if res is None:
        print("  ✗ 實際呼叫失敗（檢查 key / 網路 / UA）")
        return 1
    print(f"  ✓ 實際呼叫 OK: noul={res.answers['q']['noul']:.2f} model={res.model} "
          f"tokens={res.tokens} {res.latency_ms}ms attempts={res.attempts}")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description="jevkit — Jev 唯一認可呼叫方式")
    ap.add_argument("--selftest", action="store_true")
    ap.add_argument("--calibrate", metavar="PAIRS_JSON",
                    help='[{"score": 0.2, "positive": true}, ...]')
    ap.add_argument("--min-precision", type=float, default=0.95)
    args = ap.parse_args()
    if args.calibrate:
        data = json.load(open(args.calibrate))
        pairs = [(float(d["score"]), bool(d["positive"])) for d in data]
        print(json.dumps(calibrate_threshold(pairs, min_precision=args.min_precision),
                         ensure_ascii=False, indent=1))
        return 0
    return _selftest()


if __name__ == "__main__":
    sys.exit(main())
