# 00-spec — Privacy and audit hardening (Q1–Q6 follow-ups)

initiative: privacy-audit-hardening

## Goal
Personal data that Q1–Q6 started collecting stops leaking or living forever: session device/IP and refresh
hashes are purged on a schedule, cookies are Secure in deployed envs, admin URLs and login return URLs do not
reach referrers or GA4, and consumer logs do not print raw user ids. Audit and analytics tell the truth:
admin refunds and admin-created shop vouchers are traceable in team-audit, and hidden reviews stop counting
in analytics and recsys features.

## User stories
- Seller's old session: Given a session revoked or past its refresh expiry for more than the retention
  window, When the identity sweeper runs, Then its row (refresh hashes, device, IP) is gone; login history
  older than its retention is gone too. Edge: an active session is never touched.
- Buyer on a phone: Given a refresh or a "log out this device" action, Then identity sees the buyer's own
  IP/UA (not the frontend container's), like login does.
- Admin opens /admin/users?query=… or /admin/audit?target_id=…: Then no referrer, GA4 page_location or
  guest returnUrl carries that query.
- Deployed env over TLS terminated before the frontend: Then session, refresh and auth-event cookies are
  `Secure` even if `x-forwarded-proto` is missing.
- Ops reading team-domain/team-engagement consumer logs on a failing identity/engagement event: Then the log
  shows a key fingerprint, not the user id.
- Auditor: Given an admin refund, Then its audit row carries `refund_id`; Given an admin creating a SHOP
  voucher, Then one `voucher.admin_create` row exists (sellers creating their own shop vouchers stay unaudited).
- Analyst: Given an admin hides a review, Then team-analytics records `review_hidden` and the featurestore
  popularity/ALS datasets stop counting that review.

## Non-goals
- No DPO decisions (lawful basis, retention for audit rows, erasure of immutable audit copies) — human items.
- No change to Kafka message keys (ordering stays per user); only logging of keys changes.
- No new admin UI; no backfill of past analytics facts beyond what the consumer receives.
- Dispute events (`DisputeResolved`, ListOrderDisputes/ListDisputes) — owned by q6 engagement-dispute-escalation.

## Architecture decisions
- Session purge runs inside team-identity as a ticker addon next to the outbox relayer
  (`internal/bootstrap/lifecycle.go`), pattern of team-domain `ReservationSweeper` — Why: no new job infra.
- Secure cookies via a new server-only env `COOKIE_SECURE` (true → always Secure; unset → today's
  `x-forwarded-proto` rule) — Why: `NODE_ENV=production` is baked into every image incl. plain-http local.
- Referrer hygiene: global `Referrer-Policy: strict-origin-when-cross-origin` header in `next.config.mjs`,
  `no-referrer` metadata on `/login` and every `/admin/*` page, and a scrubbed `page_referrer` pushed with
  `page_location` in GTM — Why: GA4 reads `document.referrer` directly.
- Consumer logs print `key_fp` = first 12 hex of SHA-256(key) instead of the key — Why: keeps correlation, drops PII.
- `ReviewHidden` is a new outbox fact on `engagement.events`, keyed by listing id like `ReviewCreated`;
  analytics must deploy before engagement (unknown types are dead-lettered).

## Contracts
```proto
// platform/engagement/v1/events.proto — new event (additive)
message ReviewHidden {
  string review_id = 1;
  string listing_id = 2;
  google.protobuf.Timestamp hidden_at = 3;
}
```
- Audit catalogue (`platform/audit/v1/audit.proto` comment, both copies): add `voucher.admin_create`
  (17 → 18 actions). Wire format unchanged.
Existing contracts consumed unchanged: AuditService.WriteAuditEvent, AuthService.RefreshSession,
SessionService.RevokeSession, engagement.events envelope, identity.events.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| ReviewHidden event + voucher.admin_create catalogue entry | pah-contract | platform-core | – | provides: engagement.events/ReviewHidden, audit action voucher.admin_create | platform-core, team-engagement, team-analytics, team-promotion, team-audit (vendored protos) |
| Expired/revoked sessions and old login history are purged | identity-session-retention | team-identity | – | – | team-identity |
| Refresh and revoke carry the user's IP/UA | frontend-session-client-context | team-frontend | – | uses: AuthService.RefreshSession, SessionService.RevokeSession | team-frontend, team-identity |
| Cookies are Secure in deployed envs | frontend-cookie-secure | team-frontend | – | – | team-frontend, platform-gitops, docker-compose |
| Admin and login URLs do not leak via referrer/GA4/returnUrl | frontend-referrer-hygiene | team-frontend | frontend-cookie-secure | – | team-frontend |
| Consumer logs show a key fingerprint, not the user id | domain-consumer-key-redaction | team-domain | – | – | team-domain |
| Same for engagement consumers | engagement-consumer-key-redaction | team-engagement | – | – | team-engagement |
| Admin refund audit carries refund_id | payment-refund-audit-id | team-payment | – | uses: AuditService.WriteAuditEvent | team-payment |
| Admin-created shop vouchers are audited | promotion-shop-voucher-audit | team-promotion | pah-contract | uses: audit action voucher.admin_create | team-promotion |
| Analytics records review_hidden | analytics-review-hidden | team-analytics | pah-contract | uses: engagement.events/ReviewHidden | team-analytics |
| Hiding a review emits ReviewHidden | engagement-review-hidden-event | team-engagement | pah-contract, analytics-review-hidden, engagement-consumer-key-redaction | provides: emits ReviewHidden | team-engagement |
| Hidden reviews leave popularity/ALS features | featurestore-hidden-review-exclusion | platform-featurestore | analytics-review-hidden | – | platform-featurestore |

Hot files edited by more than one slice (serialised):
- team-frontend/src/middleware.ts, src/features/auth/actions.ts — frontend-session-client-context, frontend-cookie-secure
- team-frontend/src/lib/analytics/destinations/gtm.ts — frontend-referrer-hygiene only
- team-engagement/internal/consumer/consumer.go vs repository/review.go — distinct files, but same repo FEATURES.yaml

## Out of initiative
- C2 item "domain admin update audits an empty status": not reproducible on origin/main
  (`team-domain/internal/service/listing.go:193-222` returns before auditing on load errors). Dropped.

## NFR and data
- NFR: session sweeper runs every 1h, deletes in batches of 500, one batch per tx; must not lock active
  sessions; p99 RefreshSession latency unchanged (±5 ms).
- Personal data: sessions.device/ip/refresh_hash — purpose security; retention 30 days after revoke/refresh expiry (Q1).
  login_history.ip/user_agent — retention 180 days (Q2). refund_id — pseudonymous, audit purpose.
  review_id/listing_id in ReviewHidden — no user id.
- Risk routing expected: auth (session, cookies), pii (sessions, logs), proto (contract).

## Conventions
- Sweeper pattern: team-domain/internal/service/sweeper.go + repository purge interface; e2e via a short-retention
  compose override in platform-e2e/compose (like order-inventory.override.yaml).
- Frontend client context: reuse `clientContextFrom` + `authInterceptor({client})` (src/lib/gateway/auth.ts).
- Scenario titles prefixed per slice ("Session retention: …") for cross-change uniqueness.
- Every non-e2e scenario carries a `**VERIFIED BY**:` line.

## Clarifications — round 1 (human: "default", 2026-10-18)
- Q1 Session retention: → **delete session rows 30 days after revoke or refresh expiry** (default accepted)
- Q2 login_history retention: → **180 days** (default accepted)
- Q3 Refresh updates session ip/last_seen: → **yes, update ip/device and last_seen on rotation** (default accepted)
- Q4 COOKIE_SECURE: → **true in platform-gitops staging + prod frontend values, unset in local compose** (default accepted)
- Q5 Admin pages: → **strip all query params from page_location on /admin/*** (default accepted)
- Q6 Admin-created SHOP voucher: → **audit as voucher.admin_create** (default accepted)
- Q7 Consumer logs: → **SHA-256 fingerprint (12 hex) of the key** (default accepted)
- Q8 Hidden reviews in analytics: → **record review_hidden and exclude in featurestore SQL** (default accepted)

## Locked
2026-10-18 — Confirmed by Bùi Đăng Phúc ("default"). 12 slices; Q1–Q8 defaults: 30-day session purge, 180-day login history, refresh updates ip/last_seen, COOKIE_SECURE in staging/prod, strip admin query from page_location, voucher.admin_create audit, key fingerprint in logs, review_hidden fact + featurestore exclusion.
