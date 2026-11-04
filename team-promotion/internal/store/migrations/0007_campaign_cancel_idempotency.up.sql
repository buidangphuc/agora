ALTER TABLE flash_sale_campaigns ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS campaign_idempotency (
    actor_id     TEXT NOT NULL,
    key          TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    campaign_id  TEXT NOT NULL REFERENCES flash_sale_campaigns (id),
    created_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (actor_id, key)
);

CREATE INDEX IF NOT EXISTS campaign_idempotency_created_idx ON campaign_idempotency (created_at);
