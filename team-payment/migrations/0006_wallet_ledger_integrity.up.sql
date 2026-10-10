-- 0006_wallet_ledger_integrity.up.sql — the ledger store keeps settlement and refund rows
-- unique and sign-consistent (port-payment-ledger-integrity, design D3).
--   * reference_id: what the entry is about (the payment transaction id for
--     ORDER_SETTLEMENT / REFUND_DEDUCTION). NULL for payouts and for legacy credits.
--   * unique (type, reference_id) where referenced: one credit and one deduction per
--     payment, so redelivery and retries never double-write.
--   * type/sign CHECK (validated; existing rows comply): settlement > 0, refund
--     deduction < 0, payout < 0 except a REJECTED compensating reversal > 0.
--   * reference required for new settlement / refund-deduction rows (NOT VALID: legacy
--     unreferenced credits written by the old inline credit stay readable and count).
--   * payment_transactions.refunded_amount: what a refund took back (0..amount).
-- Before applying to a long-lived DB, expect zero rows from:
--   SELECT type, count(*) FROM wallet_ledger WHERE NOT (
--     (type = 'ORDER_SETTLEMENT' AND amount > 0) OR (type = 'REFUND_DEDUCTION' AND amount < 0)
--     OR (type = 'PAYOUT' AND (amount < 0 OR (status = 'REJECTED' AND amount > 0)))) GROUP BY type;

ALTER TABLE wallet_ledger ADD COLUMN IF NOT EXISTS reference_id VARCHAR(64) NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_wallet_ledger_type_reference
    ON wallet_ledger (type, reference_id) WHERE reference_id IS NOT NULL;

ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS wallet_ledger_type_sign_chk;
ALTER TABLE wallet_ledger ADD CONSTRAINT wallet_ledger_type_sign_chk CHECK (
    (type = 'ORDER_SETTLEMENT' AND amount > 0)
    OR (type = 'REFUND_DEDUCTION' AND amount < 0)
    OR (type = 'PAYOUT' AND (amount < 0 OR (status = 'REJECTED' AND amount > 0)))
);

ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS wallet_ledger_reference_required_chk;
ALTER TABLE wallet_ledger ADD CONSTRAINT wallet_ledger_reference_required_chk CHECK (
    type NOT IN ('ORDER_SETTLEMENT', 'REFUND_DEDUCTION') OR reference_id IS NOT NULL
) NOT VALID;

ALTER TABLE payment_transactions ADD COLUMN IF NOT EXISTS refunded_amount BIGINT NOT NULL DEFAULT 0;

ALTER TABLE payment_transactions DROP CONSTRAINT IF EXISTS payment_transactions_refunded_amount_chk;
ALTER TABLE payment_transactions ADD CONSTRAINT payment_transactions_refunded_amount_chk CHECK (
    refunded_amount >= 0 AND refunded_amount <= amount
);
