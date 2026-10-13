-- 0015: seq NOT NULL without one long scan under ACCESS EXCLUSIVE. The first
-- statement lets a run that was interrupted start over. The table holds only
-- recent rows, so one transaction for the five statements is acceptable.
ALTER TABLE engagement_outbox_events DROP CONSTRAINT IF EXISTS engagement_outbox_events_seq_nn;
ALTER TABLE engagement_outbox_events ADD CONSTRAINT engagement_outbox_events_seq_nn CHECK (seq IS NOT NULL) NOT VALID;
ALTER TABLE engagement_outbox_events VALIDATE CONSTRAINT engagement_outbox_events_seq_nn;
ALTER TABLE engagement_outbox_events ALTER COLUMN seq SET NOT NULL;
ALTER TABLE engagement_outbox_events DROP CONSTRAINT engagement_outbox_events_seq_nn;
