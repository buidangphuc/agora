-- Append-only audit trail: who (actor_id) did what (action) to which target
-- (target_type + target_id), with free-form metadata. Owned by team-audit.
-- Rows are IMMUTABLE: the service never issues UPDATE or DELETE against this
-- table — an audit log you can rewrite is not an audit log.
CREATE TABLE IF NOT EXISTS audit_events (
    id          VARCHAR(64) PRIMARY KEY,
    actor_id    VARCHAR(64) NOT NULL DEFAULT '',
    action      VARCHAR(128) NOT NULL,
    target_type VARCHAR(64) NOT NULL DEFAULT '',
    target_id   VARCHAR(64) NOT NULL DEFAULT '',
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- QueryAuditLog filters by actor and returns newest-first, so index the trail
-- by (actor_id, created_at).
CREATE INDEX IF NOT EXISTS idx_audit_events_actor_created
    ON audit_events (actor_id, created_at DESC);

-- QueryAuditLog also filters by target; index by (target_type, target_id,
-- created_at) for "everything that happened to this listing/order" scans.
CREATE INDEX IF NOT EXISTS idx_audit_events_target_created
    ON audit_events (target_type, target_id, created_at DESC);
