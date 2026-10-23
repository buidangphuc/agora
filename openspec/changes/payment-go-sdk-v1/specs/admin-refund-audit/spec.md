## MODIFIED Requirements

### Requirement: Admin refunds fail closed when audit is unavailable

If the audit event cannot be written (team-audit returns an error or times out after 2 s), an admin `RefundPayment` SHALL return `Unavailable` with message `audit trail unavailable` and the payment status, refund rows and ledger SHALL be unchanged. `UPSTREAM_AUDIT_ADDR` SHALL be required in every ENV and an empty one SHALL refuse startup; there is no mode without an audit writer.

#### Scenario: Audit error leaves the ledger unchanged
- **WHEN** team-audit returns an error during an admin refund of a PAID payment
- **THEN** the call returns `Unavailable` with message `audit trail unavailable`, the payment still reads PAID with refunded amount 0 and 0 refunds, and the seller's REFUND_DEDUCTION entries are 0
- **VERIFIED BY**: team-payment `internal/rpc` `TestAdminRefundAuditErrorFailsClosed`

#### Scenario: Audit timeout is bounded and refuses the refund
- **WHEN** team-audit does not answer within the SDK audit write bound of 2 s during an admin refund
- **THEN** the call returns `Unavailable` within 3 s and no refund is applied
- **VERIFIED BY**: team-payment `internal/rpc` `TestAdminRefundAuditTimeoutFailsClosed`

#### Scenario: Audit down refuses the admin refund end to end
- **WHEN** team-audit is stopped and an admin refunds 200000 of a PAID payment through the gateway (the gateway forwards writes once, no retry)
- **THEN** the call fails with connect code `unavailable`, and after team-audit is started again the payment read by the buyer reads PAID with refunded amount 0 and `assert_no_event` for the payment passes
- **VERIFIED BY**: platform-e2e `features/payment/admin_refund_audit.feature` `Admin refund is refused while audit is down`

#### Scenario: Payment audit: no audit upstream configured
- **WHEN** `config.Load()` runs with `ENV=local` and `UPSTREAM_AUDIT_ADDR` empty, and again with `ENV=production`
- **THEN** each call returns an error naming `UPSTREAM_AUDIT_ADDR`, so no service without an audit writer can start
- **VERIFIED BY**: team-payment `internal/config` `TestUpstreamAddrsRequiredInEveryEnv`
