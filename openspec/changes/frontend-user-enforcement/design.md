# Design

## Decisions

- Server action over client fetch: the status change is a `"use server"` action that reads the token from the session cookie, so the bearer never reaches the browser (rules 1 and 4 of the frontend conventions). The action returns `{ ok, error?, message? }`, the standard action contract, and calls `revalidatePath("/admin/users")`.
- Fixed `PermissionDenied` handling in two places only: `loginAction` (message) and `middleware.ts` (drop cookies). `Login` after a wrong password still returns `Unauthenticated` and the existing "Tên đăng nhập hoặc mật khẩu không chính xác." text, unchanged.
- Time input: `datetime-local` has no zone; the value is interpreted in UTC+07:00 (the product has no per-user timezone, same assumption as `/admin/audit`) and sent as a `Timestamp`.
- Guards use `redirectGuestToLogin(pathname, searchParams)` so the return URL keeps the filters; the Q1 scan in `src/app/guards.test.ts` forbids bare `redirect("/login")`.

- Query cap: identity rejects a `ListUsers` query over 64 bytes, so the filter parser cuts at 64 bytes in UTF-8 on a character boundary; no server round trip can fail on length.
- Restricted seller: the session JWT has only `sub`, `name` and `scopes`, so a restricted seller and a buyer look the same (no `listing.write`). Seller pages therefore never redirect a signed-in principal; the layout result carries both the buyer text and the restricted line.

## Clarifications

### Round 1 — 2026-10-16

- A14 [session] Users UI: Success shows how many sessions ended / Users UI: Reinstating shows zero sessions ended: which statuses revoke sessions? → **RESTRICTED, SUSPENDED and BANNED (including re-setting the same status) revoke all sessions; ACTIVE revokes none, so a reinstatement reports 0** (human)
- A15 [lifecycle] Users UI: Rows show the user columns: when is an expired suspension lifted? → **only at the user's next login; the page shows a SUSPENDED row with a past end time as returned, a lift sweeper is a follow-up** (human)
- A16 [copy] Users UI: Suspended user sees the fixed refusal: does the login refusal tell suspended from banned or show the end time? → **no; one generic text "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ."** (human)
- A20 [authz] Users UI: Seller layout shows the restricted notice / Users UI: Seller pages do not send a principal without the listing.write scope to login / Users UI: Restricted seller sees the notice in the browser: what does a restricted seller see on seller pages? → **a "Tài khoản bị hạn chế" notice instead of a /login redirect loop; the token carries only scopes, so the line is added to the existing no-`listing.write` result shown to buyers too** (human)
- X1 [ownership] all scenarios: duplicate scenario titles with frontend-listing-moderation → **every scenario title of this change is prefixed "Users UI: "** (human)
- X2 [gateway] Users UI: Search is forwarded as typed fields: who owns the `AccountAdminService` gateway forwarders? → **identity-user-enforcement; this slice adds no gateway task** (human)
- R1 [data] Users UI: Rows show the user columns: the reason-code list is not fixed and identity's tests use `scam` → **the contract accepts any lower_snake_case code; the UI list gains `scam` ("Lừa đảo")** (human)
- R2 [ordering] Users UI: Rows show the user columns: newest first or username order? → **accept identity's username-ascending order; the assumption is corrected** (human)
- R3 [privacy] Users UI: The users page sends no referrer: the search query sits in the URL → **keep the GET form, set `referrer: "no-referrer"` in the page metadata, and add URL/access-log exposure to the initiative's DPO sign-off list** (human)
