## 1. Code track (team-frontend-admin)

Depends on slice trust-legal-contract (generated stubs: `ListKycSubmissions`, `GetKycDocumentUrl`, `ReviewKycRequest.reason_code`)
and on verification-kyc-documents for the gateway routes. Regenerate stubs with `npm run proto` first.

- [ ] 1.1 Unit tests first (red) for `casesParams`: `tab=kyc` parsed, kyc status parser (`PENDING` default, case-insensitive `VERIFIED`/`REJECTED`, unknown falls back), cursor capped at 256 (scenarios: Unknown status or oversized cursor falls back)
- [ ] 1.2 `AdminCasesTabs` test then code: add `{ key: "kyc", label: "Xác minh người bán", href: "/admin/cases?tab=kyc" }` to `CASES_TABS`; extend `CasesTab` with `"kyc"`; add the link to the inline `DisputesTabs` in `cases/page.tsx` (scenarios: Tab appears next to the existing tabs)
- [ ] 1.3 `client.ts`: add `verification: createPromiseClient(VerificationService, transport)`; test in `client-context.test.ts` style that the bearer is attached (scenarios: Non-admin sees the 403 result)
- [ ] 1.4 `src/lib/gateway/kyc.ts` with tests (mock clients): `listKycSubmissions(status, cursor)`, `getKycDocumentUrl(id, index)`, `reviewKyc(id, decision, reasonCode)`; 5 s timeout, never throws, Connect code mapping to `unauthenticated | forbidden | not_found | failed_precondition | invalid | unavailable`; no URL or id in any log (scenarios: Gateway refusal is shown as the same 403, Expired session is sent to login, List failure shows a retry message, Unknown submission or index is refused, Audit store down fails closed)
- [ ] 1.4a Reason-code constant list and Vietnamese labels in one module shared by dialog and action (scenarios: Reject with a reason code, Forged decision or reason is refused by the action)
- [ ] 1.5 `kycActions.ts` server actions `viewKycDocumentAction` and `reviewKycAction`: admin scope check, id/index/decision/reason validation, message table, `revalidatePath("/admin/cases")` on success (scenarios: A non-admin cannot obtain a document link, A non-admin cannot review, Forged decision or reason is refused by the action, Reviewing your own submission is refused, Second decision on the same submission fails, Review failure leaves the row unchanged, Approve removes the row and verifies the seller)
- [ ] 1.6 `KycQueueView.tsx` and `KycRejectDialog.tsx` with component tests: rows, status text, empty state, "Trang sau" link, per-index "Xem tài liệu N" buttons, revealed "Mở tài liệu" link with `rel="noopener noreferrer"` and expiry, no URL before the click, approve/reject buttons on `PENDING` rows only, reject needs a reason (scenarios: Pending submissions are listed oldest first, Page boundary at 20 rows, Empty queue, Decided submissions are reachable through the filter, Document button count matches the document count, No document URL is present before the click, Reject without a reason is blocked, Viewing a document reveals a 5-minute link and writes the audit entry)
- [ ] 1.7 `cases/page.tsx`: `tab=kyc` branch with guest redirect, 403, denied, unavailable alert; page test mirroring `cases/page.test.tsx` (scenarios: Anonymous visitor is redirected to login, Non-admin sees the 403 result, Gateway refusal is shown as the same 403, Expired session is sent to login, List failure shows a retry message)
- [ ] 1.8 Gates through `scripts/asdlc/gate_digest.py` for team-frontend-admin (lint, typecheck, vitest, coverage)

## 2. E2E track (platform-e2e; FEATURES.yaml by e2e-author)

- [ ] 2.1 Feature file `admin/kyc_queue.feature` covering every scenario in `specs/admin-kyc-queue-ui/spec.md`; replace each `VERIFIED BY: TBD` with its path and scenario title
- [ ] 2.2 Steps that create pending submissions through the real RPCs (seller `CreateKycUploadUrl`, presigned PUT, `SubmitKyc`), no route fulfils or fake services; reuse the seller/admin login helpers (scenarios: Pending submissions are listed oldest first, Page boundary at 20 rows)
- [ ] 2.3 Step asserting audit entries via the existing audit query RPC filtered by action `kyc.document_view` (scenarios: Viewing a document reveals a 5-minute link and writes the audit entry, Each view is a separate audited call)
- [ ] 2.4 Step that stops team-audit (or uses the existing audit-down overlay) for the fail-closed case (scenarios: Audit store down fails closed, Review failure leaves the row unchanged)
- [ ] 2.5 Two-admin race and self-review steps (scenarios: Second decision on the same submission fails, Reviewing your own submission is refused)
- [ ] 2.6 `team-frontend-admin/FEATURES.yaml` entry `admin.kyc-queue` (e2e-author)
