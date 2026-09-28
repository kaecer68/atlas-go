-- 期貨跨市場訊號的**影子預測**儲存（階段 1）
-- 規格：docs/specs/futures-crossmarket-signal-spec.md §7
--
-- 為什麼是**獨立表**而不是共用 prediction_backtest：
--   internal/calibration/predictor_calibrator.go 以 LoadPredictionBacktestRange 讀資料，
--   而該方法的 SQL（sqlite 版）只過濾日期與 is_synthetic，**完全不過濾 model_version**，
--   之後把所有列混算成單一命中率 ⇒ 任何外來 ModelVersion 的列都會改變 predictor_* 校準分數。
--   在該前提改變前，影子列一律寫入本表 ⇒ 對 live 校準零干擾。
--
-- 缺值一律 NULL：無標籤（換倉日、序列末端）的 label_return_pct / actual_direction / hit 皆為 NULL，
-- 不得以 0 或 'neutral' 假造（0 報酬是合法值，缺標籤不是 0）。

CREATE TABLE IF NOT EXISTS futures_shadow_predictions (
    contract             TEXT             NOT NULL,
    trade_date           DATE             NOT NULL,
    model_version        TEXT             NOT NULL,
    near_month           TEXT             NULL,
    term_structure       TEXT             NULL,
    spread_points        DOUBLE PRECISION NULL,
    spread_bp            DOUBLE PRECISION NULL,
    oi_near_trend        TEXT             NULL,
    oi_near_change_pct   DOUBLE PRECISION NULL,
    near_return_pct      DOUBLE PRECISION NULL,
    foreign_oi_change    BIGINT           NULL,
    pcr_oi_ratio         DOUBLE PRECISION NULL,
    pcr_oi_ratio_change  DOUBLE PRECISION NULL,
    score                DOUBLE PRECISION NULL,
    terms_used           INTEGER          NULL,
    predicted_direction  TEXT             NULL,
    confidence           DOUBLE PRECISION NULL,
    label_return_pct     DOUBLE PRECISION NULL,
    actual_direction     TEXT             NULL,
    hit                  BOOLEAN          NULL,
    created_at           TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (contract, trade_date, model_version)
);

CREATE INDEX IF NOT EXISTS idx_futures_shadow_contract_date
    ON futures_shadow_predictions (contract, trade_date);
