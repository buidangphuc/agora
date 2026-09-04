-- 0002_rich_chat.up.sql — Rich chat messages (F2): message_type/listing_id/payload
-- on chat_messages + seller canned quick-replies.

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS message_type INT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS listing_id   VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payload      TEXT NOT NULL DEFAULT '';

-- Seller-configured canned quick replies buyers can pick from.
CREATE TABLE IF NOT EXISTS quick_replies (
    id         VARCHAR(64) PRIMARY KEY,
    seller_id  VARCHAR(64) NOT NULL,
    body       TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_quick_replies_seller ON quick_replies (seller_id, sort_order ASC);
