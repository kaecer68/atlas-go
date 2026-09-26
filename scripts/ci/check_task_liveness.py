#!/usr/bin/env python3
"""check_task_liveness.py — 每日維護可觀測性：以 daemon 的 task-liveness 為真實來源。

## 為什麼有這支腳本（E21）

`.github/workflows/daily-maintenance.yml` 原本每天呼叫 7 個**不存在**的 atlas CLI 子命令
（`weights adjust --apply`、`prism status`、`prism balance`、`prism report`、
`reflexivity report`、`reflexivity alert`）。CLI 當時會**靜默丟棄**無法識別的 positional args
並落到預設的 one-shot simulation ⇒ 每個 job 都回 exit 0、artifact 永遠是空的
（「每天都綠、什麼都沒做」）。

這些報告的真實生產者其實是 **daemon 內的背景任務**（`prism_training` 6h、
`prism_auto_balancer` 5m、`auto_daily_simulation`、`evolution_health` …，
見 `internal/apigateway/background.go` 與 `cmd/atlas/main.go`）。
本腳本因此把 CI job 的**真實來源**換成 daemon 對外公開的活性快照
`GET /api/dashboard/task-liveness`（免認證、`/api/dashboard/` 屬 public-read 白名單），
並保留原本「每日維護可觀測性」的意圖：每天從**外部** vantage point 檢查維護管線還活著。

## 判定語意（fail-closed）

- 抓不到快照（連線失敗／非 200／JSON 壞／`status=degraded`／tasks 為空）
  ⇒ **無法驗證**（exit 2）——不可因為「看不到」而變綠。
- 快照抓到但必要任務不健康 ⇒ exit 1（附逐項原因）。
- 全部必要任務健康 ⇒ exit 0，並寫出 artifact。

逐項健康條件：
  1. 任務存在於快照（缺席＝從未執行／被改名／daemon 沒在排程）
  2. `enabled` 不是 false
  3. `last_success_at` 非 null
  4. `stale` 為 false（daemon 已依 interval×3 或 time-gated 視窗算好）
  5. `consecutive_failures` <= `--max-failures`（預設 0）

## 用法

    python3 scripts/ci/check_task_liveness.py \
        --label darwinian \
        --tasks auto_daily_simulation,evolution_health \
        --artifact reports/darwinian_liveness.json

環境變數：
  ATLAS_TASK_LIVENESS_URL   覆蓋預設端點
  ATLAS_TASK_LIVENESS_FILE  改讀本機快照檔（hermetic 測試／離線除錯用）

Exit: 0 = 健康、1 = 已驗證不健康、2 = 無法驗證（用法錯誤亦為 2）。
"""

from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

# 唯一真相來源：daemon 的對外活性端點（public-read，無需 API key）。
DEFAULT_URL = "https://atlas.goluck.uk/api/dashboard/task-liveness"
DEFAULT_TIMEOUT_SECONDS = 30
DEFAULT_RETRIES = 3
DEFAULT_MAX_FAILURES = 0

EXIT_OK = 0
EXIT_UNHEALTHY = 1
EXIT_CANNOT_VERIFY = 2


class CannotVerify(Exception):
    """快照不可用／不可信 ⇒ 「無法驗證」而不是「健康」。"""


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def fetch_snapshot(url: str, timeout: int, retries: int, snapshot_file: str | None):
    """回傳 (snapshot, source)。抓不到一律 raise CannotVerify。"""
    if snapshot_file:
        path = Path(snapshot_file)
        try:
            raw = path.read_text(encoding="utf-8")
        except OSError as exc:
            raise CannotVerify(f"cannot read snapshot file {path}: {exc}") from exc
        try:
            return json.loads(raw), f"file://{path}"
        except json.JSONDecodeError as exc:
            raise CannotVerify(f"snapshot file {path} is not valid JSON: {exc}") from exc

    last_error: Exception | None = None
    for attempt in range(1, retries + 1):
        try:
            req = urllib.request.Request(
                url,
                headers={
                    "Accept": "application/json",
                    # 明確 UA：預設的 Python-urllib UA 會被部分 CDN／WAF 直接 403。
                    "User-Agent": "atlas-daily-maintenance/1.0 (+ci)",
                },
            )
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                status = getattr(resp, "status", resp.getcode())
                body = resp.read().decode("utf-8", errors="replace")
            if status != 200:
                raise CannotVerify(f"{url} returned HTTP {status}")
            return json.loads(body), url
        except CannotVerify as exc:
            last_error = exc
        except urllib.error.HTTPError as exc:
            last_error = CannotVerify(f"{url} returned HTTP {exc.code}: {exc.reason}")
        except urllib.error.URLError as exc:
            last_error = CannotVerify(f"{url} unreachable: {exc.reason}")
        except (TimeoutError, OSError) as exc:
            last_error = CannotVerify(f"{url} unreachable: {exc}")
        except json.JSONDecodeError as exc:
            last_error = CannotVerify(f"{url} returned invalid JSON: {exc}")

        print(f"  attempt {attempt}/{retries}: {last_error}", flush=True)
        if attempt < retries:
            time.sleep(2 * attempt)

    raise CannotVerify(f"cannot verify maintenance pipeline: {last_error}")


def evaluate(snapshot: dict, required: list[str], max_failures: int):
    """回傳 (rows, problems)。快照本身不可信時 raise CannotVerify。"""
    status = snapshot.get("status")
    if status == "degraded":
        raise CannotVerify(
            "daemon reported status=degraded (task-liveness store unavailable): "
            + str(snapshot.get("error", "no detail"))
        )

    tasks = snapshot.get("tasks")
    if not isinstance(tasks, list) or not tasks:
        raise CannotVerify(
            "snapshot contains no tasks — daemon may be down, or the liveness store is empty; "
            "an empty snapshot must never be treated as healthy"
        )

    by_name = {}
    for task in tasks:
        if isinstance(task, dict) and task.get("name"):
            by_name[task["name"]] = task

    rows = []
    problems: list[str] = []
    for name in required:
        task = by_name.get(name)
        if task is None:
            reason = "missing from the liveness snapshot (never ran, renamed, or no longer scheduled)"
            rows.append({"name": name, "present": False, "healthy": False, "reasons": [reason]})
            problems.append(f"{name}: {reason}")
            continue

        reasons: list[str] = []
        if task.get("enabled") is False:
            reasons.append("task is disabled")
        if not task.get("last_success_at"):
            reasons.append("no recorded success yet (last_success_at is null)")
        if task.get("stale") is True:
            reasons.append("stale: " + str(task.get("stale_reason") or "overdue (interval x3)"))
        failures = 0
        try:
            failures = int(task.get("consecutive_failures") or 0)
        except (TypeError, ValueError):
            failures = 0
        if failures > max_failures:
            detail = f"consecutive_failures={failures} > --max-failures={max_failures}"
            if task.get("last_error"):
                detail += f" (last_error: {task['last_error']})"
            reasons.append(detail)

        rows.append(
            {
                "name": name,
                "present": True,
                "healthy": not reasons,
                "reasons": reasons,
                "last_run_at": task.get("last_run_at"),
                "last_success_at": task.get("last_success_at"),
                "consecutive_failures": failures,
                "last_error": task.get("last_error", ""),
                "interval": task.get("interval", ""),
                "stale": bool(task.get("stale")),
                "source": task.get("source", ""),
            }
        )
        if reasons:
            problems.append(f"{name}: " + "; ".join(reasons))

    return rows, problems


def render_markdown(label, source, generated_at, rows, problems):
    lines = [
        f"# Daily maintenance liveness — {label}",
        "",
        f"- checked_at: {utc_now()}",
        f"- source: {source}",
        f"- snapshot generated_at: {generated_at}",
        f"- verdict: {'FAIL' if problems else 'PASS'}",
        "",
        "| task | present | healthy | last_success_at | failures | interval | note |",
        "|------|---------|---------|-----------------|----------|----------|------|",
    ]
    for row in rows:
        note = "; ".join(row["reasons"]) if row["reasons"] else "ok"
        lines.append(
            "| {name} | {present} | {healthy} | {success} | {failures} | {interval} | {note} |".format(
                name=row["name"],
                present=str(row.get("present")),
                healthy=str(row.get("healthy")),
                success=row.get("last_success_at") or "-",
                failures=row.get("consecutive_failures", "-"),
                interval=row.get("interval") or "-",
                note=note.replace("|", "/"),
            )
        )
    lines.append("")
    if problems:
        lines.append("## Problems")
        lines.append("")
        lines.extend(f"- {p}" for p in problems)
        lines.append("")
    return "\n".join(lines)


def write_artifact(path: str, label, source, generated_at, rows, problems, required, max_failures):
    artifact = Path(path)
    if artifact.parent != Path(""):
        artifact.parent.mkdir(parents=True, exist_ok=True)

    payload = {
        "label": label,
        "checked_at": utc_now(),
        "source": source,
        "snapshot_generated_at": generated_at,
        "verdict": "fail" if problems else "pass",
        "required_tasks": required,
        "max_consecutive_failures": max_failures,
        "tasks": rows,
        "problems": problems,
    }
    if artifact.suffix == ".md":
        artifact.write_text(
            render_markdown(label, source, generated_at, rows, problems) + "\n", encoding="utf-8"
        )
    else:
        artifact.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    return artifact


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        description="Verify that the daemon-side daily maintenance tasks are alive (real source, fail-closed)."
    )
    parser.add_argument("--tasks", required=True, help="逗號分隔的必要任務名稱（BTM／cron task name）")
    parser.add_argument("--label", default="maintenance", help="artifact 內的標籤（例：darwinian）")
    parser.add_argument("--artifact", default="", help="輸出檔路徑；.md 產生 Markdown，其餘產生 JSON")
    parser.add_argument("--max-failures", type=int, default=DEFAULT_MAX_FAILURES,
                        help="允許的 consecutive_failures 上限（預設 0）")
    parser.add_argument("--url", default="", help=f"覆蓋端點（預設 {DEFAULT_URL}）")
    parser.add_argument("--file", default="", help="改讀本機快照 JSON（hermetic 測試／離線除錯）")
    parser.add_argument("--summary-file", default="", help="額外把摘要追加到此檔（例：${GITHUB_STEP_SUMMARY}）")
    parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS)
    parser.add_argument("--retries", type=int, default=DEFAULT_RETRIES)
    args = parser.parse_args(argv)

    required = [t.strip() for t in args.tasks.split(",") if t.strip()]
    if not required:
        print("usage error: --tasks must list at least one task name", file=sys.stderr)
        return EXIT_CANNOT_VERIFY

    import os

    url = args.url or os.environ.get("ATLAS_TASK_LIVENESS_URL", "") or DEFAULT_URL
    snapshot_file = args.file or os.environ.get("ATLAS_TASK_LIVENESS_FILE", "")

    print(f"daily-maintenance liveness probe: label={args.label} tasks={','.join(required)}")
    print(f"  source: {snapshot_file or url}")

    try:
        snapshot, source = fetch_snapshot(url, args.timeout, args.retries, snapshot_file)
        rows, problems = evaluate(snapshot, required, args.max_failures)
    except CannotVerify as exc:
        print(f"::error::daily maintenance CANNOT be verified: {exc}", flush=True)
        print(f"CANNOT VERIFY: {exc}", file=sys.stderr)
        return EXIT_CANNOT_VERIFY

    generated_at = snapshot.get("generated_at", "unknown")
    print(f"  snapshot generated_at={generated_at} total_tasks={snapshot.get('total')} stale_count={snapshot.get('stale_count')}")
    for row in rows:
        state = "OK" if row["healthy"] else "FAIL"
        detail = "" if row["healthy"] else " — " + "; ".join(row["reasons"])
        print(f"  [{state}] {row['name']} (last_success={row.get('last_success_at') or '-'}){detail}")

    if args.artifact:
        artifact = write_artifact(
            args.artifact, args.label, source, generated_at, rows, problems, required, args.max_failures
        )
        print(f"  artifact: {artifact}")

    summary = render_markdown(args.label, source, generated_at, rows, problems)
    if args.summary_file:
        try:
            with open(args.summary_file, "a", encoding="utf-8") as handle:
                handle.write(summary + "\n")
        except OSError as exc:
            print(f"::warning::could not append to summary file {args.summary_file}: {exc}")

    if problems:
        for problem in problems:
            print(f"::error::{args.label}: {problem}")
        print(f"FAIL: {len(problems)} problem(s) in the daily maintenance pipeline", file=sys.stderr)
        return EXIT_UNHEALTHY

    print(f"PASS: {len(required)} maintenance task(s) healthy")
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
