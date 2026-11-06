ALTER TABLE referrals ADD COLUMN status TEXT NOT NULL DEFAULT 'REWARDED';
ALTER TABLE referrals ALTER COLUMN status SET DEFAULT 'ATTRIBUTED';
ALTER TABLE referrals ADD COLUMN attributed_at TIMESTAMPTZ;
UPDATE referrals SET attributed_at = created_at;
ALTER TABLE referrals ALTER COLUMN attributed_at SET NOT NULL;
ALTER TABLE referrals ALTER COLUMN attributed_at SET DEFAULT NOW();
ALTER TABLE referrals ADD CONSTRAINT referrals_status_check
    CHECK (status IN ('ATTRIBUTED', 'QUALIFIED', 'REWARDED', 'CAPPED'));

ALTER TABLE referral_rewards ADD COLUMN status TEXT NOT NULL DEFAULT 'RELEASED';
ALTER TABLE referral_rewards ALTER COLUMN status SET DEFAULT 'PENDING';
ALTER TABLE referral_rewards ADD COLUMN release_at TIMESTAMPTZ;
ALTER TABLE referral_rewards ADD COLUMN released_at TIMESTAMPTZ;
ALTER TABLE referral_rewards ADD COLUMN order_id VARCHAR(64);
ALTER TABLE referral_rewards ADD COLUMN referee_id VARCHAR(64);
UPDATE referral_rewards SET release_at = created_at, released_at = created_at;
ALTER TABLE referral_rewards ALTER COLUMN release_at SET NOT NULL;
ALTER TABLE referral_rewards ADD CONSTRAINT referral_rewards_status_check
    CHECK (status IN ('PENDING', 'RELEASED', 'VOID'));
CREATE UNIQUE INDEX uq_referral_rewards_order ON referral_rewards (order_id) WHERE order_id IS NOT NULL;
CREATE INDEX idx_referral_rewards_status_release ON referral_rewards (status, release_at);
CREATE INDEX idx_referral_rewards_user_released ON referral_rewards (user_id, released_at);

CREATE TABLE referral_accounts (
    user_id       VARCHAR(64) PRIMARY KEY,
    registered_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE referral_buyer_orders (
    buyer_id           VARCHAR(64) PRIMARY KEY,
    first_completed_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE referral_processed_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_referral_processed_events_at ON referral_processed_events (processed_at);

CREATE TABLE referral_outbox_events (
    event_id       TEXT PRIMARY KEY,
    aggregate_type TEXT        NOT NULL,
    aggregate_id   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    topic          TEXT,
    request_id     TEXT        NOT NULL DEFAULT '',
    traceparent    TEXT,
    seq            BIGSERIAL,
    status         TEXT        NOT NULL DEFAULT 'pending',
    attempts       INT         NOT NULL DEFAULT 0,
    available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    error          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX referral_outbox_events_status_idx ON referral_outbox_events (status);
CREATE INDEX referral_outbox_events_agg_idx ON referral_outbox_events (aggregate_type, aggregate_id, seq) WHERE status <> 'published';
CREATE INDEX referral_outbox_events_pending_idx ON referral_outbox_events (seq) WHERE status = 'pending';
