DROP INDEX IF EXISTS idx_payout_requests_ledger_entry_id;
ALTER TABLE payout_requests DROP COLUMN IF EXISTS ledger_entry_id;
