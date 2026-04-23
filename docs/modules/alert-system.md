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

### 發送通知

```go
notifier := monitoring.NewTelegramNotifier()
if notifier.IsConfigured() {
    notifier.Notify(alert)
}
```

### API 端點

- `GET /api/alerts` - 列出所有警報
- `GET /api/alerts/unacknowledged` - 列出未確認警報
- `POST /api/alerts/acknowledge` - 確認警報

## 監控指標

- **警報觸發率** = 觸發警報數 / 總檢查次數
- **確認延遲** = 觸發時間 → 確認時間
- **通知成功率** = 成功通知數 / 總通知數

## 注意事項

1. **未配置的 notifier**會靜默忽略（不報錯）
2. AlertStore 使用 **JSONL append-only**
3. 建議定期清理已確認的舊警報

## 測試

```bash
go test ./internal/monitoring/... -v
go test ./internal/monitoring/... -run TestAlertStore -v
```
