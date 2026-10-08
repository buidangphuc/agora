## Why

`full_team_repo` was retired on 2026-10-08. A per-repo audit (archived at
`~/Documents/agora-archive/full_team_repo/port-audit/`) found security behaviour there that agora never
received. Some of these gaps are exploitable through the public edge today:
- the gateway forwards internal-only RPCs (`ReserveStock`, `ReleaseStock`, `CommitReservation`,
  `ReleaseReservation`, `WriteAuditEvent`) to browsers;
- any caller can commit or release a voucher reservation, or spoof `buyer_id`;
- any authenticated user can read any dispute;
- drafts are visible to strangers in listing reads and in search;
- `ProcessMockPayment` cannot be switched off;
- several services echo raw driver errors to clients.

This change ports that behaviour to agora's current code and makes it verifiable through the edge, as the first wave
of the port (security first, then order/inventory/payment correctness, then AI-first).

## What Changes

- **team-gateway**:
  - Stop routing the internal-only RPCs (501 for every caller).
  - Gate `ReviewKyc`, `ResolveDispute` and `QueryAuditLog` on the `admin` scope at the edge.
  - Sanitise upstream internal and unavailable errors.
  - Require `exp` and a non-empty `sub` in JWTs.
  - Validate `X-Request-Id` and `Idempotency-Key`, and forward the key to team-order.
- **team-audit**: `WriteAuditEvent` is service-only (SERVICE principal with `audit.write`).
- **team-promotion**:
  - The saga RPCs are service-only (`promotion.reserve`).
  - `ValidateAndReserve` from a user is a preview bound to the caller (`buyer_id` forced, `preview:<id>:` namespace).
  - `Subscribe` and `CreateAdCampaign` need `listing.write`, and a campaign needs listing ownership.
  - `GetEntitlements` is service-only.
  - Ad bid and budget are capped.
  - The team-domain lookup is bounded by a timeout.
- **team-order**:
  - Upstream calls carry least-privilege, per-call scopes.
  - The voucher saga runs as `service-team-order` with `promotion.reserve`.
  - A buyer cannot buy their own listing.
- **team-engagement**:
  - `CreateDispute` checks the order's buyer and seller.
  - `GetDispute` is limited to its parties or an admin.
  - `is_shop_reply` is honoured only for the listing owner.
  - `ResolveDispute` is admin-only.
  - The order lookup is bounded by a timeout.
- **team-domain**: draft and rejected listings are hidden from non-owners, and `ListListings` defaults to published.
- **team-search**:
  - Results are published-only by default.
  - Drafts can be listed only by their own seller.
  - Saved searches require a real user.
- **team-payment**: `ProcessMockPayment` only works when `MOCK_PAYMENTS=true`.
- **team-identity**:
  - Buyer, seller and admin tokens carry `recommendations:read` and `ai:use`.
  - Login attempts are recorded in login history.
  - Password-reset issuance is logged with a token fingerprint.
- **team-ai**:
  - Scope gates on `MagicListing`, `ChatCopilot` and `SummarizeReviews`, with a seller binding on `ChatCopilot`.
  - `Recommend` is bound to the caller.
  - The static bearer fallback and the gRPC rate limiter are opt-in.
  - Fixes the quota upsert `"limit"` column bug.
- **All services** (audit, chat, notification, sharing, verification, referral, domain, order, payment, engagement,
  identity, promotion, ai): internal errors return a generic message and log the cause.
- **Staging/prod boot guards**:
  - team-payment refuses `MOCK_PAYMENTS`.
  - team-identity refuses the committed dev signing key.
  - team-order and team-search refuse in-memory storage.
  - team-domain refuses disabled Kafka/outbox or default storage keys.
  - platform-gitops sets `ENV` in the staging and prod overlays.

Services touched: team-gateway, team-audit, team-promotion, team-order, team-engagement, team-domain, team-search,
team-payment, team-identity, team-ai, team-chat, team-notification, team-sharing, team-verification, team-referral,
team-frontend (removal of dead `reserveStock`/`releaseStock` helpers), platform-gitops, platform-e2e, root compose.
No proto change.

## Capabilities

### New Capabilities
- `edge-route-policy`: what the gateway refuses to route, gates on admin, and how it reports upstream failures and
  validates tokens and request headers.
- `audit-access-control`: who may write and read audit events.
- `promotion-access-control`: voucher saga, preview, subscription, sponsored ads and entitlements access.
- `order-upstream-principals`: what identity team-order presents upstream, and the self-purchase rule.
- `dispute-and-qa-access`: dispute creation, reading and resolution, and shop-reply attribution.
- `listing-read-visibility`: draft and rejected listings in listing reads and in search.
- `payment-access-control`: availability of the mock payment path.
- `ai-access-control`: scope gates and subject binding on AI and recommendation RPCs.

### Modified Capabilities
- `auth`: role scope grants for AI and recommendations, and login history recording.
- `deploy-runtime`: services refuse unsafe configuration when `ENV` is staging or production.

## Non-goals

- Order, inventory, payment and search correctness: inventory commit, idempotent release, atomic placement, status
  transition table, settlement and refund ledger, stock projection, tombstones. That is the next port wave
  (`inventory-commit-and-idempotent-release`, `order-domain-correctness`, `order-integrity-guards`,
  `payment-payout-holdback-and-settled-consumer`, `search-stock-events`, `search-correctness-and-privacy`).
- Enforcing `ai:use` on `ShoppingAssistant`/`StreamChat`. The grant is added, but enforcement is behind
  `AI_USE_SCOPE_REQUIRED` (default off), because turning it on blocks anonymous `/assistant` users. That is a
  product decision.
- The AI-first tracking, feature store and recsys v2 work.
- Gateway streaming interceptors, upstream resilience and observability from the old gateway.

## Relationship to the carried-over changes

This change implements, on agora's code, the parts of `service-authz-hardening`, `authz-hardening-wave2` and
`gateway-and-ai-hardening` listed above. Those three changes stay active for their remaining requirements. When
they are rebased onto agora, they must drop the requirements this change archives.

## Impact

- **New env vars:**
  - team-payment: `MOCK_PAYMENTS`
  - team-promotion: `MAX_AD_BID`, `MAX_AD_BUDGET` and `UPSTREAM_CALL_TIMEOUT_SECONDS`
  - team-engagement: `UPSTREAM_CALL_TIMEOUT_SECONDS`
  - team-ai: `AI_USE_SCOPE_REQUIRED`, `GRPC_BEARER_FALLBACK_ENABLED` and `GRPC_RATE_LIMIT_ENABLED`
- **Config:**
  - Local compose and the local kind app set `MOCK_PAYMENTS=true`.
  - The staging and prod overlays set `ENV`.
- **Re-login:** tokens minted before this change lack `ai:use` and `recommendations:read` until users log in again.
