# 00-spec — P10 backend gaps (G8–G15) and review leftovers

initiative: p10-backend-gaps

## Goal
The three apps stop working around missing backend capability: an admin can stop or adjust a voucher and
manage every flash-sale campaign with server-side checks, clear duplicate/outage errors and safe retries;
a buyer signed in on one of buyer/seller apps reaches the other without signing in again; a buyer becomes a
seller from the seller app. The gateway stops logging raw user ids. Rows G8–G15 in `fe-platform/docs/gaps.md`
are closed.

## User stories
- Admin stops voucher "SALE10": new checkouts get `valid=false, reason=inactive`; an order already holding
  SALE10 still commits it. Edge: stopping an already stopped voucher is a no-op, still audited once per call.
- Admin moves SALE10's end date or raises its quota from 100 to 150. Edge: quota below `used` or any money
  field change is refused `INVALID_ARGUMENT`.
- Admin creates voucher with a taken code → "code already exists" (ALREADY_EXISTS), not a generic error.
- Admin creates a voucher while team-audit is down → "audit is down, nothing created"; while promotion is
  down → "promotion is down, nothing created" (different messages).
- Admin lists campaigns filtered by scheduled/active/ended/cancelled; cancels an active one (no new holds,
  held units still commit, sold kept); edits a scheduled one (price, stock cap, window). Edge: editing a
  started campaign → FAILED_PRECONDITION.
- Admin's create times out and they retry with the same idempotency key → the same campaign, never two.
  Same key, different payload → ALREADY_EXISTS.
- Admin creates a campaign with sale price ≥ listing price, stock cap 0, end before start or in the past, or
  an unknown listing → INVALID_ARGUMENT naming the field. Kill-switch off → lists say `flash_sale_disabled`.
- Buyer signed in on the buyer app clicks "Kênh người bán": the seller app opens signed in, no form.
  Edge: a code used twice, older than 60 s, for another app, or whose source session was revoked → the
  seller app shows its login form. Admin app never mints or redeems a code.
- Buyer opens the seller app, accepts the seller terms and is a seller at once (new token with seller scope).
  Edge: a non-ACTIVE account is refused; an existing seller calling again gets a fresh token, no change.

## Non-goals
- Seller/admin ArgoCD apps, admin ingress VPN/IP allow-list, mock audit (deferred by the user).
- Hard delete of vouchers or campaigns; editing voucher money fields (discount, min spend, max discount, scope).
- Admin approval queue for sellers; SSO into or out of the admin app.
- Changing the shop-voucher (seller-owned) flows.

## Architecture decisions
- Voucher stop is a soft state (`active=false`, `deactivated_at`); ValidateAndReserve refuses inactive
  vouchers with reason `inactive`; Commit/Release of existing holds are unchanged — Why: stopping must not
  break orders already in the saga.
- Campaign status is derived from `cancelled_at` and the window (cancelled > scheduled/active/ended);
  ReserveFlashSaleStock refuses a cancelled campaign with the existing `not_active` reason — Why: no new
  reason code for order to learn.
- Every admin write (deactivate, update voucher; cancel, update campaign) writes its audit entry first and
  fails closed UNAVAILABLE, mirroring CreateVoucher — Why: existing audit-before-write rule.
- Outage errors carry `google.rpc.ErrorInfo{domain:"promotion.platform", reason:AUDIT_UNAVAILABLE|PROMOTION_UNAVAILABLE}`;
  the gateway keeps ErrorInfo reason when it sanitizes the message and produces PROMOTION_UNAVAILABLE itself
  when promotion cannot be reached — Why: the browser must tell the two apart without a raw backend message.
- Campaign idempotency: `(admin principal, idempotency_key)` unique, stored with a payload hash, kept 24 h —
  Why: retry after timeout must be safe; key scoped per actor so keys cannot collide across admins.
- Sale price below listing price uses team-listing GetListing (existing contract) on both seller and admin
  paths; listing unreachable → UNAVAILABLE with ErrorInfo reason `LISTING_UNAVAILABLE` and nothing created
  — Why: the user chose full validation.
- SSO hand-off: identity stores only the SHA-256 of a 32-byte random code with `target_app`, source `sid`,
  user id, `expires_at = now+60s`, `redeemed_at`. Redeem is a public RPC (like RefreshSession), atomically
  marks the code used, checks the source session is live and the account ACTIVE, and opens a NEW session
  (own sid and refresh token) for the target app. Mint and redeem write team-audit entries
  (`auth.handoff_mint`, `auth.handoff_redeem`) with fail-closed UNAVAILABLE — Why: user decision; codes are
  bearer secrets.
- Code travels in the URL fragment-free path `/<app>/auth/handoff?code=…` on the target app, which redeems it
  server-side and immediately redirects to `next` without the code (Referrer-Policy no-referrer on that route)
  — Why: keep the code out of history/referrers as far as a redirect allows; 60 s and single use bound the rest.
- BecomeSeller adds role seller to the user, records `seller_terms_version` and `seller_terms_accepted_at`,
  issues a new access token on the same sid (refresh token unchanged) and publishes `UserRoleGranted` on
  `identity.events` — Why: immediate grant chosen by the user; downstream (domain seller profile) may react.
- Gateway logs `principal_fp` = kafkax.Fingerprinter over the principal id with secret `LOG_KEY_FP_SECRET` (SDK LogKeyFP)
  (the existing SDK env `LOG_KEY_FP_SECRET` via sdkconfig.LogKeyFP: local default secret when ENV=local, required and fail-fast in staging/prod) and `principal_kind` (anonymous/user/service); `principal` joins the
  go-sdk sloglint forbidden-keys — Why: no raw ids in logs, but requests of one caller stay joinable.

## Contracts
```proto
// platform/promotion/v1/promotion.proto
service VoucherService {
  // admin-only (scope admin). Audit entry voucher.deactivate first; UNAVAILABLE + ErrorInfo on audit down.
  // NOT_FOUND unknown id. Idempotent on an inactive voucher (still audited).
  rpc DeactivateVoucher(DeactivateVoucherRequest) returns (DeactivateVoucherResponse);
  // admin-only. Only ends_at and quota. INVALID_ARGUMENT: ends_at not after starts_at, quota != 0 and < used,
  // quota lowered, nothing to change. FAILED_PRECONDITION on an inactive voucher. Audit voucher.update first.
  rpc UpdateVoucher(UpdateVoucherRequest) returns (UpdateVoucherResponse);
}
message Voucher { /* existing 1-12 */ bool active = 13; google.protobuf.Timestamp deactivated_at = 14; }
message DeactivateVoucherRequest { string voucher_id = 1; }
message DeactivateVoucherResponse { Voucher voucher = 1; }
message UpdateVoucherRequest {
  string voucher_id = 1;
  optional google.protobuf.Timestamp ends_at = 2;
  optional int64 quota = 3;
}
message UpdateVoucherResponse { Voucher voucher = 1; }

enum CampaignStatus {
  CAMPAIGN_STATUS_UNSPECIFIED = 0;
  CAMPAIGN_STATUS_SCHEDULED = 1;
  CAMPAIGN_STATUS_ACTIVE = 2;
  CAMPAIGN_STATUS_ENDED = 3;
  CAMPAIGN_STATUS_CANCELLED = 4;
}
service FlashSaleService {
  // admin-only. Empty statuses = all. Newest starts_at first. Read, not audited.
  rpc ListCampaigns(ListCampaignsRequest) returns (ListCampaignsResponse);
  // admin-only. Audit campaign.cancel first. FAILED_PRECONDITION if ended. Idempotent on cancelled.
  rpc CancelCampaign(CancelCampaignRequest) returns (CancelCampaignResponse);
  // admin-only. Only while SCHEDULED (else FAILED_PRECONDITION). Same validation as create. Audit campaign.update first.
  rpc UpdateCampaign(UpdateCampaignRequest) returns (UpdateCampaignResponse);
}
message FlashSaleCampaign { /* existing 1-8 */ CampaignStatus status = 9; google.protobuf.Timestamp cancelled_at = 10; }
message CreateCampaignRequest { /* existing 1-6 */ string idempotency_key = 7; } // required, 8..64 chars [A-Za-z0-9_-]
message ListCampaignsRequest { repeated CampaignStatus statuses = 1; platform.common.v1.PageRequest page = 2; }
message ListCampaignsResponse {
  repeated FlashSaleCampaign campaigns = 1;
  platform.common.v1.PageResponse page = 2;
  bool flash_sale_disabled = 3;
}
message ListActiveCampaignsResponse { /* existing 1-2 */ bool flash_sale_disabled = 3; }
message CancelCampaignRequest { string campaign_id = 1; }
message CancelCampaignResponse { FlashSaleCampaign campaign = 1; }
message UpdateCampaignRequest {
  string campaign_id = 1;
  int64 sale_price = 2;
  int64 stock_cap = 3;
  google.protobuf.Timestamp starts_at = 4;
  google.protobuf.Timestamp ends_at = 5;
}
message UpdateCampaignResponse { FlashSaleCampaign campaign = 1; }
```
```proto
// platform/identity/v1/identity.proto
enum HandoffApp { HANDOFF_APP_UNSPECIFIED = 0; HANDOFF_APP_BUYER = 1; HANDOFF_APP_SELLER = 2; }
service AuthService {
  // Authenticated user (not service). Code is a secret bearer credential: never logged, only its hash stored.
  // Single use, 60 s. INVALID_ARGUMENT for UNSPECIFIED target. PERMISSION_DENIED for a non-ACTIVE account.
  rpc CreateHandoffCode(CreateHandoffCodeRequest) returns (CreateHandoffCodeResponse);
  // Public (like RefreshSession, not retried). UNAUTHENTICATED with one fixed message for an unknown, used,
  // expired, wrong-target code or a revoked source session. Opens a new session for target_app.
  rpc RedeemHandoffCode(RedeemHandoffCodeRequest) returns (RedeemHandoffCodeResponse);
  // Public, read-only: does not consume the code. Same fixed UNAUTHENTICATED for any invalid code.
  // Lets the target app show "continue as <username>?" before a POST redeems.
  rpc PeekHandoffCode(PeekHandoffCodeRequest) returns (PeekHandoffCodeResponse);
  // Authenticated user. Grants role seller now. INVALID_ARGUMENT for an unknown terms_version.
  // PERMISSION_DENIED for a non-ACTIVE account. Idempotent for a seller (fresh token, no event).
  rpc BecomeSeller(BecomeSellerRequest) returns (BecomeSellerResponse);
}
message CreateHandoffCodeRequest { HandoffApp target_app = 1; }
message CreateHandoffCodeResponse { string code = 1; google.protobuf.Timestamp expires_at = 2; }
message RedeemHandoffCodeRequest { string code = 1; HandoffApp target_app = 2; }
message RedeemHandoffCodeResponse { AuthResult result = 1; }
message PeekHandoffCodeRequest { string code = 1; HandoffApp target_app = 2; }
message PeekHandoffCodeResponse { string username = 1; google.protobuf.Timestamp expires_at = 2; }
message BecomeSellerRequest { string terms_version = 1; }
message BecomeSellerResponse { AuthResult result = 1; }
```
```proto
// platform/identity/v1/identity.proto (next to UserStatusChanged) — identity.events
message UserRoleGranted { string user_id = 1; string role = 2; google.protobuf.Timestamp granted_at = 3; }
```
Existing contracts consumed unchanged: ListingService.GetListing, AuditService (team-audit append),
VoucherService.ValidateAndReserve/CommitReservation/ReleaseReservation, FlashSaleService.ReserveFlashSaleStock,
AuthService.RefreshSession, AccountAdminService.SetUserStatus.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contract for all of the above | p10-gaps-contract | platform-core | – | provides: VoucherService.DeactivateVoucher, VoucherService.UpdateVoucher, FlashSaleService.ListCampaigns, FlashSaleService.CancelCampaign, FlashSaleService.UpdateCampaign, AuthService.CreateHandoffCode, AuthService.RedeemHandoffCode, AuthService.PeekHandoffCode, AuthService.BecomeSeller, identity.events/UserRoleGranted | platform-core, consumers' vendored proto + generated |
| admin stops/adjusts vouchers, clear duplicate and outage errors (G9–G11) | promotion-admin-vouchers | team-promotion | p10-gaps-contract | uses: VoucherService.DeactivateVoucher, VoucherService.UpdateVoucher | team-promotion, team-gateway, team-frontend-admin, fe-platform/docs |
| admin manages all campaigns safely (G12–G14) | promotion-admin-campaigns | team-promotion | p10-gaps-contract | uses: FlashSaleService.ListCampaigns, FlashSaleService.CancelCampaign, FlashSaleService.UpdateCampaign, ListingService.GetListing | team-promotion, team-gateway, team-frontend-admin, team-frontend (flash-sale empty state) |
| one sign-in across buyer and seller apps (G8) | identity-sso-handoff | team-identity | p10-gaps-contract | uses: AuthService.CreateHandoffCode, AuthService.RedeemHandoffCode, AuthService.PeekHandoffCode | team-identity, team-gateway, fe-platform/packages/session, team-frontend, team-frontend-seller, platform-gitops (ingress log redaction) |
| buyer can become seller through identity (G15 backend) | identity-seller-onboarding | team-identity | p10-gaps-contract | uses: AuthService.BecomeSeller, identity.events/UserRoleGranted | team-identity, team-gateway |
| buyer becomes seller from the seller app (G15 UI) | seller-onboarding-ui | team-frontend-seller | identity-seller-onboarding | uses: AuthService.BecomeSeller | team-frontend-seller, platform-e2e, fe-platform/docs |
| gateway logs no raw principal id | gateway-principal-log-fingerprint | team-gateway | – | – | team-gateway, platform-core/packages/go-sdk/.golangci.yml, platform-gitops, docker-compose |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-gateway/internal/edge/promotion.go` and the gateway method-scope table — promotion-admin-vouchers, promotion-admin-campaigns
- `team-gateway/internal/edge/identity*.go`, gateway public-RPC list — identity-sso-handoff, identity-seller-onboarding
- `team-identity/internal/identity/auth.go` — identity-sso-handoff, identity-seller-onboarding
- `team-gateway/internal/edge/interceptors.go`, `http.go` — gateway-principal-log-fingerprint, and the two promotion slices (ErrorInfo pass-through)
- `fe-platform/docs/gaps.md` — every feature slice (each removes its rows)
- `team-frontend-seller/src/app/seller/layout.tsx` — identity-sso-handoff, identity-seller-onboarding

## Out of initiative
- Voucher discount shows "-" in e2e `test_cart_checkout`, `d2_cart_page`, `d2_checkout_flow`: diagnose root
  cause (test or app) and fix as a plain bug fix, no requirement change.
- `wait_streamed`: call once in `BasePage` after every navigation for all apps, delete scattered step calls,
  document why in platform-e2e docs (test-harness change only).
- `late payment succeeds` in old main specs: already corrected on main (verified 2026-11-02), nothing to do.

## NFR and data
- NFR: hand-off redeem p95 < 150 ms locally; code TTL exactly 60 s inclusive; campaign idempotency record kept 24 h.
- Personal data: hand-off code row (user id, source sid, target app, timestamps) — purpose sign-in hand-off,
  basis contract, retention: purged 24 h after expiry. Seller terms version + acceptance time — purpose proof
  of acceptance, basis contract, retention account lifetime. `principal_fp` in gateway logs — pseudonymous,
  existing log retention. Audit rows carry actor id as today.
- Risk routing expected: auth (G8, G15, gateway), pii (hand-off rows, logs), proto.

## Conventions
- Promotion admin writes mirror `CreateVoucher` audit-before-write in `team-promotion/internal/voucher` and
  `internal/rpc`; tests next to code; spec citations `team-<svc> internal/<pkg> TestName`.
- Gateway RPC exposure and scopes follow `team-gateway/internal/edge/promotion.go` and the identity public-RPC
  list used by RefreshSession.
- FE: admin screens in `team-frontend-admin/src/features/{vouchers,campaigns}`, platform components only,
  `npm run vendor` after fe-platform package changes.
- No comments in diffs; never log raw ids (sloglint forbidden-keys).

## Locked
2026-11-02 — Confirmed by Bùi Đăng Phúc. Six slices (contract, vouchers G9-G11, campaigns G12-G14, SSO G8, seller onboarding G15, gateway log fingerprint); assumptions locked: LISTING_UNAVAILABLE on listing outage, per-admin 24h idempotency, handoff via ?code= then redirect + no-referrer, new session per app, 24h purge, BecomeSeller same sid + UserRoleGranted + no audit, LOG_KEY_FP_SECRET (SDK LogKeyFP) reused, see Clarifications.

## Clarifications 2026-11-02 (spec review round 1, answered by the user)
- Vouchers: admin cannot set quota=0; a quota=0 voucher cannot get a finite quota; new quota >= used and >= current. Validation (incl. duplicate code, NOT_FOUND) runs before the audit append, so refused calls write no audit entry; quota check + update is one conditional UPDATE. Buyer app maps reason `inactive` to "Mã giảm giá đã ngừng áp dụng." (small team-frontend task in promotion-admin-vouchers).
- Campaigns: buyer flash-sale empty state dropped (FlashSaleBlock is a stub); idempotency lookup runs before validation and a hit returns the campaign's current state. team-analytics's vendored promotion.proto is a consumer too; copy checks glob all `team-*/proto/platform/promotion/v1`.
- SSO: login-CSRF mitigation = confirmation page. GET /auth/handoff never consumes; it calls PeekHandoffCode and shows "continue as <username>?"; only a POST redeems. Code minted on click, never at render. Cap 10 live (unredeemed, unexpired) codes per user, RESOURCE_EXHAUSTED on the 11th. Derived session carries the user's roles at redeem time and is independent of the source session after redeem. Audit down on redeem leaves the code usable within its TTL. TTL override honoured only when ENV=test. Ingress/Next logs redact `code` on /auth/handoff.
- Seller onboarding: only the current terms version is accepted; suspended/banned sessions are bulk-revoked, so BecomeSeller answers UNAUTHENTICATED and the buyer goes to the seller /login; the denied notice covers PERMISSION_DENIED (restricted after page load) only; an existing seller now non-ACTIVE gets PERMISSION_DENIED. Seller-app UI moves to new slice `seller-onboarding-ui` (owner team-frontend-seller), which also MODIFIES login-return and order-lifecycle-ui main specs that still assert "Cần tài khoản Người Bán".
- Gateway fingerprint secret: reuse `LOG_KEY_FP_SECRET` / sdkconfig.LogKeyFP.
- SSO redeem POST (spec review round 2, user decision): POST /auth/handoff is accepted only when `Origin` (or `Sec-Fetch-Site: same-origin`) equals the app's own origin, else 303 to /login with no redeem call; the confirmation page sends `Content-Security-Policy: frame-ancestors 'none'`, `X-Frame-Options: DENY`, `Cache-Control: no-store`, loads no external resources; the code travels in the POST body, never the action URL; cap-refused mints write no audit entry; the peeked username is HTML-escaped and truncated; peek also returns the fixed UNAUTHENTICATED for a non-ACTIVE account.
