-- No-op: dropping topic, traceparent and seq would discard the event order the
-- later steps establish, and the v0 code ignores the extra columns.
SELECT 1;
