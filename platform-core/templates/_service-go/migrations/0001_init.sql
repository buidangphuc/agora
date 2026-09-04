-- 0001_init.sql — placeholder migration for this service's OWN database.
--
-- Tooling: golang-migrate (https://github.com/golang-migrate/migrate).
-- File-per-version, applied with:
--   migrate -path ./migrations -database "$DATABASE_URL" up
-- Convention: pair each NNNN_name.up.sql with an NNNN_name.down.sql when you
-- adopt golang-migrate's up/down split; this seed keeps a single file for
-- illustration. DB-per-service: only this service touches listing_db.

CREATE TABLE IF NOT EXISTS listings (
    id          TEXT PRIMARY KEY,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    price       BIGINT      NOT NULL,           -- minor units (e.g. VND)
    currency    TEXT        NOT NULL,           -- ISO 4217, e.g. 'VND'
    status      TEXT        NOT NULL DEFAULT 'draft',  -- draft|published|rejected
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS listings_status_idx ON listings (status);
