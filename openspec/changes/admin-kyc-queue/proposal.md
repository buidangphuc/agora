## Why

Sellers will upload real identity documents (slice verification-kyc-documents) and no payout leaves the platform for a
seller who is not `VERIFIED` (slice payment-payout-kyc-gate). Someone has to work the submissions. Today
`/admin/cases` has Báo cáo, Hàng chờ duyệt and Khiếu nại tabs but no KYC tab
(`team-frontend-admin/src/app/admin/cases/page.tsx`), `VerificationService` has no list RPC, and the admin app has no
verification client (`src/lib/gateway/client.ts`). Without this slice the KYC queue can only be worked by calling
`ReviewKyc` by hand, and nobody can look at a document.

initiative: trust-legal

## What changes

- New "Xác minh người bán" tab on `/admin/cases` (`?tab=kyc`) listing KYC submissions oldest first, 20 per page, filtered by
  status (default `PENDING`; also `VERIFIED`, `REJECTED`) with a cursor, in the same page and tab strip as the existing tabs.
  Each row shows submission id, user id, document type, document count, submitted time (UTC+07:00), status and reason code.
- Per-document "Xem tài liệu" action: a server action calls `GetKycDocumentUrl(submission_id, index)`; the gateway/service
  writes the `kyc.document_view` audit entry first and fails closed. Only on success the UI reveals a link that opens the
  5-minute presigned URL in a new tab (`noopener noreferrer`). One RPC call per click; the URL is never rendered
  server-side into the list, never logged, never persisted and never cached.
- "Duyệt" and "Từ chối" on `PENDING` rows through `ReviewKyc`. Reject requires a reason code picked from a fixed
  Vietnamese list; approve sends none.
- Gateway client: add `VerificationService` to `makeClients` in `src/lib/gateway/client.ts`; new `src/lib/gateway/kyc.ts`
  (never throws, maps Connect codes to a status union like `moderation.ts`); server actions in
  `src/features/admin/kycActions.ts`; UI in `src/features/admin/KycQueueView.tsx`; `CasesTab` gains `"kyc"`.
- The page keeps `referrer: no-referrer` and `force-dynamic`. No CSP change: documents open by navigation, not as embedded images.

Contract consumed (from slice trust-legal-contract, not redefined here): `VerificationService.ListKycSubmissions`,
`GetKycDocumentUrl`, `ReviewKyc` (with `reason_code=3`), `KycSubmission`, `VerificationStatus` (incl. `NOT_SUBMITTED=4`),
`ListKycSubmissionsRequest/Response`, `GetKycDocumentUrlRequest/Response`. The gateway route and the `admin` rule for the
two new RPCs belong to slice verification-kyc-documents (team-gateway `verification.go`/`policy.go`); this slice adds no gateway code.

## Capabilities

### New Capabilities
- `admin-kyc-queue-ui`: admin KYC tab: paged queue, audited document viewing, approve and reject with reason code.

### Modified Capabilities
none

## Impact

- team-frontend-admin: `src/lib/gateway/client.ts`, new `src/lib/gateway/kyc.ts`, `src/app/admin/cases/page.tsx`,
  `src/features/admin/{AdminCasesTabs.tsx,casesParams.ts}`, new `KycQueueView.tsx`, `KycRejectDialog.tsx`, `kycActions.ts`,
  regenerated stubs (`npm run proto`), `FEATURES.yaml` (by e2e-author).
- No other repo is edited. Needs the gateway routes and the service from verification-kyc-documents before e2e can pass.

## Non-goals

- Seller-side upload and status page (slice seller-kyc-ui), payout gate, public badge, document storage.
- Re-opening a decided submission, bulk actions, free-text rejection notes, document zoom or in-page image rendering.
- Admin MFA and splitting the `admin` role (separate security initiative); any admin with the `admin` scope sees all documents.
- An audit viewer change: `kyc.document_view` appears in the existing `/admin/audit` list through its free-text action filter.

## Non-functional requirements

- Queue page: p95 server render <= 1 s for a 20-row page excluding gateway time; gateway timeout 5 s per call (as
  `moderation.ts`); a failed list shows a Vietnamese error, never a stack or upstream address.
- `GetKycDocumentUrl` adds one audited round trip per click; the UI makes exactly 1 call per click and no retry.
- Presigned GET TTL is 5 minutes (set by the service); the UI shows the expiry and never refreshes a URL by itself.
- Availability: the tab depends on team-gateway, team-verification and team-audit; any of them down degrades only this tab.
- Limits: page size 20; cursor <= 256 chars (as `casesParams`); reason code is one of 6 fixed values <= 64 chars.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| ID document image (via presigned URL) | sensitive (face photo, ID number) | admin checks seller identity | seller's explicit consent at upload (collected in seller-kyc-ui); Decree 85/2021 platform duty (unverified, see initiative NEEDS_HUMAN) | not stored by this app; URL valid 5 min; document retention is owned by verification-kyc-documents |
| user id in queue rows | personal (pseudonymous id) | identify whose submission is reviewed | legitimate operation of the seller verification | rendered, not stored; never logged raw, never put in URLs other than the opaque submission id |
| reason code | personal-adjacent (decision about a person) | tell the seller why a submission was rejected | seller verification | stored by team-verification; this app stores nothing |
| presigned document URL | secret (bearer capability) | open one document | admin duty | memory of one client component only, dropped on navigation or after expiry; never in logs, toasts, query strings or cookies |

This app stores no KYC data; it is a viewer. No document bytes pass through the Next.js server.

## Threat model

- Assets: ID document images, the queue of who applied, the approve decision that unlocks payouts.
- Abuse cases and mitigations:
  - Non-admin or anonymous reaches the queue or a document: page redirects/403 before any RPC; gateway and team-verification
    enforce `admin` again (the UI check is only gating render).
  - Document opened with no audit trail: UI never builds a storage URL itself; the only source is `GetKycDocumentUrl`, which
    audits first and fails closed; on `UNAVAILABLE` no link is shown.
  - URL leakage: no URL in the list HTML, no `Referer` (page `no-referrer`, link `noreferrer`), no logging, `force-dynamic`, no
    cache; 5-minute TTL bounds a leaked link.
  - Self-approval: an admin reviewing their own submission is refused by team-verification; the UI shows the refusal and changes nothing.
  - Double decision / races between admins: second `ReviewKyc` fails `FAILED_PRECONDITION`; the UI shows "already handled" and refreshes the row.
  - Forged action input (arbitrary `reason_code`, bad decision, huge ids): server action validates against the fixed list and
    id shape before the RPC.
- Authz rule: render and every action require the `admin` scope in the session principal; the gateway is the enforcement point.
