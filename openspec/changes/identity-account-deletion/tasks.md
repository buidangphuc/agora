## 1. Code track (team-identity, team-gateway)

- [x] 1.1 Migration: `deletion_requested_at`, `erase_after`, extend `users_status_check` with `deleted`; down migration; PG test (deletion-status scenarios, erasure scenarios)
- [x] 1.2 Config: `ACCOUNT_DELETION_CANCEL_WINDOW`, `ACCOUNT_ERASURE_SWEEP_INTERVAL`, `UPSTREAM_ORDER_ADDR`, `UPSTREAM_PAYMENT_ADDR`; boot fails when missing (Request: obligations unreachable scenarios)
- [x] 1.3 Upstream clients for `GetUserObligations` and `GetPayoutObligations` with service principal, 2 s timeout, single attempt (Obligations scenarios: unreachable, zero, open order, balance, pending payout)
- [ ] 1.4 `RequestAccountDeletion` handler: principal check, password check like `ChangePassword`, obligations, schedule plus revoke other sessions in one transaction (Request scenarios, Scheduling revokes other sessions scenarios)
- [x] 1.5 `CancelAccountDeletion` and `GetAccountDeletionStatus` handlers (Cancel and Status scenarios)
- [x] 1.6 Store eraser transaction with `FOR UPDATE SKIP LOCKED` and `UserErased` outbox write; sweeper wired in `cmd` (Sweeper scenarios: due, same transaction, proof survives, not due, cancelled, idempotent, 72 h bound, one failing)
- [x] 1.7 Refuse `deleted` in Login, Refresh, handoff exchange; empty scopes for `deleted` (Deleted accounts scenarios)
- [x] 1.8 Log hygiene on deletion paths with sloglint keys (Logs scenario)
- [x] 1.9 team-gateway: route policy and forwarder for the three RPCs, 501 for obligation RPCs (Gateway scenarios)
- [x] 1.10 Gates via `scripts/asdlc/gate_digest.py` with `GO_SDK_REQUIRE_DOCKER=1`; README and FEATURES.yaml note left to the e2e-author

## 2. E2E track (platform-e2e)

- [x] 2.1 Feature: buyer schedules, sees status, cancels (Request: no obligations; Cancel within window; Status scenarios)
- [x] 2.2 Feature: wrong password and anonymous caller (Request: wrong password, anonymous; Gateway: anonymous)
- [x] 2.3 Feature: buyer with an open order and seller with wallet balance are blocked with named reasons (Obligations scenarios)
- [ ] 2.4 Feature: other sessions revoked, current kept (Scheduling scenarios)
- [x] 2.5 Feature: erasure after `erase_after` using a test-only short cancel window env in the e2e overlay; identity data gone, `UserErased` on `identity.events`, login refused, proof retained (Sweeper: due, UserErased, proof survives; Deleted accounts: login fails)
- [x] 2.6 Feature: obligation RPCs answer 501 at the gateway (Gateway: internal obligation RPCs)
