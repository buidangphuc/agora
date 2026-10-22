## Why

A buyer who meets a counterfeit, scam or prohibited listing has no way to tell Agora: `team-engagement` has reviews, Q&A and disputes but no report channel, and admins have no queue to work (trust-ops #3). The initiative spec (`plans/q6-trust-enforcement/00-spec.md`, locked 2026-10-16, story 5) adds listing reports: a signed-in user reports a listing with a reason code and an optional note; an admin lists open reports, closes each as `ACTIONED` or `DISMISSED`, and every close is audited first (Q3 rule). The proto (`CreateReport`, `ListReports`, `ResolveReport`, `Report`, enums) is owned by `q6-trust-contract`; moderating the listing itself is `domain-listing-moderation` (`ModerateListing`); the inbox UI is `frontend-listing-moderation`. This slice is the team-engagement storage and RPCs plus the gateway routes.

initiative: q6-trust-enforcement

## What Changes

- **team-engagement**
  - Migration `0012_reports` (up and down): table `reports` (`id`, `target_type`, `target_id`, `reason_code`, `note`, `reporter_id`, `status`, `created_at`, `resolved_at`, `resolved_by`, `resolution_note`), partial unique index on `(reporter_id, target_type, target_id) WHERE status = 'OPEN'` (idempotent create), index on `(status, created_at, id)` for the oldest-first queue, index on `(target_id)`, index on `(resolved_at)` for the sweep. The number follows the highest existing migration at build time (`0011_review_integrity` today).
  - `CreateReport` (authenticated user, scope `engagement:write`): validates target type (`LISTING` only), `target_id` UUID shape (existence not checked), reason code in `prohibited_item | counterfeit | scam | misleading | inappropriate | other`, note at most 500 characters; refuses a seller reporting their own listing; returns the existing `OPEN` report for the same (reporter, listing); otherwise stores `OPEN`.
  - `ListReports` (admin): filter by status and `target_id`, oldest first, paged.
  - `ResolveReport` (admin): `ACTIONED` or `DISMISSED` with a 1-500 character note; writes team-audit event `report.resolve` first, fail closed (`UNAVAILABLE`), then closes the report; an already closed report (same or different status) is `FAILED_PRECONDITION` with no audit write, as the contract states.
  - Retention sweep: a background job pseudonymises `reporter_id` and drops `note` on reports closed more than 2 years ago.
  - Config: `REPORT_RETENTION_DAYS` (default 730) and `REPORT_SWEEP_INTERVAL` (default 24h). Reuses the existing `UPSTREAM_AUDIT_ADDR` audit client and its service principal.
- **team-gateway**
  - `EngagementForwarder` forwards `CreateReport`, `ListReports`, `ResolveReport`; `adminProcedures` gains `ListReports` and `ResolveReport`; pinned-set and unrouted tests updated.
- **platform-e2e**: feature `engagement/listing_reports.feature` and step bindings.
- **team-order**: no change. The initiative table lists it as a touched repo, but nothing in listing reports reaches team-order; the initiative table should drop it (initiative-level fix, not this change).
- Not in this slice: closing a report does not change the listing; the admin acts on it through `ModerateListing` (`domain-listing-moderation`). No notification of the reporter. No reports on shops, reviews, Q&A or chat.

## NFR

- `CreateReport` p95 < 100 ms (one indexed insert, no upstream call). `ListReports` page of 50 p95 < 200 ms at 100k reports (index `(status, created_at, id)`).
- `ResolveReport` bounded by the audit call, `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s); audit down means no state change.
- Abuse limit: at most 20 `OPEN` reports per reporter at any time; the 21st is `RESOURCE_EXHAUSTED` (the edge rate limiter stays the first line).
- Sweep: batches of 500 rows, never holds a lock longer than one batch; survives restarts (idempotent).
- Availability: reports degrade independently of reviews and disputes; audit outage only blocks `ResolveReport`.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `reports.reporter_id` | personal (account id of the complainant) | complaint handling, dedupe, abuse limit | platform duty to handle complaints about prohibited goods (Decree 85/2021); legitimate interest | 2 years after `resolved_at` (open reports: while open), then replaced by `redacted` |
| `reports.note` (free text, may contain PII) | personal, possibly sensitive | context for the admin | same | 2 years after `resolved_at`, then emptied |
| `reports.resolution_note` (admin text) | operational | records the decision | same | kept with the row; not personal by policy, admins are told not to enter PII (UI copy, frontend slice) |
| audit event `report.resolve` | operational | accountability | Q3 audit rule | per team-audit retention; carries report id, listing id, status, resolution note, never `reporter_id` or `note` |

DPO sign-off on the basis and the 2-year retention is a NEEDS_HUMAN item of the initiative.

## Threat model

- Assets: complainant identity; the integrity of a listing's visibility (a report must never remove a listing by itself); admin decisions.
- Abuse cases:
  - Report brigading to harass a seller: a report has no automatic effect, one open report per (reporter, listing), 20 open per reporter.
  - Seller identifies and retaliates against the reporter: no RPC returns `reporter_id` except `ListReports` (admin) and the reporter's own `CreateReport` response; the seller has no read path.
  - Seller reports own listing to game the queue or probe: refused.
  - Non-admin closes or lists reports: refused at the edge and again in the service.
  - Admin closes a report without trace: audit first, fail closed.
  - Note carrying PII or script: stored as text, length-capped, never logged; rendered escaped by the frontend.
  - Probing listing ids: `target_id` is shape-checked only, so existence is not leaked.
- Authz rule: `CreateReport` requires an authenticated user with `engagement:write` (anonymous gets `UNAUTHENTICATED`); `ListReports` and `ResolveReport` require scope `admin` (`interceptor.RequireAdmin`), actor id from the verified principal, never from the request.
