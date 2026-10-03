## Context

`UI_SYSTEM_DESIGN.md` §6 lists "Phase 5: User Settings" as `Tabs`, `Descriptions`, `Input`, `Badge` (KYC
Verified / Pending). This change implements it for the nine routes above. Current state (read from
`team-frontend/src`):

| Route | Today | Client / server | State today |
|---|---|---|---|
| `/login`, `/register` | `LoginForm`/`RegisterForm` (`useFormState`, `Input`, `Button`) | page server, form client | server error string in `div` |
| `/account/addresses` | `AddressManager` + hand-rolled `AddressModal` | page server, manager client | `useState` modal, `window.confirm` delete |
| `/account/security` | inline cards, `RevokeSessionButton` | page server, button client | `useState` revoked |
| `/account/verification` | coloured `span`, `SubmitKycForm` | page server, form client | `useState`, unlabelled controls |
| `/account/referral` | inline cards, `GenerateReferralCodeButton`, `RedeemReferralForm` | page server, leaves client | n/a |
| `/account/following` | two cards, links; feed via `searchListings` per seller | server only | n/a |
| `/favorites` | header, `CollectionsManager`, `ListingGrid` | server + client manager | `?collection=` already in URL |
| `/notifications` | `NotificationsView` (whole view client) | client | tab and items in `useState` |

Tracking surfaces on these routes that MUST NOT change: `data-testid="notification-item"` + `data-type` on
each notification link, `data-testid="alert-subscription"` + `data-type`, and everything inside
`ListingGrid`/`ListingCard` on `/favorites` (impression + click tracking, placement attribution), which is
reused untouched. No `TrackLink`/`TrackImpression` is used directly in the other seven pages; none may be
added or removed by this change.

## Goals / Non-Goals

Goals: one account shell, every screen with defined loading/empty/error states, accessible forms, uniform
mutation feedback, URL-held view state.
Non-goals: new account features, orders pages, discovery components, backend changes.

## Decisions

1. **Ant Design / Ant Design Pro mapping**

   | Ant Design / Pro template | agora component / route |
   |---|---|
   | Account › Account Settings (`Menu` mode inline, selected key = route) | `AccountShell` left menu on `lg+`: `<nav aria-label>` of links, active link `aria-current="page"`; the selection IS the URL path |
   | Account Settings, narrow viewport (menu collapses to horizontal) | `AccountShell` at 375px: a horizontally scrollable strip of the same links rendered with the `Tabs` visual (links, not client tabs) |
   | Account › Account Center (profile card + tabbed list) | `/account/following` (summary `Card` + `Tabs`: Gian hàng / Sản phẩm via `?tab=`) and `/account/referral` summary `Card` + `Statistic` |
   | Page header (`PageHeader`) | `AccountShell` header: `Breadcrumb` (Tài khoản › current), `h1` 24, subtitle 14 |
   | `Descriptions` | `/account/verification` current status, `/account/referral` code summary |
   | `Badge`/`Tag` (status) | KYC: `Tag` Verified (success), Pending (promo), Rejected (danger), Not submitted (neutral); address "Mặc định" `Tag`; login history Success/Failed `Tag`; unread count `Badge` on notification tabs |
   | `Table` | `/account/security` sessions and login history (column set collapses to a stacked row at 375px) |
   | `Timeline` | `/account/referral` rewards ledger |
   | `List` (card list) | `/account/addresses` address cards, `/account/following` shop cards |
   | `Statistic` | referral invited count, rewards total |
   | `Form` + `Form.Item` (validation, `help`) | `FormItem` + `Input`/`Select` on login, register, KYC, address modal, redeem referral |
   | User › Login / Register | `/login`, `/register`: centred `Card`, `FormItem` per field, pending `Button`, inline `Alert type="error"` |
   | `Modal` / `Modal.confirm` | address create/edit (`Modal`), address delete confirm (`Modal`, replaces `window.confirm`), session revoke confirm |
   | `Tabs` | `/notifications` filter tabs (`?tab=`), `/account/following` tabs |
   | `Empty` / `Result` / `Alert` | every list's empty state; `error.tsx` per route group as `Result status="error"`; inline `Alert` with retry for failed gateway reads |
   | `Skeleton` | route `loading.tsx` for each page + Suspense around secondary blocks |
   | `Avatar` + `Image` | following shop cards (shop initial avatar, real shop name); favorites images stay inside `ListingCard` (not changed here) |
   | `message` | `ToastProvider` toast after each mutation |
   | `Pagination` | `/notifications` and the security login history when more than 20 rows (`?page=`) |

2. **Account shell, not a layout.** `AccountShell` is a server component used by the five `/account/*`
   settings pages. It is not `app/account/layout.tsx`, because that layout would also wrap
   `/account/orders*`, which `ui-phase-orders` owns. Active item comes from a `current` prop each page
   passes (no `usePathname`, so no client JS). The menu lists: Địa chỉ, Bảo mật, Xác minh, Giới thiệu, Đang
   theo dõi, Yêu thích, Thông báo (the last two are standalone pages linked from the menu), plus Đơn hàng
   linking to `/account/orders` (link only).
3. **Server components by default; client leaf islands only:** `LoginForm`, `RegisterForm`, `AddressModal`,
   `AddressActions` (edit/delete/default buttons, extracted from `AddressManager`), `RevokeSessionButton`,
   `SubmitKycForm`, `GenerateReferralCodeButton`, `RedeemReferralForm`, `CollectionsManager`,
   `NotificationPrefsForm`, `AlertSubscriptions`. `NotificationsView` is split: the
   list/tabs become server-rendered, only the islands above are client.
4. **URL state.** `/notifications?tab=all|order|chat|alert|system&page=N`; `/account/following?tab=shops|
   items`; `/favorites?collection=<id>&page=N` (collection already exists). Tabs are rendered as links so
   back/forward and shared URLs work and the server filters.
5. **Mutations.** All Server Actions in `features/{auth,address,account,notification,engagement}/actions.ts`
   return `{ ok: boolean; error?: string; data?: T }` and call `revalidatePath` for the affected route.
   `loginAction`/`registerAction` keep `redirect()` on success (documented exception: redirect throws and
   never returns); on failure they return `{ ok: false, error }`. Address create/update/delete/set-default
   are normalised (today `void`/`AddressState`).
6. **Mutation feedback contract.** Each mutating control: `Button isLoading` while pending (width kept,
   `aria-busy`), `disabled` while pending or while the form is invalid, then `toast.success` or `toast.error`.
   Destructive actions (delete address, revoke session) first open a `Modal` confirm whose confirm button
   carries the pending state.
7. **Form validation.** Login/register/KYC/address/redeem use `FormItem` with `required`, `help` text and
   `validateStatus`. Client validation (non-empty, min length 3/4 as today) runs on submit and on blur;
   server errors map to a form-level `Alert` (`role="alert"`) and, where the server names a field, to that
   `FormItem`'s `help`. Inputs keep `name`/`id` values so existing e2e locators still match.
8. **KYC a11y fix.** `SubmitKycForm`: "Loại giấy tờ" is a `Select` inside `FormItem label` (label `htmlFor` =
   select `id`), "Mã tham chiếu tài liệu" an `Input` in `FormItem` with `help` as `aria-describedby`; the
   submit `Button` is disabled until the reference is non-empty; the status `Tag` has text, not colour alone.
9. **E2E stability.** Preserved: `/login` and `/register` input `name`s (`username`, `password`, `role`),
   button text "Đăng nhập"/"Đăng ký", the "Bảo Mật" nav link, security section headings "Phiên đăng nhập" and
   "Lịch sử đăng nhập", `data-testid` attributes listed in Context, `/favorites` collections manager labels.
10. **CLS = 0.** Every page ships a `loading.tsx` whose skeleton matches the final footprint (shell + card
    heights). Tables and lists reserve row height; `Image` with fixed aspect for shop avatars. Favorites
    thumbnails are lazy because they are below the fold inside the unchanged `ListingGrid`/`ListingCard`.
11. **Type and colour.** Only 12/14/16/20/24: h1 24, section titles 20 (16 inside cards), body 14, captions
    and table meta 12. No emoji as icons in headings (decorative glyphs removed). Brand colour only on
    primary CTAs (`Button primary`), the referral code and reward amounts (price role) and unread `Badge`.
    Verified/Pending/Rejected use semantic success/promo/danger, not brand.
12. **Responsive.** 375px: shell menu becomes a one-line scrollable strip above content, cards stack
    full-width, tables become stacked rows, `Modal` goes full-width with a sticky footer, buttons are
    full-width in forms, tap targets at least 40px. Desktop (`lg` and up): 240px menu, content `max-w-3xl`
    (`max-w-5xl` for favorites).

## Risks / Trade-offs

- Splitting `NotificationsView` changes a client component into server + islands; "mark all read" has no
  backend, so it must remain visibly local (see Open Questions) to avoid implying persistence.
- Normalising action return types touches callers (`AddressManager`, `AddressModal`, tests in
  `address/actions.test.ts`, `auth/actions.test.ts`); additive adapters and updated tests mitigate.
- `/favorites` depends on `ListingGrid`, which `ui-phase-discovery` may change; this change adds no props to it.

## Open Questions

1. Decided: "Mark all read" is hidden until a backend mark-as-read RPC exists (follow-up backend change); no
   local simulation.
2. Login `?next=` return path: pages currently `redirect("/login")` and `loginAction` always goes to `/`.
   Adding `?next=` is frontend-only but changes the e2e `unauthorized_access` expectation (URL gains a
   query). Include here or defer?
3. Should `/favorites` and `/notifications` live inside `AccountShell` (consistent menu) or stay standalone
   (current, wider layout)? Proposed: standalone with the menu as a link target only.
4. Decided: the following page shows the real shop display name from the backend change
   `shop-display-name` (this change depends on it); "Shop #<6 chars>" only when the name is empty.
5. Does `ui-phase-orders` want to adopt `AccountShell` (and so a shared `AccountMenuItems` constant) from
   this change, or define its own?
