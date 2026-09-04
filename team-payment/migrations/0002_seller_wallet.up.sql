-- 0002_seller_wallet.up.sql — Schema for Seller Wallet, Payout Requests, and Wallet Transactions

CREATE TABLE IF NOT EXISTS seller_wallets (
    id         VARCHAR(64) PRIMARY KEY,
    seller_id  VARCHAR(64) NOT NULL UNIQUE,
    balance    BIGINT NOT NULL DEFAULT 0,
    currency   VARCHAR(8) NOT NULL DEFAULT 'VND',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_seller_wallets_seller_id ON seller_wallets (seller_id);

CREATE TABLE IF NOT EXISTS payout_requests (
    id             VARCHAR(64) PRIMARY KEY,
    seller_id      VARCHAR(64) NOT NULL,
    amount         BIGINT NOT NULL,
    bank_code      VARCHAR(32) NOT NULL,
    account_number VARCHAR(64) NOT NULL,
    account_name   VARCHAR(128) NOT NULL,
    status         INT NOT NULL DEFAULT 1, -- 1=PENDING, 2=PROCESSING, 3=COMPLETED, 4=REJECTED
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payout_requests_seller_id ON payout_requests (seller_id);

CREATE TABLE IF NOT EXISTS wallet_transactions (
    id           VARCHAR(64) PRIMARY KEY,
    wallet_id    VARCHAR(64) NOT NULL,
    amount       BIGINT NOT NULL,
    type         INT NOT NULL, -- 1=ORDER_SETTLEMENT, 2=PAYOUT, 3=REFUND_DEDUCTION
    reference_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_wallet_id ON wallet_transactions (wallet_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_reference_id ON wallet_transactions (reference_id);
