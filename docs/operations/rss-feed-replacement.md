# RSS Feed 替換決策(PR #930)

> **狀態**: Shipped
> **決策日期**: 2026-07-03
> **對應 PR**: #930(commit `9c2ebb24`)
> **受影響模組**: `internal/narrative/`

---

## 1. 背景

`TaiwanRSSGeopoliticalProvider`(`internal/narrative/taiwan_geopolitical_provider.go`)原本監控 4 個 RSS feed:

```go
feeds: []string{
    "https://www.cna.com.tw/cna/rss/rssfa.xml",     // CNA All News
    "https://news.ltn.com.tw/rss/focus.xml",        // Liberty Times Focus
    "https://www.cna.com.tw/cna/rss/pol/rssfa.xml", // CNA Politics
    "https://news.tvbs.com.tw/rss/news.xml",        // TVBS News
}
```

這 4 個 feed **都是綜合新聞**,非財經專業。對台股投資訊號的相關性低:
- CNA / Liberty Times / TVBS:政治社會新聞為主
- CNA Politics 過多雜訊,污染台海地緣政治訊號

---

## 2. 替換決策

### 2.1 採用 4 個財經主流源

| 新 feed | 來源 | 性質 |
|--------|------|------|
| `https://www.digitimes.com/rss/daily.xml` | DIGITIMES | 科技供應鏈權威,直接覆蓋台股核心產業(半導體、面板、零組件) |
| `https://money.udn.com/rssfeed/lists/1001` | 經濟日報(聯合報系) | 主流財經,產業/股市/金融/國際新聞 |
| `https://news.ustv.com.tw/feed` | 非凡新聞 | 24hr 財經滾動新聞 |
| `https://wwwc.twse.com.tw/rwd/zh/news/feed?type=rss` | TWSE 臺灣證券交易所 | 官方公告與重大訊息,信號純度最高 |

### 2.2 替換理由

- **DIGITIMES**:科技供應鏈是台股核心,半導體/面板/零組件相關性高
- **經濟日報**:主流財經新聞,產業覆蓋廣
- **非凡新聞**:24hr 滾動,可即時抓取市場異動
- **TWSE 證交所**:官方公告是金流訊號最高純度的 source

### 2.3 為何排除 工商時報(CTEE)

`https://www.ctee.com.tw/{rss,rss.xml,feed,feed.xml}` 全部回 403,CTEE 公開 RSS 不存在。CTEE 僅提供付費 工商e報(`epaper@ctee.com.tw`),需商業授權才能取得 RSS。

**未來加入 CTEE 的兩個路徑**:
1. 取得 工商e報付費授權,將 RSS URL 加入 `feeds` slice
2. 替換為其他公開財經 RSS(若有更具相關性者)

---

## 3. 測試計畫

- `TestTaiwanRSSGeopoliticalProvider_Feeds` 更新 expectedFeeds 至 4 個新 URL
- 既有 `TestName` / `TestKeywords` / `TestFetchScore` / `TestCompositeTaiwanGeopoliticalProvider_*` 仍 PASS(keyword 清單未動,僅換 feed 來源)
- 整合測試 `TestFetchScore` 需實際 RSS 連線(operator post-merge 驗證)

---

## 4. 風險

| 風險 | 機率 | 影響 | 緩解 |
|------|------|------|------|
| DIGITIMES RSS 改版/失效 | 低 | 中 | RSS 失效時 `countKeywordsInFeed` 返回 0,不會 panic;由 Loki 規則 `AtlasGoHighErrorLogRate` 觸發警告 |
| 經濟日報 RSS 結構改變 | 低 | 中 | 同上 |
| 4 個 RSS 同時失效 | 極低 | 高 | 觸發 4 個 feed 連續 error,Operator 需手動重新評估 source 選擇 |
| 工商時報未公開 RSS 造成監測盲點 | 中 | 低 | 4 個公開源已覆蓋主流訊號,CTEE 獨家新聞量低 |

---

## 5. 後續工作

- T10 已完成 `P1-2 us_yahoo 替代源評估` 調查,可平行思考其他替代源策略
- 觀察 1 個月 operator 經驗,看是否需再加 source 或調整 keyword
- 若 DIGITIMES / 經濟日報 feed 結構變化,需更新 `feeds` slice 與 `rssFeed` struct(unmarshal 邏輯)

---

## 6. 參考

- PR #930:commit `9c2ebb24`
- `internal/narrative/taiwan_geopolitical_provider.go`(source code)
- `internal/narrative/taiwan_geopolitical_provider_test.go`(test code)
- `docs/operations/loki-deployment.md`(4 條 LogQL rules 覆蓋 RSS 失敗情境)
- `docs/REFERENCE/TRAPS.md`(Prometheus Metric 命名空間 — 命名規範可類比 RSS URL 命名)
