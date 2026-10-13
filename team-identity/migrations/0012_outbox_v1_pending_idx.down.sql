-- Forward-only: the v1 outbox schema is not downgraded (the v0 relayer ignores seq,
-- and dropping seq would lose the event order). Kept as a no-op so the numbering stays paired.
SELECT 1;
