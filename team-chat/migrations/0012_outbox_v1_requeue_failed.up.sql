UPDATE chat_outbox_events
SET status = 'pending', attempts = 0, available_at = now(), locked_until = NULL, error = NULL
WHERE status = 'failed';
