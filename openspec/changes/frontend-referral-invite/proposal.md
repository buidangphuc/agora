## Why

A buyer can only invite a friend by reading out a code, and the friend must type it on `/account/referral`
after signing up. `/s/[code]` drops the stored `utm_*`, and the referral totals are shown with `formatPrice`
(đ) although they are xu. Initiative `q7-q37-referral-welcome` makes the invite a link, attributes the signup
from the link or a cookie, and shows pending and released xu.

initiative: q7-q37-referral-welcome

## What changes

- `/account/referral` gets a "Chia sẻ link mời" button. It calls a server action that runs
  `createShareLink("referral", <own code>, {utm_source: "referral", utm_medium: "invite"})` (existing
  `lib/gateway/sharing.ts`, `SharingService.CreateShareLink`) and copies `<origin>/s/<shortCode>`. The button is
  disabled while the buyer has no code (the existing "generate code" button stays).
- `src/app/(shop)/s/[code]/page.tsx` becomes a route handler `route.ts` (a page cannot set cookies). It resolves the
  link with `SharingService.ResolveShareLink` and answers 307 with:
  - `listing` -> `/listing/<id>`, `shop` / `seller` -> `/shop/<id>`, `referral` -> `/register?ref=<code>`;
  - every stored `utm_*` key kept on the Location query (non-`utm_` keys are dropped);
  - unknown short code or unknown target type -> `/`, no cookie, no utm.
- For `referral` only, the response sets `agora_ref` (value = the code, `HttpOnly`, `SameSite=Lax`, `Path=/`,
  `Max-Age=604800`, `Secure` via `isSecureRequest`). Listing and shop links set no `agora_ref`.
- `/register` reads `?ref` and passes it to `RegisterForm` as a hidden field. `registerAction` picks the referral code
  as: the `ref` form field if it matches `^[A-Za-z0-9_-]{1,32}$`, else the `agora_ref` cookie if it matches, else empty.
  It sends `referral_code` in `AuthService.Register` (new `RegisterRequest.referral_code`, contract slice) and, after a
  successful register, deletes `agora_ref`. A failed register keeps the cookie. `returnUrl` handling is unchanged.
- `formatXu(n)` in `src/components/ui/format.ts` returns the integer with vi-VN grouping and the suffix " xu"
  (`10000` -> `10.000 xu`). `/account/referral` shows "Xu đang chờ" (`pending_xu`) and "Xu đã nhận" (`released_xu`)
  from `GetMyReferral`, and the reward timeline shows each reward with its status badge: PENDING "Đang chờ" (with
  `release_at` as "Dự kiến nhận"), RELEASED "Đã nhận", VOID "Đã hủy" (struck through, not counted). No `formatPrice`
  remains on this page.
- The manual redeem form states the rule ("Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành.") and,
  when `RedeemReferral` fails with `FAILED_PRECONDITION`, shows the server message, or the fallback "Không thể nhập mã:
  tài khoản đã quá 7 ngày, đã có đơn hoàn thành, đã nhập mã hoặc mã là của chính bạn." when it is empty. The success
  toast says "Đã ghi nhận mã giới thiệu." (no immediate credit).
- Uses (proto from the contract slice, regenerate `src/generated`): `RegisterRequest.referral_code`,
  `GetMyReferralResponse.pending_xu/released_xu`, `ReferralReward.status/release_at`, `ReferralRewardStatus`.
- Hot file: `account/referral/page.tsx` is also edited by `frontend-welcome-checklist` (serialised after this slice).

## NFR

- `/s/<code>` adds no call beyond the existing `ResolveShareLink`; p95 under 300 ms on the local stack is an observation, not a gate.
- Register latency unchanged; no extra gateway call for the cookie or field.
- Cookie `agora_ref`: at most 32 bytes of value, 7 days, never readable by client JS, never logged or sent to analytics.
- A missing or malformed referral code never blocks registration or redirects to an error.

## Data classification

| field | classification | purpose | basis | retention |
|---|---|---|---|---|
| `agora_ref` cookie (referral code only) | pseudonymous identifier | attribute the signup to the inviter | functional | 7 days, deleted after a successful register |
| `ref` query parameter, `referral_code` in Register | pseudonymous identifier | same | contract (attribution) | URL/browser history; stored by identity/referral per their slices |
| `utm_source=referral`, `utm_medium=invite` | non-personal campaign tags | channel attribution | legitimate interest, no consent needed | in URL only; stored by team-sharing |
| pending/released xu | account balance (mock ledger) | show reward state to its owner | contract | not stored by the frontend |

No raw code or user id goes to logs or analytics; `trackEcommerce("referral_redeem")` stays unchanged. DPO sign-off is NEEDS_HUMAN
(see the initiative).

## Threat model

- Assets: inviter's code and rewards, signup attribution, the redirect endpoint.
- Abuse cases: (1) open redirect through a crafted share link: targets are built from fixed paths, ids are path-encoded and
  only `utm_*` keys are carried (values URL-encoded), so the Location never leaves the app origin. (2) Cookie injection through a
  crafted `ref`: the value is validated against `^[A-Za-z0-9_-]{1,32}$` before it is set or sent, so header injection and long
  values are impossible. (3) Cookie stuffing: an attacker cannot set `agora_ref` for a victim except by luring them to a share
  link; a later link overwrites the cookie (last click wins); self-referral and caps are enforced server-side by team-referral.
  (4) Register must not leak whether a code exists: the UI never shows a verdict. (5) Click inflation: out of scope (team-sharing).
- Authz rule: `/s/<code>` and `/register` are public; "Chia sẻ link mời" and the redeem form require the signed-in buyer (guest is sent to
  login with the existing `redirectGuestToLogin("/account/referral")`); xu totals are the caller's own (`GetMyReferral`).

## Clarifications
- 2026-11-06 (Bùi Đăng Phúc, converge): S18 conflicted with the referral-service requirement that `ListReferralRewards` omits VOID rewards. The service spec wins: the referral page lists PENDING and RELEASED rewards only; a voided reward is neither listed nor counted.
