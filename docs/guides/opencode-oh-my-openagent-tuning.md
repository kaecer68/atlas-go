# OpenCode + oh-my-openagent Token 注入防護指南

> **目的**：記錄 atlas-go 專案中 `oh-my-openagent` 插件的 hook 機制，並提供經過驗證的 token 注入防護配置。
> **基於**：原始碼分析（`~/.config/opencode/node_modules/oh-my-openagent/dist/index.js` v4.13.0）+ ACP（Active Context Pruning）1.4.1。
> **最後更新**：2026-06-27。

---

## 為何需要這份文件

`oh-my-openagent`（透過 bun 安裝於 `~/.config/opencode/`）內建多個 hook 會在 chat/session 中**自動注入檔案內容**到 context。如果不主動管理，這些注入會：
- 每次會話吃掉 1,500-3,000 token（即使任務不需要）
- 累積 session 會持續增加 context window 壓力
- 導致後續 `git status` / `find` 等簡單操作都要付昂貴的 prefix token 成本

**本文件的配置策略**：由 ACP（Active Context Pruning）全面接管 context 管理，關閉 oh-my-openagent 內建的壓縮機制以避免衝突。

---

## 注入器完整清單（基於 `index.js:118127-118249`）

| Hook 名稱 | 預設狀態 | 觸發時機 | 影響 token |
|----------|---------|---------|-----------|
| `directory-agents-injector` | ⚠️ 自動 disable（opencode ≥ 1.1.37） | 每次 `read` tool 後 | **大** |
| `directory-readme-injector` | ⚠️ 預設啟用 | 每次 `read` tool 後 | 中 |
| `hephaestus-agents-md-injector` | ⚠️ 預設啟用 | 僅 hephaestus agent 觸發 | **大** |
| `rules-injector` | ⚠️ 預設啟用 | 從 `.omo/rules/` 等注入 | **大** |
| `claude-code-hooks` | ⚠️ 預設啟用 | Claude Code 相容 hooks | 中 |
| `tool-output-truncator` | ✅ 預設啟用 | 截斷 tool 輸出（保護性）| ✅ 保護性 |
| `preemptive-compaction` | ⚠️ **建議關閉**(與 ACP 衝突) | 78% 自動 summarize | 與 ACP 衝突 |
| `anthropic-context-window-limit-recovery` | ✅ 預設啟用 | context 視窗救援 | ✅ 保護性 |
| `model-fallback` | ✅ 預設啟用 | model 備援 | ✅ 保護性 |
| 其餘（comment-checker, keyword-detector 等） | 預設啟用 | 各自功能 | 低 |

> **來源**：`index.js:152003` 的 `isHookEnabled` 邏輯：`!disabledHooks.has(hookName)`，所以**任何不在 `disabled_hooks` 的 hook 都預設啟用**。

---

## Compaction 機制歷史與衝突分析

### DCP（Dynamic Context Pruning）— oh-my-openagent 內建

oh-my-openagent 內建的 `dynamic_context_pruning`（DCP）源自上游 DCP，但**未包含後續的 37 個 bug 修復**：
- CRITICAL：State 跨 restart 未持久化 → 壓縮狀態遺失
- CRITICAL：`resetOnCompaction()` 清除所有壓縮區塊 → 所有壓縮工作被撤銷
- CRITICAL：`getCurrentTokenUsage` 回傳 0 → nudge 永遠不觸發
- HIGH：Context window leak → 壓縮訊息 reappear
- HIGH：Compression 發出後 model 停止回應

### Preemptive-compaction

oh-my-openagent 在 `index.js:102308` 寫死閾值 `PREEMPTIVE_COMPACTION_THRESHOLD = 0.78`（78%）。行為：
- 當 context 使用率達 78% **且** 有已完成的 summarize → 自動觸發壓縮
- 78% 閾值對 1M context window 來說約 780K token 才觸發 — **為時已晚**
- 不可逆：壓縮後無法 decompress
- 與 ACP 直接衝突：ACP 自主管理壓縮，preemptive 會覆蓋 ACP 的決策

### ACP（Active Context Pruning）— 推薦方案

ACP 1.4.1 是 DCP 的 hardened fork，包含 **37 個 bug 修復**，採用**模型自主壓縮**機制：

| 特性 | DCP / preemptive | ACP |
|------|-----------------|-----|
| 觸發方式 | 硬閾值（78% / 95%） | **模型自主決策**（45-55% 提醒，60-90% 分批處理）|
| 可恢復性 | ❌ 不可逆 | ✅ `decompress` 可還原 |
| GC 策略 | 無 | **3-tier batch cleanup**（60%/75%/90%）|
| Per-model 限制 | ❌ 無 | ✅ `modelMaxLimits` / `modelMinLimits` |
| 保護 user prompt | ❌ | ✅ `protectUserMessages` |
| 定期提醒 | ❌ | ✅ `nudgeFrequency: 5` |
| Prompt cache 命中率 | 低（壓縮後全重算）| **~87%** |
| 典型 context 使用率 | 78-95% | **p50 < 1%, p95 ~30%** |

> **實測數據**（來自 ACP 官方）：582M tokens / 3,024 messages / p95 context 25% / 86.2% cache hit。

---

## v3 推薦配置（ACP 方案）

### 1. oh-my-openagent.json — 關閉衝突設定

```jsonc
{
  "disabled_hooks": [
    "directory-readme-injector"
  ],
  "experimental": {
    // ACP 全面接管壓縮，oh-my-openagent 不參與
    "preemptive_compaction": false,
    "dynamic_context_pruning": {
      "enabled": false
    }
  }
}
```

**為什麼關閉**：
- `dynamic_context_pruning.enabled: false` → ACP 接管所有 context 管理
- `preemptive_compaction: false` → 避免 78% 閾值衝突（ACP 在 45-55% 就開始提醒）
- `disabled_hooks` 僅保留 `directory-readme-injector`（opencode ≥ 1.1.37 已自動 disable directory-agents-injector）

### 2. opencode.json — 關閉原生 auto-compaction

```jsonc
{
  "compaction": {
    "auto": false
  }
}
```

> **ACP 官方要求**：OpenCode 原生 compaction 與 ACP 衝突，會導致「重新展開已壓縮訊息」、「壓縮狀態丟失」等問題。

### 3. ACP 安裝（尚未安裝）

```bash
opencode plugin install opencode-acp@latest --global
```

### 4. acp.jsonc — ACP 壓縮策略

```jsonc
{
  "$schema": "https://raw.githubusercontent.com/ranxianglei/opencode-acp/master/dcp.schema.json",
  "enabled": true,
  "autoUpdate": true,
  "pruneNotification": "minimal",
  "pruneNotificationType": "chat",
  "commands": {
    "enabled": true,
    "protectedTools": []
  },
  "turnProtection": {
    "enabled": true,
    "turns": 4
  },
  "compress": {
    "mode": "range",
    "permission": "allow",
    "showCompression": true,
    "summaryBuffer": true,
    // DeepSeek v4 Pro 128K context → 55% ≈ 70K soft upper
    "maxContextLimit": "55%",
    // 45% ≈ 58K soft lower
    "minContextLimit": "45%",
    // 每 5 次 fetch 提醒一次壓縮
    "nudgeFrequency": 5,
    // 15 messages 後開始 iteration nudge
    "iterationNudgeThreshold": 15,
    // soft = 模型自主決定，不強制
    "nudgeForce": "soft",
    // 保護這些 tool 的輸出不被壓縮
    "protectedTools": [
      "task",
      "skill",
      "todowrite",
      "todoread",
      "decompress"
    ],
    // 保護 user 的 messages 不被壓縮
    "protectUserMessages": true
  },
  "strategies": {
    "deduplication": {
      "enabled": true,
      "protectedTools": []
    },
    "purgeErrors": {
      "enabled": true,
      "turns": 6,
      "protectedTools": []
    }
  },
  "gc": {
    "algorithm": "truncate",
    "promotionThreshold": 5,
    "maxBlockAge": 15,
    "maxOldGenSummaryLength": 3000,
    "majorGcThresholdPercent": "100%",
    "batchCleanup": {
      "lowThreshold": "60%",
      "highThreshold": "75%",
      "forceThreshold": "90%"
    }
  }
}
```

**配置邏輯**：
- **`maxContextLimit: "55%"`** = DeepSeek 128K → ~70K，軟上限，超過後 ACP 持續提醒壓縮
- **`minContextLimit: "45%"`** = DeepSeek 128K → ~58K，低於此不提醒
- **`nudgeFrequency: 5`** = 每 5 次 fetch 提醒一次，不過度
- **`protectUserMessages: true`** = 使用者貼的長 prompt 不會被壓縮
- **`turnProtection.turns: 4`** = 最近 4 輪 messages 受保護
- **`purgeErrors.turns: 6`** = 失敗 tool 保留 6 輪後才清理

---

## 遷移路徑（DCP → ACP）

### 從 v2（DCP + preemptive）遷移到 v3（ACP）

1. 安裝 ACP：`opencode plugin install opencode-acp@latest --global`
2. 更新 `oh-my-openagent.json`：關閉 `preemptive_compaction` 和 `dynamic_context_pruning`
3. 更新 `opencode.json`：加入 `"compaction": { "auto": false }`
4. 刪除 DCP 殘留：`rm -rf ~/.local/share/opencode/storage/plugin/dcp/`
5. 寫入 ACP 設定：`~/.config/opencode/acp.jsonc`
6. 重啟 opencode
7. 驗證：執行 `/acp context` 和 `/acp stats`

### 回退（ACP → DCP）

1. 移除 ACP plugin
2. 還原 `oh-my-openagent.json` 中的 `preemptive_compaction: true, dynamic_context_pruning.enabled: true`
3. 移除 `opencode.json` 的 `compaction.auto: false`
4. 刪除 ACP storage：`rm -rf ~/.local/share/opencode/storage/plugin/acp/`
5. 重啟 opencode

---

## 監控與驗證

### ACP 內建命令

| 命令 | 用途 |
|------|------|
| `/acp context` | Token 用量細項（system/user/assistant/tools）+ 節省量 |
| `/acp stats` | 跨 session 壓縮統計 |
| `/acp sweep [n]` | 手動壓縮最後 n 個 tool |
| `/acp manual on/off` | 切換手動模式 |

### SQLite 監控

```bash
sqlite3 ~/.local/share/opencode/opencode.db -header -column "
SELECT slug, name, created_at, input_tokens + output_tokens as total_tokens
FROM sessions
WHERE input_tokens > 0
ORDER BY total_tokens DESC
LIMIT 10;"
```

### ACP 日誌

```bash
ls ~/.config/opencode/logs/acp/
# 或啟用 debug: acp.jsonc → "debug": true
```

---

## 相關文件

- `~/.config/opencode/oh-my-openagent.json` — oh-my-openagent 使用者設定
- `~/.config/opencode/acp.jsonc` — ACP 壓縮設定
- `~/.config/opencode/opencode.json` — opencode 全域設定
- `docs/documentation-map.md` — 文件地圖
- `https://github.com/ranxianglei/opencode-acp` — ACP GitHub
