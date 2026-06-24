# 開發工具指南

> 本文件介紹 atlas-go 專案的程式碼知識圖譜工具：**GitNexus**。
>
> 適用對象：需要理解專案架構或進行重構的**人類開發者**。

---

## GitNexus

### 核心能力

GitNexus 具備「執行流（Process）」與「功能社群（Community）」雙重抽象層。它能回答「這段程式碼在系統中扮演什麼角色」，而不只是「被誰呼叫」。

**節點統計（atlas-go）：** 52,662 symbols、165,265 relationships、300 個執行流

### 常用指令

```bash
# 重建索引（修改大量程式碼後執行）
npx gitnexus analyze

# 查詢特定概念相關的執行流
gitnexus_query({query: "auth validation logic"})

# 修改前的影響範圍分析（**強制執行**）
gitnexus_impact({target: "validateUser", direction: "upstream"})

# 檢查變更影響的執行流
gitnexus_detect_changes()
```

### 使用場景

| 場景 | 指令 | 說明 |
|------|------|------|
| 改函式前評估風險 | `gitnexus_impact()` | 查看直接呼叫者、受影響的執行流、風險等級 |
| 理解系統運作 | `gitnexus_query()` | 自然語言查詢，返回執行流與相關符號 |
| 追 bug 根因 | `gitnexus_trace()` | 追蹤從 A 到 B 的完整呼叫鏈 |
| 跨模組重構 | `gitnexus_rename()` | 安全改名，理解呼叫圖 |
| PR 前檢查 | `gitnexus_detect_changes()` | 確認變更只影響預期的符號和執行流 |

### 資源入口

- `gitnexus://repo/atlas-go/context` — 專案總覽
- `gitnexus://repo/atlas-go/clusters` — 所有功能社群
- `gitnexus://repo/atlas-go/processes` — 所有執行流

---

## 索引更新時機

| 工具 | 更新時機 | 指令 |
|------|---------|------|
| GitNexus | 大規模重構後、PR 合併前 | `npx gitnexus analyze` |

---

## 相關文件

- `AGENTS.md` — AI 工具使用規則（AI 專用）
- `CLAUDE.md` — GitNexus 完整規範與工具使用準則
- `internal/AGENTS_INDEX.md` — 模組索引與成熟度
- `docs/architecture.md` — 系統架構詳細說明
