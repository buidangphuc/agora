UPDATE identity_outbox_events
SET status = 'pending', attempts = 0, available_at = now(), locked_until = NULL, error = NULL
WHERE status = 'failed';
UPDATE identity_outbox_events SET aggregate_type = 'User' WHERE status = 'pending' AND aggregate_type <> 'User';
