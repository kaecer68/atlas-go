# Phase 3.5 部署 Runbook (Stub)

> **Status**: 📝 DRAFT — placeholder created for Phase 3.5 M1 deployment-gateway implementation
> **Created**: 2026-06-30
> **Spec**: [`../specs/phase3-5-spec.md` §3.1 M1](../specs/phase3-5-spec.md#31-m1-deployment-gateway-rank-4-2d)

## 部署步驟 (待補)

待 Phase 3.5 M1 實作完成後,本檔案需補齊:

1. **fubon-proxy supervisor preflight** — `internal/portprobe/` 啟動檢查
2. **Deployment dashboard 啟用** — `internal/adminapi/deployment/HandleDeploymentDashboard` 註冊到 admin route
3. **Frontend component 載入** — `shared_web/static/js/components/deployment-dashboard.js` 經 esbuild shared plugin 載入
4. **驗收** — `curl http://localhost:18080/api/admin/live/deployment/dashboard` 回 supervisor snapshot

## 相關連結

- 起點: 同目錄 `phase3-5-spec.md` §1 (設計目標)
- 規格: `docs/specs/phase3-5-spec.md` §3.1
- L2.4 對齊: `docs/operations/l2-4-runbook.md`
- 部署 dashboard: `internal/adminapi/deployment/dashboard.go`

---

**TODO**: Phase 3.5 M1 完成後請更新本檔案。
