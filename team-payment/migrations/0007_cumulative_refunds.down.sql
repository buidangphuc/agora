-- 0007_cumulative_refunds.down.sql — back to the single-refund model (design D9, Down).
--   * PARTIALLY_REFUNDED (5) becomes REFUNDED (4): the old code does not know 5 and its
--     credit path would dead-letter such payments. Those payments are then closed.
--   * Legacy deductions reference the payment id again. Deductions written by new refunds
--     keep their rpc:/return:/cancel: references; the old hold-back does not net them,
--     which holds more than necessary (safe, never less).
--   * reference_id narrows back to 64 chars only when no value is longer.
ALTER TABLE payment_transactions DROP CONSTRAINT IF EXISTS payment_transactions_partially_refunded_chk;
ALTER TABLE payment_transactions DROP CONSTRAINT IF EXISTS payment_transactions_refunded_full_chk;

UPDATE payment_transactions SET status = 4 WHERE status = 5;

UPDATE wallet_ledger SET reference_id = substr(reference_id, length('legacy:') + 1)
WHERE type = 'REFUND_DEDUCTION' AND reference_id LIKE 'legacy:%';

DROP TABLE IF EXISTS payment_refunds;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM wallet_ledger WHERE length(reference_id) > 64) THEN
        ALTER TABLE wallet_ledger ALTER COLUMN reference_id TYPE VARCHAR(64);
    END IF;
END $$;
