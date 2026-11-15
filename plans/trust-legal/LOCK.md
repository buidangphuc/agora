# LOCK — trust-legal
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-13
spec: plans/trust-legal/00-spec.md (Locked 2026-11-13)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| trust-legal-contract | platform-core | [] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 425fcffef071 | [x] |
| identity-legal-consent | team-identity | [trust-legal-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 3e3f92c8141c | [x] |
| commerce-user-erasure | team-order | [trust-legal-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 9abf97b39733 | [x] |
| identity-account-deletion | team-identity | [identity-legal-consent, commerce-user-erasure] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 7b5734cf00b7 | [x] |
| identity-data-export | team-identity | [identity-account-deletion] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 8017c6e363c0 | [x] |
| content-user-erasure | team-engagement | [trust-legal-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 1951045e455b | [x] |
| domain-seller-erasure | team-domain | [trust-legal-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 699ad0004057 | [x] |
| analytics-user-erasure | team-analytics | [trust-legal-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | c61a1f27101c | [x] |
| verification-kyc-documents | team-verification | [trust-legal-contract] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 3d3e8d5300ca | [x] |
| payment-payout-kyc-gate | team-payment | [verification-kyc-documents] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 77906935b59e | [x] |
| frontend-legal-pages | team-frontend | [identity-legal-consent] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | e2e7ff15f69a | [x] |
| frontend-account-privacy | team-frontend | [identity-data-export, frontend-legal-pages] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d996a770553e | [x] |
| seller-kyc-ui | team-frontend-seller | [verification-kyc-documents, payment-payout-kyc-gate] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | fa44b614550e | [x] |
| admin-kyc-queue | team-frontend-admin | [verification-kyc-documents] | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 46397f7af48a | [x] |

decide (spec): `scripts/asdlc/decide.py` produced no table for any slice (system python lacks `yaml`; with the e2e venv it needs `--units`/`--out` and a kev engine that was not running). Each spec-quality-reviewer's own ISO 29148/EARS checklist is the verdict.

## Build order (cross_change_check)
cross_change_check order: trust-legal-contract -> analytics-user-erasure -> commerce-user-erasure -> content-user-erasure -> domain-seller-erasure -> identity-legal-consent -> frontend-legal-pages -> identity-account-deletion -> identity-data-export -> frontend-account-privacy -> verification-kyc-documents -> admin-kyc-queue -> payment-payout-kyc-gate -> seller-kyc-ui

Waves by depends_on:
1. trust-legal-contract (contract, sequential, merges first)
2. identity-legal-consent, commerce-user-erasure, content-user-erasure, domain-seller-erasure, analytics-user-erasure, verification-kyc-documents
3. identity-account-deletion, frontend-legal-pages, payment-payout-kyc-gate, admin-kyc-queue
4. identity-data-export, seller-kyc-ui
5. frontend-account-privacy

Hot-file pairs that must not run in parallel (00-spec "Hot files"):
- `team-gateway/internal/edge/auth.go` (+ PrivacyService forwarder): identity-legal-consent, identity-account-deletion, identity-data-export
- `team-gateway/internal/edge/policy.go`, `verification.go`: verification-kyc-documents only; `order.go`/`payment.go` 501 entries: commerce-user-erasure (also check against verification-kyc-documents if commerce edits policy.go)
- `docker-compose.services.yaml` (topic init DLQs, consumer env, MinIO bucket): commerce-user-erasure, content-user-erasure, domain-seller-erasure, analytics-user-erasure, verification-kyc-documents (all in wave 2: serialise their compose edits)
- `platform-core/tools/seed-marketplace.sh` and platform-e2e payout steps: payment-payout-kyc-gate, seller-kyc-ui
- `team-frontend/src/components/shell/ConsumerShell.tsx` (footer, account nav): frontend-legal-pages, frontend-account-privacy, seller-kyc-ui

## Cross-change check
pass — `cross_change_check.py --slices plans/trust-legal/slices.yaml` printed `cross_change_check: OK` (exit 0), no violations.

## Open issues / NEEDS_HUMAN
Escalated slices: none (every slice passed in round 1). Risk routing: `proto` (trust-legal-contract), `pii` (all erasure, consent, export, KYC slices), `auth` (identity-*, gateway routing, admin-kyc-queue), `payment` (payment-payout-kyc-gate, commerce-user-erasure).
Dependency note: every consumer slice assumes the trust-legal-contract proto names unchanged; none of the new RPCs/messages exist on this checkout yet, so wave 2+ must not start before the contract slice merges.

### A. Questions that change a spec (answer before or at sign-off)
1. Obligations during the 48 h cancel window (commerce-user-erasure, identity-account-deletion): orders/payouts opened after the request get masked in flight at erasure. Accept as is (current spec), re-check `GetUserObligations`/`GetPayoutObligations` in the sweeper and postpone erasure, or block new orders while deletion is pending?
2. Slice repo scope: (a) commerce-user-erasure's 501 pinning test lives in team-gateway, which is not in its repos — add team-gateway, or does the 501 come from embedded Unimplemented handlers with no edit? (b) trust-legal-contract replaces proto copies in about 14 consumer repos plus new team-verification `identity.proto`/`events.proto` copies, while 00-spec lists only platform-core — confirm the wider scope. (c) analytics-user-erasure adds consumers in platform-recsys, platform-featurestore and team-ai with owner team-analytics only — confirm the build wave may edit those repos.
3. `verification.proto` copy for team-payment: the contract slice does not list team-payment as a consumer; payment-payout-kyc-gate task 1.1 vendors it. Which slice owns the copy?
4. New dependencies beyond the locked "MinIO only": team-verification gains a Kafka consumer (go-sdk) for `UserErased`; platform-featurestore has no Kafka client, so its erasure is state-based on the next `materialize` run, which can miss the 1 h NFR. Allow a featurestore Kafka consumer, or fix a `materialize` cadence (and who schedules it locally)?
5. Audit catalogue pins: bumping 25 -> 26 actions updates `gaps_contract_docs_test.go` only. Do other repos (team-audit) assert the 25 count, and do `auditquery_roundtrip_test.go` (17) and `pah_contract_test.sh` (18) run against the real `audit.proto`?
6. Gateway route for anonymous `GetPublicBadges`: confirm verification-kyc-documents owns it (the contract can only document it). admin-kyc-queue likewise assumes verification-kyc-documents delivers the admin routing for `ListKycSubmissions`/`GetKycDocumentUrl`.
7. Presigned MinIO from the browser (verification-kyc-documents, seller-kyc-ui, admin-kyc-queue): bucket CORS for the seller-app origin and a browser-reachable public host are not in the spec; does the PUT need an SSE header beyond Content-Type/Content-Length; and SSE-S3 needs a local KMS key — add one, or accept encryption only in later envs?
8. KYC reject `reason_code`: admin-kyc-queue fixes six values (document_unreadable, document_expired, document_mismatch, document_type_invalid, suspected_fraud, other) with Vietnamese labels; seller-kyc-ui maps only document_unreadable. Is the list product-owned and shared by both UIs, are the labels final, and does verification accept any lower_snake_case value?
9. ReviewKyc error codes: self-review = PERMISSION_DENIED, already-decided = FAILED_PRECONDITION (assumed by admin-kyc-queue; one combined UI message for self-review and no-permission). May an admin open the documents of their own submission via `GetKycDocumentUrl`?
10. KYC submission lifecycle (verification-kyc-documents): at most 10 open upload grants per user (RESOURCE_EXHAUSTED); a new submission supersedes an earlier PENDING one; a new PENDING after VERIFIED makes the seller read PENDING (badge false, payouts blocked) — confirm, and add as scenarios.
11. Payout gate (payment-payout-kyc-gate): any non-OK answer from team-verification (incl. PermissionDenied, NotFound) maps to UNAVAILABLE, not FAILED_PRECONDITION. Seed approves demo sellers with an admin credential — which existing admin seed account, local only?
12. RecordConsent rules (identity-legal-consent): sources limited to register/banner/account/seller_onboarding; TERMS/PRIVACY_POLICY cannot be withdrawn (granted=false rejected); page size default 20 / max 100; SELLER_TERMS accepts any version in `SELLER_TERMS_VERSIONS`; not-yet-built PrivacyService RPCs return UNIMPLEMENTED in identity and gateway (vs not registered).
13. Consent and seller terms: consent rows are not linked to `users.seller_terms_version`, BecomeSeller writes no SELLER_TERMS row, and the frontend writes it best-effort after BecomeSeller. Accept best-effort as proof, or unify server-side? Legacy users: re-acceptance decided by the frontend from `ListMyConsents` (no server flag, no backfill).
14. Banner and prompt (frontend-legal-pages, frontend-account-privacy): banner ConsentRecord.version = "1"; first save by a signed-in user with no cookie records all three purposes, later only changed ones; re-acceptance prompt is a blocking dialog (Đồng ý / Đăng xuất) skipped on legal pages; seller app has no banner or prompt; default legal version `2026-11-13` shared by FE env and identity compose.
15. Account deletion assumptions (identity-account-deletion): repeated request returns the original erase_after; cancel with nothing pending = FAILED_PRECONDITION; a pending user can still log in; erasure also clears roles and deletes password-reset tokens; e2e uses a shortened `ACCOUNT_DELETION_CANCEL_WINDOW` in the overlay (domain-seller-erasure otherwise falls back to a Kafka test producer); specify failed-attempt limit/lockout on wrong password?
16. Data export (identity-data-export): sessions and login history capped at newest 1000 rows with truncated flags; reviewer asks to halve sessions and login_history alternately until under 1 MiB (plus an E11 variant), pin empty collections as `[]` in E4, and align task 2.3 wording with the e2e list. Should the export be audited (needs a 27th audit action in the contract)?
17. Frontend account privacy: deletion blocker text comes verbatim from identity — must identity return Vietnamese text, or should the FE map category codes? Which href is the "user's reviews entry point" (no /account/reviews route)? Should export map PermissionDenied to "session expired" (PV30)?
18. Commerce erasure extras: also mask `order_returns.reason`; team-promotion reuses DLQ `identity.events.promotion.dlq`; promotion deactivates owned vouchers and deletes `new_user_voucher_state`; referral deletes referral_codes/accounts/buyer_orders but keeps referrals and reward ledger.
19. Content erasure extras: also deletes helpful votes, collections, view_history, check-ins and the loyalty account (forfeits points); re-keys reviews/Q&A to the single user_id `deleted` (check unique or per-user constraints on (listing_id, user_id)); chat keeps sender/buyer/seller ids; review photos kept; disputes untouched. Reviewer minors: split retry scenario (store rollback vs handler retry), split the "or" trigger scenario, replace "byte-identical".
20. Domain seller erasure: listing image objects in `listing-images` stay undeleted; own consumer group `team-domain-erasure` and DLQ `identity.events.team-domain.dlq` vs extending the hold consumer; a live access token can create a listing after erasure (tombstone guard on CreateListing?); fault-injection seam for the rollback scenario under the no-self-mock rule.
21. Analytics erasure: warehouse re-keys to a random pseudonym (not delete) with an HMAC tombstone of no expiry; old featurestore snapshots, recsys `als_interactions` files and MLflow artifacts keep the original key; BigQuery adapter deferred.
22. Seller KYC UI: `/account/verification` returns 404 with no redirect; badge only on `/shop/[id]`; >3 files — keep none and disable submit (suggested wording) or keep first 3; payout error text by Connect code assumes the code reaches the Server Action (`messageOf()` reads only message).
23. Admin KYC queue minors: empty-state text for VERIFIED/REJECTED filters; scenario for the document link vanishing after expiry; exact 403 sub-text for a missing-scope principal; "no session cookie or unreadable/expired session" wording.

### B. Legal / DPO sign-offs (do not block the spec text)
- Confirm the applicable instruments (Decree 13/2023, 2026 PDP law), the data-subject-request deadline (the "72 h" and Art. 16 citations are unverified) and the Decree 13 Art. 9-11 citation for consent.
- Supply the text of all seven legal pages, the operator block (company name, registration number, address, MoIT notice) and the KYC explicit-consent checkbox wording.
- Set the accounting retention period for pseudonymised orders/ledger/payouts, and N years for consent records after erasure (placeholder until then).
- KYC retention: VERIFIED kept while the account exists, REJECTED/superseded deleted after 90 days; and whether VERIFIED documents are deleted on `UserErased` (current spec) or kept for a legal period.
- Keeping immutable team-audit copies (user ids, free-text reasons) after erasure.
- Whether re-keying to a pseudonym counts as erasure (warehouse, reviews/Q&A, chat ids) and the tombstone retention.
- Disputes (claimant/defendant ids, reason, evidence URLs) left untouched by erasure.
- Whether data-subject access (export) must be logged.
- Retained ML artifacts (old snapshots, MLflow) and listing images of erased sellers.
