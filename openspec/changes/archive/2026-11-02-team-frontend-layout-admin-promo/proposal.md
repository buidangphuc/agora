## Why

Plan P10 section F rebuilds the main pages in the fe-platform visual language. The admin platform voucher pages (`/admin/vouchers`, `/admin/vouchers/new`, `/admin/vouchers/[code]`) and the flash sale campaign pages (`/admin/campaigns`, `/admin/campaigns/new`) still use the dense pre-P7 layout: small headers, bare tables, forms without a surface, small-text back and pagination links below the 44 px touch target, and a native select at 36 px.

## What Changes

- Page headers: `text-2xl md:text-3xl` title with a supporting line; the primary call to action ("Tạo voucher sàn", "Tạo flash sale") is an `md` button link that is full width on mobile.
- Lists: the voucher and campaign tables sit in a flush platform `Card`; pagination links ("Trang đầu", "Trang sau") become `sm` outline button links and the empty pagination row is no longer rendered.
- Campaign lookup: the listing lookup form sits in a padded `Card`, the input grows with the row on `sm` and up, and the active campaign result is a `Card`.
- Create forms (voucher and flash sale): the form sits in a padded `Card` inside a `max-w-3xl` column, wider field gaps, an action row separated by a top border, full-width actions on mobile; the voucher discount-type select gets the platform field shape (`rounded-xl`, `text-base`, `min-h-touch`).
- Created states and the voucher detail show their `Descriptions` in a padded `Card`; `VoucherDetails` drops its own border so the card is the only surface.
- Back links become `ghost` `sm` button links (44 px).
- Presentation only: no change to server actions, data fetching, `data-testid`, roles, accessible names or headings.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend-admin/src/app/admin/vouchers/{new,[code]}/page.tsx`, `team-frontend-admin/src/app/admin/campaigns/new/page.tsx`.
- `team-frontend-admin/src/features/vouchers/{VoucherListView,VoucherDetails,CreateVoucherForm}.tsx` and `team-frontend-admin/src/features/admin/{CampaignsView,CreateCampaignForm}.tsx`.
- Visible changes: more whitespace, larger titles, card surfaces, 44 px back and pagination links.

## Non-goals

`AdminShell` and shared layouts, `src/components/ui`, fe-platform, tailwind config, the 403/404 `Result` states, other admin pages.
