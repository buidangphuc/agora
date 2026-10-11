## Context

`team-analytics` gained a principal interceptor in `ops-cockpit-real-data`
(`internal/interceptor/auth.go`), which attaches the gateway-forwarded principal without
rejecting. `RequireScopes` exists. The frontend's `/seller/analytics` calls the RPCs with
`me.id`. `team-identity/cmd/server/main.go` calls `EnsureAdmin(ctx, "admin", "admin123")`.

## Decisions

1. **Enforce at the data owner** (team-analytics), not the gateway: rule 2 keeps the
   gateway free of business rules, and a check in the service also protects against
   in-network callers. A small helper `requireSellerAccess(ctx, sellerID)`: no principal →
   `Unauthenticated`; `admin` scope → allow; principal type user and id == seller_id →
   allow; else `PermissionDenied`.
2. **Service principals** have no ownership; none call these RPCs today (checked by grep).
   If one needs to later, it should get the `admin` scope or a dedicated scope, not a bypass.
3. **Admin seeding opt-in** with env-only secrets: `SEED_ADMIN_ENABLED` (bool, default
   false), `SEED_ADMIN_USERNAME` (default `admin`), `SEED_ADMIN_PASSWORD` (required when
   enabled, ≥ 12 chars). Validation happens in config loading so the process fails fast.
   The local compose sets a dev password (e.g. `admin-dev-password-123`) in
   `docker-compose.services.yaml`; e2e `users.json` reads it from env with that default.
4. **Gitops guard**: a check in `platform-gitops/scripts` (or the existing manifest check)
   fails if any team-identity manifest sets `SEED_ADMIN_ENABLED` to true or contains
   `SEED_ADMIN_PASSWORD` as a literal.

## Risks / Trade-offs

- An existing database that already contains `admin` / `admin123` keeps that user; the
  change stops creating it but does not delete it. Documented in the README with the
  command to rotate or remove it.

## Open Questions

None blocking.
