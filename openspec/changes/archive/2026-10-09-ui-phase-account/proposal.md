## Why

The nine account and auth screens (`/login`, `/register`, `/account/addresses`, `/account/security`,
`/account/verification`, `/account/referral`, `/account/following`, `/favorites`, `/notifications`) were
built before the design system and share no shell. Concretely today:

- Each page hand-rolls the same `rounded-2xl border bg-white` card, emoji headings and `text-xs`/`text-[10px]`
  text, and there is no navigation between the account pages (they are reachable only from the top bar).
- `SubmitKycForm` has two `<label>` elements with no `htmlFor` and no control `id`, so the select and the
  text input have no accessible name; the KYC status is a hand-coloured `<span>`.
- Mutations are inconsistent: address delete uses `window.confirm`, address create/update return an
  `AddressState`, delete/set-default return `void`, so failures are silent and there is no pending or
  success/error toast on most of them.
- `/notifications` keeps its tab in `useState` (lost on reload/back, not shareable) and renders the whole page
  as one client component.
- Login/register show errors in a hand-styled `div`, with no per-field validation and no `role="alert"`.
- Lists have no skeleton fallbacks, so the pages block on several gateway calls with no `loading.tsx`.

Reference models are Ant Design Pro: Account › Account Settings (menu on the left, selection held in the
URL), Account › Account Center (profile + tabbed content), User › Login/Register (FormItem validation,
pending submit, inline error Alert), and Badge/Tag for KYC Verified/Pending. This change rebuilds the nine
screens on the `ui-core-components` set and the `ui-foundation` tokens, with no `antd` dependency.

## What Changes

- Add a server `AccountShell` (page header, left menu on desktop, scrollable tab strip on 375px) used by the
  five `/account/*` settings pages, with the active entry derived from the URL.
- Rebuild each page to the anatomy in `design.md`: Descriptions/Table/Timeline/Tag for display, FormItem +
  Input/Select for entry, Modal for address edit and delete confirm, Alert/Result/Empty for feedback.
- Fix the KYC form: every control labelled via `FormItem` (`htmlFor`/`id`, `aria-describedby`), status shown
  as `Tag`/`Badge` (Verified, Pending, Rejected, Not submitted).
- Move `/notifications` tab state into `?tab=`, keep the list server-rendered, leave only prefs and
  alert toggles as client islands. There is no "Mark all read" control (no backend); it is hidden.
- Normalise the account/address/favorites/notification/auth Server Actions to `{ ok, error?, data? }` with
  `revalidatePath`, and give every mutation pending, disabled and toast feedback.
- Add route `loading.tsx` skeletons for each page.
- `/account/following` shop cards render the real shop display name (fallback "Shop #<6 chars>" only for an
  empty name). Only real data is shown on every page; blocks without data are hidden or show `Empty`.
- Responsive at 375px and desktop; type scale 12/14/16/20/24; brand colour for primary CTAs and badges only.

Repos: `team-frontend` (code), `platform-e2e` (e2e track). Owning FEATURES.yaml: `team-frontend`.

## Capabilities

### New Capabilities
- `ui-account`: the account/auth screens, the `AccountShell`, their state, mutation and a11y contracts.

### Modified Capabilities
None. (Existing `auth.*`, `address.manage`, `engagement.collections`, `notification.center` scenarios keep
their behaviour; their selectors are preserved, see design Decision 9.)

## Non-goals

- No backend, gateway or proto change in this change (the shop display name comes from the separate change
  `shop-display-name`).
- Follow-up backend change: a mark-as-read RPC (single and all). Until it exists, "Mark all read" on
  `/notifications` is hidden, never simulated locally.
- No `antd` / `@ant-design/*` dependency.
- `/account/orders` and `/account/orders/[id]` belong to `ui-phase-orders`; `AccountShell` is built so that
  phase can adopt it but this change does not touch those routes.
- `ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar` and `SortBar` belong to `ui-phase-discovery`;
  `/favorites` reuses `ListingGrid` unchanged.
- No new features: no profile editing, password change, 2FA or file upload for KYC (the form keeps its
  document-reference text field).
- No change to tracking hooks (`TrackLink`, `TrackImpression`, `SearchImpressions`, `AnalyticsProvider`,
  recommendation placement attribution, `data-*` attributes such as `data-testid="notification-item"`,
  `data-type`, `data-testid="alert-subscription"`).

## Dependencies

- Requires the backend change `shop-display-name` (adds a shop/seller display name to the contract) for the
  `/account/following` shop cards.

## Impact

- `team-frontend/src/app/{login,register,favorites,notifications}/*`, `src/app/account/{addresses,security,
  verification,referral,following}/*` (+ `loading.tsx` each), `src/features/{auth,address,account,
  notification,engagement}/*`, a new `src/features/account/AccountShell.tsx`.
- `team-frontend/FEATURES.yaml` (new entries), `platform-e2e/tests/e2e/features/frontend/account.feature`
  (+ steps/page objects); existing `identity/addresses.feature`, `frontend/account_security.feature`,
  `auth/login.feature`, `auth/register.feature`, `engagement/collections.feature`,
  `buyer/consumer_pages.feature` must stay green.
- Depends on `ui-foundation` and `ui-core-components`.
