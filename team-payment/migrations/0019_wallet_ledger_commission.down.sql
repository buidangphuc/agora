DELETE FROM wallet_ledger WHERE type IN ('COMMISSION', 'COMMISSION_REVERSAL');

ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS wallet_ledger_type_sign_chk;
ALTER TABLE wallet_ledger ADD CONSTRAINT wallet_ledger_type_sign_chk CHECK (
    (type = 'ORDER_SETTLEMENT' AND amount > 0)
    OR (type = 'REFUND_DEDUCTION' AND amount < 0)
    OR (type = 'PAYOUT' AND (amount < 0 OR (status = 'REJECTED' AND amount > 0)))
    OR (type = 'SUBSCRIPTION_FEE' AND amount < 0)
);

ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS wallet_ledger_reference_required_chk;
ALTER TABLE wallet_ledger ADD CONSTRAINT wallet_ledger_reference_required_chk CHECK (
    type NOT IN ('ORDER_SETTLEMENT', 'REFUND_DEDUCTION', 'SUBSCRIPTION_FEE') OR reference_id IS NOT NULL
) NOT VALID;

ALTER TABLE wallet_ledger DROP COLUMN IF EXISTS commission_rate_bps;
