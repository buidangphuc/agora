CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_outbox_claim ON chat_outbox_events (available_at, created_at) WHERE status = 'pending';
