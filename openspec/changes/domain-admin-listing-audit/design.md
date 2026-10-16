# Design

## Clarifications

### Round 1 — 2026-10-15

- Q1 (permission): the admin path is EVERY `UpdateListing` / `DeleteListing` call where the caller holds the `admin` scope, including an admin who owns the listing. The human chose a uniform rule over the catalogue wording "who is not the owner". Calls by non-admin sellers on their own listing stay unaudited and independent of team-audit. Consequence: the service hook condition is `isAdmin` alone, evaluated after the load (NotFound) and the existing owner/admin check.

## Hook placement

In `ListingService.UpdateWithEvent` / `DeleteWithEvent`: Get (NotFound) -> owner/admin check (ErrForbidden) -> if `isAdmin`, audit write (error, timeout or nil writer -> `ErrAuditUnavailable`) -> repo/tx change. The actor id is the verified principal id from `principalOwner(ctx)`; `seller_id` metadata is the stored owner.

## Update `status` metadata

The lowercase stored-form status string the update applies (`draft`, `published`, `rejected`, as team-domain stores it, see `statusPublished` in handler/listing.go); when the request carries `LISTING_STATUS_UNSPECIFIED` it is the listing's current stored status.

## E2E helpers

From `audit-query-filters` `support/audit_support.py`: `admin_token`, `assert_one_event`, `assert_no_event`, `stop_audit(world)`, `start_audit(world)`. The audit-down scenario is tagged `@destructive`.

## Test coverage note

The admin-owner rule (an admin who owns the listing is audited and refused while audit is down) is covered at service level only; this is intentional, since the handler tests already cover the admin path and the e2e uses a seeded seller distinct from the admin.
