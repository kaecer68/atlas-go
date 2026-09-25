-- 000024_symbol_industry.up.sql
-- 產業層統計需要「可查詢的個股產業欄位」：過去只有約 27 檔代表性
-- 硬編碼股票，無法在真實母體上計算產業統計。第一方資料通道
-- symbol_industry 抓取 TWSE/TPEx 產業別（上游 2 位數代碼），並映射到
-- canonical L1 sector id；本表是其落地鏡像，供查詢與 join 使用。
-- Dual-dialect DDL（PostgreSQL + SQLite）：TEXT/INTEGER/DOUBLE PRECISION
-- 皆為共通型別；TIMESTAMPTZ NOT NULL DEFAULT now() 由 PostgreSQL 使用，
-- SQLite 端由 store 以 RFC3339 TEXT 寫入。
CREATE TABLE IF NOT EXISTS symbol_industry (
    symbol TEXT PRIMARY KEY,
    company_name TEXT NOT NULL DEFAULT '',
    market TEXT NOT NULL DEFAULT '',
    industry_code TEXT NOT NULL DEFAULT '',
    industry_name_zh TEXT NOT NULL DEFAULT '',
    canonical_l1 TEXT NOT NULL DEFAULT '',
    mapping_status TEXT NOT NULL DEFAULT '',
    mapping_reason TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    as_of TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_symbol_industry_canonical_l1 ON symbol_industry(canonical_l1);
