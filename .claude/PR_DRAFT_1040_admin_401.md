# PR #1040: Admin 401 Interceptor

## 目標
為 admin_web 加入 401 攔截器,參考 client_web 的 _origFetch wrapper 模式。
未登入存取 tier-gated API 時自動跳轉到登入頁,避免沉默失敗。

## 範圍
- admin_web/static/js/main.js (包 fetch 攔截)
- admin_web/static/index.html (加登入頁跳轉按鈕)

## 對應稽核項
P3.6 from [audit report](https://github.com/kaecer68/atlas-go/issues/1037)
