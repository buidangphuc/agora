## 1. Code track

Prerequisite: `trust-legal-contract` merged and each repo's generated proto regenerated from `platform-core/packages/proto` (no hand edits).

- [x] 1.1 Tests first, team-order: `internal/rpc` tests for `GetUserObligations` (OBL-1..OBL-9, OBL-11) and a Docker-backed store test for the counts (OBL-1..OBL-5), run with `GO_SDK_REQUIRE_DOCKER=1`; verify red.
- [x] 1.2 team-order: `GetUserObligations` handler (SERVICE principal + `order.internal.read` via the existing `isServiceWithScope` helper, `INVALID_ARGUMENT` on empty id), plus the store query with statuses PENDING/PAID/SHIPPED and returns PENDING/APPROVED for buyer or seller (OBL-1..OBL-9, OBL-11).
- [x] 1.3 Tests first, team-order erasure: Docker-backed tests for the `UserErased` handler (ORD-1..ORD-8), including rollback on injected failure and DLQ on malformed input; verify red.
- [x] 1.4 team-order: `internal/events` erasure consumer mirroring `team-domain/internal/events/identity.go`; store method `EraseUser` masking `shipping_address` fields via `jsonb_set` for `buyer_id = user`, `order_returns.reason`, deleting `cart_items`, in one transaction; config `ORDER_ERASURE_CONSUMER_GROUP` (default `team-order.erasure`) and `ORDER_ERASURE_DLQ_TOPIC` (default `identity.events.order.dlq`); wired in `internal/app` (ORD-1..ORD-9).
- [x] 1.5 Tests first, team-payment: `internal/rpc` tests for `GetPayoutObligations` (PAY-1..PAY-9, PAY-11) and a Docker-backed store test (PAY-1..PAY-5); verify red.
- [x] 1.6 team-payment: `GetPayoutObligations` handler with `payment.internal.read`, reusing the existing balance and hold-back split (`computeWithdrawal`) and counting `payout_requests` in PENDING/PROCESSING (PAY-1..PAY-9, PAY-11).
- [x] 1.7 Tests first, team-payment erasure: Docker-backed tests (PER-1..PER-7); verify red.
- [x] 1.8 team-payment: new consumer on `identity.events` beside the settlement consumer in `internal/app/app.go`, store method masking `account_number`/`account_name` where `seller_id = user`, config `PAYMENT_ERASURE_CONSUMER_GROUP` (default `team-payment.erasure`) and `PAYMENT_ERASURE_DLQ_TOPIC` (default `identity.events.payment.dlq`) (PER-1..PER-8).
- [x] 1.9 Tests first, team-promotion: Docker-backed tests (PRO-1..PRO-7, PRO-9); verify red.
- [x] 1.10 team-promotion: erasure handler in `internal/consumer` on its own group `team-promotion.erasure` reusing `identity.events.promotion.dlq` (the existing identity consumer's DLQ), store method deactivating owned vouchers and deleting `new_user_voucher_state` in one transaction; wired in `internal/app/app.go` (PRO-1..PRO-9).
- [x] 1.11 Tests first, team-referral: Docker-backed tests (REF-1..REF-8); verify red.
- [x] 1.12 team-referral: erasure handler in `internal/consumer`, own group `REFERRAL_ERASURE_CONSUMER_GROUP` (default `team-referral-erasure`) and `REFERRAL_ERASURE_DLQ_TOPIC` (default `identity.events.referral.dlq`), store method deleting the three row sets in one transaction; make the existing `UserRegistered` consumer skip other types (REF-1..REF-8).
- [x] 1.13 team-gateway test: `OrderService/GetUserObligations` and `PaymentService/GetPayoutObligations` answer 501 with and without a token (OBL-10, PAY-10); no routing code added.
- [x] 1.14 `docker-compose.services.yaml`: add `identity.events.order.dlq identity.events.payment.dlq identity.events.referral.dlq` to the topic init (line 26) and the erasure consumer env to the four services; update each service README contract table and `FEATURES.yaml` is left to the e2e author.
- [x] 1.15 Logging check: every new handler logs counts and hashed ids only (a test over captured log output asserts no raw user id, name, phone or account number) (ORD-1, PER-1).
- [x] 1.16 Gates through `scripts/asdlc/gate_digest.py` for team-order, team-payment, team-promotion, team-referral; `openspec validate commerce-user-erasure --strict`.

## 2. E2E track

The e2e author adds `FEATURES.yaml` entries and the `VERIFIED BY` lines.

- [x] 2.1 Feature `platform-e2e/tests/e2e/features/order/user_obligations.feature`: internal call with a service principal returns counts for a seeded buyer with open orders and a return, zeros after they close; USER and wrong-scope callers refused (OBL-1, OBL-2, OBL-3, OBL-7, OBL-8, OBL-9).
- [x] 2.2 Feature `platform-e2e/tests/e2e/features/payment/payout_obligations.feature`: seeded seller with a credit, a held amount and a pending payout; refused callers (PAY-1, PAY-2, PAY-7, PAY-8, PAY-9).
- [x] 2.3 Scenarios through the gateway that both RPCs answer 501 (OBL-10, PAY-10).
- [x] 2.4 Feature `platform-e2e/tests/e2e/features/order/user_erasure.feature`: seed a buyer with an order, return and cart, produce a `UserErased` event to `identity.events` with the existing Kafka test helper, assert the order reads "Đã xóa" recipient fields with unchanged totals and the cart is empty; second delivery changes nothing; another buyer untouched; empty `user_id` lands in `identity.events.order.dlq` (ORD-1, ORD-2, ORD-3, ORD-6, ORD-9).
- [x] 2.5 Feature `platform-e2e/tests/e2e/features/payment/user_erasure.feature`: seeded seller with a payout, same event, payout history shows "Đã xóa" with the amount unchanged; other seller untouched (PER-1, PER-2, PER-3, PER-8).
- [x] 2.6 Feature `platform-e2e/tests/e2e/features/promotion/user_erasure.feature`: seeded welcome voucher deactivated and unredeemable after the event (PRO-1, PRO-8).
- [x] 2.7 Feature `platform-e2e/tests/e2e/features/referral/user_erasure.feature`: referrer's code no longer resolves after the event; reward ledger unchanged (REF-1, REF-2).
- [x] 2.8 Local cluster check: after one `UserErased` for a user with data in all four services, every consumer finishes within 1 h (in practice seconds) and the four DLQs are empty (ORD-3, PER-3, PRO-3, REF-4).
