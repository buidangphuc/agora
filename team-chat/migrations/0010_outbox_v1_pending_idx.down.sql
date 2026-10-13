-- Forward-only: the go-sdk v1 outbox upgrade is not reversed. The columns, sequence
-- and indexes stay; a v0 relayer ignores them.
SELECT 1;
