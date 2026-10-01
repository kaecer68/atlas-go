# FinMind 層級不足 ⇒ SBL/TDCC 歷史回補探測（已知問題 follow-up，2026-10-01）

> **狀態**：**已知、刻意不修**（業主裁定 ③：降頻為每週一次探測 ＋ 標為已知）
> **範圍**：`auto_sbl_tdcc_history_backfill`（`cmd/atlas/capital_tasks.go`）
> **性質**：本檔只記錄「為什麼不修、改了什麼、剩下的訊號是什麼」。**未修改任何生產行為或設定**
> （interval 變更走正常 PR pipeline，部署由另一棒帶 MARKER 執行）。
> **一句話**：FinMind **帳號層級不足**讓 TDCC 段（`TaiwanStockHoldingSharesPer` 的**歷史**查詢）
> 必然失敗；這**不是** atlas 缺陷、**不是** key 問題、**不是** compose 空覆蓋。live 段正常，
> 失敗的只有「回補 2026-03-01 起的歷史」。處置＝**把探測頻率由 1h 降到 168h**，錯誤照舊向上，
> 層級升級後下一次探測即自動恢復。

---

## 1. 症狀（業主 2026-10-01 唯讀查證提供；本檔未重查生產）

| 觀測 | 值 |
|---|---|
| 任務 | `auto_sbl_tdcc_history_backfill` |
| `last_error` | `tdcc: history probe 2026-03-06: finmind: circuit breaker open` |
| `consecutive_failures` | **21**（同日同錯重複 21 次） |
| 近 24h `Your level is register` | **277** 筆 |
| `auto_tdcc_dispersion` / `auto_twse_sbl` | `fails=0`（**未受影響，不要動**） |
| 同源 | `maintenance_ratio_source_failed`（贊助級 dataset `TaiwanTotalExchangeMarginMaintenance`） |

## 2. 根因（可在原始碼逐行複現）

1. **失敗點**：`internal/marketdata/tdcc_provider.go` 的 `FetchDispersionHistory`
   逐個**週五**打 `FetchDatasetRaw(ctx, "TaiwanStockHoldingSharesPer", d, d)`；
   第一個失敗就 `return written, fmt.Errorf("tdcc: history probe %s: %w", probe, err)`
   （`tdcc_provider.go:130-180`，錯誤字串與症狀逐字相符）。
2. **上游回應**：HTTP 200 ＋ `{"msg":"Your level is register. Please update your user level. …"}` ＋
   `data=[]`。`internal/marketdata/finmind_client.go:720-734` 把「200 ＋ 非 success msg ＋ 空 data」
   歸類為 **`ErrQuotaExhausted`**（額度態，**刻意不計入斷路器失敗**）。
3. **語意落差**：`ErrQuotaExhausted` 的語意是「今日額度、00:00 UTC 重置」；但上游這句話說的是
   **層級（tier）不足**——**不會**隨時間重置。⇒ 現行分類把它當「等待態」，於是「等待」永遠等不到。
4. **放大器**：`finmind: circuit breaker open`（`finmind_client.go:755` 的 `ErrFinMindBreakerOpen`）
   表示共用 client 的斷路器**已經開著**；此時所有 FinMind 消費者都被短路。任務**每小時**再撞一次，
   既無進展、也持續把這個狀態寫回 `last_error`。
5. **為什麼 live 沒事**：`auto_tdcc_dispersion` 抓的是**最近**快照（同 dataset，但近期窗口），
   層級足夠；被擋住的只有**歷史**窗口（backfill 的 cursor 停在 `2026-03-01` 之後第一個拿不到資料的週五）。

## 3. 已排除（不要重查）

- **key 無效**：`FINMIND_API_KEY` 長度 168、上游回的是「層級」訊息而非 `Token illegal`（400）。
- **compose 空覆蓋**：已由 PR #2195 修掉（`quote-backfill` 的空 `FINMIND_API_KEY`）。
- **atlas 程式缺陷**：live 段的同一 dataset 在**同層級**下成功 ⇒ 差異在「查詢窗口」，不在程式。

## 4. 本次處置（PR 內容）

1. **探測頻率 1h → 168h**：`cmd/atlas/capital_tasks.go` 新增具名常數
   `sblTDCCHistoryProbeInterval = 7 * 24 * time.Hour`，`Interval` 改用它。
   - 只動**這一個**任務；`auto_twse_sbl`（1h gate）與 `auto_tdcc_dispersion`（1h gate）
     **各自獨立註冊、fails=0，未動**。
   - 新 process 起跑仍會立刻執行一次（`runTask` 的 first-run 語意不變）。
2. **已知標記（文件化 ＋ 可追溯）**：本檔 ＋ 任務註冊處的長註解 ＋ 錯誤點的短註解，
   三者互指。**沒有**新增 Prometheus silence（本專案刻意不用）、**沒有**吞錯成 `nil`。

## 5. 剩下的訊號（刻意保留，屬於可接受成本）

- 每週探測仍可能失敗一次 ⇒ `background_task` 警報（AlertStore，**error** 級）會再次出現。
  這是**正確**的：層級升級後第一次成功的探測會走
  `monitor.ResolveByIdentity("background_task", name, "task-success")`（`cmd/atlas/main.go:1572`）
  自動把該警報 resolve ⇒ **升級即自動恢復，無需改程式、也無需人工解除**。
- 代價：由「每小時 1 次」降為「每週 1 次」（1/168）。失敗計數不再小時級累積。

## 6. 恢復條件與判讀步驟

**恢復條件**：FinMind 帳號層級升到可讀歷史 `TaiwanStockHoldingSharesPer`（業主決定何時升級）。

**升級後怎麼確認自動恢復（唯讀，勿動生產）**：

```bash
# 1) 任務是否成功（consecutive_failures 歸零、last_error 清空、interval=168h0m0s）
curl -s localhost:18080/api/dashboard/task-liveness \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); print([t for t in d["tasks"] if t["name"]=="auto_sbl_tdcc_history_backfill"])'
# 2) 歷史檔是否真的在長（cursor 前進的證據；目錄＝workDir/data/state/tdcc_dispersion，
#    見 internal/apigateway/register_adapters.go:407，容器內實際路徑以 compose 掛載為準）
docker exec atlas-go sh -c 'ls data/state/tdcc_dispersion | tail'
# 3) 不要再看到層級訊息
docker logs atlas-go --since 1h 2>&1 | grep -c "Your level is register"
```

**判讀陷阱**：`circuit breaker open` 是**共用** FinMind client 的狀態，可能由**別的** FinMind
消費者造成；看到它不等於本任務是元凶。以 `tdcc: history probe <日期>` 前綴認親。

## 7. 為什麼不用 Prometheus silence

1. 本告警**不是** Prometheus rule：`monitoring/rules/*.yml` 的 alert 名稱清單（實查 2026-10-01）
   沒有任何一條對應背景任務失敗；來源是 **in-app AlertStore**：
   `cmd/atlas/background_tasks.go:31-37` 的 failure handler 在 `consecutive_failures >= 3` 時
   `monitor.Alert(AlertLevelError, "background_task", …, {"task": name, "consecutive_failures": n})`。
   ⇒ 「在 rule 的 annotations 加 `known_issue` / `runbook_url`」這個形態**不適用**。
2. 加 silences（本 repo 已移除）或 `Alert.SuppressCategories`（`configs/parameters.json`）
   都會讓**未來真正的**失敗一起消失 ⇒ 屬「掩蓋，不是修復」（同 `check-channel-consistency` 的
   `governanceForbiddenRemedies` 用語）。
3. 改以「文件 ＋ 程式註解 ＋ 保留錯誤」達成可追溯：誰看到警報，都能沿著
   `alert → task 名 → 註冊處註解 → 本檔` 一路查到判定與恢復條件。

## 8. 待 root 落帳（建議文字；本 PR **未**修改這兩個檔）

> `docs/operations/remediation-manifest.md` 與 `docs/operations/FOLLOWUPS.md` 是 root 專屬寫者。

**FOLLOWUPS.md（新條目）**：

```markdown
### FU-20261001-NN — FinMind 層級不足 ⇒ `auto_sbl_tdcc_history_backfill` 的 TDCC 歷史段永遠失敗（已知，降頻為每週）

- **狀態**：`open`（等待業主升級 FinMind 層級）
- **記錄日期**：2026-10-01
- **處置**：業主裁定 ③ —— 不改程式修資料問題（SBL 不建置），只把探測頻率由 1h 降為 168h
  （`cmd/atlas/capital_tasks.go` 的 `sblTDCCHistoryProbeInterval`），並文件化為已知問題。
- **來源**：`docs/operations/finmind-tier-sbl-tdcc-backfill-followup.md`；
  生產 2026-10-01 `last_error=tdcc: history probe 2026-03-06: finmind: circuit breaker open`、
  `consecutive_failures=21`。
- **風險**：層級未升級期間，該任務的 cursor 停在 2026-03-01 之後的第一個週五 ⇒
  SBL/TDCC 的**歷史**檔缺該窗口之後的資料（live 通道不受影響）。
- **恢復條件**：FinMind 層級升級後，下一次探測成功即自動 resolve 警報、並自動恢復前進。
```

**remediation-manifest.md（E 類，明確錯誤清單）**：

```markdown
| E-NN | `auto_sbl_tdcc_history_backfill` 的 TDCC 歷史段被 FinMind 層級擋住（`Your level is register`），
1h tick 造成小時級重試（consecutive_failures=21）| `cmd/atlas/capital_tasks.go`（interval）、
`internal/marketdata/tdcc_provider.go:150`（探測點）| 待 root 落帳 | 降頻為 168h（已實作）；
帳號層級＝業主決策，非程式問題 |
```

## 9. 相關檔案

| 位置 | 角色 |
|---|---|
| `cmd/atlas/capital_tasks.go` | 任務註冊（interval ＋ 已知問題長註解） |
| `internal/marketdata/tdcc_provider.go:130-180` | 失敗點（`history probe`） |
| `internal/marketdata/finmind_client.go:720-734` / `:755` | 層級訊息的分類（`ErrQuotaExhausted`）／斷路器 |
| `cmd/atlas/background_tasks.go:31-37` | 警報來源（`AlertLevelError`, category `background_task`） |
| `cmd/atlas/main.go:1572` | 成功即 resolve 的路徑（升級後自動恢復） |
