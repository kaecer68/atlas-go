#!/usr/bin/env python3
# scripts/ops/verify-universe-run.py — 母體(universe)執行的「真值盤查 + 判層」工具
# 薄殼入口見 scripts/ops/verify-universe-run.sh（本檔才是實作；只用 Python 標準庫）。
#
# ── 它不取代告警 ───────────────────────────────────────────────────────────────
# 本工具**不發告警、不改任何狀態、不寫任何檔案**（除了 stdout/stderr）。它的唯一目的，
# 是把「這次母體到底跑到哪一層、壞在哪一層」變成**可重跑、可稽核**的一件事：
#   · 生產上的告警（monitoring/rules/atlas_universe_scoring_alerts.yml 的 10 條）是
#     **持續性**的守門；但「今天的 06:00Z 這一輪到底跑了沒有、產物有沒有落地」這種
#     **一次性驗收問題**，需要的是「現在跑一次、印出每一層的讀數與判定」，而不是等 30 分鐘
#     pending 的規則。本工具就是那個一次性驗收的執行體。
#   · 因此：**本工具與告警的關係是「同一組真值、兩個消費者」**，不是取代關係。
#     每一條判定都會印出它在 Prometheus 上的對應規則名，讓人工可以互核 /api/v1/alerts。
#
# ── 為什麼要有它（2026-09-27 的缺口盤查）────────────────────────────────────────
# 2026-09-25 的事故（`universe-scoring-gap-20260925`）教了兩件事：
#   (1) **counter 會說謊**：`increase(atlas_universe_symbols_screened_total[7d])` 讀到
#       4906，但那其實是 legacy 錯標籤系列 `{daily="failed"}` 的殘留；正確形狀
#       `{stage="daily"|"weekly"}` 的系列全是 0。⇒ 任何讀 counter 的判定都**必須附
#       「標籤形狀檢查」**（本工具的第 L4 層第一件事就是這個）。
#   (2) **「跑了」與「產出可用」與「產物落地」是三件事**，而當時的訊號把三者混為一談。
# 2026-09-27 的修法把真值移到「輸出」（verdict 族 + 心跳，見
# internal/monitoring/metrics/universe_run.go 與 universe_run_verdict.go）。本工具進一步
# 把那個原則寫成**真值階層 + 判層判定式**（見 docs/operations/universe-run-truth-model.md）：
# 衝突時信誰、每一層的代表性失敗模式、以及每一層「答不出什麼」。
#
# ── 判定式（每一層都可稽核；實作在同名 _layer_* 函式）──────────────────────────
#   L0 傳輸面 transport : /metrics 拿不拿得到、atlas_universe_last_run_* 族在不在、
#                          Prometheus /api/v1/rules 有沒有載入本檔的規則群。
#   L1 排程面 schedule  : 「最後一次**應執行**的時刻」（台灣交易日曆 + 06:00Z）之後，
#                          有沒有任何 stage 完成過？心跳有沒有逾時？
#   L2 資料面 input     : snapshot.result.quotes_status 與 quotes_missing_*（報價品質）。
#   L3 評分面 scoring   : snapshot.result 的 symbols_built / filtered / ranked /
#                          ranked_trustworthy（空母體 / 篩選全滅 / 量測失敗 / 評分空轉）。
#   L4 發射面 emission  : ① counter 的**標籤形狀**是否合法（legacy `{daily=...}` ⇒ 假讀數）
#                          ② 輸出說 ranked>0 且新鮮，但 counter 6 天沒有增量（記帳面斷線）。
#   L5 產物面 artifact  : snapshot 這個**檔案本身**是不是這一輪的（mtime vs 宣稱的完成時刻、
#                          以及 vs 最後一次應執行時刻）。← 這是 2026-09-27 才被補上的缺口：
#                          「跑了、有產出、但產物沒落地」在舊的 9 條規則下**完全無人覆蓋**。
#
# ── exit code（給自動化用）──────────────────────────────────────────────────────
#   0 = 全綠（可含 WARN；WARN 是「這個維度還沒上線」，不是「母體壞了」）
#   1 = 至少一層出現「應告警」條件（RED）—— 與告警規則同一組判定式
#   2 = **無法判定**（證據不足：端點讀不到、交易日曆不可用、artifact schema 太舊…）。
#       這是刻意與 1 分開的：把「我不知道」報成「壞了」會浪費值班，報成「綠」會掩蓋缺口。
#   3 = 用法 / 設定錯誤
#
# ── 已知限制（誠實列出；不要誤以為它會講）──────────────────────────────────────
#   (a) 交易日曆是**從 Go 原始碼解析**的（internal/taiwanholidays/taiwan_holidays.go 的
#       time.Date 字面值 + fixedHolidays 表）。解析不到任何日期時，L1 一律回 UNKNOWN
#       （絕不猜）——與 NextUniverseRun 讀不到日曆就寧可不出心跳同一個原則。
#       被解析年份之外的日期同樣視為不可判定。
#   (b) 心跳（next_run）在「觸發時刻之後才重啟」的情況下會被重新發佈到下一輪 ⇒
#       AtlasUniverseRunOverdue **不會** firing。本工具會把這個形狀明確印出來
#       （L1 的 note：`心跳不會響`），因為它是現行規則的結構性盲點，而不是「沒事」。
#   (c) `daily_skip_non_trading` / `daily_skip_monday` 是 logging.Debug ⇒ 容器是 INFO
#       等級時**看不見**。因此 L1 不把「日誌沒有 *_start」當成唯一證據，必須有日曆佐證。
#   (d) L4 的 counter 增量需要 Prometheus 的 query API；讀不到時該層回 UNKNOWN。
#   (e) 本工具讀的是**本機可見的**檔案與端點。若它跑在容器外（生產主機的 repo checkout），
#       `--workdir` 必須指向 compose 綁定的宿主目錄（`./data:/app/data` ⇒ <repo>/data）。
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field

TOOL_VERSION = "1.0.0"

# ── 常數：真值階層的參數（改動前先想清楚它屬於哪一層）──────────────────────────

#: 排程觸發時刻 = 14:00 Asia/Taipei = 06:00 UTC（見 universe_scheduler.go 檔頭）。
TRIGGER_HOUR_UTC = 6
#: 心跳逾時的餘裕，與 AtlasUniverseRunOverdue 的 2h **必須一致**（同一組判定式）。
HEARTBEAT_OVERDUE_SECONDS = 2 * 3600
#: 「這一輪已經預告要跑、但還沒跑完」的寬限：一輪執行以分鐘計（2026-09-25 生產 ≈2 分鐘）。
DEFAULT_GRACE_SECONDS = 2 * 3600
#: verdict 的 finished 與「應執行時刻」比較時的容差（時鐘 / 排程對齊）。
RUN_FINISH_TOLERANCE_SECONDS = 120
#: 產物 mtime 與「宣稱的完成時刻」比較時的容差（檔案系統時間粒度）。
ARTIFACT_STALE_SECONDS = 120
#: 記帳面斷線的視窗，與 AtlasUniverseCounterEmissionMissing 的 [6d] **必須一致**。
EMISSION_WINDOW = "6d"

SNAPSHOT_REL = os.path.join("data", "state", "universe_snapshot.json")
REGISTRY_REL = os.path.join("data", "state", "universe.json")
HOLIDAY_SOURCE_REL = os.path.join("internal", "taiwanholidays", "taiwan_holidays.go")

#: 這一族 metric 的合法 label 名。**出現別的 label 名就是缺陷**（2026-06-22~09-25 的
#: 生產實例把 label 值當成 label 名，於是出現 `{daily="failed"}`，而正確形狀的單標籤
#: 系列被整個丟掉）。本工具用它做「counter 標籤形狀檢查」。
UNIVERSE_LABEL_NAMES = frozenset(
    {"stage", "result", "reason", "outcome", "instance", "job", "bucket", "kind"}
)
#: counter 的「值」如果被寫成 label 名，一定是這個缺陷 —— 這些字串是 stage/bucket 的值，
#: 不可能是 label 名。
LABEL_NAME_LOOKING_LIKE_VALUE = ("daily", "weekly", "coverage_check", "passed", "failed")

#: metric 名（= atlas_universe_* 家族的權威拼法；不要在本檔發明新的拼法）。
M_LAST_RUN_VALID = "atlas_universe_last_run_valid"
M_LAST_RUN_FINISHED = "atlas_universe_last_run_finished_timestamp_seconds"
M_LAST_RUN_GATHERED = "atlas_universe_last_run_symbols_gathered"
M_LAST_RUN_FILTERED = "atlas_universe_last_run_symbols_filtered"
M_LAST_RUN_RANKED = "atlas_universe_last_run_symbols_ranked"
M_LAST_RUN_TRUSTWORTHY = "atlas_universe_last_run_ranked_trustworthy"
M_LAST_RUN_OUTCOME = "atlas_universe_last_run_outcome"
M_LAST_RUN_SNAPSHOT_PERSISTED = "atlas_universe_last_run_snapshot_persisted"
M_NEXT_RUN = "atlas_universe_next_run_timestamp_seconds"
M_SNAPSHOT_PERSISTED_TOTAL = "atlas_universe_snapshot_persisted_total"
M_RANKED_TOTAL = "atlas_universe_symbols_ranked_total"
M_SCREENED_TOTAL = "atlas_universe_symbols_screened_total"
M_QUOTES_FETCHED_TOTAL = "atlas_universe_quotes_fetched_total"

STAGES = ("daily", "weekly")

#: 本檔規則群的 alert 名。CORE = 現行已部署的 9 條；NEW = 2026-09-27 這一輪新增的規則。
#: 新增規則時**必須**把它列在 NEW_RULES，並在部署完成後搬進 CORE_RULES
#: （部署時序：本 PR 的規則與 binary 一起上，且排在 09-29 驗收之後）。
RULE_GROUP = "atlas_universe_scoring"
CORE_RULES = (
    "AtlasUniverseRankedZero",
    "AtlasUniverseScreeningAllRejected",
    "AtlasUniverseQuotesMissing",
    "AtlasUniverseScreenedFamilyMissing",
    "AtlasUniverseEmptyFiltered",
    "AtlasUniverseMetricsFamilyMissing",
    "AtlasUniverseEmptyUniverse",
    "AtlasUniverseRunOverdue",
    "AtlasUniverseCounterEmissionMissing",
)
NEW_RULES = ("AtlasUniverseSnapshotNotPersisted",)

#: 每一層在 Prometheus 上的對應規則（互核用；不是判定來源）。
LAYER_RULES = {
    "L0": ("AtlasUniverseMetricsFamilyMissing", "AtlasUniverseScreenedFamilyMissing"),
    "L1": ("AtlasUniverseRunOverdue",),
    "L2": ("AtlasUniverseQuotesMissing",),
    "L3": (
        "AtlasUniverseEmptyUniverse",
        "AtlasUniverseEmptyFiltered",
        "AtlasUniverseRankedZero",
        "AtlasUniverseScreeningAllRejected",
    ),
    "L4": ("AtlasUniverseCounterEmissionMissing", "AtlasUniverseScreenedFamilyMissing"),
    "L5": ("AtlasUniverseSnapshotNotPersisted",),
}

#: 判定值。OK/RED 是「有結論」；UNKNOWN/PENDING 是「還沒辦法有結論」（exit 2）；
#: SKIPPED 是「日曆說不必跑」；WARN 是「這個維度還沒上線 / 有次要矛盾」。
V_OK, V_RED, V_UNKNOWN, V_PENDING, V_SKIPPED, V_WARN = (
    "OK",
    "RED",
    "UNKNOWN",
    "PENDING",
    "SKIPPED",
    "WARN",
)

EXIT_GREEN, EXIT_RED, EXIT_INDETERMINATE, EXIT_USAGE = 0, 1, 2, 3

#: 這個 set 的判定才影響 exit code（WARN 不影響：它是「維度未上線」，不是「母體壞了」）。
RED_VERDICTS = frozenset({V_RED})
INDETERMINATE_VERDICTS = frozenset({V_UNKNOWN, V_PENDING})


# ── 小工具：時間 / metric 文字解析 ─────────────────────────────────────────────


def fmt_ts(epoch: float | None) -> str:
    """unix epoch → RFC3339 UTC（可讀）。None → "n/a"。"""
    if epoch is None:
        return "n/a"
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(epoch))


def fmt_age(epoch: float | None, now: float) -> str:
    """epoch 距 now 多久（人類可讀）。"""
    if epoch is None:
        return "n/a"
    d = now - epoch
    suffix = " 前" if d >= 0 else " 後"
    d = abs(d)
    if d < 90:
        return f"{d:.0f}s{suffix}"
    if d < 5400:
        return f"{d / 60:.1f}m{suffix}"
    if d < 3 * 86400:
        return f"{d / 3600:.1f}h{suffix}"
    return f"{d / 86400:.1f}d{suffix}"


_SAMPLE_RE = re.compile(
    r"^(?P<name>[a-zA-Z_:][a-zA-Z0-9_:]*)"
    r"(?:\{(?P<labels>[^}]*)\})?"
    r"\s+(?P<value>[-+0-9.eE]+|NaN|[+-]Inf)"
    r"(?:\s+(?P<ts>[-+0-9.eE]+))?\s*$"
)
_LABEL_RE = re.compile(r'([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*"((?:[^"\\]|\\.)*)"')


@dataclass(frozen=True)
class Sample:
    """一行 /metrics 樣本。"""

    name: str
    labels: dict
    value: float

    def key(self) -> tuple:
        return (self.name, tuple(sorted(self.labels.items())))

    def line(self) -> str:
        inner = ",".join(f'{k}="{v}"' for k, v in sorted(self.labels.items()))
        return f"{self.name}{{{inner}}}" if self.labels else self.name


class MetricsText:
    """把 /metrics 的文字解析成「(metric, labels) → 值]。

    刻意不用任何第三方依賴（生產主機只保證有 python3 標準庫）。
    """

    def __init__(self, text: str):
        self.raw = text
        self.samples: list[Sample] = []
        self._by_key: dict = {}
        self._by_name: dict = {}
        for raw_line in text.splitlines():
            line = raw_line.strip()
            if not line or line.startswith("#"):
                continue
            m = _SAMPLE_RE.match(line)
            if not m:
                continue
            labels = dict(_LABEL_RE.findall(m.group("labels") or ""))
            try:
                value = float(m.group("value"))
            except ValueError:
                continue
            s = Sample(m.group("name"), labels, value)
            self.samples.append(s)
            self._by_key[s.key()] = s
            self._by_name.setdefault(s.name, []).append(s)

    def series(self, name: str) -> list:
        return self._by_name.get(name, [])

    def has(self, name: str) -> bool:
        return bool(self._by_name.get(name))

    def value(self, name: str, labels: dict | None = None) -> float | None:
        """取樣本值；labels=None 表示不帶 label。找不到回 None（**不是 0**）。"""
        if labels is None:
            labels = {}
        s = self._by_key.get((name, tuple(sorted(labels.items()))))
        return None if s is None else s.value

    def by_stage(self, name: str) -> dict:
        """取 name{stage=X} 的 {stage: value}（只認合法 stage 值）。"""
        out = {}
        for s in self.series(name):
            stage = s.labels.get("stage")
            if stage in STAGES:
                out[stage] = s.value
        return out

    def label_shape_problems(self) -> list:
        """counter 標籤形狀檢查（L4 的第一件事）。

        回傳 list of (metric_line, 問題說明)。合法的形狀是
        `{stage="daily"|"weekly"}`（+ 可選 result/reason）；出現
        `{daily="..."}`（label 值被當成 label 名）或**完全沒有 stage label**
        都是 2026-06-22~09-25 的生產缺陷形狀，讀它算出來的增量一定是假讀數。
        """
        problems = []
        for s in self.samples:
            if not s.name.startswith("atlas_universe_"):
                continue
            if not s.name.endswith("_total"):
                continue
            bad = [k for k in s.labels if k in LABEL_NAME_LOOKING_LIKE_VALUE]
            unknown = [k for k in s.labels if k not in UNIVERSE_LABEL_NAMES]
            if bad:
                problems.append((s.line(), f"label 名 {bad} 是 stage/bucket 的**值**，不是 label 名"))
            elif unknown:
                problems.append((s.line(), f"發明出來的 label 名 {unknown}"))
            elif "stage" not in s.labels:
                problems.append((s.line(), "counter 沒有 stage label（單標籤系列被丟進無標籤系列）"))
        return problems


# ── 小工具：讀取端點 / 檔案 ────────────────────────────────────────────────────


@dataclass
class Fetched:
    """一個外部讀取的結果：值 + 取得方式 + 錯誤。"""

    value: object = None
    source: str = ""
    error: str = ""

    @property
    def ok(self) -> bool:
        return self.value is not None and not self.error


def http_get(url: str, timeout: float = 8.0) -> Fetched:
    """唯讀取回一個 URL 的文字。永不拋例外（錯誤放在 .error）。"""
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "atlas-verify-universe-run"})
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8", "replace")
            return Fetched(value=body, source=f"GET {url} → HTTP {resp.status}, {len(body)} bytes")
    except urllib.error.HTTPError as e:
        return Fetched(source=f"GET {url}", error=f"HTTP {e.code} {e.reason}")
    except Exception as e:  # noqa: BLE001 — 任何網路/解析錯誤都只降級，不中斷
        return Fetched(source=f"GET {url}", error=f"{type(e).__name__}: {e}")


def http_get_json(url: str, timeout: float = 8.0) -> Fetched:
    f = http_get(url, timeout=timeout)
    if not f.ok:
        return f
    try:
        return Fetched(value=json.loads(f.value), source=f.source)
    except Exception as e:  # noqa: BLE001
        return Fetched(source=f.source, error=f"JSON 解析失敗: {e}")


def read_file(path: str) -> Fetched:
    """唯讀取檔。"""
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as fh:
            return Fetched(value=fh.read(), source=path)
    except FileNotFoundError:
        return Fetched(source=path, error="檔案不存在")
    except Exception as e:  # noqa: BLE001
        return Fetched(source=path, error=f"{type(e).__name__}: {e}")


def file_mtime(path: str) -> float | None:
    try:
        return os.stat(path).st_mtime
    except OSError:
        return None


# ── 台灣交易日曆（真值來源：內部的 Go 表；解析不到就「不可判定」，絕不猜）────────
#
# 為什麼要自己算：L1 的問題是「**最後一次應執行的時刻**之後有沒有跑過」。排程的判定式是
# `marketdata.IsTaiwanTradingDay`（→ internal/taiwanholidays），而「休市日不該跑」正是
# 這條判定式的另一半：少了它，長假一定會被誤判成「沒跑」。
#
# 為什麼用「解析 Go 原始碼」而不是硬編一份表：硬編就是第二份真相來源（P1-8 的教訓：
# 日曆表曾在三處各存一份）。這裡**只讀**那個檔的 `time.Date(...)` 字面值與
# `fixedHolidays` 表，等於把「同一份真相」用第二種語言讀一次；而且一旦檔案結構改變
# （解析不到日期）就整層降級為 UNKNOWN，不會安靜地算錯。

_MONTHS = {
    "January": 1, "February": 2, "March": 3, "April": 4, "May": 5, "June": 6,
    "July": 7, "August": 8, "September": 9, "October": 10, "November": 11, "December": 12,
}
_DATE_LITERAL_RE = re.compile(
    r"time\.Date\(\s*(\d{4})\s*,\s*(?:time\.)?([A-Za-z]+|\d{1,2})\s*,\s*(\d{1,2})\s*,"
)
_FIXED_ENTRY_RE = re.compile(r'\{\s*Name:\s*"[^"]*"\s*,\s*Month:\s*time\.([A-Za-z]+)\s*,\s*Day:\s*(\d{1,2})\s*\}')


@dataclass
class TradingCalendar:
    """從 internal/taiwanholidays 的 Go 表解析出來的休市日集合。"""

    source: str
    fixed_md: set = field(default_factory=set)      # 每年重複的 (月, 日)
    dated: dict = field(default_factory=dict)       # 年 → set((月, 日))
    years: set = field(default_factory=set)

    def is_trading_day(self, year: int, month: int, day: int, weekday: int) -> bool | None:
        """True/False = 判定；None = **不可判定**（該年不在解析到的年份內）。"""
        if year not in self.years:
            return None
        if weekday >= 5:  # 週六 / 週日
            return False
        if (month, day) in self.fixed_md:
            return False
        if (month, day) in self.dated.get(year, set()):
            return False
        return True

    def scan_expected_triggers(self, now: float, lookback_days: int = 40) -> tuple:
        """回傳 (status, triggers)：triggers = 06:00Z epoch 列表（由舊到新、全部 <= now）。

        這是排程判定式的核心：「最後一次應執行的時刻」＝ triggers[-1]。
        status = "ok"（日曆對整個回看窗都有話說）/ "unknown"（窗內有年份不在解析範圍內
        ⇒ 寧可不可判定，也不要猜「那個交易日不存在」而誤報綠燈）。
        """
        triggers = []
        unknown = False
        for back in range(lookback_days):
            tm = time.gmtime(now - back * 86400)
            trig = calendar_timegm(tm.tm_year, tm.tm_mon, tm.tm_mday, TRIGGER_HOUR_UTC)
            if trig > now:
                continue
            # tm_wday 與 time.gmtime 的慣例相同（Mon=0..Sun=6）⇒ 週末 == >=5，不可再位移。
            ok = self.is_trading_day(tm.tm_year, tm.tm_mon, tm.tm_mday, tm.tm_wday)
            if ok is None:
                unknown = True
                continue
            if ok:
                triggers.append(trig)
        triggers.sort()
        return ("unknown" if unknown else "ok"), triggers


def calendar_timegm(year: int, month: int, day: int, hour: int) -> float:
    """UTC epoch（不用 time.timezone，避免主機 TZ 影響）。"""
    import calendar as _cal

    return float(_cal.timegm((year, month, day, hour, 0, 0, 0, 0, 0)))


def load_trading_calendar(path: str) -> tuple:
    """回傳 (TradingCalendar | None, 說明文字)。"""
    f = read_file(path)
    if not f.ok:
        return None, f"交易日曆讀不到（{path}: {f.error}）⇒ L1 降級為不可判定"
    text = f.value
    dated: dict = {}
    years = set()
    for y, mon, d in _DATE_LITERAL_RE.findall(text):
        y, d = int(y), int(d)
        mon_num = _MONTHS.get(mon) or (int(mon) if mon.isdigit() else None)
        if not mon_num or not (1 <= d <= 31):
            continue
        dated.setdefault(y, set()).add((mon_num, d))
        years.add(y)
    fixed = set()
    for mon, d in _FIXED_ENTRY_RE.findall(text):
        mon_num = _MONTHS.get(mon)
        if mon_num:
            fixed.add((mon_num, int(d)))
    if not dated and not fixed:
        return None, f"交易日曆解析不到任何日期（{path}）⇒ L1 降級為不可判定"
    cal = TradingCalendar(source=path, fixed_md=fixed, dated=dated, years=years)
    return cal, (
        f"交易日曆解析自 {os.path.basename(path)}: {len(years)} 個年份"
        f"（{min(years)}-{max(years)}）、{sum(len(v) for v in dated.values())} 個年度休市日、"
        f"{len(fixed)} 個固定休市日"
    )


# ── 日誌證據（容器日誌；`daily_skip_*` 是 Debug ⇒ 可能看不見，故只當佐證）──────────

#: 與母體執行有關的事件名（見 internal/monitoring/universe_scheduler.go）。
LOG_EVENT_START = ("daily_refresh_start", "weekly_rebuild_start")
LOG_EVENT_OK = ("daily_refresh_ok", "weekly_rebuild_ok")
LOG_EVENT_FAIL = ("daily_refresh_failed", "weekly_rebuild_failed")
LOG_EVENT_SKIP = ("weekly_skip_holiday", "daily_skip_non_trading", "daily_skip_monday", "weekly_skip_not_monday")
LOG_EVENT_PERSIST_ERR = ("snapshot_save_error", "universe_registry_write_error")
LOG_EVENT_INTEREST = LOG_EVENT_START + LOG_EVENT_OK + LOG_EVENT_FAIL + LOG_EVENT_SKIP + LOG_EVENT_PERSIST_ERR

_LOG_LINE_RE = re.compile(r"time=(?P<ts>\S+).*?\bmsg=(?P<msg>\S+)")
_TS_RE = re.compile(
    r"^(?P<date>\d{4}-\d{2}-\d{2})T(?P<h>\d{2}):(?P<m>\d{2}):(?P<s>\d{2})"
    r"(?:\.(?P<frac>\d+))?(?P<tz>Z|[+-]\d{2}:?\d{2})$"
)


def parse_log_ts(text: str) -> float | None:
    """slog 的 RFC3339Nano（含 Z 或 +08:00）→ epoch。解析不出回 None。"""
    m = _TS_RE.match(text)
    if not m:
        return None
    import calendar as _cal

    frac = float("0." + m.group("frac")) if m.group("frac") else 0.0
    base = _cal.timegm(
        (
            int(m.group("date")[0:4]), int(m.group("date")[5:7]), int(m.group("date")[8:10]),
            int(m.group("h")), int(m.group("m")), int(m.group("s")), 0, 0, 0,
        )
    )
    tz = m.group("tz")
    if tz != "Z":
        sign = 1 if tz[0] == "+" else -1
        hh = int(tz[1:3])
        mm = int(tz.replace(":", "")[3:5]) if len(tz.replace(":", "")) >= 5 else 0
        base -= sign * (hh * 3600 + mm * 60)
    return float(base) + frac


@dataclass
class LogFacts:
    """容器日誌的事實集合（事件 → 時間列表）。"""

    source: str
    lines: int = 0
    parsed: int = 0
    events: dict = field(default_factory=dict)   # event → [epoch, ...]
    first_ts: float | None = None                # 日誌實際涵蓋的起點（**不是** --since 的請求值）
    last_ts: float | None = None
    error: str = ""

    def latest(self, event: str) -> float | None:
        ts = self.events.get(event)
        return max(ts) if ts else None

    def latest_of(self, events) -> float | None:
        found = [self.latest(e) for e in events]
        found = [t for t in found if t is not None]
        return max(found) if found else None

    def covers(self, instant: float, slack: float = 900.0) -> bool:
        """這份日誌**真的**涵蓋 instant 嗎？

        ⚠️ `docker logs --since 72h` 會被容器啟動時刻截斷（2026-09-27 生產實測：要求 72h、
        實得 5h21m）。因此「日誌裡沒有 daily_refresh_start」**只有在日誌覆蓋到那一刻時**
        才算否證。這正是本工具不把「沒有日誌」當成唯一證據的原因。
        """
        return self.first_ts is not None and self.first_ts <= instant + slack

    def latest_event_of(self, events) -> tuple:
        best = (None, None)
        for e in events:
            t = self.latest(e)
            if t is not None and (best[0] is None or t > best[0]):
                best = (t, e)
        return best


def parse_logs(text: str, source: str) -> LogFacts:
    facts = LogFacts(source=source)
    if not text:
        facts.error = "沒有日誌內容"
        return facts
    for line in text.splitlines():
        facts.lines += 1
        m = _LOG_LINE_RE.search(line)
        if not m:
            continue
        msg = m.group("msg")
        ts = parse_log_ts(m.group("ts"))
        if ts is None:
            continue
        facts.parsed += 1
        facts.events.setdefault(msg, []).append(ts)
        if facts.first_ts is None or ts < facts.first_ts:
            facts.first_ts = ts
        if facts.last_ts is None or ts > facts.last_ts:
            facts.last_ts = ts
    return facts


# ── 證據容器 ──────────────────────────────────────────────────────────────────


@dataclass
class Evidence:
    now: float
    workdir: str
    offline: bool
    metrics: Fetched
    metrics_text: MetricsText | None
    rules: Fetched
    alerts: Fetched
    logs: LogFacts
    snapshot_path: str
    snapshot_mtime: float | None
    snapshot: dict | None
    snapshot_error: str
    registry_path: str
    registry_mtime: float | None
    calendar: TradingCalendar | None
    calendar_note: str
    trigger_status: str
    expected_triggers: list
    increase_ranked: dict | None
    increase_note: str
    expect_run: str
    grace: float


@dataclass
class Layer:
    """一層的判定結果。predicate 是**可稽核的判定式文字**（人看的）。"""

    key: str
    name: str
    verdict: str
    predicate: str
    reads: list
    evidence: list = field(default_factory=list)
    note: str = ""

    @property
    def rules(self) -> tuple:
        return LAYER_RULES.get(self.key, ())


# ── 判定式（每一條都可以被單獨改壞 ⇒ 對應的 fixture 必須紅）─────────────────────
#
# 這六個函式是**唯一的判定點**：mutation 測試（tests/scripts/test-verify-universe-run.sh）
# 就是逐一改壞它們，證明對應的 fixture 會紅。任何新判定都應該長成這樣一個小函式。


def p_output_family_missing(metrics_ok: bool, family_present: bool) -> bool:
    """L0：服務活著、/metrics 有回應，但 last_run_* 輸出族完全不存在（訊號整族消失）。"""
    return metrics_ok and not family_present


def p_heartbeat_overdue(next_run, now, grace_seconds: float) -> bool:
    """L1：心跳（預告的下一次執行）已經逾時 —— 與 AtlasUniverseRunOverdue 同式。"""
    return next_run is not None and next_run + grace_seconds < now


def p_last_run_missing(t_exp, done_after, now, grace_seconds: float) -> bool:
    """L1：最後一次**應執行**的時刻已過（含寬限），但沒有任何 stage 完成過。"""
    return t_exp is not None and not done_after and now > t_exp + grace_seconds


def p_scoring_broken(result) -> bool:
    """L3：輸出壞掉 —— 空母體 / 篩選全滅 / 排名不可信 / 排名為空。"""
    if result is None:
        return False
    if _int_field(result, "symbols_built") == 0:
        return True
    if _int_field(result, "symbols_filtered") == 0:
        return True
    if result.get("ranked_trustworthy") is False:
        return True
    return _int_field(result, "symbols_ranked") == 0


def p_input_unusable(result) -> bool:
    """L2：報價輸入不可用（partial / empty / fetch_error / provider_unavailable / mock）。"""
    if result is None:
        return False
    status = result.get("quotes_status")
    if status is None:
        return False
    if status != "ok":
        return True
    return (_int_field(result, "quotes_missing_fetch_error") or 0) + (
        _int_field(result, "quotes_missing_not_attempted") or 0
    ) > 0


def p_label_shape_bad(problems) -> bool:
    """L4：counter 的標籤形狀壞掉（legacy `{daily=...}` / 無 stage label）⇒ 讀數必假。"""
    return bool(problems)


def p_emission_missing(ranked_positive: bool, fresh_stages, zero_increase_stages) -> bool:
    """L4：輸出說有產出且新鮮，但 counter 在某個 stage 的視窗內沒有增量（記帳面斷線）。

    刻意是**逐 stage**（與 AtlasUniverseCounterEmissionMissing 同式）：一個 stage 的
    健康讀數不該替另一個 stage 背書。
    """
    return bool(ranked_positive) and bool(fresh_stages) and bool(zero_increase_stages)


def p_artifact_stale(mtime, claim: float | None) -> bool:
    """L5：產物 mtime 明顯早於「宣稱已完成的執行時刻」。"""
    return mtime is not None and claim is not None and mtime < claim - ARTIFACT_STALE_SECONDS


def _int_field(result, name):
    """取整數欄位；不存在或型別不對回 None（**不是 0** —— 0 是有效讀數）。"""
    if not isinstance(result, dict):
        return None
    v = result.get(name)
    if isinstance(v, bool) or not isinstance(v, (int, float)):
        return None
    return int(v)


# ── L0 傳輸面 ────────────────────────────────────────────────────────────────


def layer_transport(ev: Evidence) -> Layer:
    """指標與規則有沒有載入（訊號「看不看得到」）。"""
    reads = [
        "GET /metrics（atlas-go:18080）",
        "GET /api/v1/rules（Prometheus）",
        "GET /api/v1/alerts（Prometheus）",
    ]
    ev_lines = []
    m_ok = ev.metrics_text is not None
    family = False
    ev_lines.append(f"metrics: {ev.metrics.source}{' — ' + ev.metrics.error if not ev.metrics.ok else ''}")
    if m_ok:
        family = ev.metrics_text.has(M_LAST_RUN_VALID)
        ev_lines.append(f"{M_LAST_RUN_VALID} 存在: {family}")
        up = ev.metrics_text.value("up", {"job": "atlas-go"})
        if up is not None:
            ev_lines.append(f'up{{job="atlas-go"}} = {up:g}')
    loaded = None
    if ev.rules.ok:
        loaded = set()
        group_found = False
        for g in (ev.rules.value or {}).get("data", {}).get("groups", []):
            if g.get("name") == RULE_GROUP:
                group_found = True
                for r in g.get("rules", []):
                    loaded.add(r.get("name"))
        ev_lines.append(f"規則群 {RULE_GROUP} 已載入: {group_found}")
        if loaded:
            ev_lines.append(f"該群 alert 數: {len(loaded)}")
    else:
        group_found = False
        ev_lines.append(f"rules 端點: {ev.rules.source}{' — ' + ev.rules.error if not ev.rules.ok else ''}")
    if ev.alerts.ok:
        data = (ev.alerts.value or {}).get("data", {})
        n = len(data.get("alerts", []))
        ev_lines.append(f"/api/v1/alerts: {n} 條（Prometheus）")
    else:
        ev_lines.append(f"alerts 端點: {ev.alerts.source} — {ev.alerts.error}")

    # 缺席的規則，分成「現行必須在」與「本輪新增（部署排在驗收之後）」。
    missing_core, missing_new = [], []
    if loaded is not None:
        missing_core = [r for r in CORE_RULES if r not in loaded]
        missing_new = [r for r in NEW_RULES if r not in loaded]

    if not m_ok and loaded is None:
        verdict, note = V_UNKNOWN, "metrics 與 rules 兩個端點都讀不到 ⇒ 無法判定訊號是否存在"
    elif p_output_family_missing(m_ok, family):
        verdict, note = V_RED, f"{M_LAST_RUN_VALID} 完全不存在（服務活著、但輸出族不見了）"
    elif loaded is not None and not group_found:
        verdict, note = V_RED, f"Prometheus 沒有載入規則群 {RULE_GROUP}（規則不會被評估）"
    elif missing_core:
        verdict, note = V_RED, f"現行規則缺席: {', '.join(missing_core)}"
    elif not m_ok:
        # 讀不到 /metrics 就**不能**說輸出族沒問題（缺席是無法判定，不是綠燈）。
        verdict, note = V_UNKNOWN, "metrics 端點讀不到 ⇒ 無法確認輸出族（規則面已確認）"
    elif missing_new:
        verdict, note = V_WARN, (
            f"本輪新增的規則尚未部署: {', '.join(missing_new)}"
            "（部署時序刻意排在 09-29 驗收之後 ⇒ 這是預期狀態，不是缺口）"
        )
    else:
        verdict, note = V_OK, f"metrics 活著、輸出族存在、{RULE_GROUP} 的 {len(CORE_RULES)} 條規則全部載入"
    return Layer(
        key="L0",
        name="傳輸面 transport",
        verdict=verdict,
        predicate=("metrics 可讀 AND last_run_* 族存在 AND /api/v1/rules 含規則群 "
                   f"{RULE_GROUP}（現行 {len(CORE_RULES)} 條）"),
        reads=reads,
        evidence=ev_lines,
        note=note,
    )


# ── L1 排程面 ────────────────────────────────────────────────────────────────


def last_trigger_ignoring_calendar(now: float) -> float:
    """不管日曆：<= now 的最大 06:00Z（今天，否則昨天）。"""
    tm = time.gmtime(now)
    t = calendar_timegm(tm.tm_year, tm.tm_mon, tm.tm_mday, TRIGGER_HOUR_UTC)
    if t > now:
        t = calendar_timegm(*time.gmtime(now - 86400)[:3], TRIGGER_HOUR_UTC)
    return t


def _log_after(logs: LogFacts, events, since: float | None) -> list:
    if since is None:
        return []
    out = []
    for e in events:
        for ts in logs.events.get(e, []):
            if ts >= since:
                out.append((ts, e))
    return sorted(out)


def layer_schedule(ev: Evidence) -> Layer:
    """「最後一次**應執行**的時刻」之後有沒有跑過？（沒跑 = 排程面）"""
    reads = [
        M_NEXT_RUN,
        M_LAST_RUN_FINISHED + "{stage}",
        M_LAST_RUN_VALID + "{stage}",
        "容器日誌: " + "/".join(LOG_EVENT_START + LOG_EVENT_SKIP),
        "交易日曆: internal/taiwanholidays",
    ]
    ev_lines = [ev.calendar_note]
    mt = ev.metrics_text
    hb = mt.value(M_NEXT_RUN) if mt else None
    valid = mt.by_stage(M_LAST_RUN_VALID) if mt else {}
    fin = mt.by_stage(M_LAST_RUN_FINISHED) if mt else {}
    # ⚠️ finished 在 process 啟動時會被「暖機」成啟動時刻而 valid=0 ⇒ 只有 valid==1
    #    的 stage 的 finished 才是「某一次真的跑完」的宣稱（否則這條線會說謊）。
    claims = {s: fin[s] for s, v in valid.items() if v == 1 and s in fin}
    ev_lines.append(f"心跳 {M_NEXT_RUN} = {fmt_ts(hb)}（{fmt_age(hb, ev.now)}）")
    for s in sorted(set(list(valid) + list(fin))):
        ev_lines.append(
            f"stage={s}: valid={valid.get(s, 'n/a')} finished={fmt_ts(fin.get(s))}（{fmt_age(fin.get(s), ev.now)}）"
        )
    # ⚠️ 「暖機哨兵」：valid == 0 的 stage 的 finished 是 **process 啟動時刻**（WarmUp 寫的），
    #    不是任何一次執行。它同時是本工具能取得的「這個 process 從什麼時候開始活著」的下界，
    #    而那個下界決定了「沒跑」可否被否證（見下面的 observable）。
    boot = [fin[s] for s in STAGES if s in fin and valid.get(s) == 0]
    boot_sentinel = min(boot) if boot else None
    ev_lines.append(f"日誌來源: {ev.logs.source}（{ev.logs.lines} 行 / {ev.logs.parsed} 行可解析；"
                    f"實際涵蓋 {fmt_ts(ev.logs.first_ts)} → {fmt_ts(ev.logs.last_ts)}）")
    ev_lines.append(f"process 啟動下界（valid==0 的 stage 的 finished = 暖機哨兵）= {fmt_ts(boot_sentinel)}")

    # 最後一次應執行時刻
    if ev.expect_run == "none":
        t_exp, cal_state = None, "suppressed"
        ev_lines.append("--expect-run none：操作者宣告『不預期有執行』⇒ 不採用日曆的應執行時刻")
    elif ev.expect_run in STAGES:
        t_exp, cal_state = last_trigger_ignoring_calendar(ev.now), "operator"
        ev_lines.append(f"--expect-run {ev.expect_run}：強制採用 06:00Z 為應執行時刻（不看日曆）")
    elif ev.calendar is None or ev.trigger_status == "unknown":
        t_exp, cal_state = None, "unknown"
        ev_lines.append("交易日曆不可判定 ⇒ 不猜『該不該跑』")
    else:
        t_exp = ev.expected_triggers[-1] if ev.expected_triggers else None
        cal_state = "ok" if t_exp is not None else "none"
    if t_exp is not None:
        ev_lines.append(f"最後一次**應執行**時刻 = {fmt_ts(t_exp)}（{fmt_age(t_exp, ev.now)}）")
    elif cal_state == "none":
        ev_lines.append("回看窗內沒有交易日 ⇒ 不預期有執行（長假）")

    # 有沒有跑過？（輸出 + 日誌兩條獨立證據）
    done_stages = []
    if t_exp is not None:
        done_stages = [s for s, t in claims.items() if t >= t_exp - RUN_FINISH_TOLERANCE_SECONDS]
    if ev.expect_run in STAGES and t_exp is not None:
        # 操作者指定了 stage ⇒ 要求**那個** stage 跑過（另一個 stage 的健康不能背書）。
        done_stages = [s for s in done_stages if s == ev.expect_run]
    ran_log = _log_after(ev.logs, LOG_EVENT_START + LOG_EVENT_OK, None if t_exp is None else t_exp - RUN_FINISH_TOLERANCE_SECONDS)
    skip_log = _log_after(ev.logs, LOG_EVENT_SKIP, None if t_exp is None else t_exp - 3600)
    done = bool(done_stages) or bool(ran_log)
    ev_lines.append(f"宣稱跑過的 stage（verdict，finished>=應執行時刻）: {done_stages or '無'}")
    ev_lines.append(
        "日誌 evidence: 起跑/完成 " + (f"{fmt_ts(ran_log[-1][0])} {ran_log[-1][1]}" if ran_log else "無")
        + "；跳過 " + (f"{fmt_ts(skip_log[-1][0])} {skip_log[-1][1]}" if skip_log else "無")
    )
    if ev.logs.error:
        ev_lines.append(f"（日誌不可用: {ev.logs.error}）")

    # 「沒跑」是一個**否證**（absence of evidence must be evidence of absence）。
    # 只有在「這個 process 當時已經活著」或「日誌真的涵蓋那一刻」時，缺席才算證據。
    # 否則（觸發時刻之後才重啟）那一輪是否跑過已經不可查 —— 那是 UNKNOWN，不是 RED。
    observable = (
        t_exp is not None
        and (
            ev.logs.covers(t_exp)                                  # 日誌真的涵蓋那一刻
            or (boot_sentinel is not None and boot_sentinel <= t_exp + 3600)  # 當時 process 已活著
        )
    )
    if t_exp is not None:
        ev_lines.append(f"缺席可否算證據（process/日誌涵蓋該時刻）: {observable}")
        if ev.snapshot_mtime is not None:
            ev_lines.append(f"產物 mtime = {fmt_ts(ev.snapshot_mtime)}；"
                            f"在應執行時刻之後 = {ev.snapshot_mtime >= t_exp}"
                            "（⚠️ 產物更新也可能是 CLI / 人工執行，不等於排程跑過）")

    overdue = p_heartbeat_overdue(hb, ev.now, ev.grace)
    if overdue:
        verdict = V_RED
        note = (f"心跳逾時：預告的下一次執行 {fmt_ts(hb)} 已過 {ev.grace / 3600:.0f}h ⇒ 排程沒跑"
                "（與 AtlasUniverseRunOverdue 同式）")
    elif cal_state == "unknown" and ev.expect_run == "auto":
        verdict, note = V_UNKNOWN, "交易日曆不可判定（解析不到 / 年份不在範圍內）⇒ 無法斷定『該不該跑』"
    elif done:
        verdict = V_OK
        note = "最後一次應執行時刻之後，確實有 stage 完成過"
    elif skip_log:
        verdict = V_SKIPPED
        note = f"日誌顯示 {skip_log[-1][1]} ⇒ 休市 / 非週一，本來就不該跑"
    elif ev.expect_run == "none":
        verdict, note = V_SKIPPED, "操作者宣告不預期有執行（--expect-run none）"
    elif cal_state == "none":
        verdict, note = V_SKIPPED, "回看窗內沒有交易日（長假）⇒ 沒有應該跑的時刻"
    elif p_last_run_missing(t_exp, done, ev.now, ev.grace) and observable:
        verdict = V_RED
        note = (f"最後一次應執行時刻 {fmt_ts(t_exp)} 已過寬限 {ev.grace / 3600:.0f}h，"
                "但沒有任何 stage 完成過 ⇒ 沒跑")
        if not overdue:
            note += ("。⚠️ AtlasUniverseRunOverdue **不會** firing：心跳已被『觸發時刻之後的重啟』"
                     "重新發佈到下一輪（現行規則的結構性盲點，本工具是唯一看得到的地方）")
    elif p_last_run_missing(t_exp, done, ev.now, ev.grace):
        verdict = V_UNKNOWN
        note = (f"最後一次應執行時刻 {fmt_ts(t_exp)} 已過寬限，而且沒有完成證據；"
                f"但 process 直到 {fmt_ts(boot_sentinel)} 才啟動（或日誌只涵蓋到 "
                f"{fmt_ts(ev.logs.first_ts)}）⇒ **那一輪是否跑過已不可查**（重啟會銷毀證據）。"
                "下一次預期執行時刻見心跳；不要把它當成『沒跑』，也不要當成綠燈")
    else:
        verdict = V_PENDING
        note = (f"應執行時刻 {fmt_ts(t_exp)} 剛過，仍在寬限 {ev.grace / 3600:.0f}h 內 "
                f"⇒ 還沒跑完不算壞（{fmt_ts(t_exp + ev.grace)} 之後才有結論）")
    return Layer(
        key="L1",
        name="排程面 schedule",
        verdict=verdict,
        predicate=(
            "心跳 next_run+grace < now ⇒ 沒跑；否則「<= now 的最大交易日 06:00Z」之後有任何 stage "
            "finished>=該時刻（或日誌 *_start/*_ok）⇒ 有跑；兩者皆非且已過寬限 **且缺席可被否證**"
            "（process 當時已活著 or 日誌涵蓋該時刻）⇒ 沒跑，否則不可判定"
        ),
        reads=reads,
        evidence=ev_lines,
        note=note,
    )


# ── L2 資料面 ────────────────────────────────────────────────────────────────


def layer_input(ev: Evidence) -> Layer:
    """報價輸入可用嗎？（輸入不新鮮 / 不完整 = 資料面）"""
    reads = ["snapshot.result.quotes_status", "snapshot.result.quotes_missing_*", M_LAST_RUN_OUTCOME]
    ev_lines = []
    result = (ev.snapshot or {}).get("result") if ev.snapshot else None
    if ev.snapshot is None:
        return Layer(
            key="L2",
            name="資料面 input",
            verdict=V_UNKNOWN,
            predicate="snapshot.result.quotes_status == ok AND quotes_missing_fetch_error + not_attempted == 0",
            reads=reads,
            evidence=[f"snapshot 讀不到（{ev.snapshot_error}）⇒ 沒有報價品質的證據"],
            note="產物不存在 ⇒ 這一層無法判定（L5 會把產物本身判紅）",
        )
    for k in (
        "quotes_status", "quotes_returned", "quotes_requested", "quotes_chunks",
        "quotes_chunks_failed", "quotes_missing_no_data", "quotes_missing_not_covered",
        "quotes_missing_fetch_error", "quotes_missing_not_attempted",
    ):
        if isinstance(result, dict) and k in result:
            ev_lines.append(f"result.{k} = {result[k]}")
    mt = ev.metrics_text
    if mt:
        outcome = {s: mt.value(M_LAST_RUN_OUTCOME, {"stage": s, "outcome": "quotes_unavailable"}) for s in STAGES}
        ev_lines.append(f"verdict 對照: last_run_outcome 的 quotes_* 為 1 的 stage = "
                        f"{[s for s, v in outcome.items() if v == 1] or '無'}")
    if not isinstance(result, dict) or "quotes_status" not in result:
        return Layer(
            key="L2",
            name="資料面 input",
            verdict=V_UNKNOWN,
            predicate="snapshot.result.quotes_status == ok",
            reads=reads,
            evidence=ev_lines + ["result.quotes_status 欄位不存在"],
            note=("artifact 的 schema 早於 #1944/#1986 ⇒ 它**答不出**報價品質"
                  "（symbols_ranked=0 也分不出『市場判定』與『量測失敗』）"),
        )
    status = result.get("quotes_status")
    if p_input_unusable(result):
        verdict = V_RED
        note = f"報價輸入不可用：quotes_status={status}（管線的排名不覆蓋整個母體）"
    else:
        verdict, note = V_OK, f"報價輸入可用：quotes_status={status}"
    return Layer(
        key="L2",
        name="資料面 input",
        verdict=verdict,
        predicate="snapshot.result.quotes_status == ok AND quotes_missing_fetch_error + quotes_missing_not_attempted == 0",
        reads=reads,
        evidence=ev_lines,
        note=note,
    )


# ── L3 評分面 ────────────────────────────────────────────────────────────────


def layer_scoring(ev: Evidence) -> Layer:
    """跑了嗎？跑出來的東西可用嗎？"""
    reads = ["snapshot.result.{symbols_built, symbols_filtered, symbols_ranked, ranked_trustworthy}",
             M_LAST_RUN_GATHERED, M_LAST_RUN_FILTERED, M_LAST_RUN_RANKED, M_LAST_RUN_TRUSTWORTHY]
    result = (ev.snapshot or {}).get("result") if ev.snapshot else None
    ev_lines = []
    if ev.snapshot is None:
        return Layer(
            key="L3",
            name="評分面 scoring",
            verdict=V_UNKNOWN,
            predicate="built > 0 AND filtered > 0 AND trustworthy != false AND ranked > 0",
            reads=reads,
            evidence=[f"snapshot 讀不到（{ev.snapshot_error}）"],
            note="沒有輸出可判（L5 會把產物本身判紅）",
        )
    for k in ("symbols_built", "symbols_filtered", "symbols_ranked", "symbols_excluded", "ranked_trustworthy",
              "ranked_fallback_reason", "full_rebuild", "timestamp"):
        if isinstance(result, dict) and k in result:
            ev_lines.append(f"result.{k} = {result[k]}")
    ranked_list = (ev.snapshot or {}).get("ranked")
    ev_lines.append(f"ranked 陣列長度 = {len(ranked_list) if isinstance(ranked_list, list) else 'n/a'}")
    mt = ev.metrics_text
    if mt:
        ev_lines.append(
            "verdict 對照（last_run_*）: "
            + "; ".join(
                f"{s}: gathered={mt.value(M_LAST_RUN_GATHERED, {'stage': s})} "
                f"ranked={mt.value(M_LAST_RUN_RANKED, {'stage': s})} "
                f"trustworthy={mt.value(M_LAST_RUN_TRUSTWORTHY, {'stage': s})}"
                for s in STAGES
            )
        )
    if p_scoring_broken(result):
        if _int_field(result, "symbols_built") == 0:
            why = "母體為空（Step 1 一檔 symbol 都沒取到）"
        elif _int_field(result, "symbols_filtered") == 0:
            why = "產業篩選把全部候選剔除（Step 2 全滅）"
        elif result.get("ranked_trustworthy") is False:
            why = f"排名不可信（ranked_fallback_reason={result.get('ranked_fallback_reason', 'n/a')}）"
        else:
            why = "評分空轉（母體非空、輸入可用，但 ranked = 0）"
        verdict, note = V_RED, why
    else:
        verdict, note = V_OK, f"輸出健康：ranked = {_int_field(result, 'symbols_ranked')}"
    if "ranked_trustworthy" not in (result or {}):
        note += "。⚠️ artifact 沒有 ranked_trustworthy 欄位（schema 早於 #1944）⇒ 分不出『市場判定』與『量測失敗』"
    return Layer(
        key="L3",
        name="評分面 scoring",
        verdict=verdict,
        predicate="built > 0 AND filtered > 0 AND ranked_trustworthy != false AND ranked > 0（缺一即 RED）",
        reads=reads,
        evidence=ev_lines,
        note=note,
    )


# ── L4 發射面 ────────────────────────────────────────────────────────────────


def layer_emission(ev: Evidence) -> Layer:
    """counter 說的是真話嗎？（標籤形狀 + 增量）"""
    reads = [M_RANKED_TOTAL + "{stage}", M_SCREENED_TOTAL + "{stage,result}",
             f"PromQL: increase({M_RANKED_TOTAL}[{EMISSION_WINDOW}])"]
    ev_lines = []
    mt = ev.metrics_text
    if mt is None:
        return Layer(
            key="L4",
            name="發射面 emission",
            verdict=V_UNKNOWN,
            predicate="標籤形狀 == {stage=...} AND NOT(輸出說 ranked>0 且新鮮 且 increase==0)",
            reads=reads,
            evidence=[f"metrics 讀不到（{ev.metrics.error}）⇒ 無法讀 counter"],
            note="沒有 /metrics 就沒有記帳面的證據",
        )
    problems = mt.label_shape_problems()
    for line, why in problems[:5]:
        ev_lines.append(f"標籤形狀問題: {line} — {why}")
    counted = {s: mt.by_stage(M_RANKED_TOTAL).get(s) for s in STAGES}
    ev_lines.append(f"counter 讀數 {M_RANKED_TOTAL}: " + ", ".join(f"{s}={v}" for s, v in counted.items()))
    ev_lines.append(f"increase 視窗 {EMISSION_WINDOW}: {ev.increase_note}")

    ranked_by_stage = mt.by_stage(M_LAST_RUN_RANKED)
    fin = mt.by_stage(M_LAST_RUN_FINISHED)
    valid = mt.by_stage(M_LAST_RUN_VALID)
    fresh_stages = [
        s for s in STAGES
        if valid.get(s) == 1 and fin.get(s) is not None
        and (ev.now - fin[s]) < 6 * 24 * 3600
    ]
    ranked_positive = any((ranked_by_stage.get(s) or 0) > 0 for s in fresh_stages)
    ev_lines.append(f"輸出說有產出且新鮮的 stage: {fresh_stages or '無'}（ranked>0 ⇒ {ranked_positive}）")

    if p_label_shape_bad(problems):
        verdict = V_RED
        note = ("counter 的標籤形狀壞掉 ⇒ **任何 increase() 讀數都是假的**"
                "（2026-06-22~09-25 的生產形狀：值被當成 label 名，正確形狀的系列全為 0）")
    elif not fresh_stages or not ranked_positive:
        verdict = V_OK
        note = "沒有『輸出說有產出且新鮮』的 stage ⇒ 發射面沒有可矛盾之處（記帳面斷線的判定式不成立）"
    elif ev.increase_ranked is None:
        verdict = V_UNKNOWN
        note = "需要 Prometheus 的 increase() 才能判『記帳面斷線』，但目前讀不到 ⇒ 無法判定"
    elif p_emission_missing(
        ranked_positive, fresh_stages, [s for s in fresh_stages if ev.increase_ranked.get(s) == 0]
    ):
        verdict = V_RED
        note = ("記帳面斷線：輸出說有 ranked>0 且新鮮，但 counter 在視窗內沒有增量 "
                f"（increase {M_RANKED_TOTAL}[{EMISSION_WINDOW}] == 0）⇒ 壞的是指標發射，不是 pipeline")
    else:
        verdict = V_OK
        note = "counter 有增量，與輸出一致"
    return Layer(
        key="L4",
        name="發射面 emission",
        verdict=verdict,
        predicate=(
            f"① {M_RANKED_TOTAL}/{M_SCREENED_TOTAL} 的每個系列都帶 stage label（不得出現 {{daily=...}}）"
            f" ② NOT(出輸出說 ranked>0 且 <6d 內完成 且 increase(...[{EMISSION_WINDOW}])==0)"
        ),
        reads=reads,
        evidence=ev_lines,
        note=note,
    )


# ── L5 產物面 ────────────────────────────────────────────────────────────────


def layer_artifact(ev: Evidence) -> Layer:
    """產物本身是不是這一輪的？（跑了、有產出、但沒落地 = 產物面）"""
    reads = [ev.snapshot_path, M_LAST_RUN_FINISHED + "{stage}",
             "容器日誌: " + "/".join(LOG_EVENT_START + LOG_EVENT_OK),
             M_LAST_RUN_SNAPSHOT_PERSISTED + "{stage}"]
    ev_lines = []
    mt = ev.metrics_text
    if ev.snapshot is None:
        return Layer(
            key="L5",
            name="產物面 artifact",
            verdict=V_RED,
            predicate="snapshot 存在（且 verdict 的 last_run_snapshot_persisted != 0）",
            reads=reads,
            evidence=[f"{ev.snapshot_path} 不存在（{ev.snapshot_error}）",
                      "下游（D6 watchlist / 覆蓋率檢查 / 人工驗收）讀的就是這個檔"],
            note="產物不存在 ⇒ 讀它的消費者看到的是『空母體』而不是『壞掉的管線』",
        )
    ev_lines.append(f"{ev.snapshot_path}: mtime={fmt_ts(ev.snapshot_mtime)}"
                    f"（{fmt_age(ev.snapshot_mtime, ev.now)}）")
    ev_lines.append(f"{ev.registry_path}: mtime={fmt_ts(ev.registry_mtime)}"
                    f"（{fmt_age(ev.registry_mtime, ev.now)}）")
    result = (ev.snapshot or {}).get("result")
    if isinstance(result, dict) and "timestamp" in result:
        ev_lines.append(f"result.timestamp（這一輪的開始時刻，寫在檔內）= {result['timestamp']}")

    claims, ran_log = [], []
    if mt:
        valid = mt.by_stage(M_LAST_RUN_VALID)
        fin = mt.by_stage(M_LAST_RUN_FINISHED)
        claims = [fin[s] for s, v in valid.items() if v == 1 and s in fin]
        ran_log = [ts for ts, _ in _log_after(ev.logs, LOG_EVENT_START + LOG_EVENT_OK, None)]
    t_exp = ev.expected_triggers[-1] if (ev.calendar and ev.expected_triggers and ev.expect_run == "auto") else None
    claim = max(claims) if claims else None
    if ran_log:
        claim = max(claim, max(ran_log)) if claim is not None else max(ran_log)
    ev_lines.append(f"宣稱的完成時刻（verdict/日誌取最新）= {fmt_ts(claim)}")
    if t_exp is not None:
        ev_lines.append(f"最後一次應執行時刻 = {fmt_ts(t_exp)}")
    persisted = mt.by_stage(M_LAST_RUN_SNAPSHOT_PERSISTED) if mt else {}
    if persisted:
        ev_lines.append(f"verdict 的產物訊號 {M_LAST_RUN_SNAPSHOT_PERSISTED}: "
                        + ", ".join(f"{s}={v:g}" for s, v in persisted.items()))
    else:
        ev_lines.append(f"verdict 的產物訊號 {M_LAST_RUN_SNAPSHOT_PERSISTED}: 不存在"
                        "（部署排在 09-29 驗收之後 ⇒ 預期如此；本層改用檔案 mtime 判定）")

    stale_claim = p_artifact_stale(ev.snapshot_mtime, claim)
    stale_expected = (
        t_exp is not None and ev.now > t_exp + ev.grace
        and ev.snapshot_mtime is not None and ev.snapshot_mtime < t_exp - ARTIFACT_STALE_SECONDS
    )
    verdict_signal_zero = any(v == 0 for v in persisted.values()) and bool(persisted)
    if verdict_signal_zero:
        verdict = V_RED
        note = (f"verdict 自己說 {M_LAST_RUN_SNAPSHOT_PERSISTED}=0 ⇒ 這一輪的產物沒有落地"
                "（Prometheus 上的 AtlasUniverseSnapshotNotPersisted 同式）")
    elif stale_claim:
        verdict = V_RED
        note = (f"產物沒有更新：宣稱在 {fmt_ts(claim)} 完成過一次執行，"
                f"但 {os.path.basename(ev.snapshot_path)} 的 mtime 停在 {fmt_ts(ev.snapshot_mtime)} "
                f"（早於宣稱時刻 {ARTIFACT_STALE_SECONDS}s 以上）")
    elif stale_expected:
        verdict = V_RED
        note = (f"產物沒有這一輪：最後一次應執行時刻 {fmt_ts(t_exp)} 之後，檔案 mtime 沒有前進"
                f"（停在 {fmt_ts(ev.snapshot_mtime)}）")
    else:
        verdict, note = V_OK, ("產物是最近一輪的" if claim or t_exp else "沒有『應該更新』的宣稱 ⇒ 無矛盾")
    if ev.registry_mtime is not None and ev.snapshot_mtime is not None:
        if ev.registry_mtime < ev.snapshot_mtime - ARTIFACT_STALE_SECONDS:
            note += (f"。⚠️ 次要矛盾：universe.json 的 mtime（{fmt_ts(ev.registry_mtime)}）"
                     "比 snapshot 舊 ⇒ registry 那一條寫入路徑可能失敗（**目前沒有對應告警**，已登記）")
    return Layer(
        key="L5",
        name="產物面 artifact",
        verdict=verdict,
        predicate=("snapshot 存在 AND mtime 不比『宣稱完成時刻』舊 2m 以上 "
                   "AND（若已過寬限）不比最後一次應執行時刻舊 2m 以上"),
        reads=reads,
        evidence=ev_lines,
        note=note,
    )


# ── 證據收集（線上 / 離線 fixture 兩種來源，**同一條判定路徑**）─────────────────


def repo_root_from_script() -> str:
    here = os.path.dirname(os.path.abspath(__file__))          # <repo>/scripts/ops
    return os.path.dirname(os.path.dirname(here))              # <repo>


def _fixture_seconds(directory: str, filename: str, default):
    """fixture 用：讀一個純 epoch 秒的檔來覆寫 mtime（不存在就沿用真實值）。"""
    f = read_file(os.path.join(directory, filename))
    if not f.ok:
        return default
    try:
        return float(f.value.strip())
    except ValueError:
        return default


def prom_query_scalars(prom_url: str, expr: str, label_key: str = "stage") -> tuple:
    """PromQL 純量查詢 → ({label_value: 值}, 說明)。

    只讀 /api/v1/query（不改狀態）。任何錯誤都回 (None, 說明) 而不是拋出。
    """
    import urllib.parse

    url = f"{prom_url.rstrip('/')}/api/v1/query?query=" + urllib.parse.quote(expr, safe="")
    f = http_get_json(url)
    if not f.ok:
        return None, f"{expr} → 讀不到（{f.error}）"
    try:
        results = f.value["data"]["result"]
    except Exception:  # noqa: BLE001
        return None, f"{expr} → 回應格式不符"
    out = {}
    for r in results:
        key = r.get("metric", {}).get(label_key)
        if key is None:
            continue
        out[key] = float(r["value"][1])
    return out, f"{expr} → {out or '（空結果）'}"


#: 生產主機實測（2026-09-27）：非互動 ssh 的 PATH=/usr/bin:/bin:/usr/sbin:/sbin **沒有 docker**，
#: OrbStack 的 CLI 在 /usr/local/bin/docker。因此候選清單比 shutil.which 更寬（仍然全部唯讀）。
DOCKER_CANDIDATES = ("docker", "/usr/local/bin/docker", "/opt/homebrew/bin/docker", "/usr/bin/docker")


def resolve_docker(explicit: str) -> str | None:
    """回傳可用的 docker 執行檔（顯式 → PATH → 已知絕對路徑）。"""
    if explicit:
        return explicit if (os.path.isabs(explicit) and os.access(explicit, os.X_OK)) else shutil.which(explicit)
    for cand in DOCKER_CANDIDATES:
        if os.path.isabs(cand):
            if os.access(cand, os.X_OK):
                return cand
        else:
            found = shutil.which(cand)
            if found:
                return found
    return None


def fetch_logs(args) -> LogFacts:
    if args.no_logs:
        return LogFacts(source="（--no-logs）", error="依參數跳過日誌證據")
    if args.logs_file:
        f = read_file(args.logs_file)
        if not f.ok:
            return LogFacts(source=args.logs_file, error=f.error)
        return parse_logs(f.value, os.path.basename(args.logs_file))
    docker = resolve_docker(args.docker)
    if not docker:
        return LogFacts(source="docker logs", error=f"找不到 docker 執行檔（試過: {', '.join(DOCKER_CANDIDATES)}）")
    cmd = [docker, "logs", "--since", args.logs_since, args.container]
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    except Exception as e:  # noqa: BLE001
        return LogFacts(source=" ".join(cmd), error=f"{type(e).__name__}: {e}")
    text = (p.stdout or "") + (p.stderr or "")
    if p.returncode != 0:
        snippet = " ".join(text.strip().splitlines()[:2])[:200] or "（無輸出）"
        return LogFacts(source=" ".join(cmd), error=f"docker logs 失敗（exit {p.returncode}）: {snippet}")
    return parse_logs(text, " ".join(cmd))


def collect_evidence(args) -> Evidence:
    now = args.now if args.now is not None else time.time()
    workdir = args.workdir or repo_root_from_script()
    offline = args.offline_dir or None

    if offline:
        d = offline
        metrics = read_file(os.path.join(d, "metrics.txt"))
        rules = read_file(os.path.join(d, "rules.json"))
        if rules.ok:
            try:
                rules = Fetched(value=json.loads(rules.value), source=rules.source)
            except Exception as e:  # noqa: BLE001
                rules = Fetched(source=rules.source, error=f"JSON 解析失敗: {e}")
        alerts = read_file(os.path.join(d, "alerts.json"))
        if alerts.ok:
            try:
                alerts = Fetched(value=json.loads(alerts.value), source=alerts.source)
            except Exception as e:  # noqa: BLE001
                alerts = Fetched(source=alerts.source, error=f"JSON 解析失敗: {e}")
        snapshot_path = os.path.join(d, "universe_snapshot.json")
        registry_path = os.path.join(d, "universe.json")
        logs = parse_logs(read_file(os.path.join(d, "logs.txt")).value or "", "logs.txt")
        inc = read_file(os.path.join(d, "increase_ranked.json"))
        increase_ranked, increase_note = None, "（offline: 沒有 increase_ranked.json）"
        if inc.ok:
            try:
                increase_ranked = {k: float(v) for k, v in json.loads(inc.value).items()}
                increase_note = f"offline fixture increase_ranked.json → {increase_ranked}"
            except Exception as e:  # noqa: BLE001
                increase_note = f"increase_ranked.json 解析失敗: {e}"
    else:
        metrics = http_get(args.metrics_url)
        rules = http_get_json(args.prom_url.rstrip("/") + "/api/v1/rules")
        alerts = http_get_json(args.prom_url.rstrip("/") + "/api/v1/alerts")
        snapshot_path = os.path.join(workdir, SNAPSHOT_REL)
        registry_path = os.path.join(workdir, REGISTRY_REL)
        logs = fetch_logs(args)
        increase_ranked, increase_note = prom_query_scalars(
            args.prom_url,
            f"increase({M_RANKED_TOTAL}{{stage=~\"daily|weekly\"}}[{EMISSION_WINDOW}])",
        )

    metrics_text = MetricsText(metrics.value) if metrics.ok else None
    snapshot, snapshot_error = None, ""
    f = read_file(snapshot_path)
    if f.ok:
        try:
            snapshot = json.loads(f.value)
        except Exception as e:  # noqa: BLE001
            snapshot_error = f"JSON 解析失敗: {e}"
    else:
        snapshot_error = f.error
    # mtime 一律以「檔案系統的事實」為準；fixture 模式允許用 snapshot_mtime.txt /
    # registry_mtime.txt 覆寫，否則離線測試就沒有辦法造出「產物比宣稱舊」這個形狀。
    snapshot_mtime = file_mtime(snapshot_path)
    registry_mtime = file_mtime(registry_path)
    if offline:
        snapshot_mtime = _fixture_seconds(d, "snapshot_mtime.txt", snapshot_mtime)
        registry_mtime = _fixture_seconds(d, "registry_mtime.txt", registry_mtime)

    calendar, calendar_note = load_trading_calendar(os.path.join(workdir, HOLIDAY_SOURCE_REL))
    trigger_status, expected = "unknown", []
    if calendar is not None:
        trigger_status, expected = calendar.scan_expected_triggers(now)
    return Evidence(
        now=now,
        workdir=workdir,
        offline=bool(offline),
        metrics=metrics,
        metrics_text=metrics_text,
        rules=rules,
        alerts=alerts,
        logs=logs,
        snapshot_path=snapshot_path,
        snapshot_mtime=snapshot_mtime,
        snapshot=snapshot,
        snapshot_error=snapshot_error,
        registry_path=registry_path,
        registry_mtime=registry_mtime,
        calendar=calendar,
        calendar_note=calendar_note,
        trigger_status=trigger_status,
        expected_triggers=expected,
        increase_ranked=increase_ranked,
        increase_note=increase_note,
        expect_run=args.expect_run,
        grace=float(args.grace_seconds),
    )


LAYER_ORDER = ("L0", "L1", "L2", "L3", "L4", "L5")


def run_layers(ev: Evidence) -> list:
    return [
        layer_transport(ev),
        layer_schedule(ev),
        layer_input(ev),
        layer_scoring(ev),
        layer_emission(ev),
        layer_artifact(ev),
    ]


# ── 輸出 ─────────────────────────────────────────────────────────────────────

_VERDICT_MARK = {
    V_OK: "OK   ",
    V_RED: "RED  ",
    V_UNKNOWN: "UNKN ",
    V_PENDING: "PEND ",
    V_SKIPPED: "SKIP ",
    V_WARN: "WARN ",
}


def overall_exit(layers) -> tuple:
    reds = [l.key for l in layers if l.verdict in RED_VERDICTS]
    indet = [l.key for l in layers if l.verdict in INDETERMINATE_VERDICTS]
    if reds:
        return EXIT_RED, reds, indet
    if indet:
        return EXIT_INDETERMINATE, reds, indet
    return EXIT_GREEN, reds, indet


def print_report(ev: Evidence, layers, args) -> None:
    out = []
    out.append(f"=== verify-universe-run v{TOOL_VERSION} — 母體(universe)執行真值盤查 + 判層 ===")
    out.append(f"now       = {fmt_ts(ev.now)} (epoch {ev.now:.0f})")
    out.append(f"workdir   = {ev.workdir}{'  [offline]' if ev.offline else ''}")
    out.append(f"metrics   = {ev.metrics.source}{(' — ' + ev.metrics.error) if not ev.metrics.ok else ''}")
    out.append(f"rules     = {ev.rules.source}{(' — ' + ev.rules.error) if not ev.rules.ok else ''}")
    out.append(f"alerts    = {ev.alerts.source}{(' — ' + ev.alerts.error) if not ev.alerts.ok else ''}")
    out.append(f"logs      = {ev.logs.source}{(' — ' + ev.logs.error) if ev.logs.error else ''}")
    out.append(f"snapshot  = {ev.snapshot_path}")
    out.append("")
    out.append("── 判層（每一層都有可稽核的判定式）──")
    for l in layers:
        out.append(f"[{_VERDICT_MARK[l.verdict]}] {l.key} {l.name} — {l.note}")
        out.append(f"        判定式: {l.predicate}")
        out.append(f"        讀: {', '.join(l.reads)}")
        if not args.quiet:
            for e in l.evidence:
                out.append(f"          · {e}")
        if l.rules:
            out.append(f"        Prometheus 對應規則: {', '.join(l.rules)}")
        out.append("")
    code, reds, indet = overall_exit(layers)
    label = {EXIT_GREEN: "GREEN", EXIT_RED: "RED", EXIT_INDETERMINATE: "INDETERMINATE"}[code]
    out.append("── 總結 ──")
    out.append(f"判定 = {label}  (exit {code})")
    if reds:
        out.append(f"  應告警的層: {', '.join(reds)}")
    if indet:
        out.append(f"  無法判定的層: {', '.join(indet)}（未知不是壞掉，也不是綠燈）")
    if not reds and not indet:
        out.append("  全部判層有結論且無一層成立『應告警』條件")
    warn = [l.key for l in layers if l.verdict == V_WARN]
    if warn:
        out.append(f"  提醒（不影響 exit code）: {', '.join(warn)}")
    out.append("")
    out.append("提醒：本工具**不取代告警**，只是把判層變成可重跑的一件事（唯讀、不改任何狀態）。")
    print("\n".join(out))


def print_json(ev: Evidence, layers, args) -> None:
    code, reds, indet = overall_exit(layers)
    print(json.dumps({
        "tool": "verify-universe-run",
        "version": TOOL_VERSION,
        "now": ev.now,
        "now_iso": fmt_ts(ev.now),
        "workdir": ev.workdir,
        "offline": ev.offline,
        "exit_code": code,
        "red_layers": reds,
        "indeterminate_layers": indet,
        "layers": [
            {
                "key": l.key,
                "name": l.name,
                "verdict": l.verdict,
                "predicate": l.predicate,
                "reads": l.reads,
                "evidence": l.evidence,
                "note": l.note,
                "prometheus_rules": list(l.rules),
            }
            for l in layers
        ],
    }, ensure_ascii=False, indent=2))


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="verify-universe-run",
        description=(
            "母體(universe)執行的唯讀真值盤查 + 判層。"
            "exit 0=全綠 / 1=有層成立『應告警』條件 / 2=無法判定 / 3=用法錯誤。"
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "範例（生產主機，唯讀）:\n"
            "  scripts/ops/verify-universe-run.sh\n"
            "  scripts/ops/verify-universe-run.sh --json\n"
            "  scripts/ops/verify-universe-run.sh --expect-run none      # 已知休市日\n"
            "  scripts/ops/verify-universe-run.sh --offline-dir <fixture> --now 1790498700\n"
        ),
    )
    p.add_argument("--workdir", help="atlas 工作目錄（預設＝本腳本所在的 repo 根目錄）")
    p.add_argument("--metrics-url", default="http://127.0.0.1:18080/metrics")
    p.add_argument("--prom-url", default="http://127.0.0.1:9090", help="Prometheus（/api/v1/rules、/api/v1/alerts、/api/v1/query）")
    p.add_argument("--container", default="atlas-go", help="取日誌的容器名")
    p.add_argument("--docker", default="", help="docker 執行檔路徑（預設自動尋找）")
    p.add_argument("--logs-file", default="", help="改用已抓下來的日誌檔（不呼叫 docker）")
    p.add_argument("--logs-since", default="168h")
    p.add_argument("--no-logs", action="store_true", help="不看日誌證據")
    p.add_argument("--offline-dir", default="", help="從 fixture 目錄讀（metrics.txt/rules.json/alerts.json/logs.txt/universe_snapshot.json/increase_ranked.json）")
    p.add_argument("--now", type=float, default=None, help="固定『現在』（epoch 秒；offline 模式必填）")
    p.add_argument("--expect-run", choices=("auto", "none", "daily", "weekly"), default="auto",
                   help="應不應該有執行：auto=用台灣交易日曆（預設）")
    p.add_argument("--grace-seconds", type=float, default=DEFAULT_GRACE_SECONDS,
                   help=f"『已預告要跑但還沒跑完』的寬限（預設 {DEFAULT_GRACE_SECONDS}）")
    p.add_argument("--json", action="store_true", help="輸出 JSON（給自動化）")
    p.add_argument("--quiet", "-q", action="store_true", help="只印結論，不印逐條證據")
    p.add_argument("--version", action="version", version=f"verify-universe-run {TOOL_VERSION}")
    return p


def main(argv=None) -> int:
    args = build_parser().parse_args(argv)
    if args.offline_dir and args.now is None:
        print("錯誤：--offline-dir 模式必須同時給 --now <epoch>（fixture 的時間基準必須固定）", file=sys.stderr)
        return EXIT_USAGE
    try:
        ev = collect_evidence(args)
    except Exception as e:  # noqa: BLE001
        print(f"錯誤：收集證據失敗（{type(e).__name__}: {e}）", file=sys.stderr)
        return EXIT_USAGE
    layers = run_layers(ev)
    if args.json:
        print_json(ev, layers, args)
    else:
        print_report(ev, layers, args)
    return overall_exit(layers)[0]


if __name__ == "__main__":
    sys.exit(main())
