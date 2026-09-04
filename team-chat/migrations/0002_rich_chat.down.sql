-- 0002_rich_chat.down.sql

DROP TABLE IF EXISTS quick_replies;

ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS payload,
    DROP COLUMN IF EXISTS listing_id,
    DROP COLUMN IF EXISTS message_type;
