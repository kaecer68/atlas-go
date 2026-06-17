# Atlas Skill 編寫模板（SKILL_TEMPLATE）

**版本**: 1.0
**日期**: 2026-06-17
**用途**: 統一手寫技能的格式、章節結構與品質標準。本模板遵循 Anthropic 技能文件規範。

---

## 模板架構

每個手寫 SKILL.md 必須包含以下章節。標記 `[必填]` 的章節不可省略。

```markdown
---
name: atlas-<skill-name>                    # [必填] 技能唯一識別碼
description: "<English description>"        # [必填] 英文描述（Anthropic 規範）
version: "1.0"                              # [必填] Semantic versioning
category: "<category>"                      # [必填] 分類：debug|feature|computation|security|robot-communication|frontend
auto_load: true|false                       # [必填] AI Coding 時是否自動載入
load_policy: "auto"|"manual_only"           # [必填] 載入策略
created: "YYYY-MM-DD"                       # [必填] 建立日期
updated: "YYYY-MM-DD"                       # [必填] 最後更新日期
target_audience: "developer"|"investor"|"both"  # 目標受眾
---

# Atlas <Skill 名稱> — <一句話描述>

## 描述（Description）[必填]

本技能的用途、解決的問題、在系統中的角色。2-5 句話即可。

## 何時觸發（When to Trigger）[必填]

明確的觸發條件列表，讓 AI 能判斷何時應載入此技能：

- 當使用者說 "<觸發短語 1>" 時
- 當任務涉及 <模組/功能> 時
- 當 <特定條件> 發生時

## 核心概念（Core Concepts）[必填]

技能涵蓋的核心概念、原理與金融工程背景。依內容複雜度使用：

### <概念 1>
- 定義
- 在 Atlas 系統中的實作位置
- 與其他技能的關聯

### <概念 2>
...

## 數據來源（Data Sources）

本技能依賴的數據管道與模組：

| 數據 | 模組/檔案 | 說明 |
|------|----------|------|
| <數據名稱> | `internal/<模組>/<檔案>` | <數據說明> |

## 實作位置（Implementation Locations）

技能概念對應的程式碼位置：

| 概念 | 檔案路徑 | 關鍵函數/結構 |
|------|---------|-------------|
| <概念> | `internal/<模組>/<檔案>.go` | `<FunctionName>` |

## 使用範例（Usage Examples）

具體的操作範例，AI 可依此模式執行：

### 範例 1: <情境>
```
<操作步驟或輸出>
```

## 驗證規則（Validation Rules）

技能的驗證與品質保證規則：

- [ ] <檢查項目 1>
- [ ] <檢查項目 2>

## 相關技能（Related Skills）

| 技能 | 關聯 |
|------|------|
| `atlas-<related>` | <關聯說明> |

## 版本歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| 1.0 | YYYY-MM-DD | 初版 |
```

---

## 品質標準

### 必備要求（No-Go 條件）
- [ ] Frontmatter 完整（name, description, version, category, auto_load, load_policy, created, updated）
- [ ] 描述章節以繁體中文撰寫（本專案語言強制規範）
- [ ] 何時觸發章節有 3+ 個明確觸發條件
- [ ] 核心概念至少 2 項，每項有定義 + 實作位置
- [ ] 數據來源引用實際存在的模組或檔案
- [ ] 實作位置引用實際存在的檔案與函數
- [ ] 技能名稱唯一，不與既有技能衝突

### 建議要求（Nice-to-Have）
- [ ] 使用範例至少 2 個
- [ ] 驗證規則列表具體可執行
- [ ] 相關技能正確交叉引用
- [ ] 技能長度控制在 100-300 行（過短缺乏深度，過長浪費 token）

---

## 分類對應（Category Mapping）

| category 值 | 說明 | auto_load | load_policy |
|-------------|------|-----------|-------------|
| `debug` | 除錯維護迭代（🔧） | `true` | `auto` |
| `feature` | 功能拓展（🚀） | `false` | `manual_only` |
| `computation` | 邏輯計算（📊） | `true` | `auto` |
| `security` | 資料安全（🛡️） | `true` | `auto` |
| `robot-communication` | 機器人溝通（🤖） | `false` | `manual_only` |
| `frontend` | 前端頁面（🖥️） | `false` | `manual_only` |

> `auto_load: false` 且 `load_policy: "manual_only"` 的技能不會在 AI Coding session 中自動載入，僅在明確需要時手動讀取，防止 token 浪費。

---

## Token 節省原則

1. **Frontmatter 第一行需明確載入策略** — AI 可在不讀取全文的情況下判斷是否載入
2. **技能長度上限 300 行** — 超出部分應拆分為子技能或移至 docs/
3. **避免跨技能重複內容** — 共用概念引用來源技能，不複製貼上
4. **實作位置引用精確到檔案** — 不引用整個模組目錄
5. **使用範例精簡** — 2-3 個關鍵範例足夠，避免過度展示

---

## Anthropic 規範合規檢查清單

依據 [Anthropic Skills 規範](https://docs.anthropic.com/en/docs/agents-and-tools/agent-skills)：

- [ ] Frontmatter 的 `name` 欄位使用 kebab-case 且全域唯一
- [ ] Frontmatter 的 `description` 以英文撰寫（AI 用於匹配載入）
- [ ] 技能內容以目標語言撰寫（本專案：繁體中文）
- [ ] 技能不重複既有技能的功能範圍
- [ ] 技能不包含機敏資訊（API keys, secrets 等）
- [ ] 技能不假設 AI 具有特定的金融工程背景知識
