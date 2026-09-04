-- 0008_loyalty.up.sql — Loyalty / daily check-in (F4).
-- team-engagement OWNS engagement_db (Rule 3).

-- loyalty_accounts: one row per user holding the running streak, coin balance,
-- and the calendar day of the last check-in. Upserted by CheckIn.
CREATE TABLE IF NOT EXISTS loyalty_accounts (
    user_id      TEXT   PRIMARY KEY,
    coin_balance BIGINT NOT NULL DEFAULT 0,
    streak       INT    NOT NULL DEFAULT 0,
    last_checkin DATE
);

-- checkins: one row per (user_id, day). The UNIQUE (user_id, day) makes a daily
-- check-in idempotent (ON CONFLICT DO NOTHING) — a repeat the same calendar day
-- is a no-op that awards no coins.
CREATE TABLE IF NOT EXISTS checkins (
    user_id    TEXT        NOT NULL,
    day        DATE        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, day)
);
CREATE INDEX IF NOT EXISTS checkins_user_idx ON checkins (user_id);
