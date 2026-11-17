## Why
Nothing in team-payment consults KYC today: `RequestPayout` and `RequestWalletPayout` check only seller ownership and the `listing.write` scope (`team-payment/internal/rpc/payment.go`, `wallet_ledger.go`). Any seller can withdraw money without identity verification. The trust-legal initiative requires that no payout leaves the platform for a seller who is not `VERIFIED` (backlog Q28 phase 1). This slice adds the gate in team-payment and makes the demo and e2e sellers verified through the real KYC RPCs so existing payout journeys keep working.

initiative: trust-legal

## What changes
- team-payment calls `VerificationService.GetVerificationStatus` (service principal `service-team-payment`, scope `verification.read`, one attempt, 2 s deadline) on both `RequestPayout` and `RequestWalletPayout`, after authz, scope and input validation and before any service or ledger call.
- Only status `VERIFIED` passes. Any other status (`PENDING`, `REJECTED`, `NOT_SUBMITTED`, `UNSPECIFIED`) fails `FAILED_PRECONDITION` "seller verification required". Any call failure or deadline fails `UNAVAILABLE` (fail closed). No ledger row, payout row or balance change on either failure.
- New required config `UPSTREAM_VERIFICATION_ADDR` (like `UPSTREAM_ORDER_ADDR`): boot fails without it. Compose sets `team-verification-svc:50064` and a `depends_on` for team-verification.
- A new upstream adapter `internal/upstream/verification.go` and a small `PayoutGate` interface in the rpc layer so tests use a real in-process gRPC server (no self-mock).
- Seed (platform-core, e2e helpers): demo and e2e sellers are verified through the real RPCs (`CreateKycUploadUrl`, presigned PUT, `SubmitKyc`, admin `ReviewKyc` approve). No fake containers, no route fulfils, no grace period.
- Docs: `team-payment/README.md` contract table and FEATURES.yaml notes (the latter by the e2e-author).

Out of scope: gating listing creation, public badge, KYC upload/queue UI, `GetPayoutObligations` (slice commerce-user-erasure), bank-account holder name matching (deferred), JWT claims as a status source, caching of verification status.

## NFR
- Latency: the gate adds at most 2 s worst case (single attempt, deadline 2 s); healthy path p95 added latency at most 50 ms locally.
- Availability: payouts depend on team-verification; when it is down payouts are refused with `UNAVAILABLE`. Reads, wallet, ledger and every other RPC are unaffected.
- Correctness: status is read fresh on every payout call; no cache. A status change between the check and the ledger write is accepted (window bounded by the 2 s deadline); a revoked verification stops the next request.
- No new dependency; boot refuses a missing `UPSTREAM_VERIFICATION_ADDR`.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| seller user id sent to team-verification | pseudonymous id | read the KYC status for the payout decision | legal obligation (platform duty to identify sellers, Decree 85/2021) | not stored by team-payment |
| verification status enum received | personal (derived from KYC) | allow or refuse the payout | same | held in memory for one request, never stored or logged |
| payout bank details (existing) | personal, financial | execute payout | contract | unchanged; masked at erasure by commerce-user-erasure |

team-payment never receives KYC documents or reason codes: it reads only `status`. Logs carry the outcome (`allowed`, `denied`, `unavailable`) and counts only, never the user id.

## Threat model
- Assets: seller wallet funds; the KYC status as the gate for moving money.
- Abuse cases: (1) unverified or rejected seller withdraws funds; (2) caller forges the status in the request; (3) verification outage turns into fail-open; (4) a buyer or other seller requests a payout against someone else's wallet; (5) status read leaks identity data.
- Authz rule: existing `sellerAccess` (own wallet, USER principal) and `listing.write` stay first; the gate is an additional condition. The status always comes from team-verification keyed by the principal id, never from the request or a token claim. team-payment holds only scope `verification.read`; team-verification ignores any other use of that scope.
- Residual risk: demo sellers are verified by seed through real RPCs, which needs an admin credential in the local stack only.
