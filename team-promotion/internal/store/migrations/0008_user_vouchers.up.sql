ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS owner_user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS kind INT NOT NULL DEFAULT 1;
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS per_user_limit INT NOT NULL DEFAULT 0;
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS new_user_only BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE vouchers ADD CONSTRAINT vouchers_new_user_owner_check CHECK (kind <> 2 OR owner_user_id <> '');
CREATE UNIQUE INDEX IF NOT EXISTS vouchers_new_user_owner_unique ON vouchers (owner_user_id) WHERE kind = 2;
CREATE INDEX IF NOT EXISTS vouchers_owner_idx ON vouchers (owner_user_id) WHERE owner_user_id <> '';

ALTER TABLE voucher_reservations ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
UPDATE voucher_reservations SET expires_at = created_at + interval '15 minutes' WHERE expires_at IS NULL;
ALTER TABLE voucher_reservations ALTER COLUMN expires_at SET NOT NULL;
CREATE INDEX IF NOT EXISTS voucher_reservations_buyer_idx ON voucher_reservations (voucher_id, buyer_id);

CREATE TABLE IF NOT EXISTS promotion_processed_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS promotion_processed_events_at_idx ON promotion_processed_events (processed_at);

CREATE TABLE IF NOT EXISTS new_user_voucher_state (
    user_id       TEXT PRIMARY KEY,
    registered    BOOLEAN NOT NULL DEFAULT false,
    has_code      BOOLEAN NOT NULL DEFAULT false,
    verdict       TEXT NOT NULL DEFAULT '',
    registered_at TIMESTAMPTZ,
    issued_at     TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS new_user_voucher_state_pending_idx ON new_user_voucher_state (registered_at)
    WHERE issued_at IS NULL AND registered AND has_code AND verdict = '';
CREATE INDEX IF NOT EXISTS new_user_voucher_state_issued_idx ON new_user_voucher_state (issued_at)
    WHERE issued_at IS NOT NULL;
