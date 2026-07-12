# eventdriven 事件驅動資金流預測規格

> 本文件為 `internal/eventdriven` 的技術規格補充；模組陷阱見 `internal/capitalflow/AGENTS.md`（本模組與 capitalflow / recommender / subscription 合併於該 cluster AGENTS.md）。

## Confidence 計算

5 日事件驅動資金流預測的信心度範圍為 `(0.5, 1.0]`，計算方式為：

```
confidence = sigmoid(net_weight × (drivers + 1))
```

- `net_weight`：事件驅動因子的淨權重。
- `drivers`：同時作用的事件數量。

## 事件類型

目前涵蓋的事件類型包括：

- ETF 換股
- MSCI 調整
- 月營收公告
- 季底作帳
- 國定假日

## 資料注意事項

- 假日效應需要 historical window ≥ 3 年才穩定。
- MSCI pre-positioning 通常在公告前一週開始反映。
- 電子 / 傳產 / 金融的營收截止日不同，需用 calendar 區分產業別。
