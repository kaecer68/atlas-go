# Alert System 操作手冊

## 概述

Alert System 提供即時警報通知，支援 Telegram、Email、Webhook 三種渠道。

## 警報類型

| 類型 | 說明 | 觸發條件 |
|------|------|----------|
| Circuit Breaker | 熔斷警報 | 單日虧損 > 5% |
| Daily Loss Warning | 日虧損警告 | 單日虧損 > 2% |
| High Concentration | 集中度警告 | 單一標的 > 30% |
| Unrealized Loss | 未實現虧損 | 單一標的虧損 > 10% |

## 配置方式

### Telegram

```bash
export ATLAS_TELEGRAM_BOT_TOKEN="your-bot-token"
export ATLAS_TELEGRAM_CHAT_ID="your-chat-id"
```

### Email

```bash
export ATLAS_EMAIL_SMTP_HOST="smtp.gmail.com"
export ATLAS_EMAIL_SMTP_PORT="587"
export ATLAS_EMAIL_FROM="alerts@your-domain.com"
export ATLAS_EMAIL_TO="admin@your-domain.com"
```

### Webhook

```bash
export ATLAS_WEBHOOK_URL="https://hooks.slack.com/services/..."
```

## 使用方法

### 建立 AlertStore

```go
store, _ := monitoring.NewAlertStore("data/state/alerts/")
store.Save(alert)
```

### 建立 Notifier

```go
// Telegram
nt := monitoring.NewTelegramNotifier(botToken, chatID)

// Webhook（headers 可為 nil）
nw := monitoring.NewWebhookNotifier(webhookURL, map[string]string{"X-API-Key": "secret"})

// Email
ne := monitoring.NewEmailNotifier(domain.AlertChannelConfig{
    EmailSMTPHost: "smtp.gmail.com",
    EmailSMTPPort: "587",
    EmailFrom:     "alerts@your-domain.com",
    EmailTo:       "admin@your-domain.com",
})
```

### 發送通知

```go
if err := nt.Notify(alert); err != nil {
    // 配置錯誤或傳送失敗都會回傳 error
}
```

### API 端點

| 方法 | 路徑 | 用途 |
|------|------|------|
| `GET` | `/api/alerts` | 列出所有警報（支援 sort / status / severity / rule / time range / pagination） |
| `GET` | `/api/alerts/unacknowledged` | 列出未確認警報 |
| `GET` | `/api/alerts/stats` | 警報統計 |
| `GET` | `/api/alerts/rules` | 警報規則配置 |
| `POST` | `/api/alerts` | 手動建立警報 |
| `POST` | `/api/alerts/acknowledge` | 確認單筆警報 |
| `POST` | `/api/alerts/acknowledge-bulk` | 批量確認警報 |
| `POST` | `/api/alerts/resolve` | 標記警報已解決 |
| `POST` | `/api/alerts/silence` | 靜音警報規則 |

## 監控指標

- **警報觸發率** = 觸發警報數 / 總檢查次數
- **確認延遲** = 觸發時間 → 確認時間
- **通知成功率** = 成功通知數 / 總通知數

## 注意事項

1. **未配置的 notifier**在 `Notify()` 會回傳 error（例如 `telegram not configured`）；`MultiNotifier.Notify()` 會跳過未配置者並彙整其餘錯誤
2. AlertStore 使用 **JSONL append-only**
3. 建議定期清理已確認的舊警報

## 測試

```bash
go test ./internal/monitoring/... -v
go test ./internal/monitoring/... -run TestAlertStore -v
```
