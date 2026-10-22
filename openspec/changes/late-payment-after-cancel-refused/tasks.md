## E2E track (platform-e2e)

- [x] 1. The scenarios `A late payment after cancel is ignored`, `A late payment of a cancelled voucher order leaves the voucher unused` (order/oic_lifecycle_guards.feature), `A late payment for a cancelled order does not credit the seller` (payment/plp_settlement_credit.feature) and `Refunding a payment whose order was cancelled deducts nothing` (payment/plp_refund_deduction.feature) already assert the refusal (PR #44); align the matching FEATURES.yaml acceptance lines.
