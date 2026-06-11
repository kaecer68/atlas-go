---
name: atlas-macro-narrative
description: "Use when analyzing macro narrative events, deriving foreign capital flow probabilities, or working with the six macro dimensions. Triggers: macro analysis, narrative event detection, Taiwan stress index, causal chain analysis."
---

## 核心使命

基於美元、美債、日圓、匯率、商品、地緣政治六大維度，推導外資流出台灣的機率與資金可能流向，為投資決策提供宏觀脈絡。

---

## 一、六大輸入維度

### 1. 美元利率（Fed Policy）
- **觀察指標**：聯邦基金利率、Fed點陣圖、鮑威爾談話
- **關鍵閾值**：
  - 升息週期：資金回流美國，新興市場承壓
  - 降息週期：資金外溢，但需區分「預防式降息」（軟著陸）vs「恐慌式降息」（衰退）
- **與台股關係**：
  - 2022-2023：激進升息，台股跌22%（2022）
  - 2024：維持高利率，但台股漲28%（AI結構性因素戰勝宏觀）

### 2. 美債收益率（US Treasury Yields）
- **觀察指標**：10Y/2Y利差、實質收益率
- **關鍵閾值**：
  - 10Y-2Y倒掛（<0）：衰退預警，歷史上每次倒掛後12-18個月出現衰退
  - 10Y突破4.5%：資產估值壓縮，科技股承壓
- **與台股關係**：
  - 美債收益率上升 → 全球資產折現率上升 → 高估值科技股受壓
  - 但AI資本支出超預期時，可抵銷此壓力（2024年實例）

### 3. 日圓利率（BOJ Policy）
- **觀察指標**：日本央行政策利率、日圓匯率、Carry Trade規模
- **關鍵閾值**：
  - BOJ加息：Carry Trade unwind風險
  - USD/JPY跌破145：強制平倉壓力
  - USD/JPY跌破140：系統性風險（2024年8月日經跌12.4%）
- **與台股關係**：
  - 日圓急升 → 全球流動性收緊 → 台股外資流出
  - 規模：1.5-2兆美元的Carry Trade，台灣是主要投資標的之一

### 4. 匯率（Currency Markets）
- **觀察指標**：USD/TWD、DXY美元指數、新台幣波動
- **關鍵閾值**：
  - 新台幣單日貶值>1%：外資撤離信號
  - DXY突破105：全球美元流動性緊張
- **與台股關係**：
  - 新台幣貶值通常伴隨外資賣超
  - 但2024年新台幣貶值6.24%，台股仍漲28%（AI因素）

### 5. 商品價格（Commodities）
- **觀察指標**：黃金、原油、銅
- **關鍵閾值**：
  - 黃金大漲+油價大漲：系統性避險（地緣政治+通脹）
  - 黃金跌+油價大漲：能源危機（資金湧入能源股，非避險）
  - 銅價上漲：全球經濟擴張，科技需求強勁
- **與台股關係**：
  - 油價>100美元：能源股受益，但通脹壓力壓縮估值
  - 黃金與科技股呈現負相關（資金輪動）

### 6. 地緣政治（Geopolitics）
- **觀察指標**：戰爭類型、地點、能源出口國vs工業國
- **關鍵閾值**：
  - 中東戰爭（伊朗、沙烏地）：油價暴漲，能源股受益
  - 歐洲戰爭（俄烏）：黃金大漲，傳統避險
  - 台海緊張：台股特有風險，外資減持
- **與台股關係**：
  - 2022俄烏：台股跌22%
  - 2024以哈：台股短暫回調後續漲
  - 2026美伊：台股劇震但快速反彈（AI基本面支撐）

---

## 二、推導框架：外資出逃機率模型

### 核心邏輯

外資是否出逃台灣，取決於**全球流動性環境**與**台灣相對吸引力**的對比：

```
外資出逃機率 = f(全球流動性緊縮壓力, 台灣結構性吸引力, 地緣政治風險)
```

### 情境矩陣

| 全球流動性 | 台灣結構性吸引力 | 地緣政治 | 外資出逃機率 | 歷史案例 |
|-----------|---------------|---------|------------|---------|
| 緊縮（美元升息+日圓升息）| 弱（無產業趨勢）| 高 | **90%+** | 2022年（俄烏+升息）|
| 緊縮 | 強（AI超級週期）| 中 | **30-50%** | 2024年（外資賣超但台股漲28%）|
| 寬鬆（Fed降息）| 強 | 低 | **<10%** | 2023年（AI啟動+降息預期）|
| 緊縮 | 弱 | 高（台海）| **95%+** | 假設情境 |

### 關鍵推導規則

**規則1：Carry Trade Unwind 風險**
```
IF BOJ加息 AND USD/JPY < 145 AND VIX > 30:
    外資出逃機率 = 80%+
    資金流向 = 回流日本（被迫平倉）
    台股影響 = 系統性賣壓（不分產業）
```

**規則2：AI結構性因素戰勝宏觀**
```
IF 台積電營收YoY > 40% AND AI資本支出預期上調:
    外資出逃影響 = 被內資承接（投信+ETF）
    台股表現 = 續漲（即使外資淨賣出）
    歷史驗證 = 2024年外資賣超6951億，台股漲28%
```

**規則3：戰爭類型決定資金流向**
```
IF 中東戰爭 AND 油價漲幅 > 20%:
    黃金表現 = 可能不漲（資金湧入能源期貨）
    受益板塊 = 能源、航運、替代能源
    受損板塊 = 高估值科技（利率預期上升）
    
IF 歐洲戰爭 AND 黃金漲幅 > 10%:
    避險模式 = 傳統避險（黃金、美元、債券）
    受益板塊 = 黃金、防禦性金融、高股息
    受損板塊 = 週期性產業、出口導向
```

**規則4：美債收益率倒掛**
```
IF 10Y-2Y利差 < 0（倒掛）:
    衰退機率 = 70%（12-18個月內）
    台股策略 = 減持週期股，增持防禦股
    注意：倒掛後台股未必立即下跌（2022年7月倒掛，台股10月才大跌）
```

---

## 三、資金流向推導

### 當外資出逃時，資金去哪裡？

| 出逃原因 | 資金流向 | 台灣受益板塊 | 台灣受損板塊 |
|---------|---------|------------|------------|
| Carry Trade unwind | 回流日本 | 無（全面承壓）| 科技股、金融股 |
| 美元升息 | 回流美國 | 無（全面承壓）| 高估值科技 |
| 地緣政治（中東）| 能源/黃金 | 能源股、航運 | AI科技股 |
| 地緣政治（台海）| 美元/債券 | 內需、高股息 | 半導體、出口 |
| 產業輪動（AI→傳產）| 傳產/金融 | 金融、高股息 | AI供應鏈 |

### 當外資不逃或回流時

| 流入原因 | 資金來源 | 台灣受益板塊 |
|---------|---------|------------|
| Fed降息 | 全球流動性寬鬆 | 科技股、成長股 |
| AI資本支出爆發 | 主動型基金 | AI供應鏈、半導體 |
| 日圓貶值 | Carry Trade重建 | 高股息（套利資金）|
| 地緣政治避險 | 歐洲資金 | 黃金、國防 |

---

## 四、與回撤機制的整合

### 宏觀風險等級映射

| 宏觀風險等級 | 觸發條件 | 回撤行動 |
|------------|---------|---------|
| **綠色（正常）** | 無上述風險信號 | 維持正常倉位（22%單檔上限）|
| **黃色（警戒）** | 單一風險信號（如USD/JPY跌破150）| 降低單檔上限至15%，增加現金至15% |
| **橙色（危險）** | 雙重風險信號（日元升息+美債倒掛）| 降低單檔上限至10%，增加現金至30% |
| **紅色（緊急）** | 系統性風險（Carry Trade unwind+台海緊張）| 強制減半或清倉，現金50%+ |

### 結構性機會豁免

```
IF 宏觀風險 = 橙色 
   AND 台積電營收YoY > 50%
   AND AI資本支出預期上調:
    豁免條件 = 核心AI持倉（台積電、鴻海、廣達）不觸發回撤
    理由 = 結構性趨勢戰勝宏觀逆風（2024年實證）
```

---

## 五、使用方式

### 在Atlas系統中的整合點

1. **MacroIngestor** (`internal/narrative/ingestor.go`)
   - 每日讀取六大維度數據
   - 計算外資出逃機率
   - 輸出 `MacroRiskLevel`（綠/黃/橙/紅）

2. **DrawdownGuard** (`internal/risk/macro_aware_drawdown.go`)
   - 接收 `MacroRiskLevel`
   - 調整回撤閾值（風險高時提前觸發）
   - 結構性機會豁免邏輯

3. **NarrativeEngine** (`internal/narrative/knowledge_base.go`)
   - 動態模板選擇（基於戰爭類型、利率環境）
   - 輸出 `CapitalFlowInference`

### 決策流程

```
每日開盤前:
1. MacroIngestor讀取數據 → 計算外資出逃機率
2. 若機率 > 70%:
   a. 檢查是否有結構性豁免條件（AI營收超預期）
   b. 若無豁免：調降倉位上限，增加現金保留
3. NarrativeEngine推導資金流向 → 調整板塊配置
4. DrawdownGuard根據宏觀風險等級調整回撤閾值
```

---

## 六、歷史驗證清單

| 日期 | 事件 | 宏觀信號 | 系統應推導 | 實際結果 | 驗證 |
|------|------|---------|-----------|---------|------|
| 2022/2 | 俄烏戰爭 | 歐洲戰爭+油價暴漲+黃金大漲 | 外資出逃90% | 台股跌22% | ✅ |
| 2024/8/5 | 日圓Carry Trade unwind | BOJ加息+日圓急升+VIX飆升 | 外資出逃80% | 日經跌12.4%，台股短暫回調 | ✅ |
| 2024全年 | 外資大賣超 | 美元高利率+地緣政治 | 外資出逃50%（被內資承接）| 台股漲28% | ✅ |
| 2025/4 | 川普關稅威脅 | 貿易戰風險+美元強勢 | 外資出逃70% | 台股單日跌9.7% | ✅ |
| 2026/3 | 美伊戰爭 | 中東戰爭+油價破百 | 能源股受益，科技股承壓 | 能源股漲停，台積電短暫回調 | ✅ |

---

## 七、持續優化方向

1. **量化模型**：將六大維度輸入機器學習模型，預測外資淨買賣超
2. **高頻數據**：整合外資期貨淨部位（領先現貨1-3天）
3. **情緒指標**：VIX、CNN恐懼貪婪指數、散戶情緒
4. **產業關聯**：建立「AI資本支出→台積電營收→台股指數」的領先指標

---

## 八、Rolling Calibration Framework（壓力指數自動校準）

**觸發情境**：
- 收到「Taiwan Stress Index 漏報」、「stress index 都是 0」、「平穩日沒訊號」等問題回報
- 收到「為什麼 ChangePct=0」、「怎麼處理 macro factor 平穩日」等問題
- 需要新增 macro factor（例如：人民幣、銅、費城半導體指數）
- 需要修改 baseline window、target median、validation 比例等校準參數
- 需要在 `TaiwanStressIndex` 上新增/修改任何校準邏輯

**必讀文件**：`docs/MACRO_CALIBRATION.md`

### 核心概念速查

- **Hybrid Signal**：`max(|level_z|, |change_pct|)`，解 DXY/JPY/Oil/Gold 平穩日訊號消失問題
- **五層**：Baseline → Scale → Regime → Validation → Maturity-Gated Scheduler
- **Maturity Gating**：`BackgroundCalibrationScheduler.RunDaily` 在 BURN_IN 模式 log skip 不執行
- **校準預設關閉**：`calibration_enabled = false`（AGENTS.md 規範），啟用前需 30 日 staging 驗證

### 修改校準邏輯的必守規則

1. **新 factor 走 map**：在 `BaselineConfig`（map）加入新 key，不要加 struct 欄位
2. **新參數走 `ParametersConfig`**：所有校準閾值一律 `config.GetParametersConfig()` 取得
3. **新 scheduler 走 `BackgroundTaskManager`**：不要在 narrative 模組內啟 goroutine
4. **改計算公式前先看偏差備註**：`docs/MACRO_CALIBRATION.md` 第五節記錄了 5 項 vs 原計畫的設計偏差（Mean/Count 替代 Baseline、map 替代 8 欄位、z-score 替代百分比等）
5. **Validation 退化不可靜默吞**：必須 log warning + 保留舊 config，不可自動降級

### 修改前的 7 步 Pre-Change Protocol

修改任何 `internal/narrative/calibration_*.go` 前：

1. 執行 `gitnexus impact` 看 `calibration_baseline.go`/`calibration_regime.go` 的 blast radius
2. 確認 `TaiwanStressIndex` 的呼叫者清單（risk/portfolio/monitoring）
3. 確認 `ParametersConfig` 是否需新增欄位（若是，需同步更新 `parameters.go`、`parameters_defaults.go`、`configs/parameters.json` 三個檔案）
4. 確認是否影響既有 `*_test.go`（76 packages test pass 為 baseline）
5. 若改 baseline 演算法：先看 `docs/MACRO_CALIBRATION.md` 第二節「關鍵設計」了解為何選擇 z-score / map / Mean-Count
6. 若改 Maturity Gating：先確認 `internal/scheduler/auto_calibration_test.go` 涵蓋 BURN_IN skip 行為
7. 改完後跑 `go build ./... && go test ./internal/narrative/... ./internal/scheduler/... && gofmt -l . && staticcheck ./...`

## 九、5 層框架與心法庫整合

本技能定義的 6 大輸入維度（Fed / US10Y / BOJ / 匯率 / 商品 / 地緣）現在被收斂為 `strategy_techniques` 的 **5 層框架（L1~L5）**：

| 本技能 6 維度 | 收斂至 strategy_techniques | 用途 |
|--------------|---------------------------|------|
| Fed 利率路徑 + DXY + US10Y | **L1 全球流動性** | 心法觸發條件 |
| 外資買賣超 + 法人動向 | **L2 外資行為** | 心法觸發條件 |
| 半導體 + 油 + 金 | **L3 產業催化** | 心法觸發條件 |
| USD_TWD + 融資 | **L4 匯率籌碼** | 心法觸發條件 |
| 地緣風險 | **L5 地緣政治** | 心法觸發條件 |

**整合路徑**：
- 6 維度 → `narrative/templates.go` 的 25+ 模板 → `strategy_techniques.StrategyFrame.Themes` 欄位
- 外資出逃機率 → `strategy_techniques` L2 心法的 `Attribution` 歸因依據
- 與回撤機制整合 → `strategy_techniques` L1~L5 各層心法的 `Regimes` 標籤

**4 核心短線指標**：外資現貨 / TSM ADR / NVDA / DXY（詳見 `atlas-taiwan-leading-indicators`）

**心法庫詳見**：`atlas-strategy-techniques` skill

---

*技能版本: 1.2*  
*最後更新: 2026-06-11*  
*適用對象: Atlas-Go AI Agent*
