-- 0007_cumulative_refunds.up.sql — one payment, many refunds (payment-refund-model, design D1/D2/D9).
--   * payment_refunds: one row per refund, keyed by its stored refund key
--     (rpc:<refund_id> | return:<return_id> | cancel:<order_id> | legacy:<payment_id>),
--     with the requested and the applied amount. The payment row lock serialises refunds.
--   * wallet_ledger.reference_id widens to 96 chars: a REFUND_DEDUCTION now references
--     its refund key, not the payment.
--   * Backfill: each REFUNDED payment with a positive refunded_amount gains one LEGACY
--     refund legacy:<payment_id>, and its deduction is re-pointed to it. No amount changes,
--     so balances and hold-back are unchanged. Legacy partial refunds stay REFUNDED (closed).
--   * status 5 = PARTIALLY_REFUNDED (0 < refunded_amount < amount, validated);
--     status 4 = REFUNDED means refunded_amount = amount for new writes (NOT VALID: legacy
--     partial REFUNDED rows and pre-0006 REFUNDED rows with refunded_amount = 0 are exempt).
-- Before applying to a long-lived DB, expect zero rows from:
--   SELECT id FROM payment_transactions WHERE status = 5;
--   SELECT d.id FROM wallet_ledger d
--   WHERE d.type = 'REFUND_DEDUCTION'
--     AND d.reference_id IS NOT NULL
--     AND NOT EXISTS (SELECT 1 FROM payment_transactions t WHERE t.id = d.reference_id);

ALTER TABLE wallet_ledger ALTER COLUMN reference_id TYPE VARCHAR(96);

CREATE TABLE IF NOT EXISTS payment_refunds (
    id               VARCHAR(96) PRIMARY KEY,
    payment_id       VARCHAR(64) NOT NULL REFERENCES payment_transactions(id),
    source           VARCHAR(16) NOT NULL
                     CHECK (source IN ('SELLER_OR_ADMIN', 'RETURN', 'ORDER_CANCEL', 'LEGACY')),
    source_id        VARCHAR(64) NOT NULL,
    requested_amount BIGINT NOT NULL CHECK (requested_amount > 0),
    amount           BIGINT NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_refunds_applied_chk CHECK (amount >= 0 AND amount <= requested_amount),
    CONSTRAINT payment_refunds_zero_only_return_chk CHECK (amount > 0 OR source = 'RETURN')
);

CREATE INDEX IF NOT EXISTS idx_payment_refunds_payment_created
    ON payment_refunds (payment_id, created_at, id);

INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount, reason, created_at)
SELECT 'legacy:' || id, id, 'LEGACY', id, refunded_amount, refunded_amount, provider_reference, updated_at
FROM payment_transactions
WHERE status = 4 AND refunded_amount > 0
ON CONFLICT (id) DO NOTHING;

UPDATE wallet_ledger SET reference_id = 'legacy:' || reference_id
WHERE type = 'REFUND_DEDUCTION'
  AND reference_id IN (SELECT payment_id FROM payment_refunds WHERE source = 'LEGACY');

ALTER TABLE payment_transactions DROP CONSTRAINT IF EXISTS payment_transactions_refunded_full_chk;
ALTER TABLE payment_transactions ADD CONSTRAINT payment_transactions_refunded_full_chk CHECK (
    status <> 4 OR refunded_amount = amount
) NOT VALID;

ALTER TABLE payment_transactions DROP CONSTRAINT IF EXISTS payment_transactions_partially_refunded_chk;
ALTER TABLE payment_transactions ADD CONSTRAINT payment_transactions_partially_refunded_chk CHECK (
    status <> 5 OR (refunded_amount > 0 AND refunded_amount < amount)
);
