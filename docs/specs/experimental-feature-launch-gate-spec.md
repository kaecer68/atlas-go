# Experimental Feature Launch Gate — Canonical Pattern

> **Status**: SHIPPED (in production via PR #1027 preflight + PR #1029 auto-cron, both Wave 11 L2.4 series)
> **Owner**: agent platform team
> **Last updated**: 2026-07-16 (added C07 sector prediction instance)

## 目的

本文件定義「**experimental feature launch gate**」(以下簡稱 **launch gate**)的 canonical pattern,讓 L2.4 observation 啟動這個**目前唯一的** launch gate 變成**未來可重用**的設計模板,避免「每次有實驗性功能上線就造一個 ad-hoc 閘門」的碎片化。

L2.4 preflight 是這個 pattern 的**第一個 instance**,目前已有 **2 個 instances** (L2.4 + C07 sector prediction, 後者為 follow-up 擴張)。

## 為什麼需要「唯一」

實驗性功能(experimental feature)上線的核心風險是「**半套 production 跑起來**」:
- 該跑的 prereq 沒跑 → 資料污染
- 該驗的條件沒驗 → silent failure
- 該通知的人沒通知 → postmortem 困難

如果每個實驗性功能各造各的 launch gate,會出現:
1. **標準碎片化**:不同 launch gate 檢查的東西不一致,難以 audit
2. **重複維護成本**:相同的 SSRF guard、5-condition gate 邏輯在 N 個地方各自維護
3. **新人無所適從**:新實驗性功能上線時不知道要 clone 哪個 pattern

「唯一」= 「**同類問題只解一次,新需求遵循同樣 pattern**」,不是「世界上只能存在一個」。

## 什麼是 Experimental Feature

符合**全部 3 個條件**的功能才需要 launch gate:

| 條件 | 說明 | L2.4 範例 |
|------|------|---------|
| 1. Feature flag 預設 off | 透過 `UseLLMSectorAgents=false` 等 config flag 控制 | `UseLLMSectorAgents` 預設 `false` |
| 2. 有 observation window | 啟動後會跑一段時間收集 metric,事後評估 | L2.4 observation 30 天 window |
| 3. 計畫 promote | 有明確的 promotion procedure(flag 翻轉、default flip) | L2.4 4-step promotion(Source upgrade → Default flip → LLMDriver removal → Version tag) |

非實驗性功能(普通 feature / bugfix / refactor)**不需要** launch gate,直接走 normal CI。

## Launch Gate Pattern 結構

每個 launch gate 由**兩個互補的實作**組成(per L2.4 設計):

| 元件 | 角色 | L2.4 範例 |
|------|------|---------|
| **Preflight (manual path)** | Operator 啟動前手動跑,輸出 yes/no | `cmd/experimental/l2-4-preflight/main.go` (5 automatable + 3 manual) |
| **Auto-cron gate (automated path)** | Scheduler 自動判斷要不要觸發,no-op if 條件不通過 | `internal/scheduler/l2_4_auto_cron.go` `ShouldL24AutoCronFire()` |

兩個路徑**共用同一套判斷邏輯**,避免「人手動 OK,scheduler 自動跳過 / 反之」的不一致。

## 5-Condition Gate 模板(供未來 launch gate 參考)

L2.4 的 `ShouldL24AutoCronFire` 設計可作為未來 launch gate 的**結構範本**:

| 條件類型 | 範例 | 為何必要 |
|---------|------|---------|
| **環境開關** | `L24AutoCronEnvVar == "true"` | 預設完全 off,需明確 opt-in |
| **Config flag** | `params.GetAutoEnabled() == true` | 對應 feature flag 開啟 |
| **資料/狀態就緒** | `observationLogPath` 存在 + 有 Day 7 entry | 確保有足夠 historical data 才啟動 |
| **時間窗口** | `cronWindowStart/End` + `cronDays` in window | 避免離峰時段觸發 |
| **circuit-breaker** | (預留,未啟用) | 防止未恢復的 upstream failure |

每個條件 hard-coded 為「不通過 → return false」,**沒有「軟啟動」「部分開啟」**的設計。

## Preflight 模板(供未來 manual path 參考)

L2.4 preflight 的結構可作為未來 preflight 的**範本**:

```go
type checkResult struct {
    Name    string
    OK      bool
    Manual  bool   // true = operator must confirm externally
    Message string
}

func main() {
    baseURL := validateLocalhostURL(os.Args[1])  // SSRF guard

    checks := []checkResult{
        checkConfigFlag(),         // automatable
        checkProviderHealth(),     // automatable
        checkDataReady(),          // automatable
        checkCircuitBreaker(),     // automatable
        checkOperatorConfirm(),    // manual (always passes, but requires external sign-off)
    }

    fail := false
    for _, c := range checks {
        marker := "✅"
        if !c.OK { marker = "❌"; fail = true }
        if c.Manual { marker = "✋" }
        fmt.Printf("%s %s: %s\n", marker, c.Name, c.Message)
    }

    if fail { os.Exit(1) }
    os.Exit(0)
}
```

關鍵設計約束:
- **SSRF guard**:只能跑 localhost(`validateLocalhostURL`)
- **5-7 個 check**:不要更多(過多 → operator 跳過;過少 → 漏風險)
- **Manual checks 必須獨立標記**:不能假設 automatable 已涵蓋
- **Exit code**:0 = pass、1 = fail、2 = invalid input(例如 non-localhost URL)

## Anti-patterns(不要做的事)

| 反模式 | 為什麼錯 | 正確做法 |
|--------|---------|---------|
| ❌ 在自己的 module 內寫 ad-hoc preflight | 標準碎片化,失去 audit trail | 遵循本 spec,clone L2.4 preflight pattern |
| ❌ 把 preflight 邏輯 generalize 為共用 library | L2.4 特定 check 難以 generalize,容易 over-engineer | 維持「instance-level」設計,只共享 pattern 概念 |
| ❌ 「我們知道自己在做什麼」繞過 gate | 失去 audit trail + silent failure 風險 | 必須跑 preflight,失敗就修 |
| ❌ 加 partial soft-check「這個條件不重要」 | 半套 production = 完全沒 production | 條件要不 hard 進 gate,要不刪除 |
| ❌ 把 preflight 跑在 production 對外 URL | SSRF 風險 | `validateLocalhostURL` SSRF guard 強制 |
| ❌ 5+ condition gate 跑在 auto-cron 但手動路徑用不同邏輯 | 「人啟動 OK,scheduler 跳過」不一致 | 兩個路徑共用同一套判斷 |

## 如何新增 Launch Gate(未來 SOP)

當新實驗性功能需要 launch gate 時:

1. **確認符合 3 條件**(flag 預設 off + observation window + promotion plan)
2. **Clone L2.4 preflight pattern**:
   - `cmd/experimental/<feature>-preflight/main.go`(命名規範)
   - 5 automatable + 3 manual checks(可調整數量,但不要 < 4)
   - SSRF guard 必備
3. **Clone L2.4 auto-cron pattern**:
   - `internal/scheduler/<feature>_auto_cron.go`
   - 5-condition hard gate
   - 預設 off,需 env var opt-in
4. **更新 allow-list**:`scripts/ci/check_no_duplicate_preflight.sh` 加入新 instance
5. **更新 spec**:在「Reference Implementations」表格加入新 row
6. **加 doc 引用**:相關 `docs/operations/<feature>-*` 互相 cross-link

**不要**:把現有 L2.4 preflight 程式碼直接 import / generalize(會失去 instance-level 設計彈性)。

## Reference Implementations

| Feature | Preflight | Auto-cron | Doc |
|---------|-----------|-----------|-----|
| **L2.4 sector agents** | `cmd/experimental/l2-4-preflight/main.go` (PR #1027) | `internal/scheduler/l2_4_auto_cron.go` (PR #1029) | `docs/operations/l2-4-runbook.md` + `l2-4-observation-spec.md` |
| **C07 sector prediction** | `cmd/experimental/c07-preflight/main.go` (PR #1201) | `cmd/experimental/c07-obs-collector/main.go` + `cmd/experimental/c07-day-evaluator/main.go` (PR #1202) — no auto-cron (request-time computation, no scheduler trigger) | ~~runbook~~（DEPRECATED 2026-08-12，已刪除） |
| **C07 sector direction predictions** (rule-based) | `cmd/experimental/c07-preflight/main.go` (PR #1200+ follow-up) | (no auto-cron — request-time computation, no scheduler trigger) | ~~runbook~~（DEPRECATED 2026-08-12，已刪除） + ~~spec~~（DEPRECATED 2026-08-12，已刪除） |

未來新增 launch gate 時,在此表加入 row。

## CI 防護(`scripts/ci/check_no_duplicate_preflight.sh`)

CI 自動掃描所有 `*preflight*.go` / `*gate*.go` 檔案,比對 allow-list:
- 在 allow-list → 通過
- 不在 allow-list → **warning**(不 fail build)

CI warning 而非 hard fail,理由:
- 「唯一性」是**設計理念**而非 hard contract
- 太嚴格會擋住合理的 deviation(例如 test-only preflight)
- Warning 仍提供 audit signal,留給 reviewer 評估

## 相關文件

- `docs/operations/l2-4-runbook.md` — L2.4 preflight 操作手冊
- `.omo/manifests/l2-4-followup.md` — L2.4 follow-up issues 鏡像
- `docs/specs/l2-4-observation-spec.md` — L2.4 metric schema spec
- `.claude/skills/atlas-experimental-feature-launch-gate/SKILL.md` — Agent skill 教學
- `scripts/ci/check_no_duplicate_preflight.sh` — CI 防護腳本

## 變更歷史

| Date | Change | Author |
|------|--------|--------|
| 2026-07-09 | Initial spec — L2.4 preflight promoted to canonical pattern | agent platform team |
