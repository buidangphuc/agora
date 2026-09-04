-- 0001_init — this service's OWN database (DB-per-service, Rule 3).
--
-- Tooling: golang-migrate (https://github.com/golang-migrate/migrate), applied
-- with `make migrate`. Each NNNN_name.up.sql pairs with an NNNN_name.down.sql.
-- Only team-domain connects to listing_db; other services reach this data via
-- gRPC, never by sharing the database.

CREATE TABLE IF NOT EXISTS listings (
    id          TEXT PRIMARY KEY,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    price       BIGINT      NOT NULL,                    -- minor units (e.g. VND)
    currency    TEXT        NOT NULL,                    -- ISO 4217, e.g. 'VND'
    status      TEXT        NOT NULL DEFAULT 'draft',    -- draft|published|rejected
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS listings_status_idx ON listings (status);
