-- 0010_engagement_outbox.up.sql — transactional outbox for engagement facts
-- (engagement-fact-events, D1). team-engagement OWNS engagement_db (Rule 3).
--
-- One row per fact (FavoriteAdded, FavoriteRemoved, SellerFollowed,
-- SellerUnfollowed, ReviewCreated), written in the same transaction as the state
-- change. `seq` fixes the publish order; the relayer wraps each row in a
-- platform.events.v1.EventEnvelope (principal_id/principal_type/request_id carry
-- the caller) and publishes it to engagement.events keyed by `key`, then stamps
-- published_at. Published rows older than 7 days are deleted by the relayer.
CREATE TABLE IF NOT EXISTS outbox (
    seq            BIGSERIAL   PRIMARY KEY,
    event_id       UUID        NOT NULL UNIQUE,
    type           TEXT        NOT NULL,
    key            TEXT        NOT NULL,
    payload        BYTEA       NOT NULL,
    principal_id   TEXT        NOT NULL DEFAULT '',
    principal_type TEXT        NOT NULL DEFAULT '',
    request_id     TEXT        NOT NULL DEFAULT '',
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS outbox_unpublished_idx ON outbox (seq) WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS outbox_published_idx ON outbox (published_at) WHERE published_at IS NOT NULL;
