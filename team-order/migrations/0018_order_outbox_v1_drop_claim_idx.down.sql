-- Forward-only: v1 does not use the v0 claim index; recreating it would only slow
-- writes. A rollback to the v0 release still works, its claim query is just slower.
SELECT 1;
