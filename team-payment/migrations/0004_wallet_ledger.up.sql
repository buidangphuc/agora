-- 0004_wallet_ledger.up.sql — Append-only seller wallet ledger (mock money).
-- Balance for a seller is the SUM(amount) over their entries. A payout records a
-- PENDING debit (negative amount); settlements/credits are positive. No real money
-- ever moves (AGENTS.md §7).

CREATE TABLE IF NOT EXISTS wallet_ledger (
    id         VARCHAR(64) PRIMARY KEY,
    seller_id  VARCHAR(64) NOT NULL,
    type       VARCHAR(48) NOT NULL,               -- ORDER_SETTLEMENT | PAYOUT | REFUND_DEDUCTION
    amount     BIGINT NOT NULL,                     -- minor units; sign = credit(+)/debit(-)
    status     VARCHAR(24) NOT NULL DEFAULT 'COMPLETED', -- PENDING | COMPLETED | REJECTED
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallet_ledger_seller_id ON wallet_ledger (seller_id);
CREATE INDEX IF NOT EXISTS idx_wallet_ledger_seller_created ON wallet_ledger (seller_id, created_at DESC, id DESC);
