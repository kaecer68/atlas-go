---
name: atlas-doc-governance
description: 文件治理守門員 — 在 AI agent 準備建立或移動 docs/ 內檔案時強制檢查歸屬規則。防止 transient content 污染永久文件目錄。觸發條件：任何涉及 docs/ 目錄的檔案建立、移動、或大量編輯，或使用者提到「文件管理」「歸檔」「spec」「manifest」。
---

# Atlas 文件治理守門員

> **目標**：防止 AI agent 將 transient 內容放入 `docs/`，確保 `docs/` 只保留 canonical、有 6 個月教學價值的內容。

## 核心規則

### 寫入前必做判斷

在 `docs/` 下建立或搬移任何檔案前，**必須**先回答：

1. 這個內容對 6 個月後的新貢獻者仍有參考價值？
2. 這個內容已穩定，未來 1 年內不會大改？
3. 這個內容屬於「規範」「手冊」「架構」「領域知識」「憲法」「playbook」其中之一？

**任一為 no → 進 `.omo/`**，不是 `docs/`。

### 目錄歸屬速查

| 內容類型 | 正確位置 | 錯誤位置 |
|---------|---------|---------|
| 審計/修復追蹤 manifest | `.omo/manifests/` | ❌ `docs/manifests/` |
| 短期 PR 待辦 | `.omo/plans/` | ❌ `docs/plans/` |
| 驗證報告 | `.omo/evidence/` | ❌ `docs/` |
| Session 交接 | `.omo/handoffs/` | ❌ `docs/handoff/`（>7天清理） |
| 技術規格 (spec) | `docs/specs/<topic>-spec.md` | ✅ |
| 操作手冊 | `docs/operations/<topic>-runbook.md` | ✅ |
| 審計/調查紀錄（內部參考） | `.omo/audit/YYYY-MM-DD-slug.md`、`.omo/investigations/` | ✅（私有） |

### `docs/manifests/` 特別規則

`docs/manifests/` **僅保留兩個永久治理模板**：
- `README.md` — manifest 機制說明
- `TEMPLATE.md` — invariant tracker 標準模板

**個別審計 manifest 必須放在 `.omo/manifests/`**，不是 `docs/manifests/`。

### Manifest 生命週期

```
建立 → .omo/manifests/YYYY-MM-DD-slug.md（從 docs/manifests/TEMPLATE.md 複製）
進行中 → .omo/manifests/（正常編輯）
完成後 →
  ├─ 含 spec-level invariant → promote 到 docs/specs/<topic>-spec.md
  ├─ 有 6 個月教學價值 → 提煉進 docs/ 對應目錄（spec/reference/operations）；僅內部參考 → .omo/audit/YYYY-MM-DD-slug.md（2026-08-17 起 docs/archive/ 已解散）
  └─ 無長期價值 → 刪除
```

### 違規目錄（禁止建立）

以下目錄名稱**禁止在 `docs/` 下建立**：
- ❌ `docs/plans/`（應放 `.omo/plans/`）
- ❌ `docs/superpowers/`（任何形式）
- ❌ `docs/wave-N/`（應放 `.omo/wave-N/`）

### 自我檢查

建立檔案前，執行：

```bash
# 檢查文檔規範
grep -A5 "完整文件歸屬對照表" docs/documentation-standard.md

# 檢查 manifest 是否合規
ls docs/manifests/  # 應只有 README.md + TEMPLATE.md

# 檢查 .omo/ 白名單
grep -A20 "完整子目錄白名單" docs/documentation-standard.md
```

## 觸發條件

此 skill 應在以下情境載入：
- 準備在 `docs/` 下建立新檔案或新目錄
- 使用者要求「建立 spec」「歸檔」「審計報告」「manifest」
- 使用者提到「文件管理」「清理 docs」
- 任何涉及 `docs/manifests/` 的操作
- PR 合併後的清理階段

## 參考

- 權威規範：`docs/documentation-standard.md`
- 文件地圖：`docs/documentation-map.md`
- 清理腳本：`scripts/cleanup-manifests.sh`

## CI 強制防護（2026-08-07 起，不可繞過）

`scripts/ci/check_docs_governance.sh` 已掛載 `make ci-quick`，**push 前硬性檢查**：

| 檢查 | 違規例 | 結果 |
|------|--------|------|
| `docs/manifests/` 只准 `README.md` + `TEMPLATE.md` | 把個別 manifest 寫進 `docs/manifests/` | ❌ push 被擋 |
| `docs/` 禁止 `plans/`、`wave-N/`、`superpowers/` 目錄 | 在 `docs/` 下新建 plan | ❌ push 被擋 |
| `docs/` 根目錄禁止新增未追蹤 `.md` | 直接在根目錄放報告 | ❌ push 被擋 |

**建立 manifest 的正確位置只有一個：`.omo/manifests/YYYY-MM-DD-slug.md`**（gitignored，不進 repo）。`docs/manifests/` 只是模板來源（README + TEMPLATE），不是存放處。

寫入 `docs/` 任何位置前，先問：「這個內容是 transient 還是永久？」transient（manifest、plan、審計追蹤、交接）→ `.omo/`；永久（spec、runbook、架構）→ `docs/`。不確定 → 先放 `.omo/`。
