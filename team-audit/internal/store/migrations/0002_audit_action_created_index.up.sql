-- QueryAuditLog can filter by exact action, newest first.
CREATE INDEX IF NOT EXISTS idx_audit_events_action_created
    ON audit_events (action, created_at DESC);
