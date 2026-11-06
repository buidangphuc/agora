## Why

Today `team-referral` credits the inviter 10,000 immediately when the invitee types a code
(`Redeem` writes the referral row and then, in a separate call, the reward row: not atomic, no status, no cap, no
check that the invitee ever buys anything). The only way to be referred is to type a code by hand, so a friend who
opens an invite link is never attributed. The service has no Kafka or outbox, so signup cannot announce itself to it
and promotion cannot learn the verdict.

Initiative `q7-q37-referral-welcome` (user stories 2, 3, 8; decisions D1, D2, D4) fixes this: attribution arrives
asynchronously from `identity.events/UserRegistered`, the inviter reward is `PENDING` when the invitee's first order
completes and is released only after that order's return window ends with no return, and rewards are capped per
inviter per day and over a lifetime.

## What Changes

initiative: q7-q37-referral-welcome

- team-referral adopts go-sdk v1.0.3 `outbox` and `kafkax` (D2): migration `002_*` adds a `referral_outbox_events`
  table, a processed-events table, state columns and the account/order facts the service needs.
- Consumer of `identity.events/UserRegistered`: stores `registered_at` for every new user; when `referral_code` is
  non-empty, creates the referral (`ATTRIBUTED`) and emits `referral.events/ReferralAttributed`, or emits
  `ReferralRejected{UNKNOWN_CODE | SELF_REFERRAL | REFERRER_CAPPED}`. Registration never depends on the outcome.
- Consumer of `order.events/OrderCompleted`: records that the buyer has a completed order, and for a buyer whose
  referral is `ATTRIBUTED` creates a `PENDING` reward of `REFERRAL_REWARD_XU` linked to the order with
  `release_at = return_window_ends_at`, moving the referral to `QUALIFIED` in the same transaction.
- Consumer of `order.events/ReturnRequested`: a `PENDING` reward of that order becomes `VOID` and the referral goes
  back to `ATTRIBUTED`.
- Sweeper (`REFERRAL_RELEASE_SWEEP_INTERVAL`, default 1m): releases due `PENDING` rewards (`RELEASED`, referral
  `REWARDED`), emits `referral.events/ReferralRewardReleased`, honouring a daily cap of 5 (day in
  `Asia/Ho_Chi_Minh`) and a lifetime cap of 50 per inviter.
- `RedeemReferral` stays but no longer credits anything: it attributes exactly like the consumer, only for an
  account younger than `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` (default 168h) that has no completed order.
- `GetMyReferral` gains `pending_xu` and `released_xu`; `ListReferralRewards` rows gain `status` and `release_at`.
- Deploy wiring: `docker-compose.services.yaml` (Kafka env, `redpanda-init` depends_on, new topics) and
  `platform-gitops/envs/services/team-referral.yaml` plus the staging/prod overlays.
- Existing scenarios `Redeeming credits the referrer` and `Self-referral and a second redeem are refused` change
  (FEATURES ids `referral-runtime.redeeming-credits-the-referrer` is removed; the self-referral one is re-worded).

### Decision: how referral knows account age and completed orders

Referral derives both from events it already consumes, with no new RPC to identity or order:
account age from `UserRegistered.registered_at` (stored for every user, with or without a code) and "has a completed
order" from `OrderCompleted` (stored per buyer). A user with no stored registration (registered before this feature
deployed) is treated as not eligible for manual redeem. `OrderService.HasCompletedOrder` is not used here (it is
the promotion slice's tool).

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `referral-service`: settings gain Kafka/outbox/referral knobs; redeem no longer credits and gets an account
  gate; adds attribution consumer, reward state machine, sweeper, caps, referral events and the xu summary.

## Impact

- `team-referral/internal/{app,config,referral,rpc,store,events,consumer}`, `team-referral/migrations/002_*`,
  `team-referral/.env.example`, `team-referral/go.mod` (go-sdk v1.0.3 already), regenerated stubs for
  `referral.proto`, `identity.proto` (UserRegistered) and `order.proto` (existing events).
- `docker-compose.services.yaml`, `platform-gitops/envs/services/team-referral.yaml`,
  `platform-gitops/envs/{staging,prod}/services/team-referral.yaml` and `platform-gitops/envs/local/values.yaml` if it carries per-service env.
- Depends on: `q7-referral-welcome-contract`, `identity-user-registered`. Consumed by `promotion-new-user-vouchers`
  and `frontend-referral-invite`.
- No gateway route change (`ReferralService` stays routed as is).

## Non-goals

Phone OTP, device/IP fingerprints, risk scores; spending xu or unifying with check-in coins; real money (xu are a
mock ledger); admin screens; notifications for released rewards; any change to team-order, team-identity or
team-promotion.

## Non-functional requirements

- A pending reward is released within 2 x `REFERRAL_RELEASE_SWEEP_INTERVAL` after `release_at` when no cap blocks it.
- Consumers are idempotent on event id: a redelivered event changes no row and emits no second event.
- Referral row, reward row, state change and outbox row commit in one transaction; a crash between them leaves none.
- Referral and verdict events appear on `referral.events` within the outbox poll interval (default 1s) plus the
  consumer lag; a poison record goes to a DLQ topic and does not block the partition.
- `RedeemReferral` and `GetMyReferral` p99 latency stays within +10% of the pre-change value (one extra indexed read each).
- Logs carry `principal_fp` / `key_fp` only; no raw user id, referral code or order id (sloglint forbidden keys).

## Data classification

| Field | Purpose | Basis | Retention |
|---|---|---|---|
| `referral_code` (in `UserRegistered`, in `referrals.code`) | attribute the signup to the inviter | contract (referral programme terms) | kept with the referral row |
| `referrer_id`, `referee_id` (tables, `referral.events`, Kafka key = referee id) | reward accounting and voucher verdict | contract | tables: life of the account; topic: `referral.events` retention is the broker default (7 days on the local stack; staging/prod set by platform infra) |
| `registered_at` per user (`referral_accounts`) | account-age gate for manual redeem | legitimate interest, fraud limiting | life of the account |
| buyer id with first completed-order time (`referral_buyer_orders`) | "has completed order" gate and first-order qualification | legitimate interest, fraud limiting | life of the account |
| `order_id` on a reward | link reward to its order for void on return | contract | with the reward row |
| processed event ids | consumer idempotency | n/a (no personal data) | purged after 7 days |

DPO sign-off is tracked as NEEDS_HUMAN at initiative level. Erasure of these rows on account deletion stays NEEDS_HUMAN (no account-deletion event exists).

## Threat model

- Assets: the xu ledger (mock), referral attribution, per-inviter caps.
- Abuse cases:
  - Self-referral and referral rings: self-referral refused in both paths; per-inviter daily and lifetime caps bound the loss.
  - Reward after a refund: reward stays `PENDING` through the return window and is voided by `ReturnRequested`.
  - Late code entry by an established account: refused by the account-age and completed-order gate.
  - Replay or duplicate events: idempotent event table, `UNIQUE(referee_id)`, `UNIQUE(order_id)`.
  - Forged identity in `RedeemReferral`: caller id comes only from the gateway principal, never from the body.
  - Race of two consumers or two sweepers on one inviter: the inviter's code row is locked `FOR UPDATE` while caps are checked, so the caps cannot be exceeded.
  - Log leakage of ids and codes: only fingerprints are logged.
- Consumers expose no RPC; the only identity they use is the event's `user_id`. Permission scenarios therefore do not apply to them, and the remaining spec_lint PERM warnings for consumer, relayer and sweeper requirements are accepted.
- Authz rule: `RedeemReferral`, `GetMyReferral`, `ListReferralRewards` act only on the caller (unchanged). Consumers and
  the sweeper have no RPC surface; `referral.events` is written only through the outbox.
