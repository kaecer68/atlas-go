# Security Audit Checklist

> **文件角色**:資安稽核流程規格,定義 atlas-go 的稽核節奏、範疇與具體檢查項。
> **適用對象**:CI 自動檢查、版本發布前稽核、人工定期稽核。
> **權威來源**:本文件僅作為索引與檢查清單,所有資安控制細節以引用之 canonical 文件為準。

## Purpose

atlas-go 採取 simulation-first 與 audit-driven 架構,但目前缺少正式稽核節奏。本文件建立五層稽核節奏(per PR / 每周 / 每月 / 每季 / 每版本),定義九大資安範疇,並對應到既有 canonical 控制項(SECURITY.md、apigateway/constitution.md、live-trading.guardrails.instructions.md 等),讓後續每一次稽核有可重複、可驗證的工作底稿。本文件不重述各 canonical 控制項的具體內容,僅作為索引與檢查清單。

## Cadence

| Tier | 頻率 | 觸發 | 執行者 | 範圍 | 產出 |
|------|------|------|--------|------|------|
| 1 | 每 PR | `pull_request` 事件 | CI 自動 | gosec、golangci-lint、apigateway 憲法檢查、constitution 違規掃描 | PR check 結果 |
| 2 | 每周 | 排程(`vuln-scan.yml` 預定於 PR #818 新增) | CI 自動 | govulncheck Go module CVE 掃描 | SARIF 報告 + workflow artifact |
| 3 | 每月 | 月初第一個工作天 | 維護者人工 | 新增 dependency、新增 env var、新增 LLM provider/能力 | 短期稽核紀錄(PR comment 或 issue) |
| 4 | 每季 | 季度初 | 維護者人工 | 本文件九大範疇全項檢查 | `.omo/audit/<YYYY-MM-DD>-quarterly-security.md` |
| 5 | 每版本 | 版本標記(`v*` tag push)觸發;維護者於 release 前手動執行 | 維護者人工 | go.mod 變更、env var 變更、apigateway 憲法合規、live flag 預設值 | 發布公告 + `.omo/audit/<YYYY-MM-DD>-release-pre-audit.md` |

> Tier 1 與 Tier 2 屬自動化控制,失敗即阻擋合併或標記告警;Tier 3 至 Tier 5 屬人工稽核(包含 Tier 5 雖由 release 事件觸發但仍由維護者手動執行),產出可追溯的稽核紀錄。Tier 5 目前無自動化 release-time audit step(僅有 release event trigger,需待 release job 啟用後整合),執行方式為維護者於版本標記前手動套用本 checklist。

## Scope Categories

九大資安範疇對應 atlas-go 現行所有 canonical 資安控制項:

1. **Secrets and credentials management**: API key、broker 帳號、Keychain 整合。
2. **Data source governance**: apigateway 憲法(6 條文 + 3 附錄)、限流、熔斷、背景任務註冊。
3. **Live trading guardrails**: `-allow-live-broker` 旗標、風險過濾順序、fail-safe 行為。
4. **LLM provider security**: API key 治理、ProviderImpl、能力路由、DataClass 閘門。
5. **Static analysis and linting**: gosec、golangci-lint、staticcheck、go vet。
6. **Dependency vulnerability scanning**: govulncheck、go.mod 直接依賴檢視。
7. **Data visibility safeguards**: 4 層零值防護(ChannelErrors、FetchResult.Fallback、MacroDataSnapshot)。
8. **Configuration and environment management**: env var 白名單、ParametersConfig、Keychain 整合。
9. **Documentation currency**: SECURITY.md、apigateway/constitution.md、AGENTS.md、traps.md 同步狀態。

## Per-Audit Checklist

### 1. Secrets and credentials management

- [ ] `.env` 未被 commit(`.gitignore` 已含 `.env`,且 `git ls-files .env` 無輸出)。
- [ ] `git log --all -p -- .env` 無歷史紀錄。
- [ ] 生產環境 secret 透過 macOS Keychain 讀取(以 `security find-generic-password` 驗證)。
- [ ] CI/CD secret 未硬編碼於 workflow YAML,僅透過 `secrets.*` 引用。
- [ ] 新增的 API key(例如 FinMind、Fugle、TEJ、Fubon)均已登錄於 `SECURITY.md` 的 API Keys 段落。
- [ ] 至少每季輪換一次 `LLM_DEEPSEEK_API_KEY` 與 `LLM_MINIMAX_API_KEY`。
- [ ] SARIF 報告中 `gosec` G101(硬編碼 credential)為 0 命中。
- [ ] `git secrets --scan-history`(若已安裝)無命中。

### 2. Data source governance

- [ ] `apigateway.Fetch(channelID)` 為唯一外部 HTTP 進入點,無程式碼繞過(以 constitution check script 驗證)。
- [ ] `grep -r "os.Getenv" --include="*.go" .` 之命中皆在 `configs/allowed_env_vars.md` 白名單內。
- [ ] 每個新 Channel 在 `internal/apigateway/limits.go` 中定義限流常數,符合 1 req / 5s 預設值或顯式偏離理由。
- [ ] 共享端點(例如 us_yahoo 與 frankfurter_fx 雖同源但應獨立)使用正確的 limiter 實例,不重複建立。
- [ ] 所有背景任務透過 `BackgroundTaskManager.Register()` 註冊,無 `go func()` 直接呼叫 `New*Provider()`(參考 4.5.2 允許例外表)。
- [ ] 健康檢查透過 `UnifiedHealthStore.Record()` 寫入,無直接操作 `channel_health.json`。
- [ ] 附錄 A 的 15 個通道清單與 `apigateway.ChannelRegistry` 註冊表一致。
- [ ] 憲法 v1.0 生效以來無新增未登記的 Channel。

### 3. Live trading guardrails

- [ ] `internal/live/AGENTS.md` 的 ANTI-PATTERNS 段未被任何 PR 違反。
- [ ] `-allow-live-broker` 旗標預設為關閉,且所有測試在未啟用此旗標下通過(`go test ./internal/live/...`)。
- [ ] 風險過濾(replay → control → risk → execute)的執行順序在程式碼中可稽核(透過 `trace_path` 驗證)。
- [ ] 市場資料缺失時 fail-safe 行為已實作(`docs/reference/traps.md` 的 live trading 安全旗標陷阱)。
- [ ] `cmd/experimental/validate-broker` 簽名驗證在每次 broker client 變更後重新執行。
- [ ] live orchestration 變更 PR 同時更新 `.github/instructions/live-trading.guardrails.instructions.md`(若守則變更)。
- [ ] 至少一次 smoke check:`go run ./cmd/atlas` 在非 live 模式下啟動成功。

### 4. LLM provider security

- [ ] 所有 LLM 呼叫透過 `DefaultRouter`,無程式碼直接呼叫 `clients/*Provider`(AGENTS.md 高頻陷阱表)。
- [ ] 新增 LLM 能力已註冊於 capability constant 與 routing table,無 hardcoded capability 字串。
- [ ] `LLM_DEEPSEEK_API_KEY` 與 `LLM_MINIMAX_API_KEY` 透過環境變數讀取,無硬編碼。
- [ ] `LLM_*_ENABLED` 系列 feature flag 預設值為 false,需顯式啟用。
- [ ] DataClass 閘門邏輯正確:Regulated data class 透過 `LLM_MINIMAX_API_KEY` (coding plan key) 時被閘門 skip。
- [ ] `/api/llm/health` endpoint 正常回傳所有 provider 狀態。
- [ ] router 版本字串(`router_version`)隨每次 routing table 變更遞增。
- [ ] LLM Provider 變更 PR 同時更新 `docs/specs/llm-routing-spec.md` 與 capability 常數表。

### 5. Static analysis and linting

- [ ] `golangci-lint` 在 CI 通過(`.github/workflows/ci-cd.yml` 的 lint job)。
- [ ] `staticcheck ./...` 0 警告。
- [ ] `go vet ./...` 0 警告。
- [ ] `gosec` SARIF 報告 0 high/critical 命中(`results.sarif` 上傳於 security job)。
- [ ] `test -z "$(gofmt -l .)"` 為空(無格式違規)。
- [ ] 覆蓋率 `go tool cover -func=coverage.out | grep total` ≥ 60%。
- [ ] 新增的 `internal/live/`、`internal/orchestrator/` 程式碼有對應的 `*_test.go`。

### 6. Dependency vulnerability scanning

- [ ] `vuln-scan.yml` workflow(預定於 PR #818 新增)最近一次執行無高風險 CVE 命中。
- [ ] `go.mod` 與 `go.sum` 一致,無未鎖定版本(`go mod tidy` 無變動)。
- [ ] 直接依賴(direct dependencies)的 Go module 數量與上次稽核相比無異常增加(>20% 變動需人工覆核)。
- [ ] 所有依賴均為公開 module,無私有/內部 registry(`go env GOPROXY` 驗證)。
- [ ] 至少一個月內已執行 `go list -m -u all` 檢查可用更新。
- [ ] 若 govulncheck 命中 CVE,於 7 天內修復或建立追蹤 issue。

### 7. Data visibility safeguards

- [ ] 所有 `apigateway.Fetch` 回傳 `FetchMetadata`(含 `latency_ms`、`rate_limit_remaining`、`timestamp`)。
- [ ] MacroDataSnapshot 卡片有 `status.data_status` 標記,無零值隱藏失敗(主要引用 `.claude/skills/atlas-data-visibility/SKILL.md`;若 `docs/specs/<data-visibility>.md` 存在則優先引用)。
- [ ] `FetchResult.Fallback` 機制在 cache miss 時正常運作。
- [ ] 零值卡片(`ChannelErrors` 為空但 channel 失敗)於前端 UI 顯示明確錯誤訊息,非靜默成功。
- [ ] 至少一次人工驗證:故意關閉一個 data source,觀察 UI 是否顯示 fallback 狀態而非零值。
- [ ] Circuit breaker 半開探測正確觸發(`apigateway/constitution.md` 第五條)。

### 8. Configuration and environment management

- [ ] 所有 env var 在 `configs/allowed_env_vars.md` 白名單內(白名單目前含 `ATLAS_API_KEY`、`ATLAS_STATE_DIR`、`ATLAS_WORK_DIR`,以及其他經過憲法第一條 1.2 節程序新增的變數)。
- [ ] 新增 env var 已更新 `docs/environment.md` 與 AGENTS.md 高頻陷阱表(若有)。
- [ ] `config.Load()` 正確讀取 `.env` 與 `~/.config/atlas-go/.env`(雙路徑)。
- [ ] `ParametersConfig` 為唯一參數管理入口,無 hardcoded magic number(參考 `docs/reference/parameter-system.md`)。
- [ ] Docker compose 環境變數與本地 `.env` 同步,無單邊修改。
- [ ] 生產資料庫連線使用 `sslmode=require`(本地開發可用 `disable`,但生產部署必須修正)。
- [ ] Grafana 預設 admin 密碼已在生產環境變更(SECURITY.md 已知限制)。

### 9. Documentation currency

- [ ] `SECURITY.md` 的「Previous Security Audits」段包含最近一次稽核紀錄(YYYY-MM-DD 條目)。
- [ ] `SECURITY.md` 涵蓋:Secret Management、Dependency Scanning、Data Source Governance、Live Trading Guardrails、LLM Providers、Data Visibility Safeguards 六個段落(PR #817 refresh 後;若未合併則以當前版本為準)。
- [ ] `internal/apigateway/CONSTITUTION.md` 版本號(v1.0+)與「修訂歷史」附錄 C 一致。
- [ ] AGENTS.md 高頻陷阱表含「資安設定」一列,指向 SECURITY.md 與 apigateway/constitution.md(PR #817 新增;若未合併則此項標 N/A)。
- [ ] `docs/reference/traps.md` 包含 live trading 安全旗標陷阱與 apigateway 憲法違反陷阱。
- [ ] 本文件(`docs/specs/security-audit-spec.md`)的引用路徑與實際 canonical 檔案位置一致。
- [ ] 30 天以上未引用的文件已評估是否刪除或移入 .omo/audit/（依 documentation-standard.md 生命週期）。

## Audit Output

每次 Tier 3 以上的人工稽核應產出下列產物:

### Tier 3(月度)

於對應維護議題或 PR comment 留底,格式自由,僅需包含:

- 稽核日期、執行者
- 本文件九大範疇中觸及項目之檢查結果(yes/no/N/A)
- 新發現的 P0/P1/P2 問題
- 後續追蹤項目(owner + due date)

### Tier 4(季度)與 Tier 5(發布前)

於 `.omo/audit/YYYY-MM-DD-<slug>.md` 建立完整報告,例如 `.omo/audit/<YYYY-MM-DD>-quarterly-security.md` 或 `.omo/audit/<YYYY-MM-DD>-<version>-release-pre-audit.md`)。報告結構建議:

```markdown
# Security Audit Report: <tier 與期間>

## 範圍
本次稽核涵蓋的 Tier 與範疇清單。

## 結果總覽
各範疇通過 / 部分通過 / 不通過 統計。

## 詳細發現
依嚴重性(P0 / P1 / P2)列出,每項含:
- 描述
- 證據(grep 輸出、SARIF 引用、commit hash)
- 建議處置
- 負責人與 due date

## 後續追蹤
未於本次關閉的項目,移至後續季度或建立 issue。
```

> 季度報告需於 `SECURITY.md` 的「Previous Security Audits」段新增對應條目。

## References

本文件為索引性質,所有資安控制細節以下列 canonical 文件為唯一權威來源:

| 主題 | 權威來源 |
|------|---------|
| 整體資安政策 | [`SECURITY.md`](../../SECURITY.md) |
| 數據源治理憲法 | [`internal/apigateway/CONSTITUTION.md`](../../internal/apigateway/CONSTITUTION.md) |
| Live trading 守則 | [`.github/instructions/live-trading.guardrails.instructions.md`](../../.github/instructions/live-trading.guardrails.instructions.md) |
| Go 程式碼守則 | [`.github/instructions/go-core.instructions.md`](../../.github/instructions/go-core.instructions.md) |
| 高頻陷阱 | [`AGENTS.md`](../../AGENTS.md) § 高頻陷阱速查 |
| 跨模組陷阱完整版 | [`docs/reference/traps.md`](../reference/traps.md) |
| 環境變數管理 | [`docs/environment.md`](../environment.md) |
| 參數管理系統 | [`docs/reference/parameter-system.md`](../reference/parameter-system.md) |
| LLM 路由規格 | [`docs/specs/llm-routing-spec.md`](llm-routing-spec.md) |
| 數據可見性四層防護 | [`.claude/skills/atlas-data-visibility/SKILL.md`](../../.claude/skills/atlas-data-visibility/SKILL.md) |
| CI/CD 安全掃描 (gosec) | [`.github/workflows/ci-cd.yml`](../../.github/workflows/ci-cd.yml) § security job |
| CI 憲法檢查 | [`.github/workflows/constitution.yml`](../../.github/workflows/constitution.yml) |
| 依賴漏洞掃描 (govulncheck) | `.github/workflows/vuln-scan.yml` (PR #818 新增) |
| env var 白名單 | [`configs/allowed_env_vars.md`](../../configs/allowed_env_vars.md) |
| 文件歸屬規範 | [`docs/documentation-standard.md`](../documentation-standard.md) |
| 文件位置地圖 | [`docs/documentation-map.md`](../documentation-map.md) |
