-- Restore the key on event_id alone. A pair recorded by several consumers keeps
-- its oldest row, so the old key can be built.
DELETE FROM processed_events a
    USING processed_events b
    WHERE a.event_id = b.event_id AND a.ctid > b.ctid;

ALTER TABLE processed_events DROP CONSTRAINT IF EXISTS processed_events_pkey;
ALTER TABLE processed_events ADD PRIMARY KEY (event_id);
