## Why

Two security issues surfaced in the cockpit reviews (both predate the UI work):

1. **IDOR on seller analytics.** `team-analytics` `GetSellerFunnel`,
   `GetRevenueBreakdown` and `GetDemandForecast` are scoped only by the `seller_id` in the
   request, and the gateway forwards it unchanged, so any caller can read any seller's
   funnel, revenue and forecast.
2. **Known admin password.** `team-identity` seeds `admin` / `admin123` unconditionally at
   startup. Deployed as-is, that is a public admin credential, and the admin scope now
   unlocks platform GMV and recent orders.

## What Changes

- team-analytics: the three seller RPCs require a principal that owns the `seller_id` or
  has the `admin` scope (`Unauthenticated` / `PermissionDenied` otherwise).
- team-identity: admin seeding is opt-in (`SEED_ADMIN_ENABLED`), with the password from
  `SEED_ADMIN_PASSWORD` (no default, minimum 12 characters, fail-fast).
- Local compose enables seeding with a dev password from env; e2e test data reads it.
- platform-gitops: team-identity manifests do not enable seeding (asserted by a check).

Repos: team-analytics, team-identity, docker-compose, platform-gitops, platform-e2e.

## Capabilities

### New Capabilities
- `seller-analytics-access`: ownership rule for seller analytics.
- `admin-bootstrap`: how the first admin is created.

## Non-goals

- No change to the gateway (it stays a forwarder; the data owner enforces).
- No admin-management UI or password rotation flow.
- No new scopes; uses the existing `admin` scope and the principal id.

## Impact

- Any caller that relied on reading another seller's analytics now gets 403 (none known;
  the frontend passes the signed-in seller's own id).
- Environments that relied on the implicit `admin` / `admin123` must set the seed env.
