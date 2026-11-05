## Context

- `team-identity/internal/authz/scopes.go` `roleScopes[RoleAdmin]` ends with `"admin", "order.admin"`.
- team-promotion `internal/rpc/common.go`: `requireAdmin` is `auth.RequireScopes(ctx, "admin")`; `requireSeller`
  accepts `admin` or `listing.write`. The admin branches of `CreateVoucher` (shop audit), `CreateCampaign` and
  `CreateAdCampaign` test `principal.HasScope("admin")`.
- team-gateway `internal/edge/policy.go` `adminProcedures` maps the five promotion admin procedures to `adminScope`.
- Precedent: authz-residuals-2 (`order.admin`): `ForceFailSaga` needs `admin` AND `order.admin`; the admin override on
  mixed order RPCs is `order.admin` alone.

## Decisions

**D1. `promotion.admin` is a role scope for the admin role only.** Added to `roleScopes[RoleAdmin]`; a unit test pins
that buyer and seller roles never get it and it is not a service-only scope.

**D2. Admin-only RPCs need both scopes.** `requireAdmin` becomes `auth.RequireScopes(ctx, "admin", "promotion.admin")`
and serves `DeactivateVoucher`, `UpdateVoucher`, `ListCampaigns`, `CancelCampaign`, `UpdateCampaign` and the PLATFORM /
UNSPECIFIED branch of `CreateVoucher`. Mirrors `ForceFailSaga`.

**D3. Mixed RPCs key the admin branch on `promotion.admin` alone.** A helper `isPromotionAdmin(p)` returns
`p.HasScope("promotion.admin")`; it drives the shop-voucher audit flag, the flash-sale admin path (no ownership check,
idempotency key) and the ad-campaign `IsAdmin` flag. `requireSeller` accepts `promotion.admin` or `listing.write`, so a
caller holding `admin` without either is denied. Mirrors `isAdminPath` in team-order.

**D4. Gateway maps the five procedures to `promotion.admin`.** The edge gate is coarse; team-promotion stays
authoritative and also requires `admin`. `CreateVoucher`, `CreateCampaign` and `CreateAdCampaign` stay ungated at
the edge (mixed RPCs).

**D5. No proto change.** RPC comments that say "scope `admin`" remain true (admin is still required); the audit proto
comment is left for a separate contract change.
