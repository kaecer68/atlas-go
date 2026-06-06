# AGENTS.md — internal/spawning

**成熟度**: evolving
**模組職責**: 自動偵測系統知識缺口並生成新 Agent，管理 Agent 生命週期（生成、訓練、驗證、淘汰）。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `SpawningManager` | `spawning_manager.go` | 協調整個生成生命週期，定時執行 spawning cycle |
| `GapDetector` | `gap_detector.go` | 分析 Agent 覆蓋率與績效，偵測知識缺口 |
| `AgentFactory` | `agent_factory.go` | 根據缺口類型產生 AgentSpec 與對應 prompt |
| `KnowledgeGap` | `gap_detector.go` | 缺口定義（類型、嚴重程度、相關產業/風格） |
| `SpawnedAgent` | `gap_detector.go` | 追蹤生成 Agent 的狀態（training → validating → candidate → accepted/rejected/extinct） |
| `SpawningConfig` | `spawning_manager.go` | 配置：最大並行生成數、訓練天數、驗證門檻、淘汰天數 |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **PromptsDir 路徑** | `SpawningConfig.PromptsDir` 必須為絕對路徑，預設 `"prompts"` 相對於 CWD。呼叫 `ManualSpawn` 前請確認工作目錄正確。 |
| **Agent 初始為 disabled** | `AgentFactory.CreateAgentForGap` 產生的 Agent `Enabled: false`，必須經過 `AcceptAgent()` 才會啟用。 |
| **Extinction 閾值** | `CheckExtinction()` 預設 20 天權重維持在 0.3（DarwinianWeightMin）以下即淘汰。修改 `MinWeightDays` 會影響淘汰速度。 |
| **Gap 優先排序** | `prioritizeGaps()` 使用 bubble sort（列表小，非效能瓶頸），嚴重程度加權後以年齡微調。 |
| **ManualSpawn 不進 spawnedAgents map** | `ManualSpawn()` 只建立並回傳 `SpawnedAgent`，不會自動加入 `spawnedAgents` map（測試有註解說明此行為）。 |

---

## 測試

- `go test ./internal/spawning/...`
- 涵蓋 GapDetector 偵測邏輯、AgentFactory 產生規則、SpawningManager 生命週期（Accept/Reject/Extinction）

(End of file - total 38 lines)
