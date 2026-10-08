## Context

See `proposal.md` for motivation and `specs/` for requirements. Item numbers (#1..#7) are the proposal numbers.
Everything below was re-read in the code at the tip of the currently checked-out branch of each repo
(`team-engagement` `feat/close-backend-gaps`, `team-order` `feat/order-integrity-guards`, `team-identity`
`feat/openspec-wave0-wave1`, `team-gateway` `feat/forward-idempotency-key`, `team-promotion` `main`, `team-audit`,
`team-verification`, `team-sharing`, `platform-e2e` each on a `fix/repo-doctor-*` branch, `platform-core`
`feat/commit-reservation`). Claims that turned out false or incomplete are listed under "Corrections to the audit".
Nothing was executed against a live stack.

Verified current state (paths relative to the repo root named in the path):

- **Gateway.** `team-gateway/internal/edge/interceptors.go:121-128` (`begin`): auth never rejects; a missing or invalid token
  resolves to the anonymous principal (id `anonymous`, `PUBLIC_SCOPES`). `forward.go:151-166` (`outgoing`) rebuilds
  `x-principal-{id,type,scopes}` from that resolved principal only, never from client headers. Precedents: stock RPCs are
  deliberately not forwarded (`internal/edge/listing.go:168-172`; the embedded `Unimplemented*Handler` answers
  `unimplemented`, asserted by `platform-e2e/tests/e2e/features/inventory/stock_rpcs_unrouted.feature`), and the cockpit HTTP
  route is behind `Edge.RequireScope("admin")` (`httpmw.go:66-90`, `server.go:149`: anonymous 401, no scope 403). A default
  per-principal/IP limiter applies to every RPC (`RATE_LIMIT_RPS=20`, `RATE_LIMIT_BURST=40`, `config/config.go:86-87`).
- **Scope vocabulary** (`team-identity/internal/authz/scopes.go`): buyer, seller and admin all hold `engagement:read|write`;
  only admin holds `admin`; `inventory.write` and `order.read` are granted to no role. `coverage_test.go:30-34` lists every
  user scope a service enforces (must be granted to some role); `:37-40` lists the explicit service-only scopes;
  `TestInventoryWriteGrantedToNoRole` (`:70-82`) checks the role table against `serviceOnlyScopes` in its first loop but the
  second loop hard-codes `inventory.write`/`order.read`. `admin` is enforced (gateway cockpit) but not declared in
  `enforcedScopes`. `Register` cannot mint an admin (`NormalizeRole`, `scopes.go:43-51`); the seeded admin is
  `EnsureAdmin("admin","admin123")` (`cmd/server/main.go:83`), available to e2e as `get_user_by_role("admin")`
  (`platform-e2e/test-data/*/users.json`). The JWT subject is the user id (`service/auth.go:96`, `Sign(u.ID, u.Username, ...)`),
  and a listing's `seller_id` is that id (`team-domain/internal/service/listing.go:93`).
- **Service-principal pattern to reuse.** `team-order/internal/upstream/domain.go:26-85`: `AsService(ctx)` marks a call so
  the client interceptor replaces the forwarded principal wholesale with `service-team-order` / `service` /
  `inventory.write` (and keeps `x-request-id`); `NewServiceStockClient` marks exactly the stock RPCs
  (`:129-160`). Consumer side: `team-domain/internal/handler/listing.go:328` (`RequireScopes(ctx,"inventory.write")` first
  statement) and `team-order/internal/handler/order.go:731-745` (`isServiceReader`: principal **type** must be `SERVICE`
  and scope `order.read`). Other senders: `team-engagement/internal/upstream/order.go:21-45` and
  `team-payment/internal/upstream/order.go:28-53` (`AsService` building outgoing metadata with one scope).
- **team-order -> team-promotion today.** `DialPromotion` (`team-order/internal/upstream/promotion.go:25-40`) installs the
  same `forwardMetadataInterceptor`: inside a buyer request it forwards the **buyer's** principal (plus `listing.read`), and
  in background work (`PaymentSettled` consumer `consumer/payment.go:184`, compensation/release
  `service/redemption.go:82`) it injects `service-team-order` with `defaultServiceScopes` (no promotion scope).
  `ValidateAndReserve` uses the order id as `reservation_id` (`service/order.go:283-290`, `redemption.go:48`).
- **Promotion behaviour.** `ValidateAndReserve` creates a `reserved` hold and never touches quota
  (`service/voucher.go:158-207`; `validate` only reads `Used`); `CommitReservation` flips the hold and calls
  `IncrementUsed` (`:260-281`); `ReleaseReservation` of a committed hold returns `released=false` (`:285-303`).
  `ListVouchers` filters on exactly `seller_id = $1` (`repository/voucher.go:113-118`), so an empty id is the public
  platform list. `interceptor/auth.go:93-105` has `RequirePrincipal` (rejects anonymous) but no scope helper.
  `team-promotion` has no upstream client package.
- **Audit.** `team-audit` has no `interceptor` package; `grpcserver/server.go:22` is `grpc.NewServer()`. Nothing in the stack
  calls `WriteAuditEvent` except `platform-e2e` (`src/api/services/audit_service.py`, `test_new_services.py:121`).
- **Engagement.** `upstream/order.go` already has the order client and `AsService`; there is no domain client and
  `UPSTREAM_DOMAIN_ADDR` is not set for engagement (`docker-compose.services.yaml:281`). `QARepository.GetQuestionByID`
  exists (`repository/qa.go:40`). The interceptor offers `RequireScopes` only (`interceptor/auth.go:136-151`).
- **Verification.** `interceptor/principal.go` reads only `x-principal-id` (no type/scopes) and falls back to the demo user.
  `team-frontend/src/app/account/verification/page.tsx` calls `getVerificationStatus()` with the caller's own id.
- **Sharing.** `interceptor/auth.go` reads `x-user-id`, a header the gateway never sends (it sends `x-principal-id`), and
  the handler never uses the caller (`handler/sharing.go`).

### Caller audit (every RPC this change touches)

Searched: `team-frontend/src` (non-generated), `platform-e2e/{src,tests}`, `platform-core/tools/*.sh`, `scripts/`, `deploy/`,
`platform-gitops/`, and every other `team-*` service (Go/Python).

| RPC | Legitimate callers found (evidence) | Decision |
|---|---|---|
| `VoucherService/ValidateAndReserve` | `team-order` saga (`service/redemption.go:48`); **frontend checkout preview** (`lib/gateway/promotion.ts` `previewVoucher`, reservation id `preview:<buyerId>:<CODE>`, buyer id from the session; used by `features/voucher/actions.ts`, `features/order/CheckoutView.tsx:127`); e2e via the UI (`promo_steps.py`) | **Keep routed.** Service principal: fields trusted. User: bind buyer from principal, preview namespace only |
| `VoucherService/CommitReservation` | `team-order` `consumer/payment.go:184` only. No frontend, e2e or script caller | **Unroute at gateway**; service-only (`promotion.reserve`) |
| `VoucherService/ReleaseReservation` | `team-order` `service/redemption.go:82` (cancel, compensation) only | **Unroute at gateway**; service-only |
| `VoucherService/CreateVoucher` | frontend `features/voucher/VoucherManager.tsx` (shown only with `listing.write`, `app/vouchers/page.tsx:20`) | Keep; `listing.write`, `PLATFORM` needs `admin` (UI scope selector offers `PLATFORM`; a seller choosing it gets an error toast, no crash, no frontend change) |
| `VoucherService/ListVouchers` / `GetVoucher` | public hub `app/vouchers/page.tsx:16`; e2e `voucher_hub.feature` | Keep public |
| `FlashSaleService/CreateCampaign` | e2e only (`promo_steps.py:170`, seller token, own listing); no frontend caller | Keep; `listing.write` + ownership (admin bypass) |
| `AuditService/WriteAuditEvent` | e2e only (`test_new_services.py:121`, with a seller token). **No service produces audit events today** | **Unroute at gateway**; service-only (`audit.write`) |
| `AuditService/QueryAuditLog` | e2e only; no frontend page | Keep routed; `admin` at edge and in service |
| `EngagementService/ResolveDispute` | e2e only (`group_b_steps.py:81-94`, false positive). No frontend caller | Keep; `admin` at edge and in service |
| `EngagementService/GetDispute`, `CreateDispute` | e2e only (`engagement_service.py`); no frontend caller | Keep; party-only / order-verified |
| `EngagementService/AnswerQuestion` | frontend `features/listing/qa/QASection.tsx:112` (form offered to every viewer, `isShopReply=true`); e2e `group_b_steps.py` | Keep; shop flag derived |
| `VerificationService/ReviewKyc` | e2e none; frontend none (no admin KYC screen is wired) | Keep; `admin` at edge and in service |
| `VerificationService/SubmitKyc`, `GetVerificationStatus` | frontend `app/account/verification` (own id); e2e `test_new_services.py` | Keep; authenticated, own or admin |
| `SharingService/CreateShareLink` | frontend `ShareButton`/`features/listing/actions.ts` (public listing page, may be logged out); e2e | Keep anonymous (D11) |

Consequence: **no frontend change is needed.** The only UI-visible behaviour changes are (a) a seller selecting scope
`PLATFORM` in the voucher form is rejected, (b) a non-owner's Q&A answer is no longer badged "Shop".

### Corrections to the audit

- **#1 (voucher saga) is real but its mechanics were mis-stated.** `ValidateAndReserve` does **not** consume quota; quota is
  consumed by `CommitReservation` (`used++`). The quota-burn path is therefore *anonymous `ValidateAndReserve` with a
  self-chosen `reservation_id` followed by anonymous `CommitReservation`*. "Release others' reservations by guessing ids" is
  much weaker than claimed: checkout holds are keyed by the order UUID (`service/order.go:283-290`) and a committed hold
  cannot be released; only `preview:<buyerId>:<CODE>` ids are predictable, and releasing those harms nobody. The real
  remaining harms: quota exhaustion, unlimited hold creation, and a third party releasing a not-yet-committed checkout hold
  whose id they obtained (then the later `PaymentSettled` commit fails and the discount was already given). `buyer_id` is
  stored on the hold for attribution only (no per-buyer cap exists), so its spoofing is integrity, not exhaustion.
- **#3 (`ListVouchers`, MEDIUM) is not a defect as the product is defined.** An empty `seller_id` is the public platform list
  the `/vouchers` hub and the `promo-vouchers` spec need; a non-empty one lists that seller's shop vouchers, which are public
  promotions. Dropped as a fix; recorded as an open question (are shop voucher codes ever meant to be private?).
- **#9 (demo user) is mis-located.** Through the gateway an unauthenticated caller reaches `team-verification` with
  `x-principal-id: anonymous` (`forward.go:161`), so the `khach_hang_shopee` fallback fires only for calls with **no**
  `x-principal-id` (direct/in-cluster calls, tests). All anonymous *gateway* callers still share the literal identity
  `anonymous` (and can `SubmitKyc`/read status as it). Both cases are fixed by requiring an authenticated principal.
- **#6 needs an e2e caveat.** `group_b_steps.py` passes `defendant_id = seller.username`, but seller ids are user ids
  (`Sign(u.ID, ...)`), so the existing scenario would be rejected by the new `defendant_id == order.seller_id` rule; the e2e
  track reads the seller id from the order (task 11.5).
- **#7 is stronger than stated:** the UI offers the answer form to every viewer with `isShopReply` defaulting to `true`
  (`QASection.tsx:112`, `actions.ts`), so any buyer answering through the normal UI is badged "Shop" today.
- **The e2e false positive (#7 in the proposal) is precisely** "the step only passes because the bug exists": the post
  helper raises on status >= 400 (`base_service.py:64-65`), so `assert res is not None` holds exactly when `ResolveDispute`
  lets a downgraded buyer through. After the fix the step fails loudly, which is why it must be rewritten, not just kept.
- Confirmed as audited, no correction: #2 (`voucher.go:33-46`, `flashsale.go:33-47`), #4 (no interceptor anywhere in
  `team-audit`), #5 (`engagement.go:459-556`), #8 (`verification.go:59-67`, `edge/verification.go:55`), #10
  (`verification.go:43-47`), sharing (#6).

## Goals / Non-Goals

**Goals:**

- Every RPC listed in the proposal enforces a principal/scope rule that matches its intended caller, **in the service**
  (authoritative), with the gateway adding a thin second gate for admin-only RPCs and dropping routes nobody outside the
  stack should call.
- Reuse the existing service-principal pattern verbatim (marker + wrapper client on the sender, `type=service` + scope check
  on the receiver, single-purpose scope per call); introduce no new mechanism.
- Every legitimate caller in the audit table keeps working unchanged, including the frontend.
- Adding a scope a service enforces cannot silently drift from what identity can issue (extend the existing drift test).

**Non-Goals (design-level):**

- No shared auth library across repos (each service keeps its own small interceptor copy, as today); no new service.
- No proto shape change, no migration, no new user scope, no token-format change.
- No attempt to authenticate service principals (ADR-0010 mTLS follow-up): a forged `x-principal-*` from inside the network
  can still present `promotion.reserve`; the NetworkPolicy remains the compensating control.
- No change to business rules of vouchers, disputes or KYC beyond who may act.

## Decisions

### D1 - Scope vocabulary: reuse `admin`; add only service-only scopes `promotion.reserve` and `audit.write`

Admin-only RPCs (`ResolveDispute`, `ReviewKyc`, `QueryAuditLog`, `PLATFORM` vouchers, campaign-for-any-listing) gate on the
existing `admin` scope, which only the admin role holds and which every current admin token already carries. The two new
scopes are **service-only**: `promotion.reserve` (held by `service-team-order`) and `audit.write` (held by any service that
records audit events, none yet). Drift-test rules (`coverage_test.go`): add `admin` to `enforcedScopes` (enforced by
`team-gateway` edge policy and the services; proves identity can issue it); add both new scopes to `serviceOnlyScopes`;
change the second loop of the negative test to iterate `serviceOnlyScopes` instead of hard-coding two names, so adding a
service-only scope automatically extends the negative check. `TestRoleScopeTable` is unchanged (role sets do not change).

Consequence: **no user token ever needs to change**, so there is no JWT-TTL (3600 s) lag window for this change. If a
dedicated user scope were introduced later it would become effective for an existing session only after re-login/expiry.

*Alternatives:* (a) dedicated user scopes `dispute.resolve`, `kyc.review`, `audit.read` granted to admin: finer-grained, but
admin is the only operator role, each needs a role-table entry, a drift-test entry and a token-lag window, for no extra
protection today; (b) one generic `service` scope for all service-to-service calls: too coarse (team-payment's `order.read`
would become able to commit vouchers), and contradicts the single-purpose scopes already in place; (c) gate on
`engagement:write` plus an in-handler role lookup: identity has no role claim on the principal, only scopes.

### D2 - Receiver-side helpers: `RequireService(ctx, scope)` and `RequireAdmin(ctx)`, one small copy per service

Each enforcing service gets the same two helpers in its existing `interceptor` package, modelled on `isServiceReader`:
`RequireService` demands principal **type** `SERVICE` *and* the scope (a user principal carrying the scope string is
rejected); both return `UNAUTHENTICATED` for no principal/anonymous and `PERMISSION_DENIED` otherwise. `team-audit` has no
interceptor at all: add the package (principal extraction copied from `team-engagement`/`team-promotion`) and register it in
`grpcserver.New`. `team-verification` replaces `principal.go` (id only) with the same extraction so type and scopes are
available. `team-promotion` already extracts the principal; it gains `RequireScopes`/`RequireService`. The check is the
**first statement** of each guarded handler, before any read, so a rejected caller learns nothing (as in
`team-domain` `ReserveStock`).

*Alternatives:* a shared Go module for the interceptor - rejected (polyrepo rule: no cross-repo code coupling beyond the
proto contract; ~60 lines duplicated per service is the established pattern); a gRPC server interceptor with a
per-method policy table - rejected for now (handlers already do their own checks everywhere else; one style).

### D3 - `ValidateAndReserve` is dual-mode; Commit/Release are service-only (#1)

- `CommitReservation`, `ReleaseReservation`: `RequireService(ctx, "promotion.reserve")`.
- `ValidateAndReserve`: anonymous/none -> `UNAUTHENTICATED`. `service` principal -> must hold `promotion.reserve`
  (`PERMISSION_DENIED` otherwise), request fields trusted (team-order derives `buyer_id` from its own authenticated
  principal). Any other authenticated principal (user) -> `buyer_id` := principal id (request value ignored),
  `reservation_id` must start with `preview:<principal id>:` else `PERMISSION_DENIED`. Because a user can neither commit nor
  release, a preview hold consumes no quota and can only accumulate `reserved` rows exactly as it does today (one per
  buyer+code, idempotent), so the user path cannot exhaust anything.

Why keep the user path at all: the frontend checkout preview (`previewVoucher`) calls it through the gateway today with
exactly this id shape and the session's own id, and frontend edits are out of scope. The namespace rule also closes id
squatting (a hold created under someone else's preview id, or replaying an order id to read its discount).

*Alternatives:* (a) **remove `ValidateAndReserve` from the gateway too** - would break the checkout voucher preview (caller
audit) -> rejected; (b) add a proto `PreviewVoucher` RPC that never creates a hold (cleanest: no hold rows from previews, no
dual mode) - requires a contract change in platform-core **and** a frontend switch, so deferred as a follow-up and noted in
non-goals; (c) leave `ValidateAndReserve` open and only lock Commit/Release - still allows unlimited hold creation and
buyer/seller spoofing, rejected.

### D4 - `team-order` calls promotion as itself, reusing the `AsService` marker with a per-call scope (#1)

Extend the marker in `team-order/internal/upstream` so it carries the scope to present (stock RPCs keep `inventory.write`;
promotion RPCs present `promotion.reserve`; never both on one call), and add `NewServicePromotionClient(inner)` wrapping
`promotionv1.VoucherServiceClient` so exactly `ValidateAndReserve`, `CommitReservation`, `ReleaseReservation` are marked —
the same shape as `NewServiceStockClient`. `cmd/server/main.go` wires the wrapped client into `WithPromotionClient` and
`WithVoucherCommitter`. The principal is identical in the buyer request, the consumer and compensation, so there is one
behaviour to test (today it differs: buyer principal in-request, default service principal in the background).

*Alternative:* a second interceptor specific to promotion - rejected (duplicates the stock interceptor; the marker already
does "replace the principal wholesale").

### D5 - Voucher/campaign creation authorization (#2)

`CreateVoucher`: `RequireScopes("listing.write")` (seller, admin); if the requested scope is `PLATFORM` also
`RequireAdmin`. `SHOP` keeps binding `seller_id` from the principal (already true). `CreateCampaign`: `listing.write`; admin
allowed for any listing; otherwise `GetListing(listing_id)` on `team-domain` as `service-team-promotion` (scope
`listing.read`, sender built exactly like `team-engagement`'s `AsService`) and require `listing.seller_id == principal id`.
Not found or another owner -> `PERMISSION_DENIED`; transport error or unconfigured address -> `UNAVAILABLE`
(fail closed; admin path needs no lookup so operations can still run campaigns). New config `UPSTREAM_DOMAIN_ADDR` (with
`.env.example`, README and compose/gitops entries; `CheckEnvExample`-style drift test if the repo has one).

*Alternatives:* (a) **admin-only campaigns** (no domain dependency, simplest) - but the existing e2e flash-sale scenario and
the marketplace model have sellers running campaigns on their own listings; kept as open question Q2; (b) trust
`listing_id` and skip ownership - the audited hole; (c) add `seller_id` to `CreateCampaignRequest` - proto change and still
client-trusted.

### D6 - `ListVouchers` / `GetVoucher` stay public (audit #3 dropped)

See "Corrections". The repository filter already makes `seller_id=""` the platform list. No code change; a spec requirement
pins public browsing so a later "tighten everything" pass cannot break the hub unnoticed. Q1 covers the private-code case.

### D7 - Audit: service-only write, admin read, write route removed, no producers wired (#4)

`WriteAuditEvent`: `RequireService("audit.write")`; `actor_id` from the request stays (a service legitimately records the
actor it acts for; the trust boundary is now "who may write", not "what they write"). `QueryAuditLog`: `RequireAdmin`.
Gateway: stop forwarding `WriteAuditEvent` (nobody outside the stack has a reason to write audit rows; the only caller today
is e2e), gate `QueryAuditLog` on `admin` at the edge. No producer is wired (no service emits audit events today); when one
does, it sends `audit.write` exactly like D4.

*Alternatives:* keep the write route but require `admin` (lets e2e keep its round trip, but makes the log attestable by a
human-held token and a compromised admin could forge history) - rejected; stamp the writer's service id into `metadata`
(provenance) - useful but not needed to close the hole, deferred.

### D8 - Engagement ownership checks reuse the order client and add one domain client (#5, #6, #7)

- `ResolveDispute`: `RequireAdmin` first; existing state rules unchanged.
- `GetDispute`: load, then allow claimant, defendant or admin; any other caller gets `NOT_FOUND`, identical to an unknown id
  (no dispute data, and no existence oracle for dispute ids; this deliberately differs from `GetOrder`, which answers
  `PERMISSION_DENIED` to a stranger).
- `CreateDispute`: extend `upstream.OrderClient` with `GetOrderParties(ctx, orderID) (buyerID, sellerID, err)` built on the
  existing `GetOrder(AsService(ctx), ...)` (already `service-team-engagement` / `order.read`). Unknown order **and** not-the-
  buyer both map to `NOT_FOUND` (same indistinguishability rule as `VerifyPurchase`, `upstream/order.go:87-116`); wrong
  `defendant_id` -> `PERMISSION_DENIED`; upstream failure or `UPSTREAM_ORDER_ADDR` unset -> `UNAVAILABLE` (fail **closed**:
  unlike reviews' "verified purchase" enrichment, this is an authorization decision).
- `AnswerQuestion`: load the question (`GetQuestionByID`, exists), read the listing from `team-domain` via a new
  `upstream.ListingClient` (`GetListing` as `service-team-engagement`, `listing.read`), and set `is_shop_reply :=
  principal.id == listing.seller_id`; the request flag is ignored. Lookup failure -> `false` (never elevate on doubt). Non-
  owners can still answer (flagged non-shop) so the always-visible answer form does not start erroring (Q3 if the product
  wants to forbid it). New config `UPSTREAM_DOMAIN_ADDR`.

*Alternatives:* a dedicated `dispute.resolve` scope (D1a); reject non-owner answers (breaks the UI for every buyer who
answers today; behaviour change -> Q3); denormalise `seller_id` into engagement tables via events (violates nothing but is a
migration + consumer for a low-volume check; a synchronous read is consistent with how `VerifyPurchase` already works).

### D9 - Verification: real principal, admin review, own-or-admin status, no fallback (#8, #9, #10)

Replace `UserIDOrDemo` with a principal extraction identical to D2 and delete `demoUserID`. `SubmitKyc` and
`GetVerificationStatus` need an authenticated principal (the status RPC with an empty or own `user_id` reads the caller's
record; another id requires `admin`). `ReviewKyc`: `RequireAdmin`. The frontend always passes the caller's own id, so it is
unaffected (and an unauthenticated visit already degrades to "unspecified" through its `catch`). Local/dev calls without the
gateway now fail `UNAUTHENTICATED` by design; compose runs through the gateway.

*Alternative:* keep the demo fallback behind a `DEV_ALLOW_DEMO_USER` flag - rejected (a flag that re-opens identity
collapse is one mis-set env var from production; nothing needs it).

### D10 - Gateway: small admin procedure policy plus unrouting, both defence in depth (proposal "team-gateway")

In `internal/edge`, a tiny static map `procedure -> required scope` (three entries, all `admin`) evaluated in the existing
unary edge interceptor right after the principal is resolved and before rate limiting/forwarding: anonymous ->
`CodeUnauthenticated` (401), missing scope -> `CodePermissionDenied` (403). It is the RPC analogue of the cockpit
`RequireScope("admin")` and keeps the gateway free of business logic (it knows *who may call*, not *what it means*). A unit
test asserts the map covers exactly the admin-only RPCs named in the specs (so the gateway and the services cannot drift
silently). Routes removed: delete the three forwarder methods (`CommitReservation`, `ReleaseReservation` in
`edge/promotion.go`, `WriteAuditEvent` in `edge/audit.go`) so the embedded `Unimplemented*Handler` answers `unimplemented`;
replace the stale code comment "(HTTP 404)" at `listing.go:170` while there if touched. `ValidateAndReserve` is kept. No
streaming RPC is involved, so the stream interceptor is untouched.

*Alternatives:* (a) services only, no edge gate - simpler, single source of truth, but one missed service check again exposes
the RPC to the internet (the very class of bug found here); (b) a generic "every RPC declares its scope at the gateway" table
- correct direction but a large cross-cutting rewrite of a "never rejects" design; out of scope; (c) path-prefix rules
(`/admin/...`) - the Connect paths are fixed by the proto packages.

### D11 - Share links stay anonymous (#6)

Decision: **keep `CreateShareLink` anonymous; no principal requirement; no new limiter.** Reasons: the public listing page
offers the share button to logged-out visitors (`ShareButton`, `features/listing/actions.ts`); the link is a public artifact
(target type/id + UTM, synthesized OG meta, no secrets); the gateway already rate-limits each anonymous peer (20 rps, burst
40) on every RPC; requiring login would change visible UI behaviour for no confidentiality gain. Record it in the capability
spec, the `team-sharing` README/FEATURES notes, and correct the misleading interceptor comment (it reads `x-user-id`, which
the gateway never sends, and the handler ignores the caller). Residual risk (row growth from one IP at <= 20 rps) is
accepted; Q4 offers a target-type allowlist/length bound if abuse appears.

### D12 - E2E false positive (#7)

`group_b_steps.py::admin_resolves_dispute` (lines ~81-94) is rewritten: log in as the seeded admin
(`get_user_by_role("admin")` + `auth.login`, no register fallback, same as `cockpit_metrics_steps.py::admin_logged_in`),
assert the returned dispute status is `RESOLVED` (not `is not None`), and add the negative case: the buyer who opened the
dispute calling `ResolveDispute` gets HTTP 403 (and a freshly registered "admin" gets 403). `CreateDispute` in the same
feature takes `defendant_id` from the order's `sellerId` instead of `seller.username`.

## Risks / Trade-offs

- **[Rollout ordering: enforcer before sender]** If `team-promotion` enforces before `team-order` sends `promotion.reserve`,
  voucher checkouts fail (`PERMISSION_DENIED` on `ValidateAndReserve` -> checkout error), and `PaymentSettled` commits fail
  (quota not consumed, consumer retries). -> Deploy order in "Migration Plan": senders first; never roll `team-order`
  back below the service-principal version while promotion enforces (the same caveat as ADR-0008's stock gate).
- **[Domain dependency for ownership checks]** `team-promotion` and `team-engagement` now depend on `team-domain`
  `GetListing`. -> Fail closed only where it is an authorization decision (`CreateCampaign` non-admin, `CreateDispute`);
  fail to the safe value for `AnswerQuestion` (`is_shop_reply=false`). Admin paths do not depend on it. Both are read-only
  gRPC (Rule 3 holds).
- **[Service principals are still unauthenticated metadata]** Anyone who can reach a service port can send
  `x-principal-type: service` + `promotion.reserve`. -> Unchanged from `inventory.write`/`order.read`; ADR-0010 NetworkPolicy
  remains the control and the mTLS follow-up stays open. The new gates do remove the *internet-reachable* path (gateway).
- **[Preview holds remain]** The user path of `ValidateAndReserve` still writes one `reserved` row per (buyer, code), as
  before. -> Bounded by the edge limiter and idempotency; a `PreviewVoucher` RPC is the proper fix (follow-up).
- **[Seller selecting `PLATFORM` in the voucher form now errors]** -> Visible only as an error toast; product may want the
  option hidden for sellers (frontend follow-up, not forced).
- **[Gateway policy drift]** Two places (edge map, services) name the admin-only RPCs. -> A unit test pins the edge map to
  exactly the RPCs named in the specs; e2e can only enter through the gateway, so it cannot prove the service checks alone,
  which are therefore covered by handler tests in each service repo (task groups 5-8).
- **[Audit round-trip test disappears]** e2e can no longer write then read an event through the gateway. -> Replaced by
  negative/positive gate scenarios plus service-level handler tests for the write->query round trip; the e2e round trip
  returns when a first producer is wired (non-goal).

## Migration Plan

No database migrations. No new user scopes, therefore no token lag: tokens issued before the change keep working unchanged;
admin tokens already carry `admin`. Order (each step is independently deployable and safe against the previous state):

1. `platform-core` proto comments + ADR amendments (docs only); `team-identity` test/constant update (no runtime change).
2. **Senders first:** `team-order` (promotion client sends `service-team-order`/`promotion.reserve`; harmless against the
   current ungated `team-promotion`). Deploy config first for the new outbound edges: `UPSTREAM_DOMAIN_ADDR` on
   `team-promotion` and `team-engagement` (compose `docker-compose.services.yaml`, `platform-gitops` values, `.env.example`).
3. **Enforcers:** `team-promotion`, `team-audit`, `team-verification`, `team-engagement` (any order among them; none depends
   on another's enforcement). After this step the stack is safe even if the gateway is old.
4. **Gateway last:** edge admin policy + unrouted RPCs. Rolling the gateway back only re-exposes routes that the services
   now reject.
5. `platform-e2e`: update scenarios in the same train as steps 3-4 (the old audit round trip and admin-dispute step fail by
   design once enforced).

Rollback: reverting an enforcer re-opens its RPCs. Reverting `team-order` below step 2 while promotion enforces breaks
voucher checkouts (no-voucher checkouts are unaffected). Reverting the gateway restores the old routes only.

Stacking: commit new work on each repo's currently checked-out branch; never rebase or force-push; `team-order`'s work sits on
`feat/order-integrity-guards` (assumes `order-domain-correctness` tasks are present, notably `AsService` for stock RPCs and
`GetOrder` accepting `order.read`).

## Open Questions

Decisions the user may want to change (they alter scope, behaviour or UI; the defaults above are what the tasks implement):

- **Q1 - Are shop voucher codes private?** Default: no (public promotions, `ListVouchers`/`GetVoucher` stay public, D6).
  If yes: `ListVouchers` with a non-empty `seller_id` would need principal == that seller or admin, and the public hub would
  show platform vouchers only (it already does).
- **Q2 - Flash sales: seller-created or admin-only?** Default: sellers may create campaigns on their own listings (adds the
  `team-promotion -> team-domain` dependency, D5). Admin-only would drop that dependency but the existing e2e flash-sale
  scenario (seller token) would switch to the admin login.
- **Q3 - May non-sellers answer product questions?** Default: yes, flagged non-shop (keeps the UI working). Forbidding it
  would make the always-visible answer form error for buyers and needs a UI change to hide it.
- **Q4 - Share-link abuse bound?** Default: no change (D11). Offer: target-type allowlist and length bounds if rows grow.
- **Q5 - Who will emit audit events?** Default: nobody in this change; the endpoint is made safe only. Naming the first
  producer would add a sender task (`audit.write` principal) and restore the e2e round trip.
