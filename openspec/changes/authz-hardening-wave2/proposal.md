## Why

`service-authz-hardening` (wave 1) closed the voucher, audit, dispute, KYC and share-link RPCs. A second code-reading audit of
the services it did not touch, plus the newest branches (`chore/proto-sync` on `team-payment`, the wallet-ledger and
notification-prefs RPCs), found that **money movement, the admin account, and several per-user data paths are still reachable
by anonymous or unrelated signed-in callers** through the public gateway (its auth interceptor never rejects: an
unauthenticated caller reaches services as `x-principal-id: anonymous`, type `anonymous`, `PUBLIC_SCOPES`). Every finding below
was re-read in code at the tip of the currently checked-out branch of each repo; corrections to the audit are in `design.md`
("Corrections to the audit"). Nothing was run against a live stack. Item numbers (#1..#14) are the audit's and are reused in
`design.md` and `tasks.md`. **PRE** = already on `main`, **NEW** = introduced by the recent branches.

1. **Anyone can settle any payment (CRITICAL, PRE).** `ProcessMockPayment` has no principal check
   (`team-payment/internal/handler/payment.go:80`, `service/payment.go:180`) and the gateway routes it
   (`team-gateway/internal/edge/payment.go:54`). An anonymous caller that knows a transaction id marks it PAID, emits
   `PaymentSettled` (the order becomes Paid) and credits the seller's wallet (`creditSellerWallet`, `service/payment.go:222`).
2. **Any signed-in user can read or drain another seller's wallet (CRITICAL, PRE + NEW).** `GetSellerWallet`, `RequestPayout`,
   `ListPayoutHistory` (`handler/payment.go:102/126/162`, PRE) and the ledger RPCs `GetWalletBalance`, `ListLedgerEntries`,
   `RequestWalletPayout` (`handler/wallet_ledger.go:22-101`, NEW) honour the request `seller_id` after `RequirePrincipal`.
   `RequestPayout` also takes a caller-chosen bank account (`payment.proto` `RequestPayoutRequest`), so a buyer can pay another
   seller's balance out to themselves; `RequestWalletPayout` has no bank fields and only debits the victim's ledger.
3. **Any user can refund any paid payment; anyone can read any transaction (HIGH, PRE).** `RefundPayment`
   (`handler/payment.go:191`) needs only a principal and accepts a payment id *or an order id*; `GetPayment`
   (`:62`) checks no principal at all, so an anonymous caller reads a transaction by order id. (`CreatePayment` is correct: it
   binds the buyer, `service/payment.go:137`.)
4. **Anyone can log in as admin in every environment (CRITICAL, PRE).** `team-identity` seeds
   `EnsureAdmin("admin","admin123")` unconditionally (`cmd/server/main.go:83`) and `AuthService/Login` is routed by the gateway.
   The admin token carries `admin` (gates dispute resolution, KYC review, audit reads, platform vouchers, all added by wave 1).
5. **A dev RSA signing key is committed and reused (HIGH).** The same private key with `JWT_KID=dev-2026` is in
   `docker-compose.services.yaml:197-198` and in `platform-gitops/platform/vault-config/vault-config.yaml:43-70` (seeds
   `svc/shared/jwt`). Anyone with repo access can mint tokens for any deployment still using it. `deploy/vault/bootstrap-vault.sh`
   (manual dev script) seeds the legacy `JWT_SECRET=dev-secret-change-me` and lets **every** service read
   `svc/data/shared/jwt` (lines 25, 35), contradicting ADR-0006; the GitOps `vault-config` restricts it to `team-identity`.
6. **Password reset hands the token to the requester (HIGH, PRE; not gateway-routed).** `RequestPasswordReset` returns the
   raw `ResetToken` in the response (`team-identity/internal/handler/auth.go:75-77`); any in-cluster peer can reset any
   account, including admin. `ChangePassword` honours a body `user_id` before the principal (`:55-59`; it still needs the old
   password, so this is a lesser issue). Neither RPC is routed at the gateway (only `Register` and `Login` are,
   `team-gateway/internal/edge/auth.go:25/41`).
7. **Every notification RPC is one shared demo user (HIGH, PRE + NEW).** All 8 RPCs use the constant `khach_hang_shopee`
   (`team-notification/internal/handler/notification.go:49/72/80`, `alerts.go:18`, `notification_prefs.go:18`); there is no
   principal interceptor (`internal/grpcserver/server.go:22`). Every caller, anonymous included, reads and marks the same inbox
   and shares one set of alert subscriptions and preferences.
8. **Referral treats `anonymous` as a real user (HIGH).** The interceptor accepts any non-empty `x-principal-id`
   (`team-referral/internal/interceptor/principal.go:58`); the gateway sends `anonymous`, so every logged-out caller is one
   shared referrer/redeemer.
9. **Seller analytics are world-readable (HIGH).** `team-analytics` has no interceptor (`internal/grpcserver/server.go:24`)
   and `GetSellerFunnel` / `GetRevenueBreakdown` (`internal/query/service.go:37/60`) trust the request `seller_id`.
10. **Draft and rejected listings are readable by anyone (MEDIUM).** `GetListing`/`ListListings` need only `listing.read`,
    which anonymous holds (`team-domain/internal/handler/listing.go:78-125`); `ListListings` applies the client `status`
    as a SQL filter and an **empty status returns every status** (`repository/listing_pg.go:136-142`).
11. **Promotion monetisation gaps (MEDIUM).** `CreateAdCampaign` needs only a principal and has no ownership or upper bound
    on `bid`/`budget` (`team-promotion/internal/handler/sponsored.go:36`, `service/sponsored.go:51`; ranking is best-bid first);
    `Subscribe` lets any signed-in user take any plan (`handler/subscription.go:51`, mock, no charge); `GetEntitlements`
    fails open: only `USER` principals are isolated (`subscription.go:86`), so an `ANONYMOUS`-typed or unknown-typed principal that passes
    `RequirePrincipal` reads any seller.
12. **Recommendations trust the request identity; a static bearer is an admin backdoor (MEDIUM).** `Recommend` uses the
    request `user_id`/`anonymous_id` (`team-ai/app/transport/grpc/servicers/recommend.py:55`). The gRPC interceptor falls back
    to `authorization: bearer <AUTH_BEARER_TOKEN>` and mints a `service` principal with `AUTH_ROLES` (default `"admin"`,
    `app/core/config/platform.py:12`; `interceptors/auth.py:81-105`, `modules/platform/identity/auth.py:28-40`). Non-local
    settings *require* the token to be set (`app/core/config/__init__.py:69`), so the fallback is armed exactly where it is
    dangerous, and `team-ai` has no NetworkPolicy in GitOps.
13. **The gateway leaks upstream errors, serves reflection, and accepts tokens without `exp` (MEDIUM).** `toConnectErr`
    (`team-gateway/internal/edge/forward.go:319-362`) copies the upstream message verbatim for every code (raw DB/driver
    errors such as `create order: %v`, `create payment: cannot read order: %v` reach clients). gRPC reflection is served
    unauthenticated (`internal/edge/server.go:106-131`). The verifier does not require `exp`
    (`internal/token/jwt.go:53-62`; identity always sets it, `team-identity/internal/token/jwt.go:69`).
14. **`team-order` presents over-broad upstream scopes (MEDIUM, partly NEW).** `defaultServiceScopes` includes
    `listing.write`, `identity.write` and `inventory.write` (`internal/upstream/domain.go:33`), which defeats the
    "one scope per marked call" rule of wave 1; the forwarded-user branch appends `listing.read,identity.read` (`:92-95`);
    `Clients.Listing` is exported as a raw client (`:17`).
15. **Share links (LOW, unchanged).** `CreateShareLink` stays anonymous (wave-1 decision D11, re-confirmed).

## What Changes

- **team-payment** (#1, #2, #3)
  - `ProcessMockPayment` requires an authenticated principal and `transaction.buyer_id == caller` (not-owner is `NOT_FOUND`), and
    only runs when `MOCK_PAYMENTS=true`; otherwise `FAILED_PRECONDITION`. A startup guard refuses `MOCK_PAYMENTS=true` when
    `ENV` is staging/stage/prod/production. The route stays at the gateway (the e2e suite cannot settle payments without it).
  - Wallet and payout RPCs: seller id **is the principal id**; a request `seller_id` is honoured only when it equals the
    principal or the caller holds `admin`, else `PERMISSION_DENIED`. Payout requests additionally need `listing.write`.
  - `RefundPayment`: only `admin` or the seller of the paid order (resolved through `team-order` as `service-team-payment`,
    `order.read`; fails closed). `GetPayment`: authenticated; only the transaction's buyer, the order's seller, or `admin`
    (anyone else, and unknown ids, are `NOT_FOUND`).
- **team-identity** (#4, #5, #6)
  - The admin seed reads `ADMIN_PASSWORD`; **unset means no admin is seeded**. In staging/stage/prod/production the service
    refuses to start if `ADMIN_PASSWORD` is `admin123`, or the signing key is the committed dev key (fingerprint) or
    `JWT_KID=dev-2026`. Local compose sets `ADMIN_PASSWORD=admin123` explicitly so e2e logins keep working.
  - `RequestPasswordReset` and `ResetPassword` never return the token unless `DEV_RETURN_RESET_TOKEN=true` (refused in strict
    ENV); otherwise only a token fingerprint is logged. `ChangePassword` binds the principal id first.
  - Scope tables/comments updated (tests only): `admin`, `listing.write` now also enforced by payment/analytics/promotion.
- **Repo config / ops** (#5): `deploy/vault/bootstrap-vault.sh` is deleted (the GitOps `vault-config` is the ADR-0006 path);
  the dev key is labelled dev-only where committed; rotation in any shared environment is recorded as an **ops task**, not code.
- **team-notification** (#7): principal interceptor + `RequirePrincipal` on all 8 RPCs; the user id is `principal.id`.
- **team-referral** (#8): the interceptor maps id `anonymous` / type `anonymous` to the anonymous principal.
- **team-analytics** (#9): principal interceptor; both RPCs require `listing.write`; `seller_id` is bound to the principal
  unless `admin`.
- **team-domain** (#10): `GetListing` returns non-published listings only to the owner, `admin`, or a service principal
  (others: `NOT_FOUND`); `ListListings` defaults an empty `status` to `published` and accepts another status only for
  `admin`/service principals. No proto change.
- **team-promotion** (#11): `CreateAdCampaign` needs `listing.write` + listing ownership (reusing wave-1's owner lookup, admin
  bypass) and bounded `bid`/`budget`; `Subscribe` needs `listing.write`; `GetEntitlements` isolation inverts to "not SERVICE".
- **team-ai** (#12): `Recommend` binds `user_id` to the principal (ignoring the request value) unless admin/service; the
  gRPC static-bearer fallback is off by default (`GRPC_BEARER_FALLBACK_ENABLED=false`), refused at startup outside local, and
  `AUTH_ROLES` defaults to empty. GitOps adds a NetworkPolicy for `team-ai`.
- **team-gateway** (#13): one place sanitizes upstream `Internal/Unknown/DataLoss/Unavailable` errors (generic message,
  request id already on the response, original logged); client-meaningful codes keep their message. gRPC reflection is behind
  `EDGE_REFLECTION_ENABLED` (default false, refused in strict ENV). The verifier requires `exp` and a non-empty `sub`.
- **team-order** (#14): unmarked upstream default scope becomes `listing.read` only; the scope append is removed;
  `Clients.Listing` is unexported and reached through `Domain`.
- **platform-gitops**: `ENV` overlays for staging/prod on `team-identity`, `team-payment` and `team-gateway`, so the
  strict-ENV guards are actually armed there (today only `team-order` sets `ENV`; every other service defaults to `local`);
  NetworkPolicy for `team-ai`.
- **platform-core / AGENTS.md**: comment-only proto edits; ADR-0003/0006/0010 amendments; AGENTS.md §4 bullets.
- **platform-e2e + owning repos' `FEATURES.yaml`**: API-level scenarios for every rule; existing steps that relied on the holes
  are updated (caller audit in `design.md`).

**BREAKING (intended, security):** anonymous/unrelated callers lose access to the RPCs above; `ProcessMockPayment` is off
outside local/compose; wallet/payout `seller_id` other than the caller's is rejected; `RefundPayment` is no longer open to
buyers; passwords resets no longer echo the token; notification/referral/analytics need a signed-in caller; draft listings
vanish for strangers. Consumers affected: `platform-e2e` only (payout/wallet steps pass `seller.username`, refund is called
with a buyer token, admin/mock env). `team-frontend` needs **no change** (every call already passes the caller's own id and a
session token; caller audit in `design.md`). Rollout order (config before enforcement; gateway and ENV activation last):
`design.md` "Migration Plan".

## Capabilities

### New Capabilities

- `payment-access-control`: gated mock settlement, principal-bound wallet/payout, refund and transaction-read authority
  (`team-payment`).
- `notification-access-control`: per-user notification inbox, alerts and preferences (`team-notification`).
- `referral-access-control`: anonymous callers cannot act as a referrer/redeemer (`team-referral`).
- `analytics-access-control`: seller analytics readable only by the seller or admin (`team-analytics`).
- `listing-read-visibility`: who may read non-published listings and filter by status (`team-domain`).
- `sponsored-and-subscription-access`: ad-campaign ownership/bounds, seller-only subscription, entitlement isolation
  (`team-promotion`).
- `ai-access-control`: recommendation identity binding, no static-bearer backdoor in deployed environments (`team-ai`).
- `edge-surface-hardening`: upstream error sanitisation, reflection gate, token expiry requirement (`team-gateway`).
- `order-upstream-principals`: least-privilege principals `team-order` presents to `team-domain`/`team-identity`
  (`team-order`).

### Modified Capabilities

- `auth`: ADDED requirements only: environment-guarded admin seed and signing key, password-reset token never returned by
  default, a documented ADR-0006-aligned secrets path (existing capability in `openspec/specs/auth`).
- `deploy-runtime`: ADDED requirements only: deployed environments declare their strict `ENV`, and `team-ai` is
  network-restricted (existing capability in `openspec/specs/deploy-runtime`).

## Impact

- Repos (code): `team-payment`, `team-identity`, `team-notification`, `team-referral`, `team-analytics`, `team-domain`,
  `team-promotion`, `team-ai`, `team-gateway`, `team-order`. Config/docs: `docker-compose.services.yaml`, `deploy/`,
  `platform-gitops`, `platform-core` (ADRs, proto comments), `AGENTS.md`. Tests: `platform-e2e`, owning repos'
  `FEATURES.yaml`. **Not touched:** `team-frontend`, `team-search`, `team-chat`, `team-engagement`, `team-verification`,
  `team-audit`, `team-sharing`.
- No database migration. No proto message/RPC shape change (`buf breaking` stays clean). **No new user scope** and no new
  service-only scope: existing `listing.write`, `admin`, `order.read` are reused, so there is no JWT-TTL token-lag window.
- New env vars (each with `.env.example` and README entries where the repo has an env-drift gate): `ADMIN_PASSWORD`,
  `DEV_RETURN_RESET_TOKEN` (identity); `MOCK_PAYMENTS` (payment); `EDGE_REFLECTION_ENABLED` (gateway);
  `GRPC_BEARER_FALLBACK_ENABLED` (team-ai); `MAX_AD_BID`, `MAX_AD_BUDGET` (promotion).
- Operational: the committed dev signing key must be **rotated in any shared environment**; staging/prod must supply a real
  key and `ADMIN_PASSWORD` (or no admin) in Vault before the `ENV` overlays are activated (ops task, `design.md` D16).

## Non-goals

- mTLS / service-identity zero-trust (ADR-0010 follow-up): service principals remain unauthenticated metadata behind the
  NetworkPolicy; this change narrows what a forged *or* legitimate principal can do.
- Frontend edits (none forced), a real payment provider, a mail/SMS delivery channel for password resets, a per-RPC scope
  table at the gateway, token refresh/revocation, `iss`/`aud` claims, rate-limit changes.
- Rewriting every raw-error site in every service: the gateway sanitizer is the single control; per-service raw `%v` sites are
  fixed only in files touched by this change (inventory in `design.md`).
- `CreateShareLink` stays anonymous (D11 of wave 1); analytics Kubernetes Service/NetworkPolicy (the GitOps manifest has no
  Service for `team-analytics`; recorded as an observation).
- Wallet/refund business rules (amounts, fees, ledger semantics) and a service-to-service refund trigger from `team-order`.
