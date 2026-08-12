# Frontend E2E Smoke Tests

Stage 7 前端 E2E 驗證 — 針對 capital-flows / narrative models 與 prediction heatmap 的冒煙測試。

## 前提

- Node.js ≥ 18
- Playwright（測試腳本會自動尋找 `playwright` npm package）
- `admin_web/node_modules/` 與 `client_web/node_modules/` 已安裝(`npm install`)
  - mock-server.mjs 啟動時會**自動 rebuild stale `dist/`**(比對 `static/index.html` 與 `dist/index.html` 的 mtime — 若 dist 缺失或較舊就跑 `node esbuild.config.mjs` rebuild,通常 < 1 秒),所以**無需手動 npm run build**

## 快速開始

### 方案 A：Production Docker（推薦）

```bash
docker compose up -d
sleep 5
BASE_URL=http://localhost:18080 node tests/frontend-e2e/stage7_smoke.mjs
```

### 方案 B：Static Mock Server（無需後端）

```bash
# 1. （可選）首次執行需安裝 admin_web/ 與 client_web/ 的 npm deps(只需做一次)
cd admin_web && npm install --no-audit --no-fund && cd ..
cd client_web && npm install --no-audit --no-fund && cd ..

# 2. 啟動 Mock Server(背景)。它會自動 rebuild stale dist,通常 < 1 秒
node tests/frontend-e2e/mock-server.mjs &
MOCK_PID=$!
sleep 1

# 3. 確認 Mock Server 正常(auto-rebuild log 顯示 "[mock-server] xxx/dist rebuilt" 表示剛 rebuild 過)
curl -s http://localhost:8001/api/narrative/models | head -c 100
curl -s http://localhost:8001/api/events/prediction | head -c 100

# 4. 執行 Smoke Test
node tests/frontend-e2e/stage7_smoke.mjs

# 5. 清理
kill $MOCK_PID 2>/dev/null
```

## 測試內容

| 測試 | 頁面 | 驗證項目 | Mock API |
|------|------|----------|----------|
| Test 1 | `/admin/capital_models` | #capitalModelsContent 含 table row | `GET /api/narrative/models` |
| Test 2 | `/client/` | #home-tier-sections 含 .event-card | `GET /api/events/calendar` + `GET /api/capital-flow/summary` + `GET /api/recommendations` |

## 截圖

所有截圖產出於 `tests/frontend-e2e/screenshots/`：

| 檔案 | 內容 |
|------|------|
| `admin-capital-models.png` | Admin 後台錢潮模型頁（含 3 個模型表格） |
| `client-home-events.png` | 投資人首頁 event cards（bonus） |

## 排查 Checklist

若任一測試失敗，依序檢查：

### 基本問題
- [ ] Mock server 是否在 port 8001 上運行？`curl -s http://localhost:8001/api/narrative/models`
- [ ] 前端是否已 build？`ls admin_web/dist/index.html` & `ls client_web/dist/index.html`
- [ ] Playwright 是否已安裝？`npx playwright --version`

### JS 載入問題
- [ ] Browser Console 中有無 404？測試腳本會列出 page error
- [ ] `admin_web/dist/index.html` 中的 script path 是否正確？
- [ ] `client_web/dist/index.html` 中的 script path 是否正確？
- [ ] Chunk file 是否正常？`ls client_web/dist/js/`

### SPA 路由問題
- [ ] Mock server 是否對 `/admin/capital_models` 回傳 index.html 而非 404？
- [ ] Client JS 的 `basePath` 計算是否正確？（/admin/ → basePath = /admin）
- [ ] 各 `pageId` 是否在 `SHELL_LOADERS` 與 `titles` 中？

### 資料格式不一致（常見原因）
- [ ] Mock API 回的 field name 是否與 JS 預期一致？

## 檔案結構

```
tests/frontend-e2e/
├── README.md              ← 本文件
├── stage7_smoke.mjs       ← Playwright smoke test
├── mock-server.mjs        ← Node.js HTTP mock server
└── screenshots/
    ├── admin-capital-models.png
    ├── client-capital-predictions.png
    └── client-home-events.png
```
