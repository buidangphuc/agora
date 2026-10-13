-- Forward-only: the v1 columns and sequence are additive, and dropping seq would
-- lose the event order the relayer depends on. A v0 relayer ignores them, so
-- rolling the service back needs no schema change.
SELECT 1;
