# Design

## Decisions
- Reports live in team-engagement beside disputes (initiative decision); `target_type` is stored as text so a later target kind needs no migration.
- Idempotent create via the partial unique index plus `INSERT ... ON CONFLICT DO NOTHING` then select of the existing OPEN row: two concurrent identical calls return the same report, no error.
- "Seller reports own listing": team-engagement has no listing upstream (initiative decision). The check uses the existing `seller_listings` table fed by `listing.events`: a row `(seller_id = caller, listing_id = target)` means own listing, `FAILED_PRECONDITION`. That table holds PUBLISHED listings only, so a seller's unpublished listing is not caught; harmless, since a non-published listing is not reachable by a buyer's report button.
- Audit first: mirrors `HideReview` (`service.HideReview`, `upstream.AuditClient`): load report, if already closed (any status) return `FAILED_PRECONDITION` without audit, call audit, then close. Audit nil client means `UNAVAILABLE`.
- Close is a compare-and-set (`UPDATE ... WHERE id = $1 AND status = 'OPEN'`), so two admins racing cannot both win; the loser gets `FAILED_PRECONDITION` (the report is already closed).
- Pseudonymisation value: `reporter_id = 'redacted'`, `note = ''`. The sweep also removes the row from the unique index domain (index is partial on OPEN, closed rows are outside it).
- No `ReportCreated` event: nothing consumes it; add it with its first consumer.

## Clarifications
### Round 1 — 2026-10-16
- Q1 [data] Should the audit event copy the reporter id or note? -> No: only report id, listing id, status and the admin's resolution note, to keep complainant data out of the audit log (author decision, matches data classification).
- Q2 [boundary] Is a resolution note required on both outcomes? -> Yes, 1-500 characters after trimming (initiative story: "close with a note"); flagged in OPEN_ISSUES.
- A19 [boundary] Listing reports: Open report cap per reporter: how many OPEN reports may one reporter hold, and what does the next one get? → **At most 20 OPEN reports per reporter; the 21st distinct listing is RESOURCE_EXHAUSTED (re-reporting an open pair returns the existing report)** (human)
- A19 [boundary] Listing reports: Empty or whitespace note is rejected: is a resolution note required on ResolveReport for both ACTIONED and DISMISSED? → **Yes, required, 1–500 characters after trim; else INVALID_ARGUMENT** (human)
- X1 [contract] Listing reports: Closing again with the same status is refused: does re-closing an already closed report with the same status return OK or FAILED_PRECONDITION? → **FAILED_PRECONDITION for any already closed report, no audit write, matching the q6-trust-contract ResolveReport comment** (human: "approve hết", contract wins)
- X2 [ownership] Listing reports: all scenarios: how are scenario titles kept unique against frontend-listing-moderation and frontend-user-enforcement? → **Every scenario title in this change is prefixed "Listing reports: "** (human, cross-slice fix)
- X3 [contract] Listing reports: Report messages: must this slice coordinate engagement.proto field numbers with q10? → **No; q10 no longer touches engagement.proto and q6-trust-contract owns the numbers** (human, cross-slice fix)
