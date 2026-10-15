## Why

`VerificationHandler.ReviewKyc` in `team-verification/internal/handler/verification.go` lets an admin approve or reject a KYC submission and leaves no trace of who decided. A verified badge is a trust signal on the marketplace, so "who approved this user" must be answerable. The initiative spec (`plans/q3-audit-log/00-spec.md`, locked 2026-10-15) requires every admin-path mutation to write one team-audit event before the change and to fail closed when team-audit is unavailable. `ReviewKyc` is always an admin path (admin-gated at the service and at the edge via `adminProcedures`) and is the `kyc.review` row of the catalogue. team-verification has no team-audit client today (README: "Consumes: nothing upstream").

initiative: q3-audit-log

## What Changes

- **team-verification**
  - New `internal/upstream/audit.go`, mirroring `team-engagement/internal/upstream/audit.go`:
    - `NewAuditClient(addr, timeout)` returns `(nil, nil)` when `UPSTREAM_AUDIT_ADDR` is empty.
    - Own outgoing-principal interceptor: id `service-team-verification`, type `service`, scopes `audit.write`; it replaces the admin's forwarded principal.
    - `WriteKycReviewEvent(ctx, actorID, submissionID, userID, decision)` calls `AuditService.WriteAuditEvent` with `action=kyc.review`, `target_type=kyc_submission`, `target_id=<submission id>`, `metadata={decision, user_id}`.
  - `ReviewKyc` order: authenticate, `admin` scope, load submission (empty id InvalidArgument, NotFound), self-review check (PermissionDenied), validate decision (InvalidArgument), refuse a non-PENDING submission (FailedPrecondition), write the audit event, then apply the review. An audit error, timeout, or a nil client returns `Unavailable` and the submission stays PENDING.
  - A small `AuditWriter` interface is injected into `VerificationHandler` so unit tests use a fake. `cmd/server/main.go` wires it only when the client is non-nil; a nil writer means `ReviewKyc` returns `Unavailable` (fail closed).
  - Config: `UPSTREAM_AUDIT_ADDR` (default empty) and `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2, must be > 0).
  - Proto: the contract slice vendors `audit.proto` into `team-verification/proto`; this slice only runs `buf generate`. `generated/` stays gitignored.
  - Docs: `.env.example` and README (Configuration, "Consumes", Behavior notes).
- **platform-e2e**
  - New feature `features/services/verification_kyc_audit.feature` plus step bindings, using `tests/e2e/support/audit_support.py` (owned by `audit-query-filters`).
  - Scenarios: audited approve, audited reject, audit-down refusal, non-admin writes no event.
- No change to the gateway (plain forwarder, ReviewKyc already admin-gated at the service), proto of `VerificationService`, or the DB schema.
- `docker-compose.services.yaml` (`UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` for team-verification) is owned by `q3-audit-log-contract`; this slice only depends on it.

## Impact

- Affected: `team-verification` (handler, new upstream package, config, main, docs), `platform-e2e` (new feature + steps).
- Behaviour change: with `UPSTREAM_AUDIT_ADDR` unset, ReviewKyc now returns `Unavailable` (previously worked). Local compose gets the address from the contract slice; any other environment must set it before this ships (human release item, as in Q2).
- Latency: one extra gRPC call on ReviewKyc only; no change to other RPCs.

## NFR

- Added latency on ReviewKyc bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s), single attempt, no retry.
- Availability: ReviewKyc depends on team-audit by design (fail closed). SubmitKyc and GetVerificationStatus never call team-audit.
- Delivery is at-least-once: if the audit write succeeds and the review update then fails, the retry writes a second event (no idempotency key, per the initiative non-goals).

## Data classification

| Field | Where | Purpose | Basis | Retention |
|---|---|---|---|---|
| admin user id (`actor_id`) | audit event | Accountability for a privileged action | Legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit policy, immutable |
| submission id (`target_id`) | audit event | Identify the reviewed submission | same | same |
| subject `user_id` (`metadata.user_id`) | audit event | Say whose KYC was decided (pii-light, a pseudonymous id) | same | same |
| `decision` | audit event | Outcome (`approve`/`reject`) | same | same |

`doc_type` and `doc_ref` are never written to the audit event and never logged by the new code. Erasure of audit rows is out of scope (initiative non-goal).

## Threat model

- Assets: integrity of the verified badge; completeness of the audit trail.
- Abuse cases:
  - An admin approves a KYC and no record exists. Mitigation: audit written before the update, fail closed.
  - Forcing team-audit down to block reviews. Accepted: it is a denial of an admin action, not of user data.
  - A caller forges `actor_id`. Mitigation: `actor_id` comes from the verified Principal, never from the request body.
  - Writing audit events as someone else. Mitigation: the service principal `service-team-verification` holds only `audit.write`; the admin's principal is not forwarded to team-audit.
  - Self-approval. Unchanged: refused (PermissionDenied) before any audit write.
- Authz rule: ReviewKyc requires an authenticated caller with the `admin` scope and a submission that is not the caller's own. Only calls that pass those checks (and input validation) write an event.
