# Phase 5: Portfolio Intelligence 架构设计

## 设计目标

从单 Agent 推荐转向多因子组合优化，实现：
1. **多 Agent 信号整合** - 不再单一 Agent 决策，而是多 Agent 投票权重
2. **风险调整仓位** - 根据波动率、相关性动态调整仓位规模
3. **动态风格配置** - 根据市场状态调整 Growth/Value/Momentum 等风格权重
4. **实时组合监控** - 组合层面风险控制和再平衡

## 核心组件

### 1. Portfolio Optimizer (`internal/portfolio/optimizer.go`)

多因子优化引擎：
- **输入**: Agent 推荐列表（symbol, side, conviction, agent_id）
- **因子计算**:
  - 动量因子 (12个月价格趋势)
  - 价值因子 (P/E, P/B 百分位)
  - 质量因子 (ROE 稳定性)
  - Agent 置信度加权
- **优化目标**: 最大化风险调整后收益 (Sharpe-like)
- **约束条件**:
  - 单票仓位上限 15%
  - 行业集中度上限 40%
  - 组合 Beta 0.8-1.2
  - 最小交易规模 1手

### 2. Risk-Adjusted Sizing (`internal/portfolio/sizing.go`)

仓位规模计算：
- **Kelly Criterion 变体**: f = (p*b - q)/b，其中 p=胜率, b=盈亏比
- **波动率缩放**: 基于 20 日 ATR 调整仓位
- **相关性惩罚**: 高相关性资产降低权重
- **流动性检查**: 基于日均成交量限制最大仓位

### 3. Regime Allocator (`internal/portfolio/regime.go`)

市场状态动态配置：
- **RISK_ON**: 偏好 Growth + Momentum，提高仓位上限
- **RISK_OFF**: 偏好 Value + Quality，降低仓位，增加现金
- **NEUTRAL**: 均衡配置，标准仓位

配置表:
```
| Regime   | Growth | Value | Momentum | Quality | Max Exposure |
|----------|--------|-------|----------|---------|--------------|
| RISK_ON  | 40%    | 20%   | 30%      | 10%     | 95%          |
| NEUTRAL  | 25%    | 25%   | 25%      | 25%     | 80%          |
| RISK_OFF | 10%    | 40%   | 15%      | 35%     | 50%          |
```

### 4. Agent Weight Manager (`internal/portfolio/agent_weights.go`)

动态 Agent 权重调整：
- **回测表现评分**: 基于最近 20 日窗口的 Agent 命中率
- **衰减机制**: 较早的表现权重更低
- **平滑处理**: 使用 EMA 避免权重剧烈波动
- **权重约束**: 单个 Agent 权重 5%-40%

### 5. Style Rotation (`internal/portfolio/style.go`)

风格轮动检测与执行：
- **风格动量计算**: 各风格指数 20 日收益
- **风格切换信号**: Growth/Value 相对强度突破阈值
- **切换缓冲期**: 避免频繁切换（最小持有 5 日）

### 6. Post-Trade Analysis (`internal/portfolio/analysis.go`)

盘后分析与归因：
- **收益归因**: 分解到 Agent、风格、个股
- **风险归因**: VaR 分解、最大回撤分析
- **执行质量**: 滑点分析、成交率统计
- **改进建议**: 基于模式识别的问题诊断

## 数据流

```
┌─────────────────┐
│  Agent Registry │
│  (13 agents)    │
└────────┬────────┘
         │ Recommendations
         ▼
┌─────────────────┐     ┌─────────────────┐
│ Agent Weight    │────▶│ Multi-Factor    │
│ Manager         │     │ Scoring         │
└─────────────────┘     └────────┬────────┘
                               │
                    ┌──────────┼──────────┐
                    ▼          ▼          ▼
              ┌────────┐  ┌────────┐  ┌────────┐
              │Momentum│  │ Value  │  │Quality │
              │Score   │  │Score   │  │Score   │
              └────────┘  └────────┘  └────────┘
                    │          │          │
                    └──────────┼──────────┘
                               ▼
                    ┌─────────────────┐
                    │ Portfolio       │
                    │ Optimizer       │
                    │ (Mean-Variance) │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
┌────────────────┐  ┌────────────────┐  ┌────────────────┐
│ Risk-Adjusted │  │ Regime         │  │ Style          │
│ Sizing         │  │ Allocator      │  │ Rotation       │
│ (Kelly+ATR)   │  │ (Dynamic)      │  │ (Momentum)     │
└────────────────┘  └────────────────┘  └────────────────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             ▼
                    ┌─────────────────┐
                    │ Final Orders    │
                    │ (with sizing)   │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │ Post-Trade      │
                    │ Analysis        │
                    └─────────────────┘
```

## 接口定义

### PortfolioOptimizer
```go
type Optimizer interface {
    Optimize(ctx context.Context, recommendations []Recommendation) (*Portfolio, error)
    SetConstraints(constraints Constraints)
    GetEfficientFrontier() []Point
}
```

### RiskSizer
```go
type RiskSizer interface {
    CalculateSize(signal Signal, portfolio Portfolio, marketData MarketData) (int, error)
    SetRiskParams(params RiskParameters)
}
```

### RegimeAllocator
```go
type RegimeAllocator interface {
    GetAllocation(regime Regime) StyleAllocation
    UpdateRegime(regime Regime)
}
```

## 配置参数

```yaml
portfolio:
  max_position_pct: 0.15          # 单票最大 15%
  max_sector_pct: 0.40            # 行业最大 40%
  max_turnover_daily: 0.20        # 日换手率最大 20%
  target_beta: 1.0                  # 目标 Beta
  beta_range: [0.8, 1.2]          # Beta 允许范围

risk_sizing:
  kelly_fraction: 0.25            # Kelly 系数 (保守 25%)
  vol_lookback: 20                # 波动率回望窗口
  max_position_by_adv: 0.01       # 日成交量 1% 限制

regime_allocation:
  risk_on:
    growth: 0.40
    value: 0.20
    momentum: 0.30
    quality: 0.10
    max_exposure: 0.95
  neutral:
    growth: 0.25
    value: 0.25
    momentum: 0.25
    quality: 0.25
    max_exposure: 0.80
  risk_off:
    growth: 0.10
    value: 0.40
    momentum: 0.15
    quality: 0.35
    max_exposure: 0.50

agent_weights:
  lookback_window: 20             # 表现回望窗口
  min_weight: 0.05                # 最小权重 5%
  max_weight: 0.40                # 最大权重 40%
  smoothing_factor: 0.3           # EMA 平滑系数
```

## 与现有系统集成

1. **Phase 4 Orchestrator** 调用 `PortfolioOptimizer.Optimize()` 替代直接下单
2. **Live State Store** 提供当前组合状态作为优化约束
3. **Event Bus** 广播风格轮动和 Agent 权重变更事件
4. **Monitoring** 新增组合层面告警（集中度、Beta 偏离等）

## 里程碑

- [ ] 架构文档 (本文件)
- [ ] Optimizer 核心实现
- [ ] Risk Sizing 模块
- [ ] Regime Allocator
- [ ] Agent Weight Manager
- [ ] Style Rotation
- [ ] Post-Trade Analysis
- [ ] 集成测试
