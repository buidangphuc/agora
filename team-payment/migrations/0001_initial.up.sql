-- 0001_initial.up.sql — Schema for team-payment (Mock Payment Transactions)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS payment_transactions (
    id                 VARCHAR(64) PRIMARY KEY,
    order_id           VARCHAR(64) NOT NULL,
    buyer_id           VARCHAR(64) NOT NULL,
    amount             BIGINT NOT NULL,
    currency           VARCHAR(8) NOT NULL DEFAULT 'VND',
    method             INT NOT NULL DEFAULT 1, -- 1=COD, 2=MOCK_MOMO, 3=MOCK_BANK, 4=MOCK_CARD
    status             INT NOT NULL DEFAULT 1, -- 1=PENDING, 2=PAID, 3=FAILED, 4=REFUNDED
    provider_reference VARCHAR(128) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_order_id ON payment_transactions (order_id);
CREATE INDEX IF NOT EXISTS idx_payment_buyer_id ON payment_transactions (buyer_id);
