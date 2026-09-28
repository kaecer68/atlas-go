-- 期貨日行情（first-party）資料層 — 階段 0
-- 規格：docs/specs/futures-bars-firstparty-spec.md §5.3
--
-- 為什麼是新表而不是沿用 quotes：
--   quotes 以 (symbol, date) 為鍵，期貨的鍵是 (contract, contract_month, date, session)；
--   混用會讓 symbol 命名空間與語意都被污染，且查詢層無法區分「現貨」與「期貨」。
--
-- 缺值一律 NULL：上游的 "-"/"NULL" 在 provider 層已轉成 Go nil，
-- **禁止**以 0 代表缺值（0 是合法成交量，缺值是「沒有這個值」）。

CREATE TABLE IF NOT EXISTS futures_bars (
    contract         TEXT             NOT NULL,
    contract_month   TEXT             NOT NULL,
    trade_date       DATE             NOT NULL,
    session          TEXT             NOT NULL,
    open             DOUBLE PRECISION NULL,
    high             DOUBLE PRECISION NULL,
    low              DOUBLE PRECISION NULL,
    close            DOUBLE PRECISION NULL,
    volume           BIGINT           NULL,
    settlement_price DOUBLE PRECISION NULL,
    open_interest    BIGINT           NULL,
    source           TEXT             NOT NULL,
    fetched_at       TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (contract, contract_month, trade_date, session)
);

CREATE INDEX IF NOT EXISTS idx_futures_bars_contract_date
    ON futures_bars (contract, trade_date);

-- 連續契約（rollover back-adjust）的 splice 稽核錨點。
CREATE TABLE IF NOT EXISTS futures_rollovers (
    contract    TEXT             NOT NULL,
    roll_date   DATE             NOT NULL,
    from_month  TEXT             NOT NULL,
    to_month    TEXT             NOT NULL,
    price_diff  DOUBLE PRECISION NULL,
    computed_at TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (contract, roll_date, from_month, to_month)
);
