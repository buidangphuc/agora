## Summary

Q7 (referral attribution + two-sided capped reward) and Q37 (new-user welcome voucher + 3-step checklist), built as 7 OpenSpec changes from `plans/q7-q37-referral-welcome/` (LOCK approved 2026-11-06).

```mermaid
sequenceDiagram
    participant FE as team-frontend
    participant ID as team-identity
    participant RF as team-referral
    participant OR as team-order
    participant PR as team-promotion
    FE->>ID: Register(referral_code from ?ref / agora_ref cookie)
    ID-->>RF: identity.events UserRegistered (outbox, same tx)
    ID-->>PR: identity.events UserRegistered
    RF-->>PR: referral.events ReferralAttributed | ReferralRejected
    PR->>PR: issue ONE NEW_USER voucher (invited 30k / welcome 20k / welcome after 30 min)
    OR-->>RF: order.events OrderCompleted (first order) → PENDING 10,000 xu
    RF->>RF: sweeper releases at return_window_ends_at (5/day VN, 50 lifetime); ReturnRequested → VOID
    PR->>OR: HasCompletedOrder (new_user_only, service principal, fails closed)
```

```text
contract      platform-core protos + 46 consumer copies (GetMyAccount, UserRegistered, referral events, ListMyVouchers, HasCompletedOrder)
team-identity UserRegistered outbox event, referral_code normalisation, GetMyAccount
team-order    HasCompletedOrder (service-only, unrouted at the gateway)
team-referral consumers, outbox relay, PENDING→RELEASED|VOID sweeper with caps, Redeem without credit, migration 002
team-promotion NEW_USER vouchers, owner/per_user_limit/new_user_only, hold expires_at, ListMyVouchers, migration 0008
team-gateway  GetMyAccount, ListMyVouchers forwarders (any signed-in principal)
team-frontend invite link + /s/[code] route + agora_ref cookie, xu totals, "Voucher của bạn", WelcomePanel + checklist
deploy        compose/redpanda-init topics + DLQs, gitops env + topic Job (7-day DLQ retention), team-order allowFrom team-promotion
```

## Evidence

- **Before:** e2e for the seven changes red (features missing); first stack run 68 passed / 20 failed, including a real bug — `/s/[code]` redirected to `http://0.0.0.0:3000/...` inside the container (fixed by a relative `Location`).
  **After (local mainx stack, 2026-11-06):**

  | e2e file | result |
  |---|---|
  | test_identity_q7 | 23/23 (Register p95 main ≈97 ms → new ≈95 ms, NFR +10 ms holds) |
  | test_order_has_completed_order | 13/13 |
  | test_referral_invite | 24/24 (S17/S18 with `referral-reward` overlay) |
  | test_welcome_checklist | 17/17 |
  | test_new_user_vouchers | 11/11 (`new-user-voucher` overlay) |
  | test_referral_reward | 6/6 ×2 (`referral-reward` overlay) |

- Unit/integration: `go vet && go test` incl. Postgres + Redpanda testcontainers in identity, order, referral, promotion, gateway edge; `make lint` 0; `npm run check` (2102 tests) + `npm run build`; gitops unittest 104 OK; `red_first` PASS per wave; `ci_local specs features repo` PASS; `spec_sync --strict` PASS for all 7 changes.
- Reviews: task-reviewer, contract-boundary and auth-scope per slice (decide.py unavailable → reviewer checklists); one BLOCKING (missing gitops DLQs) fixed.
- Not run: full regression e2e suite, feature-reviewer, privacy-reviewer, docs-sync.

## Merge Danger

**Door:** one-way

Migrations `team-referral 002` (backfills legacy rewards to RELEASED, referrals to REWARDED) and `team-promotion 0008` (new voucher columns, hold `expires_at`) change persisted data; new events start flowing on `identity.events` / `referral.events`. Redeem no longer credits the referrer directly.

**Blast Radius:** cross-service

- Signup, checkout voucher reservation (new owner / per-user / new_user_only checks, hold TTL) and the referral page all change.
- NEEDS_HUMAN before deploy: Vault service credential for team-promotion (`order.read`), DPO sign-off (Decree 13: referral_code, referrer/referee ids, `agora_ref` cookie, `owner_user_id`), topic retention for `identity.events` / `referral.events`.
- Open review notes: DLQ names `referral.{identity,order}.dlq` vs `<source>.<consumer>.dlq`; a later `UserRegistered` resets `registered_at`; Redeem does not lock the referee row; `UpdateVoucher` two sequential writes; a reward is lost if `OrderCompleted` is consumed before `UserRegistered`.
- Archive order after merge = build order in `LOCK.md`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
