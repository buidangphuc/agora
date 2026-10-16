## Why

Initiative q6-trust-enforcement gives admins a listing moderation queue (`ModerateListing`, `ListModerationQueue`) and buyers a way to report a listing (`CreateReport`, `ListReports`, `ResolveReport`), but none of it has a UI. Today a rejected listing is a dead end for the seller (no reason shown), a buyer has no recourse against a scam listing, and an admin works only through the API. This slice adds the admin pages `/admin/listings` and `/admin/cases`, a "Báo cáo" button on the product page, and the seller-center view of the new listing statuses and moderation reason.

initiative: q6-trust-enforcement

depends_on: domain-listing-moderation (RPCs `ModerateListing`, `ListModerationQueue`, new `ListingStatus` values, `Listing.moderation`, and their gateway routes with the `admin` edge policy) and engagement-listing-reports (RPCs `CreateReport`, `ListReports`, `ResolveReport` and their gateway routes). Both gateway slices own the edge routes; this slice adds none. The proto copy in `team-frontend/proto` is updated by the contract slice q6-trust-contract; this slice never edits proto copies, it only runs `npm run proto` to regenerate `src/generated`.

Repos and capability: team-frontend (UI shell only) plus platform-e2e (scenarios). Capability: moderation and report handling UI. Rules 1 and 2 hold: every call goes through the gateway; the page decode of the `admin` scope only gates rendering, the gateway edge policy and the owning service stay authoritative.

## What Changes

- New page `src/app/(shop)/admin/listings/page.tsx` (server-rendered, `dynamic = "force-dynamic"`): the moderation queue from `ListModerationQueue`, oldest submission first, 20 per page, cursor in the URL. Each row shows title, seller id, price, submission time (UTC+7), the screen flags, and three actions: "Duyệt" (APPROVE), "Từ chối" (REJECT, reason code required, optional note) and "Khóa" (BAN, reason code required, optional note). Actions are server actions calling `ModerateListing`; on success the row leaves the queue. REJECT and BAN work from `PENDING_REVIEW` or `PUBLISHED`; APPROVE only from `PENDING_REVIEW`; REJECTED and BANNED are final (decision A9).
- New page `src/app/(shop)/admin/cases/page.tsx`: tabbed inbox, tab chosen by `?tab=`. This slice ships `reports` ("Báo cáo", default) and `queue` ("Hàng chờ duyệt", the same queue view as `/admin/listings`). An unknown or missing `tab` shows `reports`. The tab strip is a data-driven list so frontend-dispute-cases adds "Khiếu nại" (`?tab=disputes`) without restructuring.
- Report tab: `ListReports` (status filter, default `OPEN`, oldest first, 20 per page). A row shows listing id, reason code, note, reporter id (admin only), created time and status. Per row: "Xử lý tin đăng" (opens the same reject / ban dialog on the reported listing via `ModerateListing`) and "Đóng báo cáo" (dialog: outcome `ACTIONED` or `DISMISSED` plus a required note, 1 to 500 chars) calling `ResolveReport`. A listing that no longer exists shows "Tin đăng đã bị gỡ".
- New `ReportListingButton` on the product page (`/listing/[id]`): "Báo cáo tin đăng" opens a dialog (reason code, optional note up to 500 chars) calling `CreateReport` with target type `LISTING`. Hidden for the listing's owner; a guest gets a link to login that returns to the product page.
- Seller center: `ListingStatusTag` learns `pending_review` ("Chờ duyệt"), `banned` ("Bị khóa") and `held` ("Tạm ẩn"); the seller edit page shows the reason code label and note from `Listing.moderation` for `rejected` and `banned`, an explanation for `held`, makes the form read-only for `banned` and `held`, and shows "đang chờ duyệt" after a save that ends `pending_review`. Screen flags are never shown to the seller.
- Gateway view layer: new `src/lib/gateway/moderation.ts` and `src/lib/gateway/reports.ts` (server-only, mapped view types, discriminated results, never throw, never return `err.message`); `src/lib/gateway/listings.ts` maps the new statuses and `moderation`.
- Admin entry links "Kiểm duyệt tin đăng" and "Hộp thư vụ việc" on the cockpit page and in the admin footer list of `ConsumerShell`.

No gateway, service or proto change here. No new dependency. Gateway route check: every RPC this slice calls already exists or is added by a slice of this initiative. `ModerateListing` and `ListModerationQueue` by domain-listing-moderation; `CreateReport`, `ListReports` and `ResolveReport` by engagement-listing-reports; `GetListing`, `ListMyListings`, `UpdateListing` are existing and routed (`team-gateway/internal/edge/listing.go`). `ScreenListing` is not called by the frontend (internal, not routed).

## Non-goals

- Dispute tab, "Khiếu nại lên Agora" button and any dispute call: frontend-dispute-cases.
- `/admin/users` and enforcement controls: frontend-user-enforcement.
- Reports on shops, reviews, Q&A or chat; KYC queue; case assignment, SLA timers, macros.
- Seller appeal flow; email or push notification of a moderation decision.
- Showing a pending listing to non-owners (the service answers NOT_FOUND; the page keeps today's `notFound()` behaviour).
- Fixing the `/seller/[id]/edit` guard that sends a signed-in user without `listing.write` (a `restricted` seller) to `/login`; owned by frontend-user-enforcement (decision A20: a "Tài khoản bị hạn chế" notice instead of the redirect loop).

## Proposed Vietnamese copy (for the human to confirm)

| where | text |
|---|---|
| `/admin/listings` title | "Kiểm duyệt tin đăng" |
| `/admin/cases` title | "Hộp thư vụ việc" |
| tabs | "Báo cáo", "Hàng chờ duyệt" |
| queue columns | "Tin đăng", "Người bán", "Giá", "Gửi lúc", "Cờ kiểm tra", "Thao tác" |
| queue empty | "Hàng chờ trống" |
| queue actions | "Duyệt", "Từ chối", "Khóa" |
| reject / ban dialog | "Lý do" (required select), "Ghi chú cho người bán (tối đa 500 ký tự)" |
| moderation reason codes | prohibited_item "Hàng cấm", counterfeit "Hàng giả", misleading "Thông tin gây hiểu lầm", inappropriate "Nội dung không phù hợp", duplicate "Tin trùng lặp", other "Khác" (`seller_enforcement` is system-set, not offered) |
| report columns | "Tin đăng", "Lý do", "Ghi chú", "Người báo cáo", "Báo cáo lúc", "Trạng thái" |
| report status filter | "Đang mở", "Đã xử lý", "Đã bác bỏ" |
| report actions | "Xử lý tin đăng", "Đóng báo cáo", outcomes "Đã xử lý" / "Bác bỏ", "Ghi chú xử lý (bắt buộc, tối đa 500 ký tự)" |
| reports empty | "Không có báo cáo nào" |
| removed listing | "Tin đăng đã bị gỡ" |
| success toasts | "Đã duyệt tin đăng.", "Đã từ chối tin đăng.", "Đã khóa tin đăng.", "Đã đóng báo cáo." |
| audit down | "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau." |
| not allowed transition | "Trạng thái hiện tại của tin đăng không cho phép thao tác này." |
| listing missing | "Tin đăng không còn tồn tại." |
| 403 title / sub-text | "Cần tài khoản Quản trị" / "Tài khoản của bạn không có quyền admin để xem trang này." |
| 403 from gateway | "Cần tài khoản Quản trị" / "Gateway từ chối quyền admin cho phiên đăng nhập này." |
| report button / dialog | "Báo cáo tin đăng", "Lý do báo cáo", "Chi tiết (tùy chọn, tối đa 500 ký tự)", "Gửi báo cáo" |
| report reasons | prohibited_item "Hàng cấm", counterfeit "Hàng giả", scam "Lừa đảo", misleading "Thông tin gây hiểu lầm", inappropriate "Nội dung không phù hợp", other "Khác" |
| report success | "Đã gửi báo cáo. Cảm ơn bạn." |
| own listing | "Bạn không thể báo cáo tin đăng của chính mình." |
| report refused (no permission) | "Bạn không có quyền thực hiện thao tác này." |
| report cap (20 open) | "Bạn đang có quá nhiều báo cáo chờ xử lý. Vui lòng thử lại sau khi các báo cáo được xử lý." |
| delete refused (banned / held) | "Tin đăng đang bị khóa hoặc tạm ẩn nên không thể xoá." |
| seller statuses | "Chờ duyệt", "Bị khóa", "Tạm ẩn", existing "Bị từ chối" |
| seller banners | rejected "Tin đăng bị từ chối: {lý do}. {ghi chú}", banned "Tin đăng bị khóa: {lý do}. {ghi chú}", held "Tin đăng đang tạm ẩn vì tài khoản của bạn bị hạn chế. Tin sẽ hiển thị lại khi tài khoản được khôi phục.", pending "Tin đăng đang chờ kiểm duyệt." |

## Non-functional requirements

- Each admin page render is one gateway call after the session check (`ListModerationQueue` or `ListReports`); client timeout 5 s as in `audit.ts`, a timeout maps to the `unavailable` state, not a crash. Backend budget: queue page 200 ms at 10k pending (initiative NFR, non-gating here). Page size fixed at 20; the page never loops over pages.
- A moderation action is one call; the UI blocks a second submit while pending. A failed action never removes the row.
- Availability: team-domain, team-engagement or team-audit down degrades only the affected tab or action (error banner, other tab usable).
- Filtering, tabs and paging are plain links and GET forms (no client JS needed); dialogs are the only client components.

## Data classification

- Queue rows: listing title, description excerpt, seller id, screen flags. Internal; no personal data beyond the seller id. Purpose: content moderation. Retention: none in the frontend (nothing stored).
- Reports shown to the admin: `reporter_id` (personal: identifies a complainant) and free-text `note` (may contain PII). Purpose: complaint handling. Basis: platform duty to act on violations (Decree 85/2021), data minimisation. Retention is owned by team-engagement (closed reports kept 2 years, then pseudonymised). The frontend renders them to admins only, never to the seller, never to other buyers, and never sends them to analytics.
- Report dialog (buyer): free-text `note` up to 500 chars; sent once to `CreateReport`, not logged, not stored client-side, not sent to the tracker. Consent basis: the buyer's explicit submission.
- Moderation `note` (admin free text) and reason code are shown to the owning seller only (service decides who gets `Listing.moderation`); `screen_flags` are admin-only (decision A11): the service strips them from the owner's reads (domain-listing-moderation), and the seller views never render them as a second layer.
- No new cookies, local storage or client persistence. The session bearer stays server-side (`server-only`). Filter and cursor values in the URL are opaque ids.

## Threat model

- Assets: the moderation queue and report inbox (admin-confidential), reporter identity, the admin session token, the listing status of every seller.
- Abuse cases:
  1. A guest or non-admin opens `/admin/listings` or `/admin/cases`: guest redirected to login through `redirectGuestToLogin`, non-admin sees the 403 result and no gateway call is made. The gateway edge policy enforces `admin` on the data; its `PermissionDenied` renders the same 403.
  2. A non-admin forges a server action call (approve, reject, ban, close report): the action takes no role or scope argument and sends the call with the caller's token; the gateway answers `PermissionDenied`, shown as "Bạn không có quyền thực hiện thao tác này."; nothing changes.
  3. A seller reads who reported their listing: reporter id is rendered only in the admin report table; no seller surface or listing payload carries it; the PDP report dialog never echoes other users' reports.
  4. A seller reads screen flags (reveals the keyword list): the service returns `screen_flags` only to admins and strips them from the owner's `GetListing` / `ListMyListings` (decision A11, enforced in domain-listing-moderation); as defence in depth the seller mapper drops them and the seller views render only reason code label and note; flags exist only in the admin queue component.
  5. Injection through listing titles, notes or flags: all rendered as React text, never HTML; ids are passed as typed proto fields.
  6. Hostile query params (`tab`, `cursor`, `status`): unknown `tab` falls back to `reports`, unknown `status` to `OPEN`, strings trimmed and capped (cursor 256), array values take the first element.
  7. Report spam by one buyer: the service returns the existing open report (idempotent) and caps a reporter at 20 open reports (`ResourceExhausted`, shown as the cap message); the dialog blocks double submit. Rate limiting stays at the gateway.
  8. Admin acts on a stale row (another admin already decided): the service answers `FailedPrecondition`, shown as the not-allowed message and the row is refreshed away on revalidate.
  9. Audit unavailable: the service fails closed (`Unavailable`); the UI shows the audit message and leaves the row so the admin can retry; the UI never retries automatically.
- Authz rule: render admin pages only when `principal.scopes.includes("admin")`; the report button needs a signed-in principal; the gateway and services decide.
- STRIDE summary: Spoofing (forged cookie scopes) mitigated by gateway verification; Tampering (forged actions) by service authz; Repudiation by the audit-first service rule (`listing.moderate`, `report.resolve`); Information disclosure by admin-only fields (reporter id, flags); Denial of service by fixed page size and timeouts; Elevation of privilege by never trusting the decoded scope for data access.

## Open assumptions

- Reason-code lists above follow the initiative contract comments; the contract slice's spec fixes the final lists. The UI labels are a table lookup with a fallback that shows the raw code, so a later code does not crash it.
- Resolved by decision A9: `ModerateListing` REJECT and BAN are allowed from `PUBLISHED` as well as `PENDING_REVIEW`, so "Admin bans a reported listing" needs no extra transition.
- Release gate: showing `reporter_id` and the report note to admins ships behind the DPO sign-off on report retention (initiative NEEDS_HUMAN), like Q1-Q5.
