## MODIFIED Requirements

### Requirement: Dispute resolution fails closed when team-audit is unavailable

`EngagementService.ResolveDispute` SHALL return `Unavailable` and SHALL leave the dispute unchanged when the team-audit write errors or times out. `UPSTREAM_AUDIT_ADDR` is required in every `ENV`, so a service without an audit connection does not start and this requirement has no unset-upstream mode. Other dispute RPCs SHALL NOT depend on team-audit.

#### Scenario: Dispute audit: Audit error leaves the dispute open
- **WHEN** team-audit returns an error during ResolveDispute for an open dispute
- **THEN** the call returns `Unavailable` and GetDispute still returns the previous status and resolution
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeAuditFailureFailsClosed`

#### Scenario: Dispute audit: timeout is Unavailable
- **WHEN** team-audit does not answer within the fixed 2 s bound during ResolveDispute
- **THEN** the audit client returns an error within 2 s plus 1 second, and ResolveDispute maps it to `Unavailable` with the dispute unchanged
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditClientWriteEventTimeout`; team-engagement `internal/service` `TestResolveDisputeAuditTimeoutUnavailable`

#### Scenario: Dispute audit: No audit upstream configured
- **WHEN** `Load` runs with `UPSTREAM_AUDIT_ADDR` unset in any `ENV` (the audit connection has no fallback, so ResolveDispute can never run without one)
- **THEN** it returns an error that names `UPSTREAM_AUDIT_ADDR` and no server is built
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyAuditAddr`

#### Scenario: Dispute audit: Update failure after audit is Internal and retryable
- **WHEN** the audit write succeeds, the repository update fails, and the call is retried with the repository healthy
- **THEN** the first call returns `Internal`, the retry returns OK, the dispute is resolved, and 2 `dispute.resolve` events exist for it
- **VERIFIED BY**: team-engagement `internal/service` `TestResolveDisputeUpdateFailureInternalThenRetry`

#### Scenario: Dispute audit: Other dispute RPCs ignore team-audit availability
- **WHEN** team-audit is down and a buyer calls CreateDispute and GetDispute
- **THEN** both calls succeed
- **VERIFIED BY**: team-engagement `internal/handler` `TestDisputeReadWriteIndependentOfAudit`
