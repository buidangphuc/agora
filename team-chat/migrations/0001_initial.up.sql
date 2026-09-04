-- 0001_initial.up.sql — Schema for Buyer<->Seller Chat (Phase 6).

CREATE TABLE IF NOT EXISTS chat_threads (
    id                  VARCHAR(64) PRIMARY KEY,
    buyer_id            VARCHAR(64) NOT NULL,
    seller_id           VARCHAR(64) NOT NULL,
    listing_id          VARCHAR(64) NOT NULL DEFAULT '',
    listing_title       VARCHAR(255) NOT NULL DEFAULT '',
    listing_image_url   TEXT NOT NULL DEFAULT '',
    last_message_text   TEXT NOT NULL DEFAULT '',
    last_message_at     TIMESTAMPTZ,
    unread_count_buyer  INT NOT NULL DEFAULT 0,
    unread_count_seller INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_chat_threads_parties UNIQUE (buyer_id, seller_id, listing_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_threads_buyer ON chat_threads (buyer_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_threads_seller ON chat_threads (seller_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS chat_messages (
    id          VARCHAR(64) PRIMARY KEY,
    thread_id   VARCHAR(64) NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    sender_id   VARCHAR(64) NOT NULL,
    sender_name VARCHAR(128) NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_thread ON chat_messages (thread_id, created_at ASC);
