# 前端審計修復 Manifest

- **模式**：Execute
- **分支**：`fix/frontend-audit-fixes`
- **Worktree**：`.worktrees/frontend-audit-fixes`
- **ATLAS_ENV**：development
- **範圍**：`admin_web/`、`client_web/`、其前端測試與 Playwright 配置；不修改後端商業邏輯
- **來源**：2026-07-22 前端審計結果

## 修復項目

### P0
- [ ] 隔離 admin_web Playwright 靜態 server，避免誤載 client_web 或 server race
- [ ] 修正 client_web `/api/stock/quote` 測試/服務路由，使 endpoint 回傳 JSON 而非 index HTML

### P1
- [ ] 接通 client_web `evolution_panel`，或完整移除其導覽與頁面殘留；本次採接通既有頁面
- [ ] 補齊 admin_web PRISM 頁面 markup 與對應載入鏈
- [ ] 修正 client_web 直接 deep-link 的初始 page activation
- [ ] 修正 onboarding overlay 對導覽按鈕的互動阻擋

### P2
- [ ] 擴充 smoke 預設頁面，納入 capital_predictions、capital_board、evolution_panel、decision-chain
- [ ] 讓 smoke 對 API 失敗與空資料有可辨識的驗證，不把 404/503 靜默視為通過
- [ ] 追查 capital_predictions 五日資料重複，若為前端資料組裝問題則修復；若為後端來源則保留證據並不越界

## 驗證

- [ ] 先跑修復前 baseline 測試並記錄
- [ ] 每個修復先建立/調整失敗測試，再實作
- [ ] admin_web 與 client_web 單元測試
- [ ] admin_web 與 client_web Playwright E2E
- [ ] 兩側 smoke
- [ ] browser 深層連結、導覽按鈕、console/network、NaN/undefined/null
- [ ] API 資料穿透與頁面顯示交叉檢查
- [ ] `make check-binaries`
- [ ] `git cleanup-tools`
- [ ] 建立 PR、等待 CI、合併後於合併結果重跑驗證

## 不在本次範圍

- 後端 `/api/events/prediction` 模型或快取修改，除非前端證據明確顯示資料被前端錯誤複製
- 其他未在審計報告列出的 UI 重構
- 不相關 Go runtime、交易、風控與資料管線修改
