CREATE INDEX IF NOT EXISTS identity_outbox_claim_idx
    ON identity_outbox_events (available_at, created_at)
    WHERE status = 'pending';
