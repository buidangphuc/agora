-- Per-user notification preferences: which notification types a user opts into
-- and how often batched (digest) notifications are delivered. One row per user.
-- Owned by team-notification. `prefs` holds the per-type opt-in map as JSONB
-- keyed by NotificationType enum name (e.g. "NOTIFICATION_TYPE_ORDER" -> true).
CREATE TABLE IF NOT EXISTS notification_prefs (
    user_id     VARCHAR(64) PRIMARY KEY,
    prefs       JSONB NOT NULL DEFAULT '{}'::jsonb,
    digest_freq INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The digest scheduler looks recipients up by cadence to bundle their pending
-- notifications (DAILY / WEEKLY), skipping DIGEST_FREQUENCY_OFF (0).
CREATE INDEX IF NOT EXISTS idx_notification_prefs_digest_freq
    ON notification_prefs (digest_freq);
