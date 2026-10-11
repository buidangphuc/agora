-- 0005_payout_ledger_link.up.sql — The wallet ledger is the single source of truth
-- for seller money. A payout request now links to the ledger debit that funded it.
-- seller_wallets / wallet_transactions are legacy and no longer read or written; they
-- are left in place (no data is dropped).

ALTER TABLE payout_requests ADD COLUMN IF NOT EXISTS ledger_entry_id VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_payout_requests_ledger_entry_id ON payout_requests (ledger_entry_id);
