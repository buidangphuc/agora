-- Forward-only. After the cutover the unpublished events live only in this table,
-- so dropping it with pending rows would lose them: refuse, and drop it only when
-- nothing is waiting to be relayed. The old `outbox` table was never touched.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM engagement_outbox_events WHERE status <> 'published') THEN
        RAISE EXCEPTION 'engagement_outbox_events holds unpublished or parked events; relay or resolve them before rolling back';
    END IF;
END $$;
DROP TABLE IF EXISTS engagement_outbox_events;
