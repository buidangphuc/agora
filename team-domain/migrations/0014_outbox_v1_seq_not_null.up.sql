-- 0014_outbox_v1_seq_not_null: seq NOT NULL through a validated CHECK first, so
-- the final SET NOT NULL needs no full scan under ACCESS EXCLUSIVE on a big
-- table (outbox.UpgradeSQL "Finish"). The first statement makes a rerun safe.
ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS outbox_events_seq_nn;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
ALTER TABLE outbox_events VALIDATE CONSTRAINT outbox_events_seq_nn;
ALTER TABLE outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_seq_nn;
