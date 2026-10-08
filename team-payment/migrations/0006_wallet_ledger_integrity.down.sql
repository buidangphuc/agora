ALTER TABLE payment_transactions DROP CONSTRAINT IF EXISTS payment_transactions_refunded_amount_chk;
ALTER TABLE payment_transactions DROP COLUMN IF EXISTS refunded_amount;
ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS wallet_ledger_reference_required_chk;
ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS wallet_ledger_type_sign_chk;
DROP INDEX IF EXISTS uq_wallet_ledger_type_reference;
ALTER TABLE wallet_ledger DROP COLUMN IF EXISTS reference_id;
