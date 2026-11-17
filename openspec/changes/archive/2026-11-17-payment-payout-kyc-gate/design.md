## Context
`RequestPayout` and `RequestWalletPayout` in `team-payment/internal/rpc` call `sellerAccess`, `requirePayoutScope`, validate amount, then the payment service. team-payment already dials team-order with `sdkupstream.NewClient(addr, sdkupstream.AsService(id, scope))` (`internal/upstream/order.go`) and requires `UPSTREAM_ORDER_ADDR`.

## Decisions
- Gate placement is the rpc handler, after all request validation and before `h.svc`. Why: both RPCs share one gate function, no ledger code runs on refusal, invalid requests never pay the network cost.
- `PayoutGate` interface in `internal/rpc` with `Check(ctx, sellerID) error`; the real implementation in `internal/upstream/verification.go` wraps `VerificationServiceClient` with a 2 s context deadline and no retry. Tests use a real in-process gRPC verification server like `audit_test.go`.
- Error mapping: status `VERIFIED` pass; any other enum value `FAILED_PRECONDITION` "seller verification required"; transport error, deadline, `Unauthenticated`, `PermissionDenied`, `NotFound` or any non-OK from team-verification `UNAVAILABLE` "verification unavailable". Why: a misconfigured scope must fail closed, not open, and must not tell the seller they are unverified.
- Service principal `service-team-payment` with scope `verification.read`, a second principal on a second connection (the order connection carries `order.read`).
- No cache: money needs the fresh status (initiative decision). Concurrent payouts each check; the existing seller-locked ledger step still prevents overdraw.
- Sellers verified before rollout through the same path: no grace flag (initiative decision), no `*_ENABLED` switch.
- Seed order: register seller, `CreateKycUploadUrl`, presigned PUT of a small PNG, `SubmitKyc`, admin `ReviewKyc` approve, all through the gateway. The shared seed and e2e payout steps are hot files serialised with slice seller-kyc-ui.

## Risks
- team-verification down blocks all payouts: accepted by the initiative (fail closed).
- `verification.proto` is not in team-payment's `proto/` tree: see open issue in tasks 1.1.
