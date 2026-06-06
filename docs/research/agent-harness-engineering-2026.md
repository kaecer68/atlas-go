# AI/LLM Agent Harness Engineering：2025–2026 技術景觀報告

> 研究日期：2026-06-03
> 方法論：5 角度平行搜尋 → 30 來源 → 主張提取 → 3 票對抗驗證 → 綜合合成
> 規模：108 agent、630 萬 tokens、805 次工具呼叫、~2 小時
> 最終結果：**3 項存活（3-0 / 2-1 投票）** / **12 項駁回（0-3 / 1-2 投票）**

---

## 一、執行摘要

AI Agent Harness Engineering 是 2025–2026 年 AI 工程化的核心戰場。它指的不是模型本身，也不是 prompt 技巧，而是**包裹在 LLM API 之外的那層基礎設施**：工具呼叫協定、執行控制迴圈、上下文管理、多代理協調、安全護欄、觀測性與狀態持久化。

經過 108 個 agent 的系統性研究與對抗驗證，**三項核心主張存活**：

1. **MCP + A2A 形成互補的雙層協定架構**（3-0 一致通過）。MCP（Anthropic, 2024 年底）標準化 agent-to-tool，A2A（Google, 2025 年 4 月）標準化 agent-to-agent。兩者皆已移交 Linux Foundation 進行廠商中立治理。

2. **架構層的錯誤遏止機制遠比個別模型品質重要**（3-0 一致通過）。p^n 複合法則主導多代理可靠性：每步驟 90% 準確率在 10 步驟後系統成功率僅 35%。結構化驗證閘可將 10-agent 系統從 ~82% 拉回 ~98%，遠超模型品質改善的邊際效益。

3. **多代理連接環境存在四類下游風險，現有治理框架無法涵蓋**（2-1 通過）。連鎖錯誤、攻擊面擴張、跨系統可審計性降低、敏感資料跨供應商流動——這些風險已被學術論文（DSN 2026、arXiv:2602.11327）、OWASP MCP Top 10、以及 EU AI Act 合規要求所證實。

**12 項廣為流傳的主張在對抗驗證中被駁回**，包括「90% 失敗來自 prompt 品質」、「Claude Code 核心架構是簡單 while-loop」、「ACP 是活躍的第四協定」等。

---

## 二、存活主張詳解

### 主張 1：MCP + A2A 雙層協定架構（3-0）

> **主張**：MCP 與 A2A 構成互補的雙層協定架構。MCP（Anthropic, 2024 年底）標準化單一 AI 對外部工具與資料源的存取介面（垂直整合），A2A（Google, 2025 年 4 月 Cloud Next 發布）標準化多個 autonomous agent 之間的發現、委派與協調（水平整合）。兩者已分別移交 Linux Foundation 的 AAIF 與 Linux Foundation 本體進行廠商中立治理。

**驗證結果**：3-0 一致通過。ReliaQuest 原文直接陳述兩協定的引入時間與架構分工；Wikipedia、InfoWorld、Dataconomy 獨立確認 MCP 於 2024-11-25 由 Anthropic 開源；Google Developer Blog、Computer Weekly、InfoWorld 確認 A2A 於 2025-04-09 Cloud Next 發布。O'Reilly Media 以「販賣機 vs. 禮賓服務」比喻區分兩者角色。DigitalOcean 教程明確陳述「MCP for tools, A2A for agents」。The Register (2026-01-30) 描述兩者為不同層級的互補協定。**無任何來源提出矛盾證據。**

**關鍵數據**（來自來源但未經獨立驗證，僅供參考）：
- MCP SDK 月下載從 2024 年 11 月 ~10 萬增至 2026 年 3 月 9,700 萬（970x）
- Block（Square/CashApp）員工報告 50-75% 時間節省
- A2A 自 2025 年 4 月以來 150+ 合作組織，Microsoft、AWS、Salesforce、SAP、ServiceNow 已生產部署

**協議格局**：

| 協議 | 推出時間 | 定位 | 狀態 |
|------|---------|------|------|
| **MCP** | 2024 年 11 月（Anthropic） | Agent-to-tool/context | Linux Foundation AAIF 治理；5,000+ servers |
| **A2A** | 2025 年 4 月（Google） | Agent-to-agent 通訊 | Linux Foundation 治理；150+ 合作組織 |
| **ACP** | 2025 年 3 月（IBM） | ~~獨立協定~~ | **2025 年 8 月正式併入 A2A**（已確認） |
| **ANP** | 2025 | 去中心化 agent discovery | 早期階段 |

> **⚠️ 駁回**：「MCP、ACP、A2A、ANP 四個協定涵蓋互補層級」的主張以 **0-3 被駁回**。ACP 已不存在為獨立協定，四協定框架已過時。所有主要雲端供應商同時部署 MCP 與 A2A，而非依序進行。arXiv:2505.02279 的階段式採用路徑不具備實作可行性。

---

### 主張 2：架構層錯誤遏止 >> 模型品質（3-0）

> **主張**：在多代理系統中，提升個別 agent 模型品質對端到端系統可靠性的貢獻遠低於架構層的錯誤遏止機制。數學基礎為 p^n 複合法則：每步驟 90% 準確率在 10 步驟後系統成功率僅 35%，即使提升至 95% 也僅達 60%——瓶頸在於錯誤的跨邊界傳播而非單點能力。

**驗證結果**：3-0 一致通過。O'Reilly 原文幾乎逐字對應 claim 敘述。**八篇以上獨立學術論文從不同方法論收斂至同一結論**：
- Zartis (2026) 提供 p^n 數學推導
- "Team of Rivals" (arXiv:2601.14351) 展示 adversarial 架構以相同模型達成 92.1% vs 60%
- CRAFT 論文發現「更強推理能力不必然轉化為更好協調」
- LIFE 調查 (arXiv:2605.14892) 警告「不穩定個別能力導致多代理協作成為不穩定模組的堆疊」
- "From Spark to Fire" (arXiv:2603.04474) 展示防禦成功率從 0.32 提升至 0.89
- Scientific Reports (2026) 發現單一 adversarial agent 降低系統準確率 10-40%

**關鍵量化洞察**：

| 場景 | 系統成功率 |
|------|-----------|
| 10 agent × 98% 每步驟（無驗證閘） | ~81.7% |
| 10 agent × 98% 每步驟（90% catch rate 驗證閘） | ~98.9% |
| 10 agent × 90% 每步驟（無驗證閘） | ~35% |
| Adversarial multi-agent 架構 | 92.1%（vs 60% single-agent） |

**核心含義**：在錯誤傳播動態面前，把資源投入模型升級的 ROI 遠低於投入架構層的驗證閘、circuit breaker 和 state checkpointing。

---

### 主張 3：多代理環境的四類下游風險（2-1）

> **主張**：已連接的多代理環境中存在四類下游風險，且現有治理框架的設計範圍無法涵蓋：
> 1. **連鎖錯誤**——單一系統錯誤經由自動化行動觸發跨系統級聯放大
> 2. **攻擊面擴張**——每個 MCP server 引入新工具介面，每個 A2A channel 創造新通訊路徑
> 3. **跨系統可審計性降低**——難以追溯哪個系統貢獻了什麼
> 4. **敏感資料跨供應商流動**——agent receipt 模式出現正是因為治理跟不上資料流動速度

**驗證結果**：2-1 通過（一位驗證者反對，原因是來源為廠商部落格而非學術出版）。

**獨立佐證豐厚**：
- DSN 2026 論文：67,057 個 MCP server 中 833 個存在漏洞
- OX Security：STDIO 漏洞影響 200K+ server，觸發 10 個 CVE
- OWASP MCP Top 10：MCP08:2025「Lack of Audit and Telemetry」
- Tigera 調查：87% CIO 已部署 agent，但 75% 缺乏即時可見性
- Salt Security：daisy-chained exploits 實證
- arXiv:2602.11327：跨代理傳播的正式模型
- Maloyan & Namiot (2026-01)：MCP 放大攻擊成功率 23-41%
- EU AI Act 第 12-14 條：可追溯性要求目前技術無法滿足
- Legion Security：credential aggregation 風險報告

**Shadow MCP 問題**（來源聲稱，但未經獨立驗證）：首次 MCP 盤點時，大多數企業發現 20-80 個活躍連線在 IT 中央審查之外配置。Nerq Q1 2026 普查中，僅 12.9% MCP server 達到「高信任」門檻。

---

## 三、被駁回的主張（重點摘要）

對抗驗證階段駁回了 12 項廣為流傳的主張。以下是最值得注意的幾項：

### 「90% 的生產失敗來自 prompt 品質」（0-3 駁回）

> 來源：Anthropic Applied AI 的 Cal 在 ZenML 演講中的個人觀察

**駁回理由**：
- 這是**個人軼事觀察，非資料驅動發現**——無調查、研究、對照實驗、方法論支撐
- **多個獨立資料來源直接矛盾**：
  - Datadog 2026：~60% 失敗來自基礎設施/容量
  - Atlan/DigitalApplied/MemU 2026：65% 失敗來自 context drift
  - PwC 2025：前三原因是整合複雜度（67%）、缺乏監控（58%）、不明確升級路徑（52%）
  - Diagrid 2026：耐久性、安全、成本、觀測性為四大失敗模式
- 如果基礎設施獨占 ~60%，prompt 獨占 90% 在**數學上不可能**
- 「90%」在業界作為敘事框架被不同廠商歸因於不同原因（Anthropic 歸因 prompt、HarrisonSec 歸因 infrastructure），暗示其為修辭手法而非實證數據

### 「Claude Code 的核心架構是簡單 while-loop」（0-3 駁回）

> 來源：ZenML 對 Anthropic 演講的二次報導

**駁回理由**：
- 來源為**間接報導**（ZenML 部落格文章轉述 YouTube 演講），非一手技術文件
- 描述的是**設計哲學偏好**，非可驗證的架構事實
- 缺乏與其他架構的**對照實驗資料**
- 無法驗證 Claude Code 的實際內部實作是否真的是「簡單 while-loop」
- 多個驗證者指出這類架構描述往往是事後簡化，不反映實際工程複雜度

### 「ACP 是四個互補協定之一」（0-3 駁回）

> 來源：arXiv:2505.02279

**駁回理由**：
- ACP（IBM Agent Communication Protocol）已於 **2025 年 8 月正式併入 A2A**
- arXiv 論文最後修訂於 2025 年 5 月，在併入公告的**三個月前**
- 真實世界的部署模式是所有主要雲端供應商**同時**部署 MCP 與 A2A，而非論文提出的階段式路徑
- 四協定框架已過時——實際上只剩三個：MCP、A2A（含 ACP）、ANP

### 「MCP Sampling 是伺服器控制」（0-3 駁回）

> 來源：arXiv:2505.02279

**駁回理由**：
- 官方 MCP 規範（2025-11-25 版）將 Sampling 明確歸類為 **Client Feature**
- 官方規範定義三個 server primitives：Resources（application-controlled）、Prompts（user-controlled）、Tools（model-controlled）——Sampling 不在此列
- 規範強調 Sampling 是「server-initiated but client-controlled」：伺服器發起請求，但客戶端保留對模型存取、選擇、權限的實際控制權

---

## 四、其他重要發現（來自來源但未進入正式主張驗證）

以下發現來自來源文章，但未作為獨立主張進入對抗驗證流程。**置信度低於上述三項存活主張**，僅供參考。

### 4.1 架構模式分類

2025-2026 的主流分類將 AI 系統分為三層：

| 層級 | 職責 | 代表 |
|------|------|------|
| **Framework**（構建時） | Agent 組合、抽象化 | LangChain、OpenAI Agents SDK、CrewAI |
| **Runtime**（執行時） | 狀態持久化、重試、fallback、錯誤恢復 | Inngest、Temporal、LangGraph |
| **Harness**（評估時） | 測試、評估、基準化、安全護欄 | Claude Code harness、自建 |

核心架構決策軸（HuggingFace 部落格）：傳統「Workflow vs. Agent」二分法應被「**Scripted Orchestration vs. Policy-Driven Orchestration**」取代。決定性問題是：「**誰在執行期決定下一步動作？**」

### 4.2 生產失敗模式（來自來源，未經正式驗證）

| 失敗模式 | 影響 | 來源 |
|---------|------|------|
| 多代理無限迴圈 | 4-agent LangChain/CrewAI 系統燃燒 $47,000（4 週） | GetOnStack/ZenML 2025 |
| Token 用量膨脹 | 多代理系統中每次請求 45x 膨脹（~3K → ~137K tokens） | GetOnStack/ZenML 2025 |
| MCP server 並發崩潰 | 500ms → 47 秒（1,000 agent 同時命中） | GetOnStack/ZenML 2025 |
| Agent Execution Tax | Gemini 2.5 Flash 浪費 22.9% 推論呼叫 | Fireworks AI (720 agent runs) |
| Victory declaration bias | Agent 標記任務完成但不驗證結果 | Faros AI (2026) |
| Context 推理衰退 | 長上下文中推理 tokens 減少 43-50% | Rodionov/Yandex "Reasoning Shift" (2026-04) |

### 4.3 設計原則（來自來源，未經正式驗證）

- **Context Engineering > Prompt Engineering**：生產可靠性取決於控制 agent 在每個互動步驟中的知識、檢索和推理環境
- **耐久執行（Durable Execution）**：使用獨立持久化、可重試的步驟，而非 monolithic in-process 迴圈。缺乏 checkpointing 導致 15-30% LLM 成本浪費（Diagrid）
- **固定控制層**：agent 不應自己決定下一步——每個狀態轉換由程式碼管理
- **五層上下文架構**（Fractal Analytics / OpenAI）：Foundational Identity → Grounded Knowledge/RAG → Dynamic State → Conversation Memory Policy → Action/Tool Layer

### 4.4 市場數據（來自來源，未經正式驗證）

| 指標 | 數據 |
|------|------|
| 進入生產環境的 Agent 專案 | ~12%（88% 失敗率） |
| LangChain/LangGraph 月下載（2025 年 10 月） | 9,000 萬，35% Fortune 500 |
| Anthropic Claude Code ARR | ~$10 億（GA 後 6 個月內） |
| Anthropic OAuth token 封鎖 | 2026 年 1 月，阻止第三方 harness 使用 Claude 訂閱 token |

---

## 五、研究限制與注意事項

1. **來源品質**：三項存活主張的主要來源均為產業部落格與技術媒體，非 peer-reviewed 學術期刊。雖然每項都有獨立學術來源的交叉驗證，但核心論述框架來自產業實務者。

2. **時間敏感性**：MCP 與 A2A 的治理結構已於 2025 年中發生變化，協定規範仍在快速迭代中。2026 年下半年的實際採用率可能與當前描述不同。

3. **分裂投票**：主張 3（四類下游風險）以 2-1 而非一致同意通過，反映驗證者對廠商來源權重判斷的分歧。

4. **覆蓋範圍缺口**：研究問題中提到的具體框架（OpenAI Agents SDK、LangChain/LangGraph、CrewAI、AutoGen）在存活主張中幾乎未被觸及——存活主張集中在協定層（MCP/A2A）與系統屬性（錯誤傳播、安全風險），而非特定框架的架構比較。關於 Claude Code 具體架構（while-loop vs DAG）、框架比較（LangGraph 的複雜度取捨）、以及新興模式（3-stage pipeline、execution tax）的主張**未通過驗證**，因此本報告無法對這些主題做出高置信度結論。

5. **被駁回主張的價值**：12 項被駁回的主張中的量化數據（如 90% 失敗來自 prompt、22.9% execution tax、50,000-150,000 token 衰減區間）不得作為事實引用，但指出了值得進一步研究的實證方向。

---

## 六、開放問題

1. **最佳架構取捨邊界**：在簡單 while-loop（Claude Code 模式）與結構化 DAG/工作流編排（LangGraph 模式）之間，是否存在可驗證的最佳架構取捨邊界？目前缺乏在不同任務複雜度、agent 數量、延遲預算下的系統性比較研究。

2. **Shadow MCP 治理**：企業應如何治理開發者透過 IDE 駐留工具（Cursor、Claude Code、Copilot）建立的、繞過中央 IT 審查的 agent-to-tool 連線？實際規模未知。

3. **完整協定棧分工**：MCP、A2A、Pilot Protocol、ANP 在完整企業 agent stack 中的確切分工與重疊邊界為何？業界對此尚無共識。

4. **驗證閘的最優設計**：什麼具體架構設計（schema-enforced generation、adversarial cross-checking、majority voting、genealogy-graph traceability）在不同 agent 數量與任務類型下提供最佳的成本-可靠性取捨？

---

## 七、建議

### 給架構師與技術決策者

1. **採用 MCP + A2A 雙層協定**（高置信度）：先解決 agent-to-tool 標準化，再處理 agent-to-agent
2. **投資架構層驗證機制，而非模型升級**（高置信度）：驗證閘、circuit breaker、結構化輸出驗證的 ROI 遠高於升級到更大的模型
3. **建立四類風險的緩解策略**（中高置信度）：連鎖錯誤的 circuit breaker、攻擊面的存取控制、跨系統的 audit trail、資料流動的治理框架
4. **採用 MCP 前先做盤點**：發現並治理未授權的 agent-to-tool 連接

### 給工程師

1. **Context engineering 優先於 prompt engineering**
2. **結構化驗證閘放在每個 agent 邊界**
3. **實施耐久執行**：checkpointing、重試、狀態持久化
4. **不要讓 agent 自己決定下一步**：固定控制層管理所有狀態轉換
5. **注意 p^n 複合法則**：多代理系統的可靠性瓶頸在錯誤傳播，不在單點能力

---

## 來源

| # | 標題 | URL |
|---|------|-----|
| 1 | MCP or A2A? What to Know Before You Connect AI (ReliaQuest) | https://reliaquest.com/blog/mcp-a2a-what-to-know-before-you-connect-ai/ |
| 2 | MCP vs. A2A vs. Open Responses (dev.to) | https://dev.to/jangwook_kim_e31e7291ad98/mcp-vs-a2a-vs-open-responses-ai-agent-communication-protocols-in-2026-what-to-actually-use-5goh |
| 3 | MCP, A2A, and Pilot Protocol Are Not Competing (dev.to) | https://dev.to/artem_a/mcp-a2a-and-pilot-protocol-are-not-competing-your-agent-stack-probably-needs-all-three-323e |
| 4 | MCP Adoption in 2026 (Knak) | https://knak.com/blog/mcp-adoption-in-2026-what-marketers-need-to-know/ |
| 5 | MCP for Enterprise Agents (AgentMode) | https://agentmodeai.com/mcp-enterprise-agent-tooling/ |
| 6 | A2A and MCP in 2026 (dev.to) | https://dev.to/chunxiaoxx/a2a-and-mcp-in-2026-different-layers-one-agent-stack-169j |
| 7 | Workflow vs. Agent (HuggingFace) | https://huggingface.co/blog/MengkangHu/workflow-vs-agent |
| 8 | Building Production AI Agents (ZenML/Anthropic) | https://www.zenml.io/llmops-database/building-production-ai-agents-lessons-from-claude-code-and-enterprise-deployments |
| 9 | Architecture Patterns of Autonomous Coding Agents (ZenML) | https://www.zenml.io/llmops-database/architecture-and-production-patterns-of-autonomous-coding-agents |
| 10 | Intentive: Fit-for-purpose AI Orchestration (GitHub) | https://github.com/katasec/intentive |
| 11 | Agent Frameworks, Runtimes, and Harnesses (C# Corner) | https://www.c-sharpcorner.com/article/understanding-agent-frameworks-runtimes-and-harnesses-in-modern-ai-systems/ |
| 12 | Speed vs. Control vs. Flexibility (Ekimetrics) | https://ekimetrics.github.io/blog/AI_Architecture_Selection/ |
| 13 | LLM Frameworks Compared 2026 (Morph) | https://www.morphllm.com/llm-frameworks |
| 14 | AI Agent 開發框架調研報告 (CSDN) | https://blog.csdn.net/2401_85390073/article/details/158354103 |
| 15 | A Survey of Agent Interoperability Protocols (arXiv) | https://arxiv.org/abs/2505.02279 |
| 16 | Harness Engineering (Faros AI) | https://www.faros.ai/blog/harness-engineering |
| 17 | Your Agent Needs a Harness, Not a Framework (Inngest) | https://inngest.vercel.app/blog/your-agent-needs-a-harness-not-a-framework |
| 18 | Agentic Harness Patterns Skill (GitHub) | https://github.com/keli-wen/agentic-harness-patterns-skill |
| 19 | Production Deployment Challenges (ZenML) | https://www.zenml.io/llmops-database/production-deployment-challenges-and-infrastructure-gaps-for-multi-agent-ai-systems |
| 20 | The Missing Layer (Gradient Flow) | https://gradientflow.com/the-missing-layer-why-your-ai-agent-fails-and-what-actually-fixes-it/ |
| 21 | Context Engineering with OpenAI (Fractal Analytics) | https://fractal.ai/blog/context-engineering-openai |
| 22 | Build Better AI Agents (Google Developer Blog) | https://developers.googleblog.com/build-better-ai-agents-5-developer-tips-from-the-agent-bake-off/ |
| 23 | Production-Ready AI Agent Checklist (Arthur.ai) | https://www.arthur.ai/blog/checklist-to-launch-a-production-ready-ai-agent |
| 24 | Why CrewAI's Architecture Fails (TDS) | https://towardsdatascience.com/why-crewais-manager-worker-architecture-fails-and-how-to-fix-it/ |
| 25 | Why AI Agents Fail in Production (Diagrid) | https://www.diagrid.io/blog/why-ai-agents-fail-in-production |
| 26 | Agent Execution Tax (Fireworks AI) | https://fireworks.ai/blog/agent-execution-tax |
| 27 | The Hidden Cost of Agentic Failure (O'Reilly) | https://www.oreilly.com/radar/the-hidden-cost-of-agentic-failure/ |
| 28 | Harness: Popular Now, Soon Obsolete? (36Kr) | https://eu.36kr.com/en/p/3764520322138887 |
| 29 | Problems with Generalist Multi-Agent Frameworks (Corti) | https://www.corti.ai/stories/the-problems-with-generalist-multi-agent-frameworks |
| 30 | A Study On Agentic Frameworks (Forbes) | https://www.forbes.com/councils/forbestechcouncil/2025/09/26/a-study-on-agentic-frameworks-what-the-market-is-teaching-about-ais-next-layer/ |

---

*報告由 deep-research workflow 生成：108 agent、6,311,477 tokens、805 工具呼叫、~2 小時。3 項主張經 3 票對抗驗證存活，12 項被駁回。每個驗證判決包含獨立查核證據。原始 workflow 輸出：`/private/tmp/claude-501/-Users-kaecer-workspace-atlas/d5acf338-d16f-431b-b3af-cd69114bb69e/tasks/wm8d0m5vt.output`*
