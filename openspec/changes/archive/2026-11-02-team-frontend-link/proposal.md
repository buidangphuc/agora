## Why

C1b and C2 moved Button, Card, Input and Checkbox onto `@fe-platform/ui`. C3 does the same for links: team-frontend styles `next/link` by hand in about 50 files, with three copies of a "link that looks like a button" class string (`cart/linkButton.ts`, `order/linkStyles.ts`, `home/linkButton.ts`) and two `LinkButton` components. One platform Link gives the buyer app a single look for text links and button-shaped links, 44 px touch targets on the button shapes, and Next client navigation.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): `Link` accepts `variant` (`inline` default, `subtle`, or a Button variant `primary`/`secondary`/`ghost`/`danger` drawn from the button tokens), `size` and `fullWidth`; it still wraps `next/link`. Tokens add `link.subtle`.
- New facade `team-frontend/src/components/ui/Link.tsx` over the platform Link (`outline` and `white` map to `secondary`, like the Button facade); exported from the ui barrel.
- Styled text links and button-like links in `team-frontend/src` (including `features/seller` and `features/admin`) use the facade; `className` moves to `variant`/`size`/`fullWidth`, and layout or typography that the platform does not own (margins, `line-clamp`, `truncate`, text size, `hidden md:inline-flex`) moves to a wrapper `span`.
- `features/account/LinkButton` and `features/seller/LinkButton` delegate to the facade (seller `className` prop removed, no caller used it). The three link-button class helpers are deleted.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src` (about 55 files, 94 Link elements), `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `fe-platform/packages/{tokens,ui}`, `fe-platform/.changeset/ui-link-variants.md`, `fe-platform/docs/agents/catalog.json`.
- Visible changes: inline links are underlined in the link colour; button-shaped links are at least 44 px high with the airy radius; small table actions (`Xem`, `Sửa`, `Chi tiết`) become ghost buttons; footer and title links keep the surrounding colour and turn link-coloured on hover.

## Non-goals

Structural links that wrap content (cards, list rows, category tiles, chat product card, `TrackLink`), navigation components with their own active state (Tabs, Pagination, CompactPagination, Breadcrumb, Stepper, AccountShell, SellerNavList, admin status tabs, FilterSidebar), links on dark surfaces (ConsumerShell header, Hero voucher link, CockpitView, FloatingChatBubble), the checkout logo and the StudentRental chips. Select, Table, Dialog, Toast, Alert swaps; dark theme.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; Playwright selectors by role, accessible name, `href` and `data-testid` keep working.
