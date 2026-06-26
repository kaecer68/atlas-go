# OpenCode + oh-my-openagent Token 注入防護指南

> **目的**：記錄 atlas-go 專案中 `oh-my-openagent` 插件的 hook 機制，並提供經過驗證的 token 注入防護配置。
> **基於**：原始碼分析（`~/.config/opencode/node_modules/oh-my-openagent/dist/index.js` v4.13.0）+ opencode v1.17.11。
> **最後更新**：2026-06-26。

## 為何需要這份文件

`oh-my-openagent`（透過 bun 安裝於 `~/.config/opencode/`）內建多個 hook 會在 chat/session 中**自動注入檔案內容**到 context。如果不主動管理，這些注入會：
- 每次會話吃掉 1,500-3,000 token（即使任務不需要）
- 累積 session 會持續增加 context window 壓力
- 導致後續 `git status` / `find` 等簡單操作都要付昂貴的 prefix token 成本

---

## 注入器完整清單（基於 `index.js:118127-118249`）

| Hook 名稱 | 預設狀態 | 觸發時機 | 影響 token |
|----------|---------|---------|-----------|
| `directory-agents-injector` | ⚠️ 自動 disable（opencode ≥ 1.1.37） | 每次 `read` tool 後，注入該檔案所在目錄的 AGENTS.md | 若啟用：**大**（取決於目錄深度）|
| `directory-readme-injector` | ⚠️ 預設啟用 | 每次 `read` tool 後，注入該檔案所在目錄的 README.md | 中 |
| `hephaestus-agents-md-injector` | ⚠️ 預設啟用 | **僅 hephaestus agent 觸發時**，從 root 向上找所有 AGENTS.md 注入 user message | **大**（最多 1-2 個，向上找）|
| `rules-injector` | ⚠️ 預設啟用 | 從 `.omo/rules/`, `.claude/rules/`, `.github/instructions/`, `~/.omo/rules/` 等注入 | **大**（多個來源）|
| `claude-code-hooks` | ⚠️ 預設啟用 | Claude Code 相容 hooks | 中 |
| `tool-output-truncator` | ✅ 預設啟用 | 截斷 tool 輸出（保護性）| ✅ 保護性 |
| `preemptive-compaction` | ✅ 預設啟用 | 預先壓縮 context | ✅ 保護性 |
| `anthropic-context-window-limit-recovery` | ✅ 預設啟用 | context 視窗救援 | ✅ 保護性 |
| `model-fallback` | ✅ 預設啟用 | model 備援 | ✅ 保護性 |
| `comment-checker` | 預設啟用 | 檢查 comments | 低 |
| `keyword-detector` | 預設啟用 | 偵測關鍵字觸發 mode | 低 |
| `session-notification` | 預設啟用 | session 通知 | 低 |
| `auto-update-checker` | 預設啟用 | 自動更新檢查 | 低 |
| `think-mode` | 預設啟用 | thinking mode 注入 | 中（inject thinking pattern）|
| `empty-task-response-detector` | 預設啟用 | 偵測空回應 | 低 |
| `todo-continuation-enforcer` | 預設啟用 | todo 繼續 | 中 |
| `non-interactive-env` | 預設啟用 | 非互動環境偵測 | 低 |
| `interactive-bash-session` | 預設啟用 | 互動 bash session | 低 |
| `keyword-detector` | 預設啟用 | 關鍵字偵測 | 低 |
| `category-skill-reminder` | 預設啟用 | category skill 提醒 | 低 |
| `codegraph-bootstrap` | 預設啟用 | codegraph 引導 | 中 |
| `ast-grep-sg-provision` | 預設啟用 | ast-grep 工具準備 | 低 |
| `agent-usage-reminder` | 預設啟用 | agent 使用提醒 | 低 |
| `tool-pair-validator` | 預設啟用 | 工具配對驗證 | 低 |
| `auto-slash-command` | 預設啟用 | 自動 slash command | 低 |
| `team-tool-gating` | 預設啟用（若 team_mode 啟用）| team 工具 gating | 低 |
| `team-mailbox-injector` | 預設啟用（若 team_mode 啟用）| team mailbox 注入 | 中 |
| `team-mode-status-injector` | 預設啟用（若 team_mode 啟用）| team 狀態注入 | 中 |
| `edit-error-recovery` | 預設啟用 | 編輯錯誤恢復 | 低 |
| `comment-checker` | 預設啟用 | comment 檢查 | 低 |
| `session-todo-status` | 預設啟用 | session todo 狀態 | 低 |
| `prometheus-md-only` | 預設啟用 | prometheus md 限制 | 低 |
| `sisyphus-junior-notepad` | 預設啟用 | junior notepad | 中 |
| `task-resume-info` | 預設啟用 | task resume 資訊 | 低 |
| `start-work` | 預設啟用 | start work 觸發 | 中 |
| `atlas` | 預設啟用 | atlas agent 鉤子 | 中 |
| `ralph-loop` | 預設啟用 | ralph loop | 中 |
| `no-sisyphus-gpt` | 預設啟用 | 禁用 sisyphus gpt | 低 |
| `no-hephaestus-non-gpt` | 預設啟用 | 禁用 hephaestus 非 gpt | 低 |
| `question-label-truncator` | 預設啟用 | question label 截斷 | 低 |

> **來源**：`index.js:152003` 的 `isHookEnabled` 邏輯很簡單：`!disabledHooks.has(hookName)`，所以**任何不在 `disabled_hooks` 的 hook 都預設啟用**。

---

## Truncator 機制

所有注入器共用 `createDynamicTruncator`（`index.js:23142-23148`），其行為：

- **預設上限**：`DEFAULT_TARGET_MAX_TOKENS = 50000`（`index.js:23120`）
- **動態調整**：`maxOutputTokens = min(usage.remainingTokens * 0.5, 50000)`（`index.js:23133`）
- **保留開頭**：`preserveHeaderLines = 3`（保留檔案前 3 行）
- **context 滿時**：`[Output suppressed - context window exhausted]`

**關鍵觀察**：truncator 只截斷到 50K token（動態），但**累積多個注入器**仍可能灌滿 context。

---

## 推薦配置

### `~/.config/opencode/oh-my-openagent.json`（使用者層級）

當前已禁用 `directory-readme-injector`：

```json
{
  "disabled_hooks": [
    "directory-readme-injector"
  ]
}
```

**建議**（基於 atlas-go 特性，**不要再加入新的 disabled_hooks**——保留「少即是多」原則）：

```json
{
  "disabled_hooks": [
    "directory-readme-injector"
  ]
}
```

> 為何不再加：當前 opencode 1.17.11 已自動 disable `directory-agents-injector`，其他 hooks 多為保護性（truncator、compaction、model-fallback）。**`hephaestus-agents-md-injector` 雖然會注入，但只向上找 1 個 AGENTS.md（root），且有 truncator 保護**——可接受。

### 專案層級（`AGENTS.md`）

**不要讓 `AGENTS.md:44-48` 列出 26 個 inline 連結**。AI 看到 inline 連結會傾向預先讀全部子檔案，造成**手動觸發的 token 浪費**（非 oh-my-openagent 觸發，是 AI 自主行為）。

**推薦**：

```markdown
## 模組路由

**所有 26 個模組的 AGENTS.md 路徑**：`internal/<mod>/AGENTS.md`

> 觸發條件：準備修改 `internal/<mod>/` 下任一檔案時，**先讀該目錄的 AGENTS.md**。
> 不要預先讀全部子 AGENTS.md——只在需要時讀取。
```

**預估效益**：避免 AI 在每個任務開始時主動讀 26 個子 AGENTS.md（1,500+ token）。

---

## 監控 token 注入

### 觀察指標

```bash
# 1. 看 opencode 啟動日誌（看哪些 hook 載入）
~/.config/opencode/bin/opencode --print-logs 2>&1 | grep -E "directory-agents|directory-readme|hephaestus-agents"

# 2. 看 context window 使用率
# 在 opencode session 中呼叫 hook: getContextWindowUsage (透過 truncator.getUsage)
```

### 觸發評估的時機

- 當會話 token 突然增加 1,000+ 但無明確 read → 可能是 `hephaestus-agents-md-injector` 觸發
- 當 `read` 後看到 AGENTS.md 內容出現在 output → `directory-agents-injector` 啟用（但 opencode ≥ 1.1.37 已自動關閉）
- 當 context window 達 80% → `preemptive-compaction` 應該自動觸發

---

## 專案層級可做的事

### 1. 精簡 `internal/*/AGENTS.md`（高 ROI）

- 目標：所有 `internal/*/AGENTS.md` < 80 行
- 現狀：Top 5 個仍 > 100 行（`llm` 184, `narrative` 146, `monitoring` 114, `live` 111, `portfolio` 110）
- 預估效益：每次觸發 hephaestus 時少注入 200-400 token

### 2. 精簡 `.github/instructions/*.md`

- 當前：`go-core.instructions.md` 103 行、`experiments-guardrails.instructions.md` 62 行、`live-trading.guardrails.instructions.md` 43 行
- 預估效益：每次會話少注入 200-400 token

### 3. 精簡根 `AGENTS.md`

- 當前：99 行（合規 < 160 行）
- 移除 inline 連結（保留純文字模組名）

### 4. 評估 `hephaestus-agents-md-injector` 是否值得 disable

- **若 atlas-go 工作流程以 `sisyphus` 為主**：hephaestus 觸發頻率低，保留可接受
- **若主要用 hephaestus**：考慮 disable，但會失去「自動注入 project context」便利

---

## 升級時的注意事項

當 `oh-my-openagent` 升級（autoupdate: true）時：

1. 檢查新版本是否有新的注入器
2. 重新跑 `index.js:118127-118249` 的 hook 清單
3. 更新本文件

當 opencode 升級時：

1. 檢查 `OPENCODE_NATIVE_AGENTS_INJECTION_VERSION`（`index.js:24185`）是否變動
2. 確認 `directory-agents-injector` 仍自動 disable

---

## 相關文件

- `docs/DOCUMENTATION_STANDARD.md` — 文件存放規範
- `docs/guides/new-workspace-startup.md` — 工作區起步 SOP
- `AGENTS.md` — 專案入口（建議精簡模組路由表）
- `~/.config/opencode/oh-my-openagent.json` — oh-my-openagent 使用者設定
- `~/.config/opencode/opencode.json` — opencode 全域設定
