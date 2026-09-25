# docs/operations/FOLLOWUPS.md — 待辦 / 已知限制追蹤

> **用途**：記錄 atlas-go 在調查、修法或部署過程中**已知但刻意延後**的項目
> （含「已知限制」與「只建議未實作」的硬化方向），避免只存在於 PR 說明或 agent 回報中而流失。
>
> **不是**：缺陷主題的完整規格（放 `docs/` canonical 文件）、
> 也不是 channel 級長期故障（那放 `internal/monitoring/known_issues.go`）。
>
> **格式**：每個條目一個 `###`；`狀態` 只允許 `open` / `in-progress` / `done`。
> 完成時**不刪除**條目，改成 `done` 並補 `完成於`（保留決策痕跡）。
> 來源一律附可重現的命令或檔案:行號。

---

## 遺留項目

### FU-20260925-01 — `symbolIndustryCoverage.Coverage()` 只反映最近一次成功 Reload（TTL 6h）⇒ 靜默陳舊

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **為何放在 `docs/operations/` 而非 repo 根目錄**：atlas-go 原本**沒有** repo 根目錄的
  `FOLLOWUPS.md`（該路徑是 **a2a-dev** 的跨 repo 慣例）；且本 repo 的 `docs/` 根目錄受
  `scripts/ci/check_docs_governance.sh` 治理（根目錄新增未追蹤 `.md` 會被 CI 擋下），
  而 `docs/operations/README.md` 自述本目錄即為「操作 runbook 與**事件後續追蹤**文件」。
  ⇒ 依本 repo 慣例改放此處；若日後要與 a2a-dev 對齊，需一併更新該 governance 白名單與文件地圖。
- **來源**：PR #1983（`fix/universe-coverage-substrate`，**已於 2026-09-25 合併** squash commit `cd1f5b02`）+ 本 repo
  `docs/operations/universe-scoring-ranked-zero-20260925.md` §9.3
- **現況**：`cmd/atlas/symbol_industry_substrate.go` 的 `storeSymbolIndustrySubstrate.Coverage()`
  （新增，供 `monitoring.CheckUniverseCoverage` 的 `industry.SymbolIndustryCoverageReporter`
  型別斷言使用）讀的是記憶體視圖，只在 `Reload` **成功**時更新，TTL `substrateReloadTTL = 6h`。
- **風險**：store 在最後一次成功 Reload **之後**才壞掉時，覆蓋率稽核會**沿用舊視圖**並回報
  看似正常的比率，直到下一個 Reload 週期才可能改變。即「檢查回報有東西，而不是東西對不對」——
  與 #1944 / #1953 / 本 session 的 false-green 主題同族。目前亦**無法區分**
  「載入失敗」與「載入成功但母體為空」。
- **影響面**：`universe_scheduler` 的 `coverage_check`（daily / weekly）與其告警；
  不影響評分或排序（稽核路徑 only）。
- **最小修法建議（只建議，未實作）**：
  1. `Coverage()` 回傳附 **`as_of`**（最近一次成功 Reload 的時間戳），讓稽核能揭露資料年齡；
  2. 告警規則增加「substrate 最近成功 Reload 超過 N 小時」
     （需與 `universe-scoring-ranked-zero-20260925.md` §6.2 同組；注意 §7.2 的
     `atlas_universe_*` counter 灌爆問題，優先改用 gauge）；
  3. substrate 曝露 `last_reload_ok` / `load_error`，區分「載入失敗」與「母體為空」。
- **驗收條件**：稽核輸出可看到資料年齡；且「store 壞掉」在一個 Reload TTL 內會被告警指出。

### FU-20260925-02 — 舊明文 DB 密碼已從現行檔案移除，但**輪替**仍待業主決定（另有 2 個檔案未動）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：任務 Q（分支 `fix/secrets-and-monitoring-guard-q`）。
  **已移除明文的現行檔案**：`docs/operations/docker-compose.prod.yml`（`DATABASE_URL` / `POSTGRES_PASSWORD`）、
  `docs/operations/docker-compose.crons.yml`（`DATABASE_URL` ×4）、
  `.claude/skills/atlas-imac-prod-guard/SKILL.md`（復原 SOP 內的字面值）。
  改法＝一律從 compose 插值／env 檔取值；未提供值時 `${VAR:?...}` **直接讓 `docker compose` 失敗**（fail-closed），
  不會靜默用錯值。防再犯＝`scripts/secret-scan.sh` 新增 3 個通用憑證樣式 +
  「可部署設定檔（`*.yml`/`*.yaml`）不降級為 warn-only」，並由
  `scripts/ci/check_secrets.sh`（CI job `secret-scan`）與 `make ci-static` 把關。
- **為何仍需輪替**：舊值**已在 git 歷史中**（本 repo 為 PUBLIC ⇒ 永久可見）。
  「現行檔案不再含明文」**不等於**「憑證安全」；輪替是唯一補救，且屬**業主決定**（見任務 Q 授權範圍）。
- **仍含同一組明文的現行檔案（本次**未動**，超出授權檔清單）**：
  `tasks/misleading-mechanisms-fix-plan-ds4pro-20260828.md`、
  `tasks/stockpicker-misleading-mechanisms-audit-k3-20260828.md`（各 1 行）。
  兩者皆為 `.md` ⇒ 在 secret-scan 屬 **warn-only**（不擋 CI）；建議與輪替一併處置，或明確標為歷史封存。
- **附帶發現（a2a-dev，非本 repo）**：新的 DSN 樣式會在 a2a-dev 的 `docs/operations/`（舊 iMac
  runbook）、`docs/audits/`、`docs/governance/reports/` 等文件命中（warn-only，不擋 CI），
  是否為真憑證需人工確認 ⇒ 已回報上層，未在本次動任何 a2a-dev 文件。
- **驗收條件**：業主回覆「已輪替」或「不輪替（接受風險）」並補記於此；`tasks/*.md` 的處置一併決定。

---

---

## 已定案的判準（決策紀錄）

### 2026-09-25 — secret-scan 的 block/warn 分界與降噪原則（任務 Q；業主明確背書）

- **可部署設定檔（`*.yml`/`*.yaml`，非 `*.example*`/`*sample*`/`*template*`）→ block 級，即使路徑在 `docs/` 底下。**
  理由（因果）：`docs/operations/*.yml` 是**會被 `docker compose` 拿去跑**的設定，不是散文；
  在那裡出現字面憑證就是真外洩 —— 本次外洩（`docker-compose.prod.yml` 的明文 DB 密碼）
  能長期存活，正是因為它當時落在 warn-only 類別。
- **散文 `.md`、範例／模板 → 維持 warn-only。**
  理由（反脆弱）：把散文升成 block 只會逼人不停加 allowlist；**allowlist 一多，護欄就會被繞過**。
- **降噪設計刻意不排除 `host.docker.internal`**：那正是 prod DSN 的主機，必須保持會被抓到。
  （排除的是 RFC 2606 保留域名／本機位址／placeholder 字／程式碼取值／純字母且 <16 字的假 key／路徑型 env 預設值。）
- **`check_monitoring_single_source.py` 的 R2 用行掃描、不引入 YAML 依賴**：GitHub runner 不保證有 PyYAML；
  限制已明列於該檔檔頭，且生產外側仍有 a2a-dev `scripts/drift-check.sh [9/9]`（`docker inspect`）作為第二道。

## 相關文件

- [universe-scoring-ranked-zero-20260925.md](universe-scoring-ranked-zero-20260925.md)
  — SmartUniverseBuilder `symbols_ranked=0` 根因報告（含 §6.2 缺口與 §8 防再犯檢查建議）
- [README.md](README.md) — 本目錄索引
- [local-deploy.md](local-deploy.md) — 部署與 `.env` 分工（prod DSN 為何不放 `.env`）
