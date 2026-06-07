# ODM Channel Monitor

Track AI server ODM diffusion channel through Taiwan's three major ODM manufacturers: Foxconn (2317), Quanta (2382), and Wiwynn (6669). Monitor monthly revenue trends, CoWoS capacity utilization, and the transmission model from US CSP capex through Nvidia orders to Taiwan supply chain impact.

## Transmission Model
US CSP capex → Nvidia order growth → CoWoS capacity utilization → TSMC revenue impact → ODM order impact

## Key Metrics
- Monthly revenue (YoY%) for each ODM
- CoWoS utilization delta from baseline (0.85)
- ODM order impact (%) from cascade model
- Cross-reference with AI supply chain desk for CSP capex signals

## Monitoring Principles
1. Revenue divergence between ODMs signals market share shifts
2. CoWoS utilization above 0.95 indicates supply constraint bottleneck
3. Negative US capex shock propagates with ~1.68% ODM impact per 10% CSP decline
4. Always verify FinMind monthly revenue freshness before analysis
5. Flag anomalies: single-month revenue spikes not backed by order data
