-- Flyway migration V1 — initial schema for listing_db.
--
-- Flyway is THE migration tool for this service. Migrations are versioned,
-- immutable once applied, and named V<n>__<description>.sql. Applied via
-- `make migrate` (see Makefile) / on startup once the Postgres + Flyway wiring
-- in build.gradle.kts is enabled. DB-per-service: only team-service touches
-- listing_db.
--
-- This is a PLACEHOLDER shape matching domain.model.Listing. Adjust before you
-- run it for real (indexes, constraints, audit columns, etc.).

CREATE TABLE IF NOT EXISTS listings (
    id          TEXT PRIMARY KEY,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    -- Money as integer minor units (e.g. VND) — never float. See Listing.kt.
    price_minor BIGINT      NOT NULL,
    currency    TEXT        NOT NULL,  -- ISO 4217, e.g. 'VND'
    -- Mirrors ListingStatus: DRAFT | PUBLISHED | REJECTED (UNSPECIFIED unused at rest).
    status      TEXT        NOT NULL DEFAULT 'DRAFT',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_listings_status ON listings (status);
