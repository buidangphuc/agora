-- 0009_outbox_v1_seq_not_null — step 4 of 8 (outbox.UpgradeSQL Finish): seq NOT NULL.
-- The first statement makes a rerun after an interruption safe.

ALTER TABLE identity_outbox_events DROP CONSTRAINT IF EXISTS identity_outbox_events_seq_nn;
ALTER TABLE identity_outbox_events ADD CONSTRAINT identity_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
ALTER TABLE identity_outbox_events VALIDATE CONSTRAINT identity_outbox_events_seq_nn;
ALTER TABLE identity_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE identity_outbox_events DROP CONSTRAINT identity_outbox_events_seq_nn;
