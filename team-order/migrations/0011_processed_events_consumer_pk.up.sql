ALTER TABLE processed_events DROP CONSTRAINT IF EXISTS processed_events_pkey;
ALTER TABLE processed_events ADD PRIMARY KEY (consumer, event_id);
