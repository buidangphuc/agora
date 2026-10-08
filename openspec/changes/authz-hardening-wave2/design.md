## Context

See `proposal.md` for motivation and `specs/` for requirements. Item numbers (#1..#14) are the audit/proposal numbers.
Everything below was re-read in code at the tip of the currently checked-out branch of each repo: `team-payment`
`chore/proto-sync`, `team-identity` and `team-analytics` `feat/openspec-wave0-wave1`, `team-gateway`
`feat/forward-idempotency-key`, `team-order` `feat/order-integrity-guards`, `team-promotion` `feat/service-authz-hardening`,
`team-domain` `feat/release-requires-reservation-id`, `team-notification` `feat/12-features`, `team-referral`
`fix/repo-doctor-team-referral`, `team-ai` `feat/llm-path-resilience`, `platform-gitops` `fix/repo-doctor-platform-gitops`,
`platform-core` `feat/commit-reservation`, `platform-e2e` `fix/repo-doctor-e2e`, `team-frontend`
`feat/checkout-idempotency-key` (read only). Claims that turned out false or incomplete are under "Corrections to the audit".
Nothing was executed against a live stack.

Verified current state (paths relative to the repo named):

- **Gateway principal.** Auth never rejects: no or invalid token resolves to id `anonymous`, type `anonymous`, `PUBLIC_SCOPES`
  (`team-gateway/internal/edge/forward.go:130-143`, `interceptors.go:120-127`; default `PUBLIC_SCOPES=listing.read,search:read`,
  `config/config.go:80`). `outgoing` rebuilds `x-principal-*` from that principal only (`forward.go:151-166`), so a client
  cannot forge scopes. The edge interceptor already stamps `X-Request-Id` on every error (`interceptors.go:178-180`).
- **Receiver-side helpers to reuse.** `team-promotion/internal/interceptor/auth.go`: `RequirePrincipal` (`:93`, rejects id
  `anonymous` and type anonymous), `RequireScopes` (`:110`), `IsService` (`:129`), `RequireService` (`:136`), `RequireAdmin`
  (`:148`); `team-chat/internal/interceptor/auth.go` (`RequirePrincipal`, same anonymous rejection) is the pattern named for
  `team-notification`; `team-payment/internal/interceptor/auth.go` already has `RequirePrincipal` only.
- **Owner lookup to reuse.** `team-promotion/internal/handler/flashsale.go` `authorizeCampaign` (`listing.write`, admin
  bypass with no lookup, `team-domain` `GetListing` as `service-team-promotion`, not-found/not-owner `PERMISSION_DENIED`,
  transport failure `UNAVAILABLE`) plus `internal/upstream/listing.go`. `team-engagement/internal/upstream/listing.go` is the
  same client.
- **ENV-guard pattern to reuse.** `team-order/internal/config/config.go:169-204` (`durableStorageEnvs`,
  `RequiresDurableStorage`, `RequireDurableStorage`), called at boot from `cmd/server/storage.go:15`. Every Go service has
  `Runtime.Env` (`ENV`, default `local`) in its config.
- **Deployed ENV is mostly unset.** In `platform-gitops` only `envs/{staging,prod}/services/team-order.yaml` sets `ENV`
  (`staging` / `production`); every other service defaults to `local`, so an ENV-strict guard in `team-identity`,
  `team-payment` or `team-gateway` is **inert in staging/prod until an overlay sets `ENV`** (D16). Local/compose sets
  `ENV=local` per service in `docker-compose.services.yaml`.
- **NetworkPolicies (ADR-0010).** `envs/services/*.yaml`: `team-payment` allows only `team-gateway`; `team-identity` allows
  `team-gateway`, `team-order`; `team-order` allows gateway, payment, engagement; `team-domain` allows gateway, order,
  promotion, engagement. `team-ai` has **no** `networkPolicy` (`envs/services/team-ai.yaml`); Prometheus (label `app:
  prometheus`) scrapes `team-ai:8000` (`platform/monitoring/prometheus.yaml:14-17`). `team-analytics` is a worker Deployment
  with no Service or NetworkPolicy (`platform/team-analytics/consumer.yaml:4`), not in the ApplicationSet.
- **Existing frontend posture.** Every seller page that calls a changed RPC redirects unless `hasScope("listing.write")` and
  passes `me.id` (wallet, analytics, plans, ads use `ListMyListings`).

### Caller audit (every RPC this change touches)

Searched: `team-frontend/src` (non-generated), `platform-e2e/{src,tests,test-data}`, `platform-core/tools`, `scripts/`,
`deploy/`, `platform-gitops/`, `docker-compose*.yaml`, and every other `team-*` service.

| RPC | Legitimate callers found (evidence) | Backward compatible for a normal logged-in user? | Decision |
|---|---|---|---|
| `Payment/ProcessMockPayment` | frontend `features/payment/MockPaymentView.tsx:31` -> `features/order/actions.ts:145` -> `lib/gateway/payment.ts:111` (buyer session); e2e `payment_service.py:14-38`, `flows/market_flow.py:170-189`, `order_guards_steps.py:163`, `order_lifecycle_extra_steps.py:184`, `group_a_steps.py:32/58/105/146`, `group_c_steps.py:111`, `flows/order_flow.py:57`, `promotion_access_steps.py:223`, `cancel_voucher_steps.py:61`, `order_read_access_steps.py:80/86` (all intended to be the paying buyer) | Yes (buyer pays own order) | Keep routed; owner + `MOCK_PAYMENTS` (D4) |
| `Payment/GetPayment` | frontend `app/checkout/pay/[id]/page.tsx:21` (`getPayment(undefined, orderId)`, buyer); e2e none | Yes | Parties/admin only (D5) |
| `Payment/RefundPayment` | **no frontend RPC call** (`lib/gateway/payment.ts:145-157` is a local no-op mock); e2e `group_a_steps.py:130` (after seller approves a return: seller token) and `:160` (**buyer token**); `payment_service.py:40-49` | n/a | admin or order seller (D5); e2e `:160` must use seller/admin |
| `Payment/GetWalletBalance`, `ListLedgerEntries`, `RequestWalletPayout` | frontend `app/seller/wallet/page.tsx:18-19`, `features/seller/WalletPayoutButton.tsx` -> `actions.ts:13-18` -> `payment.ts:192-224` (all pass `me.id`) | Yes | Bind to principal (D3) |
| `Payment/GetSellerWallet`, `RequestPayout`, `ListPayoutHistory` | **no frontend caller**; e2e `group_c_steps.py:137-165` via `payment_service.py:51-82` passing `seller.username` (not the user id) | n/a | Bind to principal; e2e must drop `sellerId` |
| `Identity/Login` (admin) | e2e `flows/account_flow.py:13-35`, `cockpit_metrics_steps.py:38-43`, `group_b_steps.py:90-91`, `authz_common_steps.py:41-63`, `scope_grants_steps.py:30-57`, `dispute_access_steps.py`, `test-data/{docker,local}/users.json:25`; no frontend login for admin | Yes if compose sets `ADMIN_PASSWORD=admin123` | D6, D7 |
| `Identity/RequestPasswordReset`, `ResetPassword`, `ChangePassword` | not routed at the gateway (`edge/auth.go` routes only Register/Login); e2e `auth_steps.py:49` (swallows any error, asserts nothing); frontend none | n/a | D8 |
| `Notification/*` (8 RPCs) | frontend `lib/gateway/notification.ts`, `app/notifications/page.tsx`, `app/listing/[id]/page.tsx:64` (only when logged in), `components/alerts/AlertToggle.tsx`, `features/notification/actions.ts` (each wrapped in try/catch returning empty lists); e2e `outbox_ordering_steps.py:37-58` (buyer token), `notification_alert_steps.py`, `test_unread_badge.py`, `notifications_page.py` | Yes (session token always sent); a logged-out visitor sees empty lists instead of the shared demo inbox | D10 |
| `Referral/*` | frontend `lib/gateway/referral.ts`, `app/account/referral/page.tsx`, link in `app/layout.tsx:54`; e2e `test_new_services.py:61-71` (buyer token) | Yes | D10 |
| `Analytics/GetSellerFunnel`, `GetRevenueBreakdown` | frontend `app/seller/analytics/page.tsx:18-19` (`me.id`, `listing.write`); e2e none | Yes | D3 |
| `Listing/GetListing` | frontend `app/listing/[id]/page.tsx:42`, `app/seller/[id]/edit/page.tsx:16` (owner), `favorites`, `RecentlyViewedRow`, `lib/gateway/search.ts:111/177`, `recommendations.ts:74` (already drop non-published hits), `notifications/page.tsx:28`; services: `team-order/internal/service/cart.go:56` (buyer), `team-promotion`/`team-engagement` `upstream/listing.go` (service, `listing.read`); e2e `edge_hardening_steps.py:416` (published), `search_visibility_steps.py:248` (seller token), `readiness.py:22` (unknown id) | Yes (published and own drafts unchanged; a buyer following a link to a now-draft listing gets not-found, which `getListing` maps to `null`) | D11 |
| `Listing/ListListings` | frontend `app/page.tsx:78`, `app/shop/[id]/page.tsx:19` (status `published`), `app/api/listings/route.ts:11` (status from the query, **default empty = all statuses today**); e2e none | Yes (published only by default) | D11 |
| `Promotion/CreateAdCampaign`, `Subscribe`, `GetEntitlements` | frontend `features/seller/AdCampaignForm.tsx` (listings from `ListMyListings`), `SubscribeButton` -> `actions.ts:33`, `app/seller/plans/page.tsx:16` (`me.id`); e2e none | Yes for sellers (pages already require `listing.write`) | D12 |
| `Recommendation/Recommend` | frontend `lib/gateway/recommendations.ts:58` (`userId = own id`, `anonymousId ""`); e2e `recsys_serving_contract_steps.py:58-64`, `recommendations_steps.py:66`, `scope_grants_steps.py:67` (own subject or empty) | Yes | D13 |
| gateway error text, reflection, `exp` | frontend toasts show `err.message`; e2e asserts messages only for preserved codes (`ai_service_hardening_steps.py:156/201/219`, `search_visibility_steps.py:270`, `market_flow.py:134`, `edge_hardening_steps.py:458`); no reflection or no-`exp` token user | Yes; Internal/Unavailable toasts read `internal error` / `service unavailable` | D14 |

Consequence: **no frontend change is required.** UI-visible differences: logged-out visitors no longer see the shared demo
inbox/preferences; strangers get not-found on draft listing links; Internal/Unavailable errors show generic text.

### Corrections to the audit

- **#2 "drain to their own bank account" applies to `RequestPayout` only.** `RequestWalletPayout` has no bank fields
  (`payment.proto` `RequestWalletPayoutRequest` = `seller_id`, `amount`); it debits the victim's ledger without paying the
  attacker. Both are still integrity holes; the read holes are real for all six RPCs.
- **#3 is right, and the frontend never calls `RefundPayment`:** `refundPayment` in `lib/gateway/payment.ts:145-157` is a
  client-side no-op, so the only caller is e2e (`group_a_steps.py:130/160`, the second with a **buyer** token).
- **#5 "every service can read `shared/jwt`" is true only for services passed to `bootstrap-vault.sh`** (default list: gateway
  and identity; lines 12, 35). The script is manual dev tooling; GitOps `vault-config` is the real path and is correct
  (identity only). The dev key is committed in three places (compose `:197-198`, `vault-config.yaml:43-70`, and its own docs).
- **#6 exploitability is narrower than "any in-cluster peer":** with ADR-0010 NetworkPolicies only `team-gateway` and
  `team-order` pods can reach `team-identity` in deployed envs, and neither calls `RequestPasswordReset`. In local compose any
  container can. Still CRITICAL-adjacent because the target is the admin account; fixed by not returning the token.
- **#7 confirmed** (8 RPCs). Existing rows keyed by the literal `khach_hang_shopee` become unreachable; they are demo data.
- **#10 is a filter, not SQL injection.** `status` is a bound parameter (`listing_pg.go:136-141`, `($3 = '' OR status = $3)`).
  There is no `deleted` status (`DeleteListing` hard-deletes, `listing_pg.go:295-303`); the statuses are
  `draft|published|rejected` and the DB default is **`draft`**. The real defect: `GetListing` ignores status, and an **empty
  `status` returns every status** (and the frontend `/api/listings` route sends empty by default).
- **#11 `GetEntitlements` "fail open" is not reachable through the gateway:** the gateway only emits types user, service and
  anonymous, and `RequirePrincipal` already rejects anonymous. It is reachable only by an in-cluster caller that omits
  `x-principal-type`. Kept as defence in depth, not as an internet-facing finding. `Subscribe`/`CreateAdCampaign` are
  internet-reachable as stated.
- **#12 the proposed remedy contradicts existing validation.** `Settings.validate_runtime_safety` *requires*
  `AUTH_BEARER_TOKEN` outside local (`app/core/config/__init__.py:69`) and the REST API (`app/api/v1/completions/router.py:11-23`)
  authenticates with that same static bearer; refusing to boot "when `AUTH_BEARER_TOKEN` is set" would break production. The
  remedy is to disable only the **gRPC** fallback (D13). Also: GitOps leaves `ENVIRONMENT` unset (default `dev`, which counts as
  local) and Vault bags carry no `AUTH_BEARER_TOKEN`, so today the fallback answers `auth_not_configured` there: the backdoor
  is **latent** (armed the moment the documented token is set), not live. `AUTH_ROLES` is read only by the gRPC fallback
  (`identity/auth.py:35`), so emptying its default changes nothing for REST. And anonymous callers never reach the `Recommend` body:
  `PUBLIC_SCOPES` lacks `recommendations:read`; the identity-spoofing risk is an authenticated user passing another user's id.
- **#13 location:** `toConnectErr` is at `forward.go:319-362` (not ~168). It is also used by the chat stream path
  (`chat.go:39/47`). Reflection is `server.go:106-131`. Identity always stamps `exp` and `sub` (`token/jwt.go:62-70`); `iss`
  and `aud` are never set, so requiring them would reject every token (out of scope).
- **#14:** the `defaultServiceScopes` branch (`domain.go:88-91`) runs only for an unmarked call with **no** incoming principal,
  and no such call exists today (`GetListing` is only at `service/cart.go:56`, `ListAddresses` only at
  `handler/order.go:78`, both inside buyer requests). The live branch is the append (`:92-95`). `identity.read` /
  `identity.write` are not a scope anywhere in the vocabulary (`team-identity/internal/authz/scopes.go`; identity gates on
  principal presence only) and `listing.read` is held by every user, so the append adds nothing real; `listing.write`,
  `identity.write` and `inventory.write` in the default are latent footguns, not active escalations.
- **Confirmed as audited, no correction:** #1 (`handler/payment.go:80`), #4 (`cmd/server/main.go:83`), #8
  (`interceptor/principal.go:58`), #9 (`grpcserver/server.go:24`, whose own comment argues no interceptor is needed).
- **Not re-opened:** share links (LOW) stay anonymous (wave-1 D11).

## Goals / Non-Goals

**Goals:**

- Every RPC named in the proposal enforces an identity/ownership rule **in the owning service** (authoritative), reusing the
  existing interceptor helpers and the `listing.write` / `admin` / `order.read` scopes; no new scope, no contract change.
- Every dev convenience that is a hole becomes **fail-closed by default and opt-in for local** (`ADMIN_PASSWORD`,
  `MOCK_PAYMENTS`, `DEV_RETURN_RESET_TOKEN`, `EDGE_REFLECTION_ENABLED`, `GRPC_BEARER_FALLBACK_ENABLED`), with an
  ENV-strict startup refusal as a second layer, and the deployed ENV actually declared in GitOps.
- Every legitimate caller in the audit table keeps working, including the frontend; e2e steps that relied on a hole are
  listed so the e2e track can change them.

**Non-Goals (design-level):**

- No shared auth library across repos (each service keeps its small interceptor copy, as today).
- No authentication of service principals (mTLS stays ADR-0010 follow-up), no proto shape change, no migration, no new scope.
- No attempt to fix existing shared-environment data (an admin row already seeded with `admin123`) in code (D7 ops task).

## Decisions

### D1 - Scope vocabulary: reuse `listing.write`, `admin`, `order.read`; nothing new

Sellers and admins are the only roles holding `listing.write`; only admin holds `admin`; `order.read` is service-only and
already held by `service-team-payment`. Wallet/payout/analytics/ads/subscription are seller features, so `listing.write` is the
natural gate (the frontend pages already require it). `admin` overrides ownership binding. **No user token has to change,
so there is no JWT-TTL (3600 s) lag window.** Drift test: `team-identity/internal/authz/coverage_test.go` only gets its
`enforcedScopes` descriptions updated (`listing.write` now also enforced by payment, analytics, promotion; `admin` by payment,
analytics, domain, promotion) and `scopes.go` comments; `serviceOnlyScopes` is unchanged (still the four wave-1 scopes).

*Alternatives:* new scopes `wallet.read`/`payout.write`/`analytics.read` (finer, but each needs a role-table entry, a drift-test
entry and a token-lag window, for no protection beyond `listing.write`); a service-only `payment.refund` scope (see D5).

### D2 - Receiver-side helpers: one small copy per service, same shape as `team-promotion`

`team-payment`, `team-analytics` and `team-notification` get `RequirePrincipal` (rejecting id `anonymous` and type anonymous),
`RequireScopes`, `IsAdmin`; `team-analytics` and `team-notification` also get the principal-extraction interceptor
(`team-chat`'s, registered via `grpc.ChainUnaryInterceptor`); `team-domain` already has `RequireScopes` and `principalOwner`
(`handler/listing.go:490`) and gains `isService`; `team-promotion` already has all helpers. The check is the first statement of
a guarded handler, before any read.

*Alternative:* a per-method policy table in a server interceptor - rejected again (handlers already do their own checks; one style).

### D3 - Seller-bound RPCs (payment wallet/payout, analytics): principal id is the seller id

One helper per service, `effectiveSellerID(ctx, requested) (string, error)`: require principal; `requested == ""` or
`== principal.id` -> principal id; else `IsAdmin` -> requested; else `PERMISSION_DENIED`. `team-payment`'s existing
`resolveSellerID` (`wallet_ledger.go:22`) and the three inline copies in `payment.go` collapse into it. Payout RPCs and analytics
additionally `RequireScopes("listing.write")` first; wallet/ledger/payout **reads** need only the principal (a buyer's own
wallet is empty and harmless; the seller pages already redirect non-sellers). `PERMISSION_DENIED` (not `NOT_FOUND`) because the
caller supplied the id and there is nothing to enumerate for an id-keyed wallet.

*Alternatives:* ignore a mismatching `seller_id` silently and use the principal (hides client bugs, changes which data a
caller thinks they queried); `listing.write` on the reads too (extra rejection, no extra protection).

### D4 - Mock settlement: principal + buyer ownership + `MOCK_PAYMENTS`, route kept (decision B)

`ProcessMockPayment` order of checks: principal (`UNAUTHENTICATED`) -> `MOCK_PAYMENTS` off (`FAILED_PRECONDITION`, before any
read) -> load transaction -> `tx.BuyerID != principal.id` or not found -> `NOT_FOUND`. `MOCK_PAYMENTS` lives in a new
`config.Settings` group, **default `false`** (fail-closed even when `ENV` is unset, which is how GitOps runs today); the
compose `team-payment` service sets it `true` explicitly. A `RequireNoMockPayments()` boot check, modelled on
`RequireDurableStorage` (strict `ENV` list `staging, stage, prod, production`), refuses to start with the flag true there.
The gateway route stays: the e2e suite settles payments through it (every payment-dependent scenario; `market_flow.py:181`),
so unrouting would break the stack's only settlement path (the decision's "only if e2e can still settle" condition is not met).

*Alternatives:* admin-only settle (breaks the buyer UI and e2e); a signed settlement token created by `CreatePayment` (good for
a real provider, out of scope); default `true` with prod guard only (depends on GitOps declaring `ENV`, which it does not).

### D5 - Refund and read authority: admin or order seller, parties for reads

`GetPayment` and `RefundPayment` need the order's seller. `team-payment` already reads orders as `service-team-payment` with
`order.read` (`upstream/order.go:47-53`, used at `service/payment.go:119`). Add `orderParties(ctx, orderID)` (buyer from the
transaction, seller from `GetOrder`). `GetPayment`: principal; buyer -> ok without lookup; admin -> ok; else lookup and
compare seller; everything else (and unknown ids) `NOT_FOUND`. `RefundPayment`: principal; admin -> ok (no lookup); else
lookup; seller only; not found / not seller `NOT_FOUND`; lookup failure `UNAVAILABLE` (fail closed: an authorization decision).
The payment id vs order id fallback (`service/payment.go:293-296`) is kept.

*Alternatives:* (a) **admin-only refund** - safest but the e2e return-approval flow (seller approves, then refund) and the
marketplace model use the seller; (b) **service-only `payment.refund` scope + `team-order` triggers the refund on return
approval** - the correct end state (no human in the loop), but no producer exists today and the frontend refund is a
no-op, so it would add a sender, a scope, a drift-test entry and a saga step; recorded as Q1/follow-up.

### D6 - Identity: strict-ENV guard on admin password and signing key (decision A)

A `Settings.RequiresHardenedSecrets()` (same env list as `team-order`) plus two boot checks in `run()` before any listener:
`ADMIN_PASSWORD == "admin123"` refuses; `JWT_KID == "dev-2026"` or `fingerprint(signer.PublicKey()) == devKeyFingerprint`
refuses (the constant is the SHA-256 of the DER-encoded public key of the committed dev key, so no extra copy of the private key
is committed; the unit test builds a signer from a labelled testdata PEM, which is already public in this repo).
Non-strict envs keep working with the dev key (compose/kind). The guard is **only as good as the `ENV` the deployment
declares**, hence D16.

*Alternative:* refuse by `kid` only (a rotated key reusing the kid name would be blocked unnecessarily, and a renamed dev kid
would pass); fingerprint + kid covers both.

### D7 - `ADMIN_PASSWORD`: unset means no admin, in every environment

`EnsureAdmin(ctx, "admin", password)` is called only when `ADMIN_PASSWORD != ""`. Local compose sets
`ADMIN_PASSWORD=admin123` explicitly (and the local kind `argocd/apps/team-identity.yaml` inline env, so cluster e2e keeps
working). This goes one step beyond "unset in prod: do not seed" on purpose: GitOps does not set `ENV`, so a rule keyed on
ENV alone would keep seeding `admin123` in a deployed environment. **Ops task (not code):** `EnsureAdmin` is create-if-absent,
so a shared database that already contains the `admin` row seeded with `admin123` keeps it; reset that password (or delete the
row and restart with a real `ADMIN_PASSWORD`) in every shared environment before relying on the guard.

*Alternative:* `EnsureAdmin` re-syncs the hash from `ADMIN_PASSWORD` on every boot - rejected (silently overwrites a password
an operator changed by other means; a rotation flow belongs in a separate change).

### D8 - Password reset never echoes the token (decision C)

`RequestPasswordReset` returns `reset_token = ""` unless `DEV_RETURN_RESET_TOKEN=true` (default false; strict ENV + true ->
refuse to start). It logs `password_reset.issued` with `user_id`, `expires_at` and the first 12 hex characters of the token's
SHA-256 (the stored hash already is `hashToken`, `service/auth.go:186`), never the raw token: a raw token in logs is the same
leak by another route. The service method keeps returning the raw token to the handler (the handler decides), so the existing
service tests stand. `ChangePassword` resolves the principal first and ignores a body `user_id` for a non-admin principal.
Consequence: with no mail channel, reset cannot complete in a deployed environment (it was unreachable through the gateway
anyway); a delivery channel is a follow-up (Q3).

### D9 - Secrets path: delete the legacy script, label the dev key, rotation is ops

Delete `deploy/vault/bootstrap-vault.sh` (HS256 `JWT_SECRET`, `shared/jwt` readable by every service it is run for) and fix the
two references (`deploy/README.md:35`, `platform-gitops/argocd/apps/vault.yaml:17`); the GitOps `vault-config` is the ADR-0006
path. Add a "DEV KEY - never use outside local; rotate in any shared environment" comment at the two committed copies (compose
`:197`, `vault-config.yaml:43`) without changing key material (local e2e depends on the matching JWKS). Record in ADR-0006 that
deployed environments supply their own key through Vault (`svc/shared/jwt`, identity-only) and that the code refuses the dev key
there (D6). **Rotating** the dev key in any shared cluster is an ops task tracked in tasks group 15, not a code change.

*Alternative:* rewrite the script to ADR-0006 - rejected (it duplicates `vault-config`; two provisioning paths is how this drifted).

### D10 - Notification, referral: principal-keyed, anonymous rejected

`team-notification`: add the `team-chat` interceptor package, register it in `grpcserver.New` (`server.go:22`), replace the five
constant sites with `RequirePrincipal` + `principal.GetId()` (handlers `notification.go`, `alerts.go`, `notification_prefs.go`;
`alertUserID`/`prefsUserID` become thin wrappers or are deleted). The Kafka listing consumer is untouched: it delivers to the
user ids stored on alert subscriptions, which are now real ids. `team-referral`: `interceptorWithPrincipal` also reads
`x-principal-type` and treats id `anonymous` or type `anonymous` as `Anonymous: true` (the handler's `callerID` already rejects
that); no handler change.

*Alternative for referral:* switch it to the `team-chat` interceptor - larger diff for the same effect.

### D11 - Listing visibility: owner, admin or service for non-published

`GetListing`: load, then if `status != "published"` allow only `principal.id == seller_id`, `admin`, or (type `SERVICE` and
`listing.read`); else `NOT_FOUND` (same body as unknown id: no existence oracle). `ListListings`: empty `status` -> `published`;
`status == "published"` -> as today; any other value -> admin or service principal, else `PERMISSION_DENIED`. The service
principals that need non-published reads today are `service-team-promotion` and `service-team-engagement` (ownership
lookups, already `listing.read`, type `service`); `team-order`'s cart read is a forwarded buyer and correctly stops at
published. The repository/SQL and the wire contract are unchanged (status arrives as a string filter). Seller-center drafts
stay on `ListMyListings` (`listing.write`, `seller_id = principal`).

*Alternatives:* coerce a forbidden status to `published` silently (answers a question the caller did not ask; hides client
bugs); filter `ListListings` by owner for sellers (an owner-scoped list is `ListMyListings`); add a proto `status` default
(contract change).

### D12 - Promotion: reuse wave-1 ownership lookup for ad campaigns; bound amounts; seller-only Subscribe

Extract `authorizeCampaign` (`flashsale.go`) into a shared handler helper `authorizeListingOwner(ctx, listingID)` used by
`CreateCampaign` and `CreateAdCampaign` (the `ListingOwnerLookup` is already injected into the flash-sale handler; inject it
into `SponsoredHandler` too). `CreateAdCampaign` sets `SellerID` from the principal as today, and `service.CreateAdCampaign`
rejects `bid > MAX_AD_BID` or `budget > MAX_AD_BUDGET` with `ErrInvalidAdCampaign` (-> `INVALID_ARGUMENT`); config defaults
`MAX_AD_BID=1_000_000` and `MAX_AD_BUDGET=1_000_000_000` (minor units; generous versus the UI, tighter than unbounded;
revisit with the product owner, Q4). `Subscribe`: `RequireScopes("listing.write")` (plans are seller plans; the page already
requires it; mock, no charge). `GetEntitlements`: `principal.GetType() != SERVICE && sellerID != principal.GetId()` ->
`PERMISSION_DENIED` (inverts the USER-only check at `subscription.go:86`).

*Alternatives:* admin-only ad campaigns (breaks the seller ads page); `bid <= budget` as an extra rule (a product rule the
form does not enforce; left out, Q4); leave `Subscribe` open because it is a mock (mock plans still gate real entitlements
such as listing limits).

### D13 - team-ai: bind `Recommend`; gRPC bearer fallback behind an explicit local flag

`recommend.py`: read the call principal (`current_principal()`); type `user` -> `user_id = principal.id`,
`anonymous_id` ignored; type `anonymous` -> `user_id = ""`, request `anonymous_id` kept (a device id cannot be bound to a
principal); `admin` scope or type `service` -> request values honoured. `interceptors/auth.py`: call
`authenticate_bearer_token` only if `settings.GRPC_BEARER_FALLBACK_ENABLED`, else abort `UNAUTHENTICATED` when no
`x-principal-id` metadata. `Settings.validate_runtime_safety` refuses `GRPC_BEARER_FALLBACK_ENABLED=true` when
`not ENVIRONMENT.is_local`. `AUTH_ROLES` default becomes `""` (`core/config/platform.py:12`); `.env.example` keeps
`AUTH_ROLES=admin` for local tooling, and unit tests that use the fallback set the flag. The flag defaults **false** even in
local so the safe behaviour does not depend on `ENVIRONMENT`, which GitOps leaves at its `dev` default. The REST API's static
bearer and the production token validation are unchanged. GitOps adds `networkPolicy: {enabled: true, allowFrom:
[team-gateway, prometheus]}` to `envs/services/team-ai.yaml` (staging/prod; the local kind app keeps its inline block,
ADR-0010 exempts local).

*Alternatives:* remove the fallback entirely (breaks local `grpcurl` tooling and the existing tests for no extra safety given
the flag); refuse to boot whenever a token is set (contradicts existing validation, see "Corrections").

### D14 - Gateway: one sanitiser, reflection flag, required `exp`

**Sanitiser in `toConnectErr`** (the single place every forwarder and the chat stream already pass through): for upstream codes
`Internal`, `Unknown`, `DataLoss` (and the `!ok` non-status branch) return `connect.NewError(code, errors.New("internal error"))`;
for `Unavailable` `errors.New("service unavailable")`; wrap the original in a small typed cause (`*upstreamError`, exposes
`Original()`), so the edge interceptor, which already owns the request id (`interceptors.go:122-125,178-180`), logs
`slog.Error("edge.upstream_error", request_id, method, original)` when `errors.As` finds it (unary and streaming paths); the
id reaches the client in `X-Request-Id`. Every other code keeps `st.Message()`. The generic texts equal what `team-ai` and
`team-domain` already send (`internal error`), so the e2e assertion `res.message == "internal error"`
(`ai_service_hardening_steps.py:219`) holds. **Reflection:** `Config.Server.ReflectionEnabled`
(`EDGE_REFLECTION_ENABLED`, default false); `server.go:106-131` registers the two reflection handlers only when true; a strict
`ENV` with it true refuses to start (`RequireNoReflection`); compose sets it true for local tooling. **Token expiry:**
`jwt.WithExpirationRequired()` in the `ParseWithClaims` options (`jwt.go:62`) and a non-empty `Claims.Subject` check
after parse; failure resolves to anonymous exactly as any invalid token (`forward.go:132-141`).

*Alternatives:* sanitise in each forwarder (22 services, the error this change exists to avoid); sanitise in the interceptor by
code only (would also rewrite the gateway's own `unavailable`/`internal` errors, hiding useful messages such as the
checkout kill-switch path, which uses `FAILED_PRECONDITION` anyway); fix every upstream `%v` site instead (the real inventory
is 100+ sites, see D18; the sanitiser is the control, the per-service cleanup is hygiene).

### D15 - team-order: least-privilege unmarked principals, no raw client

In `forwardMetadataInterceptor` (`upstream/domain.go:70-101`): the no-incoming-principal default becomes
`service-team-order` with scope `listing.read` only (constant `defaultServiceScopes = "listing.read"`); the `else` append of
`listing.read,identity.read` is removed so a forwarded principal is sent unchanged. `Clients.Listing` (`:17`) is unexported and
`cmd/server/main.go:150` (`NewCartService(..., upstreamClients.Listing, ...)`) uses `upstreamClients.Domain`
(`DomainClient.GetListing` is on the interface and unwrapped by `serviceStockClient`, so a cart read still forwards the buyer).
`domain_test.go:89` (asserts the old default list) is updated. Safe because no current call reaches the default branch and
`identity.*` is not enforced anywhere (Corrections).

### D16 - GitOps: declare the strict ENV, then add the NetworkPolicy; activation is the LAST step

Add overlays `envs/staging/services/{team-identity,team-payment,team-gateway}.yaml` (`env.ENV: staging`) and
`envs/prod/services/...` (`env.ENV: production`), copying the `team-order.yaml` overlay shape (header comment explaining which
guard each arms). Add `networkPolicy` to `envs/services/team-ai.yaml` (D13). Do **not** set `team-ai`'s `ENVIRONMENT`
to `production` here: that arms `AUTH_BEARER_TOKEN`-required/`DOCS_ENABLED=false`/`RATE_LIMIT_BACKEND=redis` validation and
needs a provisioned secret first (Q5). Because activation makes identity refuse the dev key and the default admin password,
**activate only after the shared environment's Vault holds a real `JWT_PRIVATE_KEY`/`JWT_KID` and `ADMIN_PASSWORD`** (ops
task 15.x); otherwise the rollout turns into an outage, which is the intended fail-closed behaviour but should be a choice.

### D17 - Compose and local GitOps: make every convenience explicit

`docker-compose.services.yaml`: `team-identity` `ADMIN_PASSWORD=admin123` (comment: local only); `team-payment`
`MOCK_PAYMENTS=true`; `team-gateway` `EDGE_REFLECTION_ENABLED=true`. Local `platform-gitops/argocd/apps/team-identity.yaml`
(kind): `ADMIN_PASSWORD: admin123`; local `team-payment` app: `MOCK_PAYMENTS: "true"`. `DEV_RETURN_RESET_TOKEN` and
`GRPC_BEARER_FALLBACK_ENABLED` stay unset (nothing in local e2e needs them). Each new variable is added to the owning repo's
`.env.example` and README (the Go services have an env-drift test, `make check-env` / `config_test.go`).

### D18 - Raw upstream errors: sanitised centrally, fixed per service only where touched

The gateway sanitiser (D14) removes the exposure for every service at once. The follow-up inventory of sites that embed raw
`err` in `Internal`/`Unavailable` (counted as `status.Errorf(codes.{Internal,Unknown,Unavailable,DataLoss}, ...)`): `team-order`
26 (e.g. `handler/order.go:127/159/258/295/319/340/401/552/617`), `team-engagement` 16, `team-promotion` 15, `team-payment` 10,
`team-identity` 8 (`handler/address.go:75`, `session.go:51`), `team-chat` 7, `team-domain` 6 (`handler/listing.go:246`), `team-notification`
5, and 1 each in `team-referral`, `team-sharing`, `team-verification`, `team-audit`. This change fixes (logs the error,
returns `status.Error(codes.Internal, "internal error")`) only the sites inside handlers it already edits: `team-payment`
(`payment.go`, `wallet_ledger.go`), `team-notification`, `team-promotion` (`sponsored.go`, `subscription.go`), `team-analytics`
(`service.go`), `team-order` none. The rest is listed for a later hygiene pass; none is exposed once the sanitiser ships.

## Follow-ups

### Hygiene backlog: raw `%v` errors in `codes.Internal` (task 15.4)

Re-grepped at the working tree (`status.Errorf(codes.Internal, ... %v|%w ...)` in `team-*/internal/handler`, tests excluded). The gateway
sanitiser (D14) hides these from clients, so this is log/hygiene debt, not an exposure. Fix pattern: log the error, return
`status.Error(codes.Internal, "internal error")`. Counts differ from D18's earlier estimate because D18 also counted
Unknown/Unavailable/DataLoss and was taken before the fixes below.

Fixed (no remaining site): `team-payment` (`11cd423`), `team-order` (`dd04f2c`; the one remaining `Unavailable` at `handler/order.go:126` wraps a
sentinel, not an upstream error), `team-notification`, `team-referral` (`ea84a06`), `team-analytics`, `team-promotion` `sponsored.go` and
`subscription.go` (`b550940`).

Not touched (file:line under `internal/handler/`):

- `team-promotion` (10): `voucher.go:76,90,100,145,166,181`; `flashsale.go:94,105,117,138`
- `team-engagement` (16): `reviews.go:34,52`; `wishlist.go:32,49,78,103,133`; `engagement.go:320,345,373,410,441,466,532,563,607`
- `team-identity` (8): `address.go:43,75,110,130,151`; `session.go:51,75,94`
- `team-chat` (7): `chat.go:69,100,154,199,258,293,317`
- `team-domain` (6): `listing.go:267,289,324,381,411,440`
- `team-audit` (1): `audit.go:35`; `team-sharing` (1): `sharing.go:33`; `team-verification` (1): `verification.go:95`

## Risks / Trade-offs

- **[Strict-ENV guards are inert until GitOps sets `ENV`]** -> D16 makes the overlays an explicit, last task; the fail-closed
  defaults (`MOCK_PAYMENTS`, `ADMIN_PASSWORD`, reflection, fallback flag) protect even while `ENV` is unset.
- **[Activation can refuse to boot identity/payment/gateway in a shared env]** (dev key still there, `MOCK_PAYMENTS` set, reflection
  set). -> Activate per environment after the ops prerequisites; rollback = remove the overlay. Identity outage blocks logins
  but not already-issued tokens until expiry (gateway keeps its cached JWKS).
- **[Existing admin/`admin123` row in shared DBs]** -> D7 ops task; the guard cannot see a hash in the DB.
- **[Sanitiser hides useful messages from the frontend toasts]** -> only for Internal/Unknown/DataLoss/Unavailable; the original
  is in the gateway log keyed by the `X-Request-Id` the client also receives; client-actionable codes keep messages.
- **[`PERMISSION_DENIED` vs `NOT_FOUND` inconsistency]** -> the rule is: a caller-supplied seller id is `PERMISSION_DENIED`
  (nothing to enumerate); a caller-supplied object id (transaction, payment, listing) is `NOT_FOUND` for strangers.
- **[Draft listing links now 404 for strangers, including buyers who favourited a listing later unpublished]** -> intended;
  frontend `getListing` returns `null`.
- **[Orphaned demo notification rows]** (`user_id = 'khach_hang_shopee'`) -> harmless demo data; not migrated.
- **[Rollout ordering]** the only cross-service hazard is `team-promotion`'s ownership lookup depending on `team-domain`'s
  service-principal visibility; both directions are safe (service principals already see everything today).
- **[Service principals remain unauthenticated metadata]** unchanged (ADR-0010); NetworkPolicy remains the control.
- **[Refund authority default]** seller-of-order may refund their own order; a dishonest seller can refund a buyer's payment
  (mock money, same party that approves returns today). Q1 covers moving to a service-triggered refund.

## Migration Plan

No database migrations. No new user scopes, therefore no token lag. Each step is deployable on its own against the previous
state; new env vars are ignored by old binaries.

0. **Ops prerequisites for shared environments (before step 9):** real `JWT_PRIVATE_KEY`/`JWT_KID` (Vault `svc/shared/jwt`,
   identity-only), `ADMIN_PASSWORD` (or none), reset/delete the existing `admin` row seeded with `admin123`, rotate the committed
   dev key wherever it was ever used, decide `team-ai` bearer (Q5).
1. `platform-core` proto comments and ADR amendments; `AGENTS.md` section 4 bullets (docs only).
2. **Config first:** compose/local GitOps env (D17), repo `.env.example`/README entries. Harmless to old binaries.
3. **Enforcers, any order among them** (none depends on another's enforcement): `team-identity` (inert until step 9),
   `team-payment`, `team-notification`, `team-referral`, `team-analytics`, `team-domain`, `team-promotion`, `team-order`, `team-ai`.
   After this step the stack is safe even with an old gateway.
4. **Gateway last among code:** sanitiser, reflection flag, `exp` required. Rolling the gateway back only re-exposes text.
5. **`platform-e2e`** updated in the same train as step 3 (payout/wallet/refund steps fail by design once enforced).
6. **GitOps activation last:** `ENV` overlays (staging first, then prod) and the `team-ai` NetworkPolicy.

Rollback: reverting an enforcer re-opens its RPCs; reverting step 6 disarms the strict guards but keeps the fail-closed
defaults; nothing in this change makes an older build incompatible with newer data.

Stacking: commit new work on each repo's currently checked-out branch (list in Context and in `tasks.md`); never rebase or
force-push. `team-promotion` and `team-domain` branches already carry wave-1/inventory work this change builds on. The root
repo holds prior uncommitted work (compose, `AGENTS.md`, `openspec/`, `deploy/`): stage only the paths a task names.

## Decided by default (no user input needed; change in review if you disagree)

- Wallet/ledger/payout reads need only a principal; payout writes, analytics and `Subscribe` need `listing.write` (D1, D3, D12).
- A mismatching `seller_id` is `PERMISSION_DENIED`; a non-owned transaction/payment/listing is `NOT_FOUND` (Risks).
- Refund = admin or the order's seller; `GetPayment` = buyer, order seller or admin (D5).
- `MOCK_PAYMENTS`, `ADMIN_PASSWORD`, `DEV_RETURN_RESET_TOKEN`, `EDGE_REFLECTION_ENABLED`, `GRPC_BEARER_FALLBACK_ENABLED` all
  default to off/unset; compose sets the two that e2e needs (`ADMIN_PASSWORD`, `MOCK_PAYMENTS`) plus reflection for tooling.
- Unset `ADMIN_PASSWORD` seeds no admin in **any** environment (D7, stricter than the literal decision, same intent).
- The dev key fingerprint and `dev-2026` kid are both refused in strict ENV; key material in the repo is not changed.
- A forbidden `ListListings` status is `PERMISSION_DENIED` (not silently coerced) (D11).
- Gateway generic messages are exactly `internal error` and `service unavailable`; request id travels in `X-Request-Id`.
- `team-ai` keeps its REST static bearer and production validation; only the gRPC fallback is gated (D13).
- `team-sharing` `CreateShareLink` stays anonymous (wave-1 D11).

## Open Questions

Only decisions that change scope, behaviour or UI; the defaults above are what the tasks implement.

- **Q1 - Who may refund?** Default: admin or the order's seller. Alternatives: admin-only (safest; the e2e return flow moves to
  the admin login), or a service-triggered refund (`team-order` calls `RefundPayment` on return approval with a new service-only
  `payment.refund` scope; needs a sender task, a drift-test entry, and removes the human).
- **Q2 - A stranger asking `ListListings` for `draft`/`rejected`:** default `PERMISSION_DENIED`; alternative: silently return
  published only.
- **Q3 - Password reset delivery:** with the token withheld there is no way to complete a reset in a deployed environment until
  an email/SMS channel exists. Default: accept (the RPC is not routed at the gateway and nothing calls it). Do you want a
  delivery channel scoped as a follow-up change?
- **Q4 - Ad amount caps:** defaults `MAX_AD_BID=1_000_000`, `MAX_AD_BUDGET=1_000_000_000` (minor units), no `bid <= budget`
  rule. Confirm the numbers or supply product limits.
- **Q5 - `team-ai` in deployed environments:** who provisions `AUTH_BEARER_TOKEN` (required once `ENVIRONMENT` is set to a
  non-local value) so GitOps can set `ENVIRONMENT=staging|production` for `team-ai`? Default: not set here.
- **Q6 - Existing shared-environment admin row:** reset/delete it as a documented ops step (default), or add a one-shot
  `ADMIN_PASSWORD_RESET=true` boot option to identity?
